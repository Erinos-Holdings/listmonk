package main

// Fork (join link) -- integrations listmonk/JOIN-LINK-SPEC invariants I1-I8 against a real
// database: the plan (join_plan, via get-join-plan), the one write (join-brand) and the two
// handlers (JoinPage / JoinConfirm) wired over them. Shares newBrandFixture / newLinkHarness
// (LISTMONK_TEST_PG opt-in; a plain go test skips). Fixture names are acme.test / zeta.test only.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/joinpage"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

type joinFixture struct {
	*brandFixture
	// acme's win-back list: private, tagged winback (the import's list).
	winback int
	// acme's re-engagement queue proper (reengage tag) and a no-master brand's only list.
	reengage, bare int
}

func newJoinFixture(t *testing.T) *joinFixture {
	f := &joinFixture{brandFixture: newBrandFixture(t)}
	f.db.MustExec(`UPDATE brands SET from_email = 'Acme <hello@acme.test>' WHERE slug = 'acme'`)
	f.winback = f.list("acme win-back", "private", "brand:acme")
	f.db.MustExec(`UPDATE lists SET tags = tags || '{winback}'::VARCHAR(100)[] WHERE id = $1`, f.winback)
	f.reengage = f.list("acme reengage queue", "private", "brand:acme")
	f.db.MustExec(`UPDATE lists SET tags = tags || '{reengage:1:1}'::VARCHAR(100)[] WHERE id = $1`, f.reengage)
	f.bare = f.list("bare segment", "private", "brand:bare")
	return f
}

// joinBody is a campaign body carrying the Join link (D4 row 6a: the plan joins only from a
// campaign whose body or template carries JoinURL).
const joinBody = `<p>Oui !</p><a href="{{ JoinURL }}">Join</a>`

// langCamp creates a regular campaign on lists with attribs.lang = lang ("" = none) whose body
// carries the Join link.
func (f *joinFixture) langCamp(name, lang string, lists ...int) (int, string) {
	uu := f.camp(name, lists...)
	f.db.MustExec(`UPDATE campaigns SET body = $2 WHERE uuid = $1`, uu, joinBody)
	if lang != "" {
		f.db.MustExec(`UPDATE campaigns SET attribs = JSONB_BUILD_OBJECT('lang', $2::TEXT) WHERE uuid = $1`, uu, lang)
	}
	var id int
	if err := f.db.Get(&id, `SELECT id FROM campaigns WHERE uuid = $1`, uu); err != nil {
		f.t.Fatal(err)
	}
	return id, uu
}

// bareSub creates a subscriber with no list rows and the given attribs JSON.
func (f *joinFixture) bareSub(email, attribs string) (int, string) {
	var (
		id int
		uu string
	)
	if err := f.db.QueryRow(`INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), $1, $1, $2::JSONB) RETURNING id, uuid`,
		email, attribs).Scan(&id, &uu); err != nil {
		f.t.Fatal(err)
	}
	return id, uu
}

