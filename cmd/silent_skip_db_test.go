package main

// Fork (60-day opens) -- integrations listmonk/SIXTY-DAY-OPENS-SPEC T1-T6, T8, T9 (the route
// permission) and T11 against a real database, through the REAL statements: brand_silent_skips and
// open_observable_provider (schema.sql), the claim (next-campaigns, through store.NextCampaigns), the
// batches (next-campaign-subscribers, through store.NextSubscribers), get-campaign-lang-audience
// (core.FillAudience), get-campaign-silent-audience (the GET handler), get-campaign-matrix and
// query-analytics-campaigns. Shares newLinkHarness and ssFixture (LISTMONK_TEST_PG opt-in). T1 runs
// the claim and the batches through the statements as they were before this change
// (cmd/testdata/*.pre-silent.sql, verbatim from the previous commit) beside the current ones.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/goyesql/v2"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// silentWorld is one brand (acme) with a target list T (the broadcast under test goes to T; T has no
// history of its own) and history lists whose finished, observable broadcasts a member is counted
// on by holding an old row there.
type silentWorld struct {
	f        ssFixture
	h        *linkHarness
	target   int // T, brand:acme
	three    int // L3: three observable broadcasts started 10, 20 and 30 days ago
	two      int // L2: two observable broadcasts (10, 20 days)
	aged     int // Lold: broadcasts 10, 20 and 61 days ago (two in the window)
	unobs    int // Lunobs: three broadcasts with no attributed view
	beta     int // another brand's list
	observer int // a subscriber who opens every observable broadcast (never on T)
	b3       []int
	betaCamp int
	link     int // the Manage Preferences link
}

func newSilentWorld(t *testing.T) *silentWorld {
	h := newLinkHarness(t)
	f := ssFixture{t: t, h: h}
	w := &silentWorld{f: f, h: h}
	w.target = f.list("Acme newsletter", "brand:acme")
	w.three = f.list("Acme three", "brand:acme")
	w.two = f.list("Acme two", "brand:acme")
	w.aged = f.list("Acme aged", "brand:acme")
	w.unobs = f.list("Acme untracked", "brand:acme")
	w.beta = f.list("Beta newsletter", "brand:beta")
	w.observer = f.sub("observer@acme.test", "enabled")
	for _, ago := range []string{"10 days", "20 days", "30 days"} {
		w.b3 = append(w.b3, w.bcast("L3 "+ago, ago, true, w.three))
	}
	for _, ago := range []string{"10 days", "20 days"} {
		w.bcast("L2 "+ago, ago, true, w.two)
	}
	for _, ago := range []string{"10 days", "20 days", "61 days"} {
		w.bcast("Lold "+ago, ago, true, w.aged)
	}
	for _, ago := range []string{"10 days", "20 days", "30 days"} {
		w.bcast("Lunobs "+ago, ago, false, w.unobs)
	}
	w.betaCamp = w.bcast("Beta", "800 days", true, w.beta)
	if err := h.db.Get(&w.link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://manage-preferences.curatedfor.you/p') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	return w
}

// bcast inserts a finished regular broadcast started `ago` before now on lists, with a max
// subscriber id above every fixture id; observable = the observer's view (a non-NULL subscriber),
// else only an anonymous view (individual tracking off).
func (w *silentWorld) bcast(name, ago string, observable bool, lists ...int) int {
	w.f.t.Helper()
	id := w.f.camp(name, false, "regular", "finished", `{}`, lists...)
	w.h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - $2::INTERVAL, max_subscriber_id = 2000000000, sent = 1 WHERE id = $1`, id, ago)
	if observable {
		w.h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id, created_at) VALUES ($1, $2, NOW() - $3::INTERVAL)`, id, w.observer, ago)
	} else {
		w.h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id, created_at) VALUES ($1, NULL, NOW() - $2::INTERVAL)`, id, ago)
	}
	return id
}

// member creates a subscriber on T (row now) and on the history lists with a row created and last
// changed 90 days ago (before every broadcast in the window).
func (w *silentWorld) member(email string, history ...int) int {
	w.f.t.Helper()
	s := w.f.sub(email, "enabled")
	w.f.join(s, w.target, "confirmed")
	for _, l := range history {
		w.f.join(s, l, "confirmed")
		w.h.db.MustExec(`UPDATE subscriber_lists SET created_at = NOW() - INTERVAL '90 days', updated_at = NOW() - INTERVAL '90 days',
			confirmed_at = NOW() - INTERVAL '90 days'
			WHERE subscriber_id = $1 AND list_id = $2`, s, l)
	}
	return s
}

func (w *silentWorld) view(camp, sub int, ago string, client any) {
	w.f.t.Helper()
	w.h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id, created_at, client) VALUES ($1, $2, NOW() - $3::INTERVAL, $4)`, camp, sub, ago, client)
}

func (w *silentWorld) stamp(sub int, seed string) {
	w.f.t.Helper()
	w.h.db.MustExec(`UPDATE subscribers SET attribs = attribs || JSONB_BUILD_OBJECT('open_seed', $2::JSONB) WHERE id = $1`, sub, seed)
}

