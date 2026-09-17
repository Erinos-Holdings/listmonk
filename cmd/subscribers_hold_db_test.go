package main

// Fork (holds) -- integrations SUNSET-SPEC I1, I2, I3, I4 and the DB half of I8, against a real
// database. Shares newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go), whose
// schema.sql carries the v6.2.10 trigger and functions.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/goyesql/v2"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

type holdRow struct {
	Status      string          `db:"status"`
	Meta        json.RawMessage `db:"meta"`
	UpdatedAt   time.Time       `db:"updated_at"`
	ConfirmedAt *time.Time      `db:"confirmed_at"`
}

func (r holdRow) meta(t *testing.T) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal(r.Meta, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

type holdFixture struct {
	*linkHarness
	brandUUID string
	brandID   int
}

func newHoldFixture(t *testing.T) *holdFixture {
	h := newLinkHarness(t)
	f := &holdFixture{linkHarness: h}
	if err := h.db.QueryRow(`INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Brand', 'private', 'single') RETURNING id, uuid`).Scan(&f.brandID, &f.brandUUID); err != nil {
		t.Fatal(err)
	}
	return f
}

// sub creates a subscriber with a row on list at status, its updated_at pushed back an hour so
// every later bump is observable.
func (f *holdFixture) sub(email string, list int, status string) (int, string) {
	var (
		id int
		uu string
	)
	if err := f.db.QueryRow(`INSERT INTO subscribers (uuid, email, name) VALUES (gen_random_uuid(), $1, $1) RETURNING id, uuid`, email).Scan(&id, &uu); err != nil {
		f.t.Fatal(err)
	}
	f.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, $3)`, id, list, status)
	f.age(id, list)
	return id, uu
}

func (f *holdFixture) age(id, list int) {
	f.db.MustExec(`UPDATE subscriber_lists SET updated_at = NOW() - INTERVAL '1 hour' WHERE subscriber_id=$1 AND list_id=$2`, id, list)
}

func (f *holdFixture) row(id, list int) holdRow {
	var r holdRow
	if err := f.db.Get(&r, `SELECT status, meta, updated_at, confirmed_at FROM subscriber_lists WHERE subscriber_id=$1 AND list_id=$2`, id, list); err != nil {
		f.t.Fatalf("row %d/%d: %v", id, list, err)
	}
	return r
}

func (f *holdFixture) rows(list int) int {
	var n int
	f.db.Get(&n, `SELECT COUNT(*) FROM subscriber_lists WHERE list_id=$1`, list)
	return n
}

func (f *holdFixture) hold(ids []int, rule string, bound *time.Time, relabel bool) models.HoldResult {
	f.t.Helper()
	out, err := f.app.core.HoldSubscriptions(ids, f.brandID, map[string]any{
		"reason": "never-engaged", "source": "test", "rule": rule,
	}, bound, relabel)
	if err != nil {
		f.t.Fatalf("HoldSubscriptions: %v", err)
	}
	return out
}

func skipWhy(res models.HoldResult, id int) string {
	for _, s := range res.Skipped {
		if s.ID == id {
			return s.Why
		}
	}
	return ""
}

// manage calls ManageSubscriberLists as a user holding every list permission, so the handler's
// own validation and response shape are what is exercised.
func (f *holdFixture) manage(body string) (int, string) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/subscribers/lists", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.UserHTTPCtxKey, auth.User{PermissionsMap: map[string]struct{}{
		auth.PermListGetAll: {}, auth.PermListManageAll: {},
	}})
	if err := f.app.ManageSubscriberLists(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, strings.TrimSpace(func() string { b, _ := json.Marshal(he.Message); return string(b) }())
		}
		f.t.Fatalf("ManageSubscriberLists: %v", err)
	}
	return rec.Code, rec.Body.String()
}

// TestHold_FlipsConfirmedNeverInserts -- I1. The default mode turns a confirmed and an
// unconfirmed row into a hold (unsubscribed + meta.hold, updated_at bumped, at stamped), never
// inserts a row for an id that is not on the list, and reports that id. The handler validates
// reason/source, rejects more than one list, and answers {held, skipped}.
func TestHold_FlipsConfirmedNeverInserts(t *testing.T) {
	f := newHoldFixture(t)
	conf, _ := f.sub("c@x", f.brandID, "confirmed")
	unconf, _ := f.sub("u@x", f.brandID, "unconfirmed")
	var stranger int
	f.db.Get(&stranger, `INSERT INTO subscribers (uuid, email, name) VALUES (gen_random_uuid(), 's@x', 's') RETURNING id`)
	before := f.row(conf, f.brandID).UpdatedAt
	nRows := f.rows(f.brandID)

	code, body := f.manage(`{"action":"hold","ids":[` + itoa(conf) + `,` + itoa(unconf) + `,` + itoa(stranger) + `],"target_list_ids":[` + itoa(f.brandID) + `],"hold":{"reason":"never-engaged","source":"test","rule":"r1","at":"1999-01-01T00:00:00Z"}}`)
	if code != http.StatusOK {
		t.Fatalf("hold: %d %s", code, body)
	}
	var resp struct {
		Data models.HoldResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Held != 2 || len(resp.Data.Skipped) != 1 || resp.Data.Skipped[0].ID != stranger || resp.Data.Skipped[0].Why != "not_on_list" {
		t.Fatalf("response: %s", body)
	}
	if f.rows(f.brandID) != nRows {
		t.Fatal("hold inserted a row")
	}

	for _, id := range []int{conf, unconf} {
		r := f.row(id, f.brandID)
		hold, _ := r.meta(t)["hold"].(map[string]any)
		if r.Status != "unsubscribed" || hold == nil || hold["reason"] != "never-engaged" || hold["source"] != "test" || hold["rule"] != "r1" {
			t.Fatalf("row %d: status %s meta %s", id, r.Status, r.Meta)
		}
		if at, _ := hold["at"].(string); at == "" || strings.HasPrefix(at, "1999") {
			t.Fatalf("row %d: at not server-stamped: %v", id, hold["at"])
		}
		if !r.UpdatedAt.After(before) {
			t.Fatalf("row %d: updated_at not bumped", id)
		}
	}

	// A hold on the D2 definition counts as held.
	var held int
	f.db.Get(&held, `SELECT COUNT(*) FROM subscriber_lists sl JOIN subscribers s ON s.id = sl.subscriber_id
		WHERE sl.list_id=$1 AND sl.status='unsubscribed' AND sl.meta ? 'hold' AND s.status <> 'blocklisted'`, f.brandID)
	if held != 2 {
		t.Fatalf("held by definition = %d, want 2", held)
	}

	// Validation.
	for name, b := range map[string]string{
		"no reason":  `{"action":"hold","ids":[1],"target_list_ids":[` + itoa(f.brandID) + `],"hold":{"source":"test"}}`,
		"bad reason": `{"action":"hold","ids":[1],"target_list_ids":[` + itoa(f.brandID) + `],"hold":{"reason":"bored","source":"test"}}`,
		"no source":  `{"action":"hold","ids":[1],"target_list_ids":[` + itoa(f.brandID) + `],"hold":{"reason":"manual"}}`,
		"two lists":  `{"action":"hold","ids":[1],"target_list_ids":[` + itoa(f.brandID) + `,999],"hold":{"reason":"manual","source":"test"}}`,
		"bad bound":  `{"action":"hold","ids":[1],"target_list_ids":[` + itoa(f.brandID) + `],"hold":{"reason":"manual","source":"test"},"if_updated_before":"yesterday"}`,
	} {
		if code, body := f.manage(b); code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d %s", name, code, body)
		}
	}
}

// TestHold_NeverRelabelsOptOut_IdempotentSameRule -- I2 (default mode). A plain unsubscribed row
// is a real opt-out and is never stamped; a re-run under the same rule is a no-op; a changed
// rule re-stamps without touching updated_at or releasing the hold.
func TestHold_NeverRelabelsOptOut_IdempotentSameRule(t *testing.T) {
	f := newHoldFixture(t)
	optOut, _ := f.sub("o@x", f.brandID, "unsubscribed")
	conf, _ := f.sub("c@x", f.brandID, "confirmed")

	res := f.hold([]int{optOut, conf}, "r1", nil, false)
	if res.Held != 1 || skipWhy(res, optOut) != "opt_out" {
		t.Fatalf("first run: %+v", res)
	}
	if r := f.row(optOut, f.brandID); r.meta(t)["hold"] != nil {
		t.Fatalf("opt-out stamped: %s", r.Meta)
	}

	f.age(conf, f.brandID)
	first := f.row(conf, f.brandID)
	res = f.hold([]int{conf}, "r1", nil, false)
	if res.Held != 0 || skipWhy(res, conf) != "already_held" {
		t.Fatalf("same-rule re-run: %+v", res)
	}
	if again := f.row(conf, f.brandID); !again.UpdatedAt.Equal(first.UpdatedAt) || string(again.Meta) != string(first.Meta) {
		t.Fatalf("same-rule re-run changed the row: %s -> %s", first.Meta, again.Meta)
	}

	res = f.hold([]int{conf}, "r2", nil, false)
	if res.Held != 1 {
		t.Fatalf("changed rule: %+v", res)
	}
	r := f.row(conf, f.brandID)
	m := r.meta(t)
	if hold, _ := m["hold"].(map[string]any); hold == nil || hold["rule"] != "r2" || m["hold_released"] != nil {
		t.Fatalf("re-stamp: %s", r.Meta)
	}
	if !r.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatal("re-stamp bumped updated_at")
	}

	// Same rule, new bookkeeping (the re-permission asks counter): merged in, at and updated_at
	// kept, still held. Repeating the identical call is a no-op again.
	stampedAt := m["hold"].(map[string]any)["at"]
	book := map[string]any{"reason": "never-engaged", "source": "test", "rule": "r2", "asks": 1}
	if res, err := f.app.core.HoldSubscriptions([]int{conf}, f.brandID, book, nil, false); err != nil || res.Held != 1 {
		t.Fatalf("bookkeeping merge: %+v %v", res, err)
	}
	r = f.row(conf, f.brandID)
	hold := r.meta(t)["hold"].(map[string]any)
	if hold["asks"] != float64(1) || hold["at"] != stampedAt || hold["rule"] != "r2" || !r.UpdatedAt.Equal(first.UpdatedAt) || r.Status != "unsubscribed" {
		t.Fatalf("bookkeeping merge wrote %s", r.Meta)
	}
	if res, _ := f.app.core.HoldSubscriptions([]int{conf}, f.brandID, book, nil, false); res.Held != 0 || skipWhy(res, conf) != "already_held" {
		t.Fatalf("identical bookkeeping call: %+v", res)
	}
}

// TestHold_RelabelMode -- I2 (relabel). Stamps exactly the plain-unsubscribed rows, never changes
// status or updated_at, and skips+reports a confirmed id and an already-held id.
func TestHold_RelabelMode(t *testing.T) {
	f := newHoldFixture(t)
	plain, _ := f.sub("p@x", f.brandID, "unsubscribed")
	conf, _ := f.sub("c@x", f.brandID, "confirmed")
	held, _ := f.sub("h@x", f.brandID, "confirmed")
	f.hold([]int{held}, "old", nil, false)
	f.age(held, f.brandID)
	// An opt-out recorded after an earlier hold (hold_released to unsubscribed) is consent and is
	// never relabelled (review L2).
	rel, _ := f.sub("rel@x", f.brandID, "unsubscribed")
	f.db.MustExec(`UPDATE subscriber_lists SET meta='{"hold_released":{"at":"2026-09-01T00:00:00Z","to":"unsubscribed","hold":{"rule":"x"}}}' WHERE subscriber_id=$1 AND list_id=$2`, rel, f.brandID)
	relBefore := f.row(rel, f.brandID)

	plainBefore := f.row(plain, f.brandID)
	confBefore := f.row(conf, f.brandID)
	heldBefore := f.row(held, f.brandID)

	res := f.hold([]int{plain, conf, held, rel}, "hcn-tier-D", nil, true)
	if res.Held != 1 || skipWhy(res, conf) != "confirmed" || skipWhy(res, held) != "already_held" || skipWhy(res, rel) != "opt_out_after_hold" {
		t.Fatalf("relabel: %+v", res)
	}
	if rr := f.row(rel, f.brandID); string(rr.Meta) != string(relBefore.Meta) || !rr.UpdatedAt.Equal(relBefore.UpdatedAt) {
		t.Fatalf("released opt-out touched: %s", rr.Meta)
	}

	p := f.row(plain, f.brandID)
	if hold, _ := p.meta(t)["hold"].(map[string]any); p.Status != "unsubscribed" || hold == nil || hold["rule"] != "hcn-tier-D" {
		t.Fatalf("plain row: %s %s", p.Status, p.Meta)
	}
	if !p.UpdatedAt.Equal(plainBefore.UpdatedAt) {
		t.Fatal("relabel bumped updated_at")
	}
	if c := f.row(conf, f.brandID); c.Status != "confirmed" || c.meta(t)["hold"] != nil || !c.UpdatedAt.Equal(confBefore.UpdatedAt) {
		t.Fatalf("confirmed row touched: %s %s", c.Status, c.Meta)
	}
	if hr := f.row(held, f.brandID); string(hr.Meta) != string(heldBefore.Meta) || !hr.UpdatedAt.Equal(heldBefore.UpdatedAt) {
		t.Fatalf("held row touched: %s", hr.Meta)
	}
}

// TestHold_ConsentBoundAtomic -- I3. if_updated_before is evaluated in the writing statement: a
// row updated at or after the bound is skipped and reported, in both modes.
func TestHold_ConsentBoundAtomic(t *testing.T) {
	f := newHoldFixture(t)
	old, _ := f.sub("old@x", f.brandID, "confirmed")
	fresh, _ := f.sub("fresh@x", f.brandID, "confirmed")
	plainFresh, _ := f.sub("pf@x", f.brandID, "unsubscribed")

	bound := time.Now().Add(-30 * time.Minute)
	f.db.MustExec(`UPDATE subscriber_lists SET updated_at = $1 WHERE subscriber_id = ANY($2)`, bound, "{"+itoa(fresh)+","+itoa(plainFresh)+"}")

	res := f.hold([]int{old, fresh}, "r", &bound, false)
	if res.Held != 1 || skipWhy(res, fresh) != "updated_after_bound" {
		t.Fatalf("default mode: %+v", res)
	}
	if r := f.row(fresh, f.brandID); r.Status != "confirmed" {
		t.Fatalf("bound ignored: %s", r.Status)
	}

	res = f.hold([]int{plainFresh}, "r", &bound, true)
	if res.Held != 0 || skipWhy(res, plainFresh) != "updated_after_bound" {
		t.Fatalf("relabel mode: %+v", res)
	}
}

// TestHoldRelease_EveryStatusWriter -- I4, table-driven over the status writers that can reach a
// held row. A real status change and an explicit unsubscribe release the hold (hold_released,
// updated_at bumped); the hold action's re-stamp, the blocklist writers and upstream's
// retain-status paths (admin edit, bulk add with no status, CSV import without overwrite) do
// not; unsubscribe-by-campaign never reaches a held row; a status write with no hold present
// never writes hold_released.
func TestHoldRelease_EveryStatusWriter(t *testing.T) {
	f := newHoldFixture(t)
	co := f.app.core
	qs := parseQueries(t)
	stmt := func(name string) *sqlx.Stmt {
		st, err := f.db.Preparex(qs[name].Query)
		if err != nil {
			t.Fatalf("prepare %s: %v", name, err)
		}
		return st
	}

	var campUUID string
	var campID int
	f.db.QueryRow(`INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id) VALUES (gen_random_uuid(), 'c', 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id, uuid`).Scan(&campID, &campUUID)
	f.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'Brand')`, campID, f.brandID)

	const (
		released     = "released"
		releasedOut  = "released-unsubscribed"
		kept         = "kept"
		keptNotCount = "kept-not-counted"
	)
	n := 0
	cases := []struct {
		name  string
		want  string
		write func(id int, uu, email string)
	}{
		{"add confirmed (mirror, D9 b)", released, func(id int, _, _ string) {
			if err := co.AddSubscriptions([]int{id}, []int{f.brandID}, "confirmed", true); err != nil {
				t.Fatal(err)
			}
		}},
		{"confirm-subscription-optin on the brand list", released, func(_ int, uu, _ string) {
			if err := co.ConfirmOptionSubscription(uu, []string{f.brandUUID}, models.JSON{}); err != nil {
				t.Fatal(err)
			}
		}},
		{"unsubscribe-subscribers-from-lists", releasedOut, func(id int, _, _ string) {
			if err := co.UnsubscribeLists([]int{id}, []int{f.brandID}, nil); err != nil {
				t.Fatal(err)
			}
		}},
		{"unsubscribe-subscribers-from-lists-by-query", releasedOut, func(id int, _, _ string) {
			if err := co.UnsubscribeListsByQuery("", "subscribers.id = "+itoa(id), nil, []int{f.brandID}, "", models.SubscriberFilter{}); err != nil {
				t.Fatal(err)
			}
		}},
		{"hold re-stamp with a changed rule", kept, func(id int, _, _ string) {
			f.hold([]int{id}, "other-rule", nil, false)
		}},
		{"blocklist-subscribers", keptNotCount, func(id int, _, _ string) {
			if err := co.BlocklistSubscribers([]int{id}); err != nil {
				t.Fatal(err)
			}
		}},
		{"blocklist-subscribers-by-query", keptNotCount, func(id int, _, _ string) {
			if err := co.BlocklistSubscribersByQuery("", "subscribers.id = "+itoa(id), nil, "", models.SubscriberFilter{}); err != nil {
				t.Fatal(err)
			}
		}},
		{"upsert-blocklist-subscriber (import)", keptNotCount, func(_ int, _, email string) {
			if _, err := stmt("upsert-blocklist-subscriber").Exec("00000000-0000-0000-0000-000000000001", email, "n", "{}"); err != nil {
				t.Fatal(err)
			}
		}},
		{"record-bounce unsubscribe action (same-status, no meta)", kept, func(_ int, uu, email string) {
			if _, err := stmt("record-bounce").Exec(uu, email, "", "hard", "test", "{}", time.Now(), 1, "unsubscribe"); err != nil {
				t.Fatal(err)
			}
		}},
		{"unsubscribe-by-campaign (guarded no-op)", kept, func(_ int, uu, _ string) {
			if err := co.UnsubscribeByCampaign(uu, campUUID, false); err != nil {
				t.Fatal(err)
			}
		}},
		{"admin edit form (update-subscriber-with-lists, retain)", kept, func(id int, _, email string) {
			if _, _, err := co.UpdateSubscriberWithLists(id, models.Subscriber{Email: email, Name: "renamed", Status: "enabled"},
				[]int{f.brandID}, nil, false, false, false, nil, false); err != nil {
				t.Fatal(err)
			}
		}},
		{"bulk add with no status", kept, func(id int, _, _ string) {
			if err := co.AddSubscriptions([]int{id}, []int{f.brandID}, "", false); err != nil {
				t.Fatal(err)
			}
		}},
		{"upsert-subscriber (import, no status overwrite)", kept, func(_ int, _, email string) {
			if _, err := stmt("upsert-subscriber").Exec("00000000-0000-0000-0000-000000000002", email, "n", "{}", "{"+itoa(f.brandID)+"}", "confirmed", false, false); err != nil {
				t.Fatal(err)
			}
		}},
	}

	for _, tc := range cases {
		n++
		email := "w" + itoa(n) + "@x"
		id, uu := f.sub(email, f.brandID, "confirmed")
		f.hold([]int{id}, "r1", nil, false)
		f.age(id, f.brandID)
		before := f.row(id, f.brandID)

		tc.write(id, uu, email)

		r := f.row(id, f.brandID)
		m := r.meta(t)
		rel, _ := m["hold_released"].(map[string]any)
		var counted int
		f.db.Get(&counted, `SELECT COUNT(*) FROM subscriber_lists sl JOIN subscribers s ON s.id = sl.subscriber_id
			WHERE sl.subscriber_id=$1 AND sl.list_id=$2 AND sl.status='unsubscribed' AND sl.meta ? 'hold' AND s.status <> 'blocklisted'`, id, f.brandID)

		switch tc.want {
		case released, releasedOut:
			to := "confirmed"
			if tc.want == releasedOut {
				to = "unsubscribed"
			}
			if m["hold"] != nil || rel == nil || rel["to"] != to || rel["hold"] == nil || rel["at"] == nil {
				t.Fatalf("%s: want released to %s, got %s %s", tc.name, to, r.Status, r.Meta)
			}
			if r.Status != to {
				t.Fatalf("%s: status %s", tc.name, r.Status)
			}
			if !r.UpdatedAt.After(before.UpdatedAt) {
				t.Fatalf("%s: updated_at not bumped on release", tc.name)
			}
		case kept:
			if m["hold"] == nil || rel != nil || r.Status != "unsubscribed" || counted != 1 {
				t.Fatalf("%s: want hold kept and counted, got %s %s (counted %d)", tc.name, r.Status, r.Meta, counted)
			}
		case keptNotCount:
			if m["hold"] == nil || rel != nil || r.Status != "unsubscribed" || counted != 0 {
				t.Fatalf("%s: want hold key kept, not counted, got %s %s (counted %d)", tc.name, r.Status, r.Meta, counted)
			}
		}
	}

	// A status write with no hold present never writes hold_released: confirm then unsubscribe
	// then re-confirm a plain row.
	id, _ := f.sub("plain@x", f.brandID, "confirmed")
	if err := co.UnsubscribeLists([]int{id}, []int{f.brandID}, nil); err != nil {
		t.Fatal(err)
	}
	if err := co.UnsubscribeLists([]int{id}, []int{f.brandID}, nil); err != nil {
		t.Fatal(err)
	}
	if err := co.AddSubscriptions([]int{id}, []int{f.brandID}, "confirmed", false); err != nil {
		t.Fatal(err)
	}
	if m := f.row(id, f.brandID).meta(t); len(m) != 0 {
		t.Fatalf("no-hold writes produced meta %v", m)
	}
}