func (f *joinFixture) addRow(sub, list int, status, meta string) {
	f.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status, meta) VALUES ($1, $2, $3, $4::JSONB)`, sub, list, status, meta)
}

// snapshot is every subscribers and subscriber_lists row, as one comparable string.
func (f *joinFixture) snapshot() string {
	var s string
	if err := f.db.Get(&s, `SELECT COALESCE((SELECT JSONB_AGG(x ORDER BY x.id)::TEXT FROM (SELECT id, uuid, email, name, attribs, status, updated_at FROM subscribers) x), '')
		|| '|' || COALESCE((SELECT JSONB_AGG(y ORDER BY y.subscriber_id, y.list_id)::TEXT FROM (SELECT subscriber_id, list_id, status, meta, created_at, updated_at, confirmed_at FROM subscriber_lists) y), '')`); err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *joinFixture) plan(subUUID, campUUID string) models.JoinPlan {
	p, err := f.app.core.GetJoinPlan(subUUID, campUUID)
	if err != nil {
		f.t.Fatalf("GetJoinPlan: %v", err)
	}
	return p
}

// call drives a join handler and returns the rendered template and its data.
func (f *joinFixture) call(method, campUUID, subUUID string) (string, joinTpl) {
	e := echo.New()
	rend := &capturingRenderer{}
	e.Renderer = rend
	req := httptest.NewRequest(method, "/subscription/join/"+campUUID+"/"+subUUID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("campUUID", "subUUID")
	c.SetParamValues(campUUID, subUUID)
	h := f.app.JoinPage
	if method == http.MethodPost {
		h = f.app.JoinConfirm
	}
	if err := h(c); err != nil {
		f.t.Fatalf("%s join: %v", method, err)
	}
	if rec.Code != http.StatusOK {
		f.t.Fatalf("%s join: HTTP %d", method, rec.Code)
	}
	out, ok := rend.data.(joinTpl)
	if !ok {
		f.t.Fatalf("rendered %s with %T", rend.name, rend.data)
	}
	return rend.name, out
}

func (f *joinFixture) attribs(sub int) map[string]any {
	var raw json.RawMessage
	if err := f.db.Get(&raw, `SELECT attribs FROM subscribers WHERE id = $1`, sub); err != nil {
		f.t.Fatal(err)
	}
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		f.t.Fatal(err)
	}
	return m
}

func (f *joinFixture) hasRow(sub, list int) bool {
	var n int
	f.db.Get(&n, `SELECT COUNT(*) FROM subscriber_lists WHERE subscriber_id = $1 AND list_id = $2`, sub, list)
	return n == 1
}

// joinCase is one subscriber in one starting state, and the outcome the plan must give.
type joinCase struct {
	name    string
	setup   func(f *joinFixture) (campUUID, subUUID string)
	outcome string
}

// joinCases covers every D4 outcome, every D6 refusal and every existing master-row state.
func joinCases() []joinCase {
	onWinback := func(f *joinFixture, email string, extra func(sub int)) (string, string) {
		_, camp := f.langCamp("acme win-back "+email, "fr", f.winback)
		id, uu := f.bareSub(email, `{}`)
		f.addRow(id, f.winback, "confirmed", `{}`)
		if extra != nil {
			extra(id)
		}
		return camp, uu
	}
	return []joinCase{
		{"no campaign", func(f *joinFixture) (string, string) {
			_, uu := f.bareSub("nocamp@x", `{}`)
			return "00000000-0000-4000-8000-000000000000", uu
		}, models.JoinOutcomeNoCampaign},
		{"no brand (untagged list)", func(f *joinFixture) (string, string) {
			_, camp := f.langCamp("plain", "", f.untagged)
			_, uu := f.bareSub("nobrand@x", `{}`)
			return camp, uu
		}, models.JoinOutcomeNoBrand},
		{"no brand (two brands)", func(f *joinFixture) (string, string) {
			_, camp := f.langCamp("two", "", f.winback, f.zeta)
			_, uu := f.bareSub("twobrands@x", `{}`)
			return camp, uu
		}, models.JoinOutcomeNoBrand},
		{"no master", func(f *joinFixture) (string, string) {
			_, camp := f.langCamp("bare", "", f.bare)
			_, uu := f.bareSub("nomaster@x", `{}`)
			return camp, uu
		}, models.JoinOutcomeNoMaster},
		{"refused: blocklisted", func(f *joinFixture) (string, string) {
			return onWinback(f, "bl@x", func(id int) { f.db.MustExec(`UPDATE subscribers SET status = 'blocklisted' WHERE id = $1`, id) })
		}, models.JoinOutcomeRefused},
		{"refused: disabled", func(f *joinFixture) (string, string) {
			return onWinback(f, "dis@x", func(id int) { f.db.MustExec(`UPDATE subscribers SET status = 'disabled' WHERE id = $1`, id) })
		}, models.JoinOutcomeRefused},
		{"refused: complaint", func(f *joinFixture) (string, string) {
			return onWinback(f, "cpl@x", func(id int) {
				f.db.MustExec(`INSERT INTO bounces (subscriber_id, type, source) VALUES ($1, 'complaint', 'ses')`, id)
			})
		}, models.JoinOutcomeRefused},
		{"refused: deliverability hold on another brand", func(f *joinFixture) (string, string) {
			return onWinback(f, "dh@x", func(id int) { f.addRow(id, f.zeta, "unsubscribed", `{"hold":{"reason":"deliverability"}}`) })
		}, models.JoinOutcomeRefused},
		{"refused: manual hold", func(f *joinFixture) (string, string) {
			return onWinback(f, "mh@x", func(id int) { f.addRow(id, f.zeta, "unsubscribed", `{"hold":{"reason":"manual"}}`) })
		}, models.JoinOutcomeRefused},
		{"refused: unknown hold reason (fails closed)", func(f *joinFixture) (string, string) {
			return onWinback(f, "uh@x", func(id int) { f.addRow(id, f.zeta, "unsubscribed", `{"hold":{"reason":"mystery"}}`) })
		}, models.JoinOutcomeRefused},
		{"refused: deliverability hold released to unsubscribed", func(f *joinFixture) (string, string) {
			return onWinback(f, "rel@x", func(id int) {
				f.addRow(id, f.zeta, "unsubscribed", `{"hold_released":{"to":"unsubscribed","hold":{"reason":"deliverability"}}}`)
			})
		}, models.JoinOutcomeRefused},
		{"refused: brand-list opt-out", func(f *joinFixture) (string, string) {
			return onWinback(f, "bo@x", func(id int) { f.addRow(id, f.step, "unsubscribed", `{}`) })
		}, models.JoinOutcomeRefused},
		{"refused: win-back-list opt-out", func(f *joinFixture) (string, string) {
			return onWinback(f, "wo@x", func(id int) { f.setStatus(id, f.winback, "unsubscribed", `{}`) })
		}, models.JoinOutcomeRefused},
		{"master held (never-engaged)", func(f *joinFixture) (string, string) {
			return onWinback(f, "held@x", func(id int) { f.addRow(id, f.news, "unsubscribed", `{"hold":{"reason":"never-engaged"}}`) })
		}, models.JoinOutcomeRefused},
		{"master final-held", func(f *joinFixture) (string, string) {
			return onWinback(f, "final@x", func(id int) {
				f.addRow(id, f.news, "unsubscribed", `{"hold":{"reason":"never-engaged","final_at":"2026-01-01T00:00:00Z"}}`)
			})
		}, models.JoinOutcomeRefused},
		{"master unconfirmed (pending)", func(f *joinFixture) (string, string) {
			return onWinback(f, "pend@x", func(id int) { f.addRow(id, f.news, "unconfirmed", `{}`) })
		}, models.JoinOutcomeCannot},
		{"master confirmed", func(f *joinFixture) (string, string) {
			return onWinback(f, "conf@x", func(id int) { f.addRow(id, f.news, "confirmed", `{}`) })
		}, models.JoinOutcomeAlready},
		// D4 row 6a (S4-2): the link is live only where it was sent.
		{"6a: a campaign without JoinURL (a rewritten unsubscribe URL)", func(f *joinFixture) (string, string) {
			camp, uu := onWinback(f, "notag@x", nil)
			f.db.MustExec(`UPDATE campaigns SET body = '<a href="{{ UnsubscribeURL }}">u</a>' WHERE uuid = $1`, camp)
			var n int
			f.db.Get(&n, `SELECT COUNT(*) FROM campaigns c JOIN templates t ON (t.id = c.template_id) WHERE c.uuid = $1 AND t.body LIKE '%JoinURL%'`, camp)
			if n != 0 {
				f.t.Fatalf("fixture template carries JoinURL")
			}
			return camp, uu
		}, models.JoinOutcomeCannot},
		{"6a: a non-recipient of a JoinURL campaign", func(f *joinFixture) (string, string) {
			_, camp := f.langCamp("acme win-back other", "fr", f.winback)
			id, uu := f.bareSub("nonrecip@x", `{}`)
			f.addRow(id, f.zeta, "confirmed", `{}`) // on another list, never on the campaign's
			return camp, uu
		}, models.JoinOutcomeCannot},
		{"6a: a recipient whose campaign carries JoinURL in its TEMPLATE only", func(f *joinFixture) (string, string) {
			camp, uu := onWinback(f, "tpltag@x", nil)
			var tpl int
			if err := f.db.Get(&tpl, `INSERT INTO templates (name, type, subject, body) VALUES ('join tpl', 'campaign', '', '<a href="{{ JoinURL }}">j</a>{{ template "content" . }}') RETURNING id`); err != nil {
				f.t.Fatal(err)
			}
			f.db.MustExec(`UPDATE campaigns SET body = 'plain', template_id = $2 WHERE uuid = $1`, camp, tpl)
			return camp, uu
		}, models.JoinOutcomeJoin},
		{"6a: a recipient of a JoinURL campaign on a non-winback list (a transactional list)", func(f *joinFixture) (string, string) {
			_, camp := f.langCamp("acme receipt join", "", f.txn)
			id, uu := f.bareSub("txnrecip@x", `{}`)
			f.addRow(id, f.txn, "confirmed", `{}`)
			return camp, uu
		}, models.JoinOutcomeJoin},
		{"join: never-engaged hold on ANOTHER brand is not a refusal", func(f *joinFixture) (string, string) {
			return onWinback(f, "ne@x", func(id int) { f.addRow(id, f.zeta, "unsubscribed", `{"hold":{"reason":"never-engaged"}}`) })
		}, models.JoinOutcomeJoin},
		{"join: no master row", func(f *joinFixture) (string, string) {
			return onWinback(f, "join@x", nil)
		}, models.JoinOutcomeJoin},
	}
}

// TestJoin_PlanWritesNothing -- I1: GET never writes. For every D4 outcome the plan and the GET
// handler leave subscribers and subscriber_lists byte-identical; the GET renders the outcome's page.
func TestJoin_PlanWritesNothing(t *testing.T) {
	f := newJoinFixture(t)
	seen := map[string]bool{}
	for _, c := range joinCases() {
		camp, sub := c.setup(f)
		before := f.snapshot()
		p := f.plan(sub, camp)
		if p.Outcome != c.outcome {
			t.Fatalf("%s: outcome %q, want %q", c.name, p.Outcome, c.outcome)
		}
		tpl, data := f.call(http.MethodGet, camp, sub)
		if tpl != "join" || data.State != joinpage.State(c.outcome) {
			t.Fatalf("%s: GET rendered %s/%s", c.name, tpl, data.State)
		}
		if after := f.snapshot(); after != before {
			t.Fatalf("%s: the GET wrote\n before %s\n after  %s", c.name, before, after)
		}
		seen[c.outcome] = true
	}
	for _, o := range []string{models.JoinOutcomeNoCampaign, models.JoinOutcomeNoBrand, models.JoinOutcomeNoMaster,
		models.JoinOutcomeRefused, models.JoinOutcomeAlready, models.JoinOutcomeCannot, models.JoinOutcomeJoin} {
		if !seen[o] {
			t.Fatalf("outcome %s not covered", o)
		}
	}
}

// TestJoin_Outcomes -- I2: only `join` writes. Every other outcome, each D6 refusal (a brand-list
// and a win-back-list opt-out included) and each existing master-row state (held, final-held,
// pending, confirmed) leaves the database unchanged under the POST and renders no result page.
func TestJoin_Outcomes(t *testing.T) {
	f := newJoinFixture(t)
	for _, c := range joinCases() {
		camp, sub := c.setup(f)
		before := f.snapshot()
		tpl, data := f.call(http.MethodPost, camp, sub)
		after := f.snapshot()
		if c.outcome == models.JoinOutcomeJoin {
			if tpl != "joined" || after == before {
				t.Fatalf("%s: POST rendered %s and wrote=%v, want a join", c.name, tpl, after != before)
			}
			continue
		}
		if after != before {
			t.Fatalf("%s: POST wrote for outcome %s\n before %s\n after  %s", c.name, c.outcome, before, after)
		}
		if tpl != "join" || data.State != joinpage.State(c.outcome) {
			t.Fatalf("%s: POST rendered %s/%s", c.name, tpl, data.State)
		}
	}
}

// TestJoin_NewSubscriber -- I3: a join is a NEW subscriber. confirmed_at is stamped (no backfill);
// a running delay-0 evergreen on the master list in the joiner's language selects them once
// confirmed_at is past the 600 s public-list hold, never before, and a language-mismatched one
// never does.
func TestJoin_NewSubscriber(t *testing.T) {
	f := newJoinFixture(t)
	_, camp := f.langCamp("acme win-back fr", "fr", f.winback)
	id, uu := f.bareSub("new@x", `{}`)
	f.addRow(id, f.winback, "confirmed", `{}`)

	evergreen := func(lang string) int {
		ev, _ := f.langCamp("acme welcome "+lang, lang, f.news)
		f.db.MustExec(`UPDATE campaigns SET evergreen = TRUE, status = 'running', send_delay_secs = 0,
			started_at = NOW() - INTERVAL '1 day' WHERE id = $1`, ev)
		return ev
	}
	evFR, evEN := evergreen("fr"), evergreen("en")

	if tpl, _ := f.call(http.MethodPost, camp, uu); tpl != "joined" {
		t.Fatalf("POST rendered %s", tpl)
	}
	var ca sql.NullTime
	if err := f.db.Get(&ca, `SELECT confirmed_at FROM subscriber_lists WHERE subscriber_id = $1 AND list_id = $2`, id, f.news); err != nil {
		t.Fatal(err)
	}
	if !ca.Valid {
		t.Fatalf("confirmed_at not stamped: the join wrote under backfill")
	}

	selected := func(ev int) bool {
		var subs []models.Subscriber
		if err := f.q.NextEvergreenSubscribers.Select(&subs, ev, 100); err != nil {
			t.Fatal(err)
		}
		f.db.MustExec(`DELETE FROM campaign_sends WHERE campaign_id = $1`, ev)
		for _, s := range subs {
			if s.ID == id {
				return true
			}
		}
		return false
	}
	if selected(evFR) {
		t.Fatalf("selected inside the 600 s public-list hold")
	}
	f.db.MustExec(`UPDATE subscriber_lists SET confirmed_at = NOW() - INTERVAL '700 seconds' WHERE subscriber_id = $1 AND list_id = $2`, id, f.news)
	if !selected(evFR) {
		t.Fatalf("the fr welcome did not select the fr joiner past the hold")
	}
	if selected(evEN) {
		t.Fatalf("the en welcome selected a fr joiner (no English fallback)")
	}
}

// TestJoin_TargetsAndRemoval -- I4: only the master row is inserted; the only deletion is the
// campaign list's row when that list is tagged winback; a campaign on an untagged private list,
// a journey step list or a queue list deletes nothing; no other row changes.
func TestJoin_TargetsAndRemoval(t *testing.T) {
	f := newJoinFixture(t)
	type rowState struct {
		Status    string    `db:"status"`
		Meta      string    `db:"meta"`
		UpdatedAt time.Time `db:"updated_at"`
	}
	rowsOf := func(sub int) map[int]rowState {
		var rows []struct {
			ListID int `db:"list_id"`
			rowState
		}
		if err := f.db.Select(&rows, `SELECT list_id, status, meta::TEXT AS meta, updated_at FROM subscriber_lists WHERE subscriber_id = $1`, sub); err != nil {
			t.Fatal(err)
		}
		out := map[int]rowState{}
		for _, r := range rows {
			out[r.ListID] = r.rowState
		}
		return out
	}

	for _, c := range []struct {
		name    string
		list    int
		deleted bool
	}{
		{"win-back list", f.winback, true},
		{"untagged private segment", f.segment, false},
		{"journey step list", f.step, false},
		{"re-engagement queue", f.reengage, false},
	} {
		_, camp := f.langCamp("acme "+c.name, "fr", c.list)
		email := strings.ReplaceAll(c.name, " ", "-") + "@x"
		id, uu := f.bareSub(email, `{}`)
		others := []int{f.winback, f.segment, f.step, f.reengage, f.zeta, f.txn}
		for _, l := range others {
			f.addRow(id, l, "confirmed", `{"k":"v"}`)
		}
		f.db.MustExec(`UPDATE subscriber_lists SET updated_at = NOW() - INTERVAL '1 hour' WHERE subscriber_id = $1`, id)
		before := rowsOf(id)

		if tpl, _ := f.call(http.MethodPost, camp, uu); tpl != "joined" {
			t.Fatalf("%s: POST rendered %s", c.name, tpl)
		}
		after := rowsOf(id)
		if _, ok := after[f.news]; !ok {
			t.Fatalf("%s: no master row", c.name)
		}
		for _, l := range others {
			a, ok := after[l]
			if l == c.list && c.deleted {
				if ok {
					t.Fatalf("%s: the winback-tagged campaign row survived", c.name)
				}
				continue
			}
			if !ok || a != before[l] {
				t.Fatalf("%s: list %d row changed: %+v -> %+v (present %v)", c.name, l, before[l], a, ok)
			}
		}
		if len(after) != len(before)+1-map[bool]int{true: 1, false: 0}[c.deleted] {
			t.Fatalf("%s: rows %d -> %d", c.name, len(before), len(after))
		}
	}
}

// TestJoin_LostRace -- I5: a concurrent writer that commits the master row while join-brand
// waits on it makes the INSERT a no-op, and then nothing else is written (no stamp, lang or
// deletion); the handler re-reads the plan and renders that outcome.
func TestJoin_LostRace(t *testing.T) {
	f := newJoinFixture(t)
	_, camp := f.langCamp("acme win-back race", "fr", f.winback)
	id, uu := f.bareSub("race@x", `{}`)
	f.addRow(id, f.winback, "confirmed", `{}`)

	tx, err := f.db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'unconfirmed')`, id, f.news); err != nil {
		t.Fatal(err)
	}

	type result struct {
		tpl  string
		data joinTpl
		err  error
	}
	done := make(chan result, 1)
	go func() {
		e := echo.New()
		rend := &capturingRenderer{}
		e.Renderer = rend
		req := httptest.NewRequest(http.MethodPost, "/subscription/join/"+camp+"/"+uu, nil)
		c := e.NewContext(req, httptest.NewRecorder())
		c.SetParamNames("campUUID", "subUUID")
		c.SetParamValues(camp, uu)
		err := f.app.JoinConfirm(c)
		d, _ := rend.data.(joinTpl)
		done <- result{rend.name, d, err}
	}()

	// Wait until join-brand is blocked on the uncommitted row, then let the other writer win.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var n int
		f.db.Get(&n, `SELECT COUNT(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`)
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("join-brand never blocked on the concurrent row")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	r := <-done
	if r.err != nil {
		t.Fatalf("JoinConfirm: %v", r.err)
	}
	if r.tpl != "join" || r.data.State != joinpage.StateCannot {
		t.Fatalf("rendered %s/%s, want the re-read outcome (cannot)", r.tpl, r.data.State)
	}
	if st := f.statusOf(id, f.news); st != "unconfirmed" {
		t.Fatalf("master row %s, want the winner's unconfirmed", st)
	}
	if m := f.meta(id, f.news); m["join"] != nil {
		t.Fatalf("the lost INSERT recorded meta.join: %v", m)
	}
	a := f.attribs(id)
	if a["confirms"] != nil || a["lang"] != nil {
		t.Fatalf("the lost INSERT stamped attribs: %v", a)
	}
	if !f.hasRow(id, f.winback) {
		t.Fatalf("the lost INSERT deleted the win-back row")
	}
}