// fn is brand_silent_skips(camp, preview) as sorted subscriber ids.
func (w *silentWorld) fn(camp int, preview bool) []int {
	w.f.t.Helper()
	var ids []int
	if err := w.h.db.Select(&ids, `SELECT subscriber_id FROM brand_silent_skips($1, $2) ORDER BY subscriber_id`, camp, preview); err != nil {
		w.f.t.Fatalf("brand_silent_skips(%d, %v): %v", camp, preview, err)
	}
	return ids
}

func (w *silentWorld) providers(camp int) map[int]string {
	w.f.t.Helper()
	var rows []struct {
		ID       int    `db:"subscriber_id"`
		Provider string `db:"provider"`
	}
	if err := w.h.db.Select(&rows, `SELECT subscriber_id, provider FROM brand_silent_skips($1, TRUE)`, camp); err != nil {
		w.f.t.Fatal(err)
	}
	out := map[int]string{}
	for _, r := range rows {
		out[r.ID] = r.Provider
	}
	return out
}

func has(ids []int, id int) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// T3 (I2, I9, I14) -- the rule, one member per flipped condition, each flipped alone.
func TestSilentSkipRule(t *testing.T) {
	w := newSilentWorld(t)
	f := w.f
	b1 := w.b3[0]

	skipped := map[string]int{
		"gmail, three sends, silent":     w.member("silent@gmail.com", w.three),
		"googlemail":                     w.member("Silent.Two@GoogleMail.com", w.three),
		"yahoo country domain":           w.member("silent@yahoo.co.uk", w.three),
		"aol":                            w.member("silent@aol.com", w.three),
		"a starter stamp 61 days old":    w.member("stamp61@gmail.com", w.three),
		"a gmail-proxy view 61 days old": w.member("oldview@gmail.com", w.three),
	}
	sent := map[string]int{
		"another provider":                    w.member("other@outlook.com", w.three),
		"two sends":                           w.member("two@gmail.com", w.two),
		"a send 61 days old":                  w.member("aged@gmail.com", w.aged),
		"a gmail-proxy view in the window":    w.member("proxy@gmail.com", w.three),
		"an automation view":                  w.member("auto@gmail.com", w.three),
		"a bare-mozilla view":                 w.member("bare@gmail.com", w.three),
		"a NULL-client view":                  w.member("nullclient@gmail.com", w.three),
		"a Manage Preferences click":          w.member("click@gmail.com", w.three),
		"a view on a brand evergreen":         w.member("evergreen@gmail.com", w.three),
		"a starter stamp 59 days old":         w.member("stamp59@gmail.com", w.three),
		"an outlook-windows view 2 years ago": w.member("outlook@gmail.com", w.three),
		"three broadcasts, none observable":   w.member("unobs@gmail.com", w.unobs),
	}
	w.stamp(skipped["a starter stamp 61 days old"], `{"acme": {"at": "`+isoDaysAgo(t, w, 61)+`", "source": "zma"}}`)
	w.view(b1, skipped["a gmail-proxy view 61 days old"], "61 days", "gmail-proxy")
	w.view(b1, sent["a gmail-proxy view in the window"], "5 days", "gmail-proxy")
	w.view(b1, sent["an automation view"], "5 days", "automation")
	w.view(b1, sent["a bare-mozilla view"], "5 days", "bare-mozilla")
	w.view(b1, sent["a NULL-client view"], "5 days", nil)
	w.h.db.MustExec(`INSERT INTO link_clicks (campaign_id, link_id, subscriber_id, created_at) VALUES ($1, $2, $3, NOW() - INTERVAL '3 days')`,
		b1, w.link, sent["a Manage Preferences click"])
	welcomeList := f.list("Acme welcome", "brand:acme")
	welcome := f.camp("Acme welcome", true, "regular", "running", `{}`, welcomeList)
	w.view(welcome, sent["a view on a brand evergreen"], "2 days", "gmail-proxy")
	w.stamp(sent["a starter stamp 59 days old"], `{"acme": {"at": "`+isoDaysAgo(t, w, 59)+`", "source": "zma"}}`)
	w.view(w.betaCamp, sent["an outlook-windows view 2 years ago"], "730 days", "outlook-windows")

	// Held during the three broadcasts, returned yesterday: the row's status change bumps updated_at
	// (the hold-release trigger), so none of the three counts (D1.2, S1).
	held := w.member("held@gmail.com", w.three)
	w.h.db.MustExec(`UPDATE subscriber_lists SET status = 'unsubscribed', meta = '{"hold": {"reason": "never-engaged"}}' WHERE subscriber_id = $1 AND list_id = $2`, held, w.three)
	w.h.db.MustExec(`UPDATE subscriber_lists SET created_at = NOW() - INTERVAL '90 days', updated_at = NOW() - INTERVAL '90 days' WHERE subscriber_id = $1 AND list_id = $2`, held, w.three)
	w.h.db.MustExec(`UPDATE subscriber_lists SET status = 'confirmed' WHERE subscriber_id = $1 AND list_id = $2`, held, w.three)
	w.h.db.MustExec(`UPDATE subscriber_lists SET updated_at = NOW() - INTERVAL '1 day' WHERE subscriber_id = $1 AND list_id = $2`, held, w.three)
	sent["held during the three broadcasts, returned yesterday"] = held

	// Unsubscribed 40 days ago, re-opted in today through the REAL writer (add-subscribers-to-lists
	// with an explicit status -- the storefront re-join and the Manage Preferences tick). That writer
	// does not move updated_at, so the row still carries the unsubscribe's time; the re-confirmation
	// stamps confirmed_at, which is what keeps the three broadcasts sent meanwhile from counting
	// (integrations SIXTY-DAY-OPENS-SPEC Stage 4 V1).
	back := w.member("back@gmail.com", w.three)
	w.h.db.MustExec(`UPDATE subscriber_lists SET status = 'unsubscribed', updated_at = NOW() - INTERVAL '40 days' WHERE subscriber_id = $1 AND list_id = $2`, back, w.three)
	if _, err := w.h.q.AddSubscribersToLists.Exec(pq.Array([]int{back}), pq.Array([]int{w.three}), "confirmed"); err != nil {
		t.Fatalf("re-add through add-subscribers-to-lists: %v", err)
	}
	sent["unsubscribed 40 days ago, re-opted in today through the real writer"] = back

	on := f.camp("Acme mid", false, "regular", "draft", `{"silent_skip": true}`, w.target)
	got := w.fn(on, false)
	for name, id := range skipped {
		if !has(got, id) {
			t.Errorf("%s (%d): sent, want skipped (rows %v)", name, id, got)
		}
	}
	for name, id := range sent {
		if has(got, id) {
			t.Errorf("%s (%d): skipped, want sent", name, id)
		}
	}
	if len(got) != len(skipped) {
		t.Errorf("rows %v, want exactly the %d skipped members", got, len(skipped))
	}
	if p := w.providers(on); p[skipped["yahoo country domain"]] != "yahoo" || p[skipped["aol"]] != "yahoo" ||
		p[skipped["gmail, three sends, silent"]] != "gmail" || p[skipped["googlemail"]] != "gmail" {
		t.Errorf("providers %v", p)
	}
	// The preview ignores the stored switch.
	off := f.camp("Acme off", false, "regular", "draft", `{}`, w.target)
	if got := w.fn(off, true); len(got) != len(skipped) {
		t.Errorf("preview of a toggle-off campaign: %v, want %d rows", got, len(skipped))
	}

	// Applicability (I9): evergreen, opt-in, untagged list -> no rows even with the attrib true.
	scratch := f.list("Scratch")
	for _, id := range []int{
		w.f.camp("Journey", true, "regular", "draft", `{"silent_skip": true}`, w.target),
		w.f.camp("Optin", false, "optin", "draft", `{"silent_skip": true}`, w.target),
		w.f.camp("Untagged", false, "regular", "draft", `{"silent_skip": true}`, scratch),
	} {
		for _, preview := range []bool{false, true} {
			if got := w.fn(id, preview); len(got) != 0 {
				t.Errorf("campaign %d (preview %v): %v, want no rows", id, preview, got)
			}
		}
	}
}