// TestRepermission_ConfirmStampsOnlyItsRow -- I8's DB half. Confirming via the opt-in link on the
// re-permission list stamps confirmed_at (and flips status) on that row only; the held brand-list
// row is untouched.
func TestRepermission_ConfirmStampsOnlyItsRow(t *testing.T) {
	f := newHoldFixture(t)
	var repID int
	var repUUID string
	f.db.QueryRow(`INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Brand updates', 'private', 'double', '{repermission}') RETURNING id, uuid`).Scan(&repID, &repUUID)

	id, uu := f.sub("r@x", f.brandID, "confirmed")
	f.hold([]int{id}, "r1", nil, false)
	if err := f.app.core.AddSubscriptions([]int{id}, []int{repID}, "unconfirmed", false); err != nil {
		t.Fatal(err)
	}
	f.age(id, f.brandID)
	brandBefore := f.row(id, f.brandID)
	if r := f.row(id, repID); r.Status != "unconfirmed" || r.ConfirmedAt != nil {
		t.Fatalf("re-permission row before confirm: %+v", r)
	}

	if err := f.app.core.ConfirmOptionSubscription(uu, []string{repUUID}, models.JSON{}); err != nil {
		t.Fatal(err)
	}

	if r := f.row(id, repID); r.Status != "confirmed" || r.ConfirmedAt == nil {
		t.Fatalf("re-permission row after confirm: %s %v", r.Status, r.ConfirmedAt)
	}
	b := f.row(id, f.brandID)
	if b.Status != "unsubscribed" || !sameTime(b.ConfirmedAt, brandBefore.ConfirmedAt) || string(b.Meta) != string(brandBefore.Meta) || !b.UpdatedAt.Equal(brandBefore.UpdatedAt) {
		t.Fatalf("brand row touched by the opt-in confirm: %s %s (confirmed_at %v, updated %v vs %v)", b.Status, b.Meta, b.ConfirmedAt, b.UpdatedAt, brandBefore.UpdatedAt)
	}
}