// TestJoin_BrandResolution -- I6 (brand resolution): the brand is the campaign's one brand tag,
// a transactional sending list included (the target is still the master); two brands or none
// resolve no brand; a brand with no single public active list resolves no master.
func TestJoin_BrandResolution(t *testing.T) {
	f := newJoinFixture(t)
	resID, uu := f.bareSub("res@x", `{}`)
	// A recipient of the transactional sending list (D4 row 6a needs a row on the campaign's list).
	f.addRow(resID, f.txn, "confirmed", `{}`)

	_, txnCamp := f.langCamp("acme receipt", "", f.txn)
	if p := f.plan(uu, txnCamp); p.Outcome != models.JoinOutcomeJoin || p.Brand != "acme" || p.ListID != f.news {
		t.Fatalf("transactional sending list: %+v, want join acme on %d", p, f.news)
	}
	_, wbCamp := f.langCamp("acme wb", "fr", f.winback)
	if p := f.plan(uu, wbCamp); p.Brand != "acme" || p.ListID != f.news || p.Lang != "fr" || p.FromEmail != "Acme <hello@acme.test>" || p.Email != "res@x" {
		t.Fatalf("win-back campaign plan: %+v", p)
	}
	_, zetaCamp := f.langCamp("zeta", "", f.zeta)
	if p := f.plan(uu, zetaCamp); p.Brand != "zeta" || p.ListID != f.zeta {
		t.Fatalf("zeta: %+v", p)
	}
	_, two := f.langCamp("two", "", f.winback, f.zeta)
	if p := f.plan(uu, two); p.Outcome != models.JoinOutcomeNoBrand || p.Brand != "" {
		t.Fatalf("two brands: %+v", p)
	}
	_, mixed := f.langCamp("mixed", "", f.winback, f.untagged)
	if p := f.plan(uu, mixed); p.Brand != "acme" {
		t.Fatalf("a tagged list beside an untagged one: %+v", p)
	}
	_, bare := f.langCamp("bare", "", f.bare)
	if p := f.plan(uu, bare); p.Outcome != models.JoinOutcomeNoMaster || p.Brand != "bare" {
		t.Fatalf("no master: %+v", p)
	}
	// Two public active lists of one brand (the liyora case) -- no master.
	f.list("zeta second public", "public", "brand:zeta")
	if p := f.plan(uu, zetaCamp); p.Outcome != models.JoinOutcomeNoMaster {
		t.Fatalf("two public lists: %+v", p)
	}
}