// isoDaysAgo is the stored starter-stamp shape (D6), n days before the database's now.
func isoDaysAgo(t *testing.T, w *silentWorld, n int) string {
	t.Helper()
	var s string
	if err := w.h.db.Get(&s, `SELECT TO_CHAR((NOW() - MAKE_INTERVAL(days => $1)) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`, n); err != nil {
		t.Fatal(err)
	}
	return s
}

// T2 (I1) -- toggle off: no rows, silent_skipped 0 (and the served total is Smart Sending's alone).
func TestSilentSkipOff(t *testing.T) {
	w := newSilentWorld(t)
	silent := w.member("silent@gmail.com", w.three)
	w.member("plain@acme.test", w.three)
	for _, attribs := range []string{`{}`, `{"silent_skip": false}`} {
		id := w.f.camp("Off "+attribs, false, "regular", "draft", attribs, w.target)
		if got := w.fn(id, false); len(got) != 0 {
			t.Fatalf("%s: %v, want no rows", attribs, got)
		}
		cm, err := w.h.app.core.GetCampaign(id, "", "")
		if err != nil {
			t.Fatal(err)
		}
		w.h.app.core.FillAudience(&cm)
		if cm.SilentSkipOn || !cm.AudienceSilentSkipped.Valid || cm.AudienceSilentSkipped.Int != 0 || cm.AudienceSkipped.Int != 0 {
			t.Fatalf("%s: on %v silent %v total %v, want false / 0 / 0", attribs, cm.SilentSkipOn, cm.AudienceSilentSkipped, cm.AudienceSkipped)
		}
	}
	on := w.f.camp("On", false, "regular", "draft", `{"silent_skip": true}`, w.target)
	cm, _ := w.h.app.core.GetCampaign(on, "", "")
	w.h.app.core.FillAudience(&cm)
	if !cm.SilentSkipOn || cm.AudienceSilentSkipped.Int != 1 || cm.AudienceSkipped.Int != 1 || cm.Audience.Int != 2 {
		t.Fatalf("on: on %v silent %v total %v audience %v, want true / 1 / 1 / 2 (silent %d)", cm.SilentSkipOn, cm.AudienceSilentSkipped, cm.AudienceSkipped, cm.Audience, silent)
	}
}

