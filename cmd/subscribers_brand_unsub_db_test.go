package main

// Fork (brand unsubscribe) -- integrations listmonk/BRAND-UNSUBSCRIBE-SPEC invariants A1 (R1/R2),
// A2, A3, A4, A5, A7 and A9, against a real database. An unsubscribe by the person themselves
// means the BRAND, not the list, and the rule is enforced in TWO queries -- the campaign-link path
// (unsubscribe-by-campaign, behind the footer link and the one-click List-Unsubscribe header) and
// the admin brand route (unsubscribe-brand, behind our Manage Preferences page and the backwards
// repair) -- so every invariant that lives in both is asserted twice, once per query. Shares
// newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

type brandFixture struct {
	*linkHarness
	// acme's marketing lists.
	news, step, queue, segment int
	// acme's transactional list (A3) and zeta's newsletter (A4).
	txn, zeta int
	// A list with no brand tag at all (A2/D5).
	untagged int
}

func newBrandFixture(t *testing.T) *brandFixture {
	h := newLinkHarness(t)
	f := &brandFixture{linkHarness: h}
	h.db.MustExec(`INSERT INTO brands (slug, from_email) VALUES ('acme', 'hello@acme.test'), ('zeta', 'hello@zeta.test'), ('bare', 'hello@bare.test')`)
	f.news = f.list("acme news", "public", "brand:acme")
	f.step = f.list("acme no-purchase day 5", "private", "brand:acme")
	f.queue = f.list("acme reengage 1", "private", "brand:acme")
	f.segment = f.list("acme imported segment", "private", "brand:acme")
	f.txn = f.list("acme receipts", "private", "brand:acme")
	f.zeta = f.list("zeta news", "public", "brand:zeta")
	f.untagged = f.list("no brand", "public", "")
	h.db.MustExec(`UPDATE lists SET transactional = TRUE WHERE id = $1`, f.txn)
	return f
}

func (f *brandFixture) list(name, typ, tag string) int {
	tags := []string{}
	if tag != "" {
		tags = append(tags, tag)
	}
	var id int
	if err := f.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags)
		VALUES (gen_random_uuid(), $1, $2::list_type, 'single', $3) RETURNING id`, name, typ, pq.Array(tags)); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// camp creates a regular campaign on the given lists and returns its UUID.
func (f *brandFixture) camp(name string, lists ...int) string {
	var (
		id int
		uu string
	)
	if err := f.db.QueryRow(`INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id)
		VALUES (gen_random_uuid(), $1, 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id, uuid`, name).Scan(&id, &uu); err != nil {
		f.t.Fatal(err)
	}
	for _, l := range lists {
		f.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name)
			VALUES ($1, $2, (SELECT name FROM lists WHERE id = $2))`, id, l)
	}
	return uu
}

// sub creates a subscriber confirmed on every given list, with updated_at pushed back an hour so
// every later bump is observable.
func (f *brandFixture) sub(email string, lists ...int) (int, string) {
	var (
		id int
		uu string
	)
	if err := f.db.QueryRow(`INSERT INTO subscribers (uuid, email, name) VALUES (gen_random_uuid(), $1, $1) RETURNING id, uuid`, email).Scan(&id, &uu); err != nil {
		f.t.Fatal(err)
	}
	for _, l := range lists {
		f.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, id, l)
	}
	f.db.MustExec(`UPDATE subscriber_lists SET updated_at = NOW() - INTERVAL '1 hour' WHERE subscriber_id = $1`, id)
	return id, uu
}

type brandRow struct {
	Status    string          `db:"status"`
	Meta      json.RawMessage `db:"meta"`
	UpdatedAt time.Time       `db:"updated_at"`
}

func (f *brandFixture) row(sub, list int) brandRow {
	var r brandRow
	if err := f.db.Get(&r, `SELECT status, meta, updated_at FROM subscriber_lists WHERE subscriber_id=$1 AND list_id=$2`, sub, list); err != nil {
		f.t.Fatalf("row %d/%d: %v", sub, list, err)
	}
	return r
}

func (f *brandFixture) meta(sub, list int) map[string]any {
	m := map[string]any{}
	if err := json.Unmarshal(f.row(sub, list).Meta, &m); err != nil {
		f.t.Fatal(err)
	}
	return m
}