// TestJoin_RecordAndStamp -- I7: the master row carries meta.join {at, campaign_id}; attribs
// gains confirms[brand] = {at, from: join} beside the existing confirms; attribs.lang is set to
// the campaign's language only when absent; no signups key is written.
func TestJoin_RecordAndStamp(t *testing.T) {
	f := newJoinFixture(t)
	campID, camp := f.langCamp("acme wb stamp", "fr", f.winback)

	id, uu := f.bareSub("stamp@x", `{"confirms":{"zeta":{"at":"2026-01-01T00:00:00Z","from":"optout"}},"signups":{"zeta":{"at":"2026-01-01T00:00:00Z"}}}`)
	f.addRow(id, f.winback, "confirmed", `{}`)
	if tpl, _ := f.call(http.MethodPost, camp, uu); tpl != "joined" {
		t.Fatalf("POST rendered %s", tpl)
	}

	m := f.meta(id, f.news)
	j, _ := m["join"].(map[string]any)
	if j == nil {
		t.Fatalf("no meta.join: %v", m)
	}
	at, _ := j["at"].(string)
	if _, err := time.Parse(time.RFC3339, at); err != nil || !strings.HasSuffix(at, "Z") {
		t.Fatalf("meta.join.at %q is not a UTC instant", at)
	}
	if cid, _ := j["campaign_id"].(float64); int(cid) != campID {
		t.Fatalf("meta.join.campaign_id %v, want %d", j["campaign_id"], campID)
	}
	if len(m) != 1 {
		t.Fatalf("master meta carries more than join: %v", m)
	}

	a := f.attribs(id)
	confirms, _ := a["confirms"].(map[string]any)
	acme, _ := confirms["acme"].(map[string]any)
	if acme == nil || acme["from"] != "join" || acme["at"] != at {
		t.Fatalf("confirms.acme = %v, want {at %s, from join}", confirms["acme"], at)
	}
	if confirms["zeta"] == nil {
		t.Fatalf("the existing confirms entry was dropped: %v", confirms)
	}
	if a["lang"] != "fr" {
		t.Fatalf("lang %v, want fr (absent before)", a["lang"])
	}
	signups, _ := a["signups"].(map[string]any)
	if len(signups) != 1 || signups["acme"] != nil {
		t.Fatalf("signups changed: %v", signups)
	}

	// A subscriber who already has a language keeps it.
	id2, uu2 := f.bareSub("stamp-en@x", `{"lang":"en"}`)
	f.addRow(id2, f.winback, "confirmed", `{}`)
	if tpl, _ := f.call(http.MethodPost, camp, uu2); tpl != "joined" {
		t.Fatalf("POST rendered %s", tpl)
	}
	if a := f.attribs(id2); a["lang"] != "en" || a["signups"] != nil {
		t.Fatalf("existing lang overwritten or signups written: %v", a)
	}
}

// TestJoin_Idempotent -- I8: a second Confirm finds `already` and writes nothing.
func TestJoin_Idempotent(t *testing.T) {
	f := newJoinFixture(t)
	_, camp := f.langCamp("acme wb idem", "fr", f.winback)
	id, uu := f.bareSub("idem@x", `{}`)
	f.addRow(id, f.winback, "confirmed", `{}`)

	if tpl, _ := f.call(http.MethodPost, camp, uu); tpl != "joined" {
		t.Fatalf("first POST rendered %s", tpl)
	}
	before := f.snapshot()
	tpl, data := f.call(http.MethodPost, camp, uu)
	if tpl != "join" || data.State != joinpage.StateAlready {
		t.Fatalf("second POST rendered %s/%s, want already", tpl, data.State)
	}
	if after := f.snapshot(); after != before {
		t.Fatalf("the second POST wrote\n before %s\n after  %s", before, after)
	}
	var n int
	f.db.Get(&n, `SELECT COUNT(*) FROM subscriber_lists WHERE subscriber_id = $1 AND list_id = $2`, id, f.news)
	if n != 1 {
		t.Fatalf("%d master rows", n)
	}
}