// --- T1 (I1) -- the differential over the real claim path ---

func preSilentStmt(t *testing.T, db *sqlx.DB, file, name string) *sqlx.Stmt {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	qs, err := goyesql.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	q, ok := qs[name]
	if !ok {
		t.Fatalf("%s: no %s", file, name)
	}
	st, err := db.Preparex(q.Query)
	if err != nil {
		t.Fatalf("prepare %s: %v", name, err)
	}
	return st
}

type claimPath struct {
	nextCampaigns, nextSubscribers *sqlx.Stmt
}

// drain claims camp (every other campaign excluded, with zero sent counts) and batches it to
// exhaustion through the path's statements, the way store.NextCampaigns / NextSubscribers do.
func (p claimPath) drain(t *testing.T, h *linkHarness, camp int, others []int) []int {
	t.Helper()
	zeros := make([]int64, len(others))
	ids := make([]int64, len(others))
	for i, o := range others {
		ids[i] = int64(o)
	}
	if _, err := p.nextCampaigns.Exec(pq.Int64Array(ids), pq.Int64Array(zeros)); err != nil {
		t.Fatalf("next-campaigns: %v", err)
	}
	var got []int
	for i := 0; i < 50; i++ {
		var camps []runningCamp
		if err := h.q.GetRunningCampaign.Select(&camps, camp); err != nil {
			t.Fatal(err)
		}
		if len(camps) == 0 {
			t.Fatalf("campaign %d not running after the claim", camp)
		}
		var listIDs []int
		for _, c := range camps {
			listIDs = append(listIDs, c.ListID)
		}
		var out []models.Subscriber
		if err := p.nextSubscribers.Select(&out, camps[0].CampaignID, camps[0].CampaignType, camps[0].LastSubscriberID,
			camps[0].MaxSubscriberID, pq.Array(listIDs), 2, camps[0].Lang); err != nil {
			t.Fatalf("next-campaign-subscribers: %v", err)
		}
		if len(out) == 0 {
			break
		}
		for _, s := range out {
			got = append(got, s.ID)
		}
	}
	sort.Ints(got)
	return got
}

type skipRow struct {
	Sub     int         `db:"subscriber_id"`
	Journey *int        `db:"journey_campaign_id"`
	DueList *int        `db:"due_list_id"`
	Reason  *string     `db:"reason"`
	Sent    pq.NullTime `db:"journey_sent_at"`
	DueAt   pq.NullTime `db:"due_at"`
}

func skipRowsOf(t *testing.T, h *linkHarness, camp int) []skipRow {
	t.Helper()
	var rows []skipRow
	if err := h.db.Select(&rows, `SELECT subscriber_id, journey_campaign_id, due_list_id, reason, journey_sent_at, due_at
		FROM campaign_smart_skips WHERE campaign_id = $1 ORDER BY subscriber_id`, camp); err != nil {
		t.Fatal(err)
	}
	return rows
}