// setStatus forces a row's status and meta, the way the sunset's hold writer and an earlier
// opt-out leave them.
func (f *brandFixture) setStatus(sub, list int, status, meta string) {
	f.db.MustExec(`UPDATE subscriber_lists SET status=$3, meta=$4::JSONB, updated_at = NOW() - INTERVAL '1 hour'
		WHERE subscriber_id=$1 AND list_id=$2`, sub, list, status, meta)
}

func (f *brandFixture) statusOf(sub, list int) string { return f.row(sub, list).Status }

// brandUnsub calls the D4 endpoint as a user holding every list permission.
func (f *brandFixture) brandUnsub(body string) (int, string) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/subscribers/brand-unsubscribe", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.UserHTTPCtxKey, auth.User{PermissionsMap: map[string]struct{}{
		auth.PermListGetAll: {}, auth.PermListManageAll: {},
	}})
	if err := f.app.BrandUnsubscribeSubscribers(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			b, _ := json.Marshal(he.Message)
			return he.Code, string(b)
		}
		f.t.Fatalf("BrandUnsubscribeSubscribers: %v", err)
	}
	return rec.Code, rec.Body.String()
}

// TestBrandUnsubscribe_CampaignLinkReachesTheBrand -- A1 (R1/R2), A3, A4. A campaign sent on a
// STEP list unsubscribes every marketing list of the brand (newsletter, step, queue, private
// segment), leaves the brand's transactional list alone, and never touches a second brand.
func TestBrandUnsubscribe_CampaignLinkReachesTheBrand(t *testing.T) {
	f := newBrandFixture(t)
	camp := f.camp("acme day 5", f.step)
	id, uu := f.sub("a@x", f.news, f.step, f.queue, f.segment, f.txn, f.zeta)

	if err := f.app.core.UnsubscribeByCampaign(uu, camp, false); err != nil {
		t.Fatal(err)
	}

	for _, l := range []int{f.news, f.step, f.queue, f.segment} {
		if got := f.statusOf(id, l); got != "unsubscribed" {
			t.Fatalf("list %d: %s, want unsubscribed", l, got)
		}
	}
	// A3 -- transactional lists are never unsubscribed by a brand unsubscribe.
	if got := f.statusOf(id, f.txn); got != "confirmed" {
		t.Fatalf("transactional list: %s, want confirmed", got)
	}
	// A4 -- another brand is never touched.
	if got := f.statusOf(id, f.zeta); got != "confirmed" {
		t.Fatalf("second brand: %s, want confirmed", got)
	}
}

// TestBrandUnsubscribe_CampaignOwnListsAlwaysIncluded -- D3's union. A campaign sent FROM the
// brand's transactional list still unsubscribes that list (the person was emailed from it), on
// top of the brand's marketing lists. Without the union a one-click unsubscribe would do less
// than it did before the expansion.
func TestBrandUnsubscribe_CampaignOwnListsAlwaysIncluded(t *testing.T) {
	f := newBrandFixture(t)
	camp := f.camp("acme receipt promo", f.txn)
	id, uu := f.sub("b@x", f.news, f.txn, f.zeta)

	if err := f.app.core.UnsubscribeByCampaign(uu, camp, false); err != nil {
		t.Fatal(err)
	}
	if got := f.statusOf(id, f.txn); got != "unsubscribed" {
		t.Fatalf("the campaign's own (transactional) list: %s, want unsubscribed", got)
	}
	if got := f.statusOf(id, f.news); got != "unsubscribed" {
		t.Fatalf("brand newsletter: %s, want unsubscribed", got)
	}
	if got := f.statusOf(id, f.zeta); got != "confirmed" {
		t.Fatalf("second brand: %s, want confirmed", got)
	}

	// The same union for an untagged list beside a tagged one: campaign_brand resolves (one
	// distinct tag) and the untagged list is in no brand set, so only the union reaches it.
	camp2 := f.camp("acme mixed", f.news, f.untagged)
	id2, uu2 := f.sub("b2@x", f.news, f.untagged, f.step)
	if err := f.app.core.UnsubscribeByCampaign(uu2, camp2, false); err != nil {
		t.Fatal(err)
	}
	for _, l := range []int{f.news, f.untagged, f.step} {
		if got := f.statusOf(id2, l); got != "unsubscribed" {
			t.Fatalf("mixed campaign, list %d: %s, want unsubscribed", l, got)
		}
	}
}