func parseQueries(t *testing.T) goyesql.Queries {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join("..", "queries", "*.sql"))
	qs := goyesql.Queries{}
	for _, fn := range files {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		mp, err := goyesql.ParseBytes(b)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range mp {
			qs[k] = v
		}
	}
	return qs
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// TestHold_NewStampDropsReleased -- a NEW hold on a row that carries a stale hold_released
// (declined or opted out after an earlier hold, then re-joined) drops the released record, so a
// row is never both held and released and the §6 report queries count it once (review L1).
func TestHold_NewStampDropsReleased(t *testing.T) {
	f := newHoldFixture(t)
	id, _ := f.sub("again@x", f.brandID, "confirmed")
	f.db.MustExec(`UPDATE subscriber_lists SET meta='{"hold_released":{"at":"2026-09-01T00:00:00Z","to":"unsubscribed","hold":{"rule":"x"}}}' WHERE subscriber_id=$1 AND list_id=$2`, id, f.brandID)
	if res := f.hold([]int{id}, "r2", nil, false); res.Held != 1 {
		t.Fatalf("hold: %+v", res)
	}
	m := f.row(id, f.brandID).meta(t)
	if m["hold"] == nil || m["hold_released"] != nil {
		t.Fatalf("stale hold_released survived a new stamp: %v", m)
	}
}