// The differential: one seeded database holding members who WOULD be silent-skipped, a Smart
// Sending skip, a brand hold, two languages and a double opt-in list; for silent_skip absent and
// false, two identical broadcasts, one claimed and batched through the pre-silent statements, one
// through the current query set. Identical recipients, to_send and skip rows (reason aside).
func TestSilentSkipDifferential(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	old := claimPath{
		nextCampaigns:   preSilentStmt(t, h.db, "next-campaigns.pre-silent.sql", "next-campaigns"),
		nextSubscribers: preSilentStmt(t, h.db, "next-campaign-subscribers.pre-silent.sql", "next-campaign-subscribers"),
	}
	cur := claimPath{nextCampaigns: h.q.NextCampaigns, nextSubscribers: h.q.NextCampaignSubscribers}

	double := f.list("Acme VIP", "brand:acme")
	h.db.MustExec(`UPDATE lists SET optin = 'double' WHERE id = $1`, double)
	silentA := w.member("silent-a@gmail.com", w.three)
	silentB := w.member("silent-b@yahoo.com", w.three)
	fr := w.member("fr@gmail.com", w.three)
	h.db.MustExec(`UPDATE subscribers SET attribs = '{"lang": "fr"}' WHERE id = $1`, fr)
	plain := w.member("plain@acme.test")
	smart := w.member("smart@gmail.com", w.three)
	journeyList := f.list("Acme first order", "brand:acme")
	journey := f.camp("Acme thanks", true, "regular", "running", `{}`, journeyList)
	f.send(journey, smart, "1 hour")
	heldSub := w.member("held@gmail.com", w.three)
	h.db.MustExec(`UPDATE subscriber_lists SET status = 'unsubscribed', meta = '{"hold": {"reason": "never-engaged"}}' WHERE subscriber_id = $1 AND list_id = $2`, heldSub, w.three)
	h.db.MustExec(`UPDATE settings SET value = 'true' WHERE key = 'app.brand_hold_scope'`)
	dblConfirmed := f.sub("dbl@gmail.com", "enabled")
	f.join(dblConfirmed, double, "confirmed")
	dblPending := f.sub("pending@gmail.com", "enabled")
	f.join(dblPending, double, "unconfirmed")

	for _, attribs := range []string{`{}`, `{"silent_skip": false}`} {
		a := f.camp("Old "+attribs, false, "regular", "running", attribs, w.target, double)
		b := f.camp("New "+attribs, false, "regular", "running", attribs, w.target, double)
		// The silent members are in the audience and WOULD be skipped with the toggle on.
		if p := w.fn(b, true); !has(p, silentA) || !has(p, silentB) {
			t.Fatalf("fixture: preview %v lacks the silent members %d %d", p, silentA, silentB)
		}
		var all []int
		h.db.Select(&all, `SELECT id FROM campaigns WHERE id NOT IN ($1, $2)`, a, b)
		gotOld := old.drain(t, h, a, append(append([]int{}, all...), b))
		gotNew := cur.drain(t, h, b, append(append([]int{}, all...), a))
		if fmt.Sprint(gotOld) != fmt.Sprint(gotNew) {
			t.Fatalf("%s: recipients differ\n old %v\n new %v", attribs, gotOld, gotNew)
		}
		if !has(gotNew, silentA) || !has(gotNew, silentB) || !has(gotNew, plain) || !has(gotNew, dblConfirmed) ||
			has(gotNew, smart) || has(gotNew, heldSub) || has(gotNew, dblPending) {
			t.Fatalf("%s: recipients %v (silent %d %d plain %d dbl %d smart %d held %d pending %d)", attribs, gotNew,
				silentA, silentB, plain, dblConfirmed, smart, heldSub, dblPending)
		}
		var toOld, toNew int
		h.db.Get(&toOld, `SELECT to_send FROM campaigns WHERE id = $1`, a)
		h.db.Get(&toNew, `SELECT to_send FROM campaigns WHERE id = $1`, b)
		if toOld != toNew || toNew != len(gotNew) {
			t.Fatalf("%s: to_send old %d new %d, recipients %d", attribs, toOld, toNew, len(gotNew))
		}
		ro, rn := skipRowsOf(t, h, a), skipRowsOf(t, h, b)
		if len(ro) != len(rn) || len(rn) != 1 {
			t.Fatalf("%s: skip rows old %+v new %+v", attribs, ro, rn)
		}
		for i := range ro {
			if ro[i].Sub != rn[i].Sub || ro[i].Journey == nil || rn[i].Journey == nil || !ro[i].Sent.Time.Equal(rn[i].Sent.Time) ||
				*ro[i].Journey != *rn[i].Journey || ro[i].DueList != nil || rn[i].DueList != nil {
				t.Fatalf("%s: skip row %d old %+v new %+v", attribs, i, ro[i], rn[i])
			}
			if ro[i].Reason != nil || rn[i].Reason == nil || *rn[i].Reason != "lookback" {
				t.Fatalf("%s: reasons old %v new %v, want NULL / lookback", attribs, ro[i].Reason, rn[i].Reason)
			}
		}
		h.db.MustExec(`UPDATE campaigns SET status = 'cancelled' WHERE id IN ($1, $2)`, a, b)
	}
}