// TestBrandUnsubscribe_NoBrandOrTwoBrands -- A2/D5. A campaign whose lists carry no brand tag, or
// more than one, unsubscribes the campaign's own lists only: exactly today's behaviour.
func TestBrandUnsubscribe_NoBrandOrTwoBrands(t *testing.T) {
	f := newBrandFixture(t)

	untaggedCamp := f.camp("no brand blast", f.untagged)
	id, uu := f.sub("c@x", f.untagged, f.news, f.step, f.zeta)
	if err := f.app.core.UnsubscribeByCampaign(uu, untaggedCamp, false); err != nil {
		t.Fatal(err)
	}
	if got := f.statusOf(id, f.untagged); got != "unsubscribed" {
		t.Fatalf("the campaign's own list: %s, want unsubscribed", got)
	}
	for _, l := range []int{f.news, f.step, f.zeta} {
		if got := f.statusOf(id, l); got != "confirmed" {
			t.Fatalf("list %d: %s, want confirmed (no brand resolved)", l, got)
		}
	}

	twoBrands := f.camp("acme + zeta", f.news, f.zeta)
	id2, uu2 := f.sub("d@x", f.news, f.step, f.zeta)
	if err := f.app.core.UnsubscribeByCampaign(uu2, twoBrands, false); err != nil {
		t.Fatal(err)
	}
	for _, l := range []int{f.news, f.zeta} {
		if got := f.statusOf(id2, l); got != "unsubscribed" {
			t.Fatalf("campaign list %d: %s, want unsubscribed", l, got)
		}
	}
	if got := f.statusOf(id2, f.step); got != "confirmed" {
		t.Fatalf("step list: %s, want confirmed (two brands ⇒ no expansion)", got)
	}
}

// TestBrandUnsubscribe_HeldPersonIsARealOptOut -- A5, in BOTH queries. A held person who clicks
// Unsubscribe in an ask for a queue they have already LEFT (the queue-1 row deleted by an earlier
// move) still records a real opt-out on the brand's newsletter row: unsubscribed +
// hold_released.to = 'unsubscribed'. A FINAL hold is released too -- the person's own act is the
// one thing that ends one.
func TestBrandUnsubscribe_HeldPersonIsARealOptOut(t *testing.T) {
	f := newBrandFixture(t)

	check := func(label string, final bool, act func(subID int, subUUID string)) {
		hold := `{"hold":{"reason":"never-engaged","source":"sunset","rule":"never-engaged","at":"2026-05-01T00:00:00Z"}}`
		if final {
			hold = `{"hold":{"reason":"never-engaged","source":"sunset","rule":"never-engaged","at":"2026-05-01T00:00:00Z","final_at":"2026-11-01T00:00:00Z"}}`
		}
		id, uu := f.sub(label+"@x", f.news, f.queue, f.zeta)
		// The hold lives on the brand's newsletter row; the queue-1 row is gone (V5).
		f.setStatus(id, f.news, "unsubscribed", hold)
		f.db.MustExec(`DELETE FROM subscriber_lists WHERE subscriber_id=$1 AND list_id=$2`, id, f.queue)
		before := f.row(id, f.news).UpdatedAt

		act(id, uu)

		r := f.row(id, f.news)
		m := f.meta(id, f.news)
		rel, _ := m["hold_released"].(map[string]any)
		if r.Status != "unsubscribed" || rel == nil || rel["to"] != "unsubscribed" {
			t.Fatalf("%s: status %s meta %s", label, r.Status, r.Meta)
		}
		if _, still := m["hold"]; still {
			t.Fatalf("%s: hold not released: %s", label, r.Meta)
		}
		if !r.UpdatedAt.After(before) {
			t.Fatalf("%s: updated_at not bumped on the released row", label)
		}
		if got := f.statusOf(id, f.zeta); got != "confirmed" {
			t.Fatalf("%s: second brand %s, want confirmed", label, got)
		}
	}

	// The campaign-link query: the ask was sent on queue 2, a list the person is not even on.
	queue2 := f.list("acme reengage 2", "private", "brand:acme")
	askCamp := f.camp("acme ask 2", queue2)
	check("held-camp", false, func(id int, uu string) {
		if err := f.app.core.UnsubscribeByCampaign(uu, askCamp, false); err != nil {
			t.Fatal(err)
		}
	})
	check("heldfinal-camp", true, func(id int, uu string) {
		if err := f.app.core.UnsubscribeByCampaign(uu, askCamp, false); err != nil {
			t.Fatal(err)
		}
	})

	// The D4 brand query, the same two cases.
	check("held-brand", false, func(id int, uu string) {
		if code, body := f.brandUnsub(`{"ids":[` + itoa(id) + `],"brand":"acme"}`); code != http.StatusOK {
			t.Fatalf("brand unsubscribe: %d %s", code, body)
		}
	})
	check("heldfinal-brand", true, func(id int, uu string) {
		if code, body := f.brandUnsub(`{"ids":[` + itoa(id) + `],"brand":"acme"}`); code != http.StatusOK {
			t.Fatalf("brand unsubscribe: %d %s", code, body)
		}
	})
}