// TestHold_ByQueryRefused -- the SQL-fragment endpoint refuses action hold by name (spec D3:
// ids only), before any list permission filtering could turn it into a write.
func TestHold_ByQueryRefused(t *testing.T) {
	f := newHoldFixture(t)
	e := echo.New()
	body := `{"query":"subscribers.id = 1","action":"hold","target_list_ids":[` + itoa(f.brandID) + `],"hold":{"reason":"manual","source":"test","rule":"r"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/subscribers/query/lists", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.UserHTTPCtxKey, auth.User{PermissionsMap: map[string]struct{}{
		auth.PermListGetAll: {}, auth.PermListManageAll: {}, auth.PermSubscribersSqlQuery: {},
	}})
	err := f.app.ManageSubscriberListsByQuery(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest || !strings.Contains(strings.ToLower(he.Message.(string)), "explicit ids") {
		t.Fatalf("want 400 refusing hold by query, got %v / %d %s", err, rec.Code, rec.Body.String())
	}
	var n int
	f.db.Get(&n, `SELECT COUNT(*) FROM subscriber_lists WHERE meta ? 'hold'`)
	if n != 0 {
		t.Fatalf("by-query hold wrote %d rows", n)
	}
}

// TestSunsetPredicate_PassesTableAllowlist -- the D7 rule predicate, run through the subscriber
// query API, must pass validateQueryTables: the EXPLAIN plan of subscription_engagement(list)
// names campaign_send_failures, which the allowlist has to carry (found live 2026-09-16).
func TestSunsetPredicate_PassesTableAllowlist(t *testing.T) {
	f := newHoldFixture(t)
	pred := "subscribers.id IN (SELECT subscriber_id FROM subscription_engagement(" + itoa(f.brandID) + ") " +
		"WHERE anchor_at <= NOW() - INTERVAL '120 days' AND eligible_sends >= 6 " +
		"AND first_eligible_send_at >= '2026-08-31T00:00:00Z' AND last_view_at IS NULL AND last_click_at IS NULL)"
	if _, _, err := f.app.core.QuerySubscribers("", pred, []int{f.brandID}, "", models.SubscriberFilter{}, "", "", 0, 10); err != nil {
		t.Fatalf("sunset predicate rejected: %v", err)
	}
}