// T4 (I3, I4) -- a toggle-on broadcast: sent == to_send, a recorded skip never in a batch, and a
// re-claim after a pause records nothing at or below the checkpoint.
func TestSilentSkipClaim(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	// o1 is the lowest id and the first batch's recipient: two sends only, so not silent at the claim.
	o1 := w.member("o1@gmail.com", w.two)
	silent := []int{w.member("s1@gmail.com", w.three), w.member("s2@yahoo.com", w.three)}
	sends := []int{o1, w.member("o2@acme.test", w.three), w.member("o3@gmail.com", w.two), w.member("o4@acme.test")}
	b := f.camp("Acme mid", false, "regular", "running", `{"silent_skip": true}`, w.target)
	st := &store{queries: h.q, core: h.app.core}
	if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
		t.Fatalf("next-campaigns: %v", err)
	}
	rows := skipRowsOf(t, h, b)
	if len(rows) != 2 || rows[0].Sub != silent[0] || rows[1].Sub != silent[1] {
		t.Fatalf("skip rows %+v, want %v", rows, silent)
	}
	for _, r := range rows {
		if r.Reason == nil || *r.Reason != "silent" || r.Journey != nil || r.DueList != nil {
			t.Fatalf("skip row %+v, want reason silent and no pairs", r)
		}
	}
	var toSend int
	h.db.Get(&toSend, `SELECT to_send FROM campaigns WHERE id = $1`, b)
	var got []int
	for i := 0; i < 10; i++ {
		out, err := st.NextSubscribers(b, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) == 0 {
			break
		}
		for _, s := range out {
			got = append(got, s.ID)
		}
		if i == 0 {
			// I4 -- pause after one batch (o1 sent, the checkpoint is o1), o1 becomes silent (a third
			// counted send), re-claim: no row for o1, the earlier rows kept, to_send unchanged.
			if len(out) != 1 || out[0].ID != o1 {
				t.Fatalf("first batch %v, want [%d]", got, o1)
			}
			f.join(o1, w.three, "confirmed")
			h.db.MustExec(`UPDATE subscriber_lists SET created_at = NOW() - INTERVAL '90 days', updated_at = NOW() - INTERVAL '90 days',
				confirmed_at = NOW() - INTERVAL '90 days'
				WHERE subscriber_id = $1 AND list_id = $2`, o1, w.three)
			probe := f.camp("Probe", false, "regular", "draft", `{"silent_skip": true}`, w.target)
			if !has(w.fn(probe, false), o1) {
				t.Fatalf("fixture: o1 is not silent now")
			}
			h.db.MustExec(`UPDATE campaigns SET status = 'paused' WHERE id = $1`, b)
			h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE id = $1`, b)
			if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
				t.Fatalf("re-claim: %v", err)
			}
			after := skipRowsOf(t, h, b)
			if len(after) != 2 || after[0].Sub != silent[0] || after[1].Sub != silent[1] {
				t.Fatalf("after the re-claim skip rows %+v, want only %v (none for %d at the checkpoint)", after, silent, o1)
			}
			var again int
			h.db.Get(&again, `SELECT to_send FROM campaigns WHERE id = $1`, b)
			if again != toSend {
				t.Fatalf("re-claim to_send %d, want %d", again, toSend)
			}
		}
	}
	sort.Ints(got)
	for _, s := range silent {
		if has(got, s) {
			t.Fatalf("a recorded skip %d was batched: %v", s, got)
		}
	}
	for _, s := range sends {
		if !has(got, s) {
			t.Fatalf("%d not sent: %v", s, got)
		}
	}
	if len(got) != toSend {
		t.Fatalf("sent %d (%v) != to_send %d", len(got), got, toSend)
	}
}

// T5 (I5) -- a member silent-skipped on three finished broadcasts, whose earlier sends age out, has
// fewer than three counted sends and is sent; brand_eligible_sends does not count the skipped ones.
func TestSilentSkipNotASend(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	x := w.member("x@gmail.com", w.three)
	y := w.member("y@acme.test", w.three)
	h.db.MustExec(`UPDATE subscriber_lists SET created_at = NOW() - INTERVAL '90 days', confirmed_at = NOW() - INTERVAL '90 days'
		WHERE list_id = $1`, w.target)
	st := &store{queries: h.q, core: h.app.core}
	for i, ago := range []string{"9 days", "8 days", "7 days"} {
		c := f.camp(fmt.Sprintf("Mid %d", i), false, "regular", "running", `{"silent_skip": true}`, w.target)
		if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
			t.Fatal(err)
		}
		if r := skipRowsOf(t, h, c); len(r) != 1 || r[0].Sub != x {
			t.Fatalf("mid %d skip rows %+v, want %d", i, r, x)
		}
		h.db.MustExec(`UPDATE campaigns SET status = 'finished', started_at = NOW() - $2::INTERVAL WHERE id = $1`, c, ago)
		h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id) VALUES ($1, $2)`, c, y)
	}
	// The three L3 broadcasts age out of the window.
	h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - INTERVAL '70 days' WHERE id = ANY($1)`, pq.Array(w.b3))
	probe := f.camp("Probe", false, "regular", "draft", `{"silent_skip": true}`, w.target)
	if got := w.fn(probe, false); has(got, x) {
		t.Fatalf("x still skipped with only skipped broadcasts in the window: %v", got)
	}
	el := map[int]int{}
	var rows []eligRow
	if err := h.db.Select(&rows, `SELECT * FROM brand_eligible_sends($1)`, w.target); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		el[r.SubscriberID] = r.Sends
	}
	if el[x] != 3 || el[y] != 6 {
		t.Fatalf("brand_eligible_sends x %d y %d, want 3 (the skipped three excluded) / 6", el[x], el[y])
	}
}

// eligRow -- brand_eligible_sends' two counted columns (h.db is the app's Unsafe handle, so the
// first_eligible_send_at column is simply not scanned).
type eligRow struct {
	SubscriberID int `db:"subscriber_id"`
	Sends        int `db:"eligible_sends"`
}

// T6 (I6) -- a member who is both a look-back skip and silent gets ONE row, reason lookback, the
// journey pair intact; Smart Sending's counts are the same with the toggle on and off.
func TestSilentSkipPrecedence(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	both := w.member("both@gmail.com", w.three)
	only := w.member("only@gmail.com", w.three)
	journeyList := f.list("Acme first order", "brand:acme")
	j := f.camp("Acme thanks", true, "regular", "running", `{}`, journeyList)
	f.send(j, both, "1 hour")

	on := f.camp("On", false, "regular", "draft", `{"silent_skip": true}`, w.target)
	off := f.camp("Off", false, "regular", "draft", `{}`, w.target)
	aOn, err := h.app.core.CampaignLangAudience(on)
	if err != nil {
		t.Fatal(err)
	}
	aOff, _ := h.app.core.CampaignLangAudience(off)
	if aOn.Skipped != aOff.Skipped || aOn.Skipped != 1 || aOn.SilentSkipped != 1 || aOff.SilentSkipped != 0 {
		t.Fatalf("on %+v off %+v, want Smart Sending 1 / 1 and silent 1 / 0", aOn, aOff)
	}
	h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE id = ANY($1)`, pq.Array([]int{on, off}))
	st := &store{queries: h.q, core: h.app.core}
	if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
		t.Fatal(err)
	}
	rows := skipRowsOf(t, h, on)
	if len(rows) != 2 || rows[0].Sub != both || rows[1].Sub != only {
		t.Fatalf("rows %+v", rows)
	}
	if rows[0].Reason == nil || *rows[0].Reason != "lookback" || rows[0].Journey == nil || *rows[0].Journey != j || !rows[0].Sent.Valid {
		t.Fatalf("the both-row %+v, want lookback with journey %d", rows[0], j)
	}
	if rows[1].Reason == nil || *rows[1].Reason != "silent" {
		t.Fatalf("the silent row %+v", rows[1])
	}
	offRows := skipRowsOf(t, h, off)
	if len(offRows) != 1 || offRows[0].Sub != both || *offRows[0].Reason != "lookback" {
		t.Fatalf("toggle-off rows %+v", offRows)
	}
}