// TestBrandUnsubscribe_PlainUnsubscribedRowUntouched -- A7, in BOTH queries. A plain
// already-unsubscribed row (no hold) gets no status write and no updated_at bump, so an earlier
// opt-out's timestamp -- which the sync Lambda's resurrection guard reads -- never advances.
func TestBrandUnsubscribe_PlainUnsubscribedRowUntouched(t *testing.T) {
	f := newBrandFixture(t)
	camp := f.camp("acme broadcast", f.news)

	for _, tc := range []struct {
		label string
		act   func(id int, uu string)
	}{
		{"camp", func(id int, uu string) {
			if err := f.app.core.UnsubscribeByCampaign(uu, camp, false); err != nil {
				t.Fatal(err)
			}
		}},
		{"brand", func(id int, uu string) {
			if code, body := f.brandUnsub(`{"ids":[` + itoa(id) + `],"brand":"acme"}`); code != http.StatusOK {
				t.Fatalf("brand unsubscribe: %d %s", code, body)
			}
		}},
	} {
		id, uu := f.sub("plain-"+tc.label+"@x", f.news, f.step)
		f.setStatus(id, f.news, "unsubscribed", `{}`)
		before := f.row(id, f.news).UpdatedAt

		tc.act(id, uu)

		r := f.row(id, f.news)
		if r.Status != "unsubscribed" || !r.UpdatedAt.Equal(before) {
			t.Fatalf("%s: plain unsubscribed row rewritten (status %s, updated_at %v -> %v)", tc.label, r.Status, before, r.UpdatedAt)
		}
		// The rest of the brand still moves, so the no-op is the row predicate and not a
		// query that did nothing at all.
		if got := f.statusOf(id, f.step); got != "unsubscribed" {
			t.Fatalf("%s: step list %s, want unsubscribed", tc.label, got)
		}
	}
}

// TestBrandUnsubscribe_BlocklistBranchUnchanged -- A9/D6, and D6 is LITERAL. The blocklist branch
// of the campaign link is untouched by the expansion: every list of every brand, the transactional
// list included, plus the subscriber blocklisted. It is already maximal. And D3's hold clause is
// scoped OUT of it: an already-unsubscribed HELD row keeps its `meta.hold` AND its `updated_at`,
// so this writer keeps agreeing with the admin `blocklist-subscribers` one.
func TestBrandUnsubscribe_BlocklistBranchUnchanged(t *testing.T) {
	f := newBrandFixture(t)
	camp := f.camp("acme blast", f.news)
	id, uu := f.sub("e@x", f.news, f.step, f.txn, f.zeta, f.untagged)

	// A held row on one of the brand's lists, and a FINAL held row on another.
	f.setStatus(id, f.step, "unsubscribed", `{"hold":{"reason":"never-engaged","source":"sunset","rule":"never-engaged","at":"2026-05-01T00:00:00Z"}}`)
	f.setStatus(id, f.zeta, "unsubscribed", `{"hold":{"reason":"never-engaged","source":"sunset","rule":"never-engaged","at":"2026-05-01T00:00:00Z","final_at":"2026-11-01T00:00:00Z"}}`)
	heldBefore := f.row(id, f.step).UpdatedAt
	finalBefore := f.row(id, f.zeta).UpdatedAt

	if err := f.app.core.UnsubscribeByCampaign(uu, camp, true); err != nil {
		t.Fatal(err)
	}
	for _, l := range []int{f.news, f.step, f.txn, f.zeta, f.untagged} {
		if got := f.statusOf(id, l); got != "unsubscribed" {
			t.Fatalf("blocklist: list %d is %s, want unsubscribed", l, got)
		}
	}
	var status string
	f.db.Get(&status, `SELECT status FROM subscribers WHERE id = $1`, id)
	if status != "blocklisted" {
		t.Fatalf("subscriber status %s, want blocklisted", status)
	}

	// D6: the held rows are untouched by the blocklist -- hold kept, no release, no bump.
	for _, tc := range []struct {
		label  string
		list   int
		before time.Time
	}{{"held", f.step, heldBefore}, {"final held", f.zeta, finalBefore}} {
		m := f.meta(id, tc.list)
		if m["hold"] == nil {
			t.Fatalf("blocklist released the %s row's hold: %s", tc.label, f.row(id, tc.list).Meta)
		}
		if m["hold_released"] != nil {
			t.Fatalf("blocklist wrote hold_released on the %s row: %s", tc.label, f.row(id, tc.list).Meta)
		}
		if !f.row(id, tc.list).UpdatedAt.Equal(tc.before) {
			t.Fatalf("blocklist bumped the %s row's updated_at (%v -> %v)", tc.label, tc.before, f.row(id, tc.list).UpdatedAt)
		}
	}
}