// T8 (I8) -- the function over hostile data returns without error and fails toward sending.
func TestSilentSkipHostileData(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	seeds := map[string]bool{ // seed -> still skipped
		`"x"`:                   true,
		`7`:                     true,
		`{}`:                    true,
		`{"acme": "x"}`:         true,
		`{"acme": {}}`:          true,
		`{"acme": {"at": 5}}`:   false, // "5" sorts above "2026-..." -- a signal (fails toward sending)
		`{"acme": [1, 2]}`:      true,
		`{"acme": {"at": "x"}}`: false, // sorts high -- a signal (fails toward sending)
		`[1]`:                   true,
	}
	ids := map[string]int{}
	i := 0
	for seed := range seeds {
		i++
		id := w.member(fmt.Sprintf("hostile%d@gmail.com", i), w.three)
		w.stamp(id, seed)
		ids[seed] = id
	}
	gone := w.member("gone@gmail.com", w.three)
	w.view(w.b3[0], gone, "2 years", "outlook-windows")
	h.db.MustExec(`DELETE FROM subscribers WHERE id = $1`, gone)
	nullSub := w.member("nullsub@gmail.com", w.three)
	h.db.MustExec(`UPDATE subscribers SET attribs = 'null'::JSONB WHERE id = $1`, nullSub)

	on := f.camp("On", false, "regular", "draft", `{"silent_skip": true}`, w.target)
	got := w.fn(on, false)
	for seed, want := range seeds {
		if has(got, ids[seed]) != want {
			t.Errorf("seed %s: skipped %v, want %v", seed, has(got, ids[seed]), want)
		}
	}
	// A non-boolean written by SQL is off.
	for _, v := range []string{`"yes"`, `1`, `"true"`, `null`, `{}`} {
		id := f.camp("Odd "+v, false, "regular", "draft", `{"silent_skip": `+v+`}`, w.target)
		if got := w.fn(id, false); len(got) != 0 {
			t.Errorf("silent_skip %s: %v, want off", v, got)
		}
		cm, err := h.app.core.GetCampaign(id, "", "")
		if err != nil {
			t.Fatal(err)
		}
		h.app.core.FillAudience(&cm)
		if cm.SilentSkipOn || cm.AudienceSilentSkipped.Int != 0 {
			t.Errorf("silent_skip %s served on %v silent %v", v, cm.SilentSkipOn, cm.AudienceSilentSkipped)
		}
	}
	// And the claim with the toggle on over the same data.
	h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE id = $1`, on)
	st := &store{queries: h.q, core: h.app.core}
	if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
		t.Fatalf("claim over hostile data: %v", err)
	}
}

// T9 (the route, DB-backed) -- GET /api/campaigns/:id/silent-audience answers {skipped, gmail, yahoo}
// excluding Smart Sending skips, and refuses a user without the campaign's list exactly as the
// campaign GET does; the served silent_skip field.
func TestSilentAudienceRoute(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	w.member("g@gmail.com", w.three)
	w.member("y@yahoo.com", w.three)
	smart := w.member("smart@gmail.com", w.three)
	journeyList := f.list("Acme first order", "brand:acme")
	f.send(f.camp("Acme thanks", true, "regular", "running", `{}`, journeyList), smart, "1 hour")
	other := f.list("Other", "brand:acme")
	camp := f.camp("Draft", false, "regular", "draft", `{}`, w.target)

	call := func(u auth.User, handler echo.HandlerFunc) (int, string) {
		e := echo.New()
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		c.Set("id", camp)
		c.Set(auth.UserHTTPCtxKey, u)
		if err := handler(c); err != nil {
			if he, ok := err.(*echo.HTTPError); ok {
				return he.Code, fmt.Sprint(he.Message)
			}
			t.Fatalf("handler: %v", err)
		}
		return rec.Code, rec.Body.String()
	}
	member := baUser([]string{auth.PermCampaignsGet}, w.target)
	code, body := call(member, h.app.GetCampaignSilentAudience)
	var resp struct {
		Data struct {
			Skipped int `json:"skipped"`
			Gmail   int `json:"gmail"`
			Yahoo   int `json:"yahoo"`
		} `json:"data"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &resp) != nil || resp.Data.Skipped != 2 || resp.Data.Gmail != 1 || resp.Data.Yahoo != 1 {
		t.Fatalf("silent-audience %d %s, want 200 {2, 1, 1} (the Smart Sending skip excluded)", code, body)
	}
	outsider := baUser([]string{auth.PermCampaignsGet}, other)
	gc, gb := call(outsider, h.app.GetCampaign)
	sc, sb := call(outsider, h.app.GetCampaignSilentAudience)
	if gc == http.StatusOK || gc != sc || gb != sb {
		t.Fatalf("outsider: campaign GET %d %q, silent-audience %d %q -- want the same refusal", gc, gb, sc, sb)
	}
	// The served field: absent -> false (default off); true -> true.
	_, cb := call(member, h.app.GetCampaign)
	var cg struct {
		Data map[string]any `json:"data"`
	}
	json.Unmarshal([]byte(cb), &cg)
	if v, ok := cg.Data["silent_skip"].(bool); !ok || v {
		t.Fatalf("served silent_skip %v, want false", cg.Data["silent_skip"])
	}
	h.db.MustExec(`UPDATE campaigns SET attribs = '{"silent_skip": true}' WHERE id = $1`, camp)
	_, cb = call(member, h.app.GetCampaign)
	cg.Data = nil
	json.Unmarshal([]byte(cb), &cg)
	if v, ok := cg.Data["silent_skip"].(bool); !ok || !v || cg.Data["audience_silent_skipped"] != 2.0 || cg.Data["audience_skipped"] != 3.0 {
		t.Fatalf("served silent_skip %v silent %v total %v, want true / 2 / 3", cg.Data["silent_skip"], cg.Data["audience_silent_skipped"], cg.Data["audience_skipped"])
	}
}