// TestBrandUnsubscribe_Endpoint -- D4's route, A3 and A4 on that route, and its two boundaries:
// an unknown brand is a 400, a KNOWN brand with no marketing lists is a no-op 200.
func TestBrandUnsubscribe_Endpoint(t *testing.T) {
	f := newBrandFixture(t)
	id, _ := f.sub("f@x", f.news, f.step, f.queue, f.segment, f.txn, f.zeta)

	if code, body := f.brandUnsub(`{"ids":[` + itoa(id) + `],"brand":"acme"}`); code != http.StatusOK {
		t.Fatalf("brand unsubscribe: %d %s", code, body)
	}
	for _, l := range []int{f.news, f.step, f.queue, f.segment} {
		if got := f.statusOf(id, l); got != "unsubscribed" {
			t.Fatalf("list %d: %s, want unsubscribed", l, got)
		}
	}
	if got := f.statusOf(id, f.txn); got != "confirmed" { // A3
		t.Fatalf("transactional list: %s, want confirmed", got)
	}
	if got := f.statusOf(id, f.zeta); got != "confirmed" { // A4
		t.Fatalf("second brand: %s, want confirmed", got)
	}

	// An unknown brand is a 400 -- a typo or a stale caller, never a silent no-op.
	if code, _ := f.brandUnsub(`{"ids":[` + itoa(id) + `],"brand":"nosuchbrand"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown brand: %d, want 400", code)
	}
	if code, _ := f.brandUnsub(`{"ids":[` + itoa(id) + `],"brand":""}`); code != http.StatusBadRequest {
		t.Fatalf("empty brand: %d, want 400", code)
	}
	if code, _ := f.brandUnsub(`{"ids":[],"brand":"acme"}`); code != http.StatusBadRequest {
		t.Fatalf("no ids: %d, want 400", code)
	}

	// Without lists:manage_all the route is a 403, never a partial write: the target set is
	// computed in SQL, so there is no list-id set to intersect with a narrower grant.
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/subscribers/brand-unsubscribe", strings.NewReader(`{"ids":[`+itoa(id)+`],"brand":"acme"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := e.NewContext(req, httptest.NewRecorder())
	c.Set(auth.UserHTTPCtxKey, auth.User{PermissionsMap: map[string]struct{}{auth.PermListGetAll: {}}})
	if err := f.app.BrandUnsubscribeSubscribers(c); err == nil {
		t.Fatal("a caller without lists:manage_all was allowed")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusForbidden {
		t.Fatalf("without lists:manage_all: %v", err)
	}

	// A known brand with no lists yet (provisioned, not listed) is a no-op 200.
	id2, _ := f.sub("g@x", f.news)
	if code, body := f.brandUnsub(`{"ids":[` + itoa(id2) + `],"brand":"bare"}`); code != http.StatusOK {
		t.Fatalf("empty brand set: %d %s, want 200", code, body)
	}
	if got := f.statusOf(id2, f.news); got != "confirmed" {
		t.Fatalf("an empty brand set wrote something: %s", got)
	}

	// D9 -- no row is ever inserted: a subscriber on none of the brand's lists stays off them.
	id3, _ := f.sub("h@x", f.zeta)
	if code, body := f.brandUnsub(`{"ids":[` + itoa(id3) + `],"brand":"acme"}`); code != http.StatusOK {
		t.Fatalf("brand unsubscribe: %d %s", code, body)
	}
	var n int
	f.db.Get(&n, `SELECT COUNT(*) FROM subscriber_lists WHERE subscriber_id=$1`, id3)
	if n != 1 {
		t.Fatalf("rows for a subscriber off the brand = %d, want 1 (no insert)", n)
	}
}