// T11 (I12) -- the matrix and analytics-campaigns reason counts sum to skipped, a NULL-reason row of
// each Smart Sending half read by the due_list_id column test.
func TestSilentSkipReasonCounts(t *testing.T) {
	h := newLinkHarness(t)
	f := ssFixture{t: t, h: h}
	acme := f.list("Acme newsletter", "brand:acme")
	b := f.camp("Acme mid", false, "regular", "finished", `{}`, acme)
	h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - INTERVAL '1 day', sent = 10 WHERE id = $1`, b)
	ins := func(sub int, journey any, due any, reason any) {
		h.db.MustExec(`INSERT INTO campaign_smart_skips (campaign_id, subscriber_id, journey_campaign_id, journey_sent_at, due_list_id, due_at, reason)
			VALUES ($1, $2, $3, CASE WHEN $3::INT IS NULL THEN NULL ELSE NOW() END, $4, CASE WHEN $4::INT IS NULL THEN NULL ELSE NOW() END, $5)`,
			b, sub, journey, due, reason)
	}
	ins(1, 9, nil, "lookback")
	ins(2, nil, 9, "lookahead")
	ins(3, nil, nil, "silent")
	ins(4, nil, nil, "silent")
	ins(5, 9, nil, nil) // NULL reason, look-back half
	ins(6, nil, 9, nil) // NULL reason, look-ahead half
	want := [4]int{6, 2, 2, 2}

	raw, err := h.app.core.GetCampaignMatrix(true, nil, false, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var mx struct {
		Rows []struct {
			ID        int  `json:"id"`
			Skipped   *int `json:"skipped"`
			Lookback  *int `json:"skipped_lookback"`
			Lookahead *int `json:"skipped_lookahead"`
			Silent    *int `json:"skipped_silent"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &mx); err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, r := range mx.Rows {
		if r.ID != b {
			continue
		}
		seen = true
		if r.Skipped == nil || r.Lookback == nil || r.Lookahead == nil || r.Silent == nil {
			t.Fatalf("matrix row %+v carries a null", r)
		}
		got := [4]int{*r.Skipped, *r.Lookback, *r.Lookahead, *r.Silent}
		if got != want || got[1]+got[2]+got[3] != got[0] {
			t.Fatalf("matrix %v, want %v (summing)", got, want)
		}
	}
	if !seen {
		t.Fatalf("matrix rows %s", raw)
	}
	ac, err := h.app.core.QueryAnalyticsCampaigns([]int{b}, "", true, nil, false, 10, nil)
	if err != nil || len(ac) != 1 {
		t.Fatalf("analytics campaigns %v %v", ac, err)
	}
	a := ac[0]
	got := [4]int{a.Skipped.Int, a.SkippedLookback.Int, a.SkippedLookahead.Int, a.SkippedSilent.Int}
	if got != want {
		t.Fatalf("analytics %v, want %v", got, want)
	}
	// A journey's reason counts are null, as its skipped is.
	j := f.camp("Acme welcome", true, "regular", "running", `{}`, acme)
	ac, _ = h.app.core.QueryAnalyticsCampaigns([]int{j}, "", true, nil, false, 10, nil)
	if len(ac) != 1 || ac[0].Skipped.Valid || ac[0].SkippedSilent.Valid || ac[0].SkippedLookback.Valid || ac[0].SkippedLookahead.Valid {
		t.Fatalf("journey %+v, want null counts", ac)
	}
}
