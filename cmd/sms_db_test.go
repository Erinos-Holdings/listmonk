package main

// Fork (SMS Admin) -- integrations listmonk/SMS-LINKS-ADMIN-SPEC I3 (every /api/sms/* route requires
// sms:manage), the promo half of I6 (the promo routes work while the link proxy is off), I8 (the
// report) and I9 (marking), against a real database through the real router (twofaHarness, built by
// initHTTPHandlers: the session middleware and pm are the production ones). LISTMONK_TEST_PG opt-in.
// Fixture names are example.test only.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lib/pq"
)

// smsEveryOtherPermission is every key of permissions.json except sms:manage.
func smsEveryOtherPermission(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "permissions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var groups []struct {
		Group       string   `json:"group"`
		Permissions []string `json:"permissions"`
	}
	if err := json.Unmarshal(b, &groups); err != nil {
		t.Fatal(err)
	}
	out := []string{}
	seenSMS := false
	for _, g := range groups {
		for _, p := range g.Permissions {
			if p == "sms:manage" {
				seenSMS = true
				if g.Group != "sms" {
					t.Fatalf("sms:manage is in group %q, want sms", g.Group)
				}
				continue
			}
			out = append(out, p)
		}
	}
	if !seenSMS {
		t.Fatal("permissions.json has no sms:manage")
	}
	return out
}

type smsDBFixture struct {
	h      *twofaHarness
	other  *twofaClient // every permission but sms:manage
	sms    *twofaClient // sms:manage only
	anon   *twofaClient // no session
	smsWho string
}

func newSMSDBFixture(t *testing.T) *smsDBFixture {
	t.Helper()
	h := newTwofaHarness(t)
	var otherRole, smsRole int
	if err := h.db.Get(&otherRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Everything but SMS', $1) RETURNING id`,
		pq.StringArray(smsEveryOtherPermission(t))); err != nil {
		t.Fatal(err)
	}
	if err := h.db.Get(&smsRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'SMS', '{sms:manage}') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	otherID := h.user("allbutsms", otherRole)
	smsID := h.user("marketing", smsRole)
	if _, err := cacheUsers(h.app.core, h.app.auth); err != nil {
		t.Fatal(err)
	}
	h.db.MustExec(`INSERT INTO brands (slug, from_email) VALUES ('thirstygirl', 'hello@example.test'), ('ruze', 'hello@example.test')`)
	return &smsDBFixture{h: h, other: h.sessionClient(otherID), sms: h.sessionClient(smsID), anon: h.client(), smsWho: "listmonk:marketing"}
}

var smsRouteTable = []struct{ method, path, body string }{
	{http.MethodGet, "/api/sms/links", ""},
	{http.MethodPost, "/api/sms/links", `{"slug":"tg-nov","target":"https://example.test/"}`},
	{http.MethodPut, "/api/sms/links/tg-nov", `{"target":"https://example.test/"}`},
	{http.MethodDelete, "/api/sms/links/tg-nov", ""},
	{http.MethodGet, "/api/sms/promo-codes", ""},
	{http.MethodPost, "/api/sms/promo-codes", `{"code":"newCustomerSMS"}`},
	{http.MethodDelete, "/api/sms/promo-codes/1", ""},
}

// I3 -- a user holding every other permission (campaigns:get_analytics and campaigns:manage
// included) is refused 403 on every route; no session is refused; nothing is written.
func TestSMSRoutePermissions(t *testing.T) {
	f := newSMSDBFixture(t)
	other := smsEveryOtherPermission(t)
	for _, p := range []string{"campaigns:get_analytics", "campaigns:manage", "campaigns:manage_all", "settings:manage"} {
		if !strings.Contains(strings.Join(other, ","), p) {
			t.Fatalf("the all-but-sms role lacks %s", p)
		}
	}
	f.h.db.MustExec(`INSERT INTO sms_promo_codes (code, created_by) VALUES ('keep-me', 'listmonk:x')`)

	for _, r := range smsRouteTable {
		rec := f.other.do(r.method, r.path, "application/json", r.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("all-but-sms %s %s: %d %s, want 403", r.method, r.path, rec.Code, rec.Body.String())
		}
		rec = f.anon.do(r.method, r.path, "application/json", r.body)
		if rec.Code < 400 || rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
			t.Errorf("no session %s %s: %d, want refused", r.method, r.path, rec.Code)
		}
	}
	var n int
	f.h.db.Get(&n, `SELECT COUNT(*) FROM sms_promo_codes`)
	if n != 1 {
		t.Fatalf("a refused request wrote: %d marking rows, want 1", n)
	}

	// The sms:manage holder reaches every route (the link routes answer 503 here, the proxy being
	// off -- I6's promo half: the promo routes still work).
	for _, r := range smsRouteTable {
		rec := f.sms.do(r.method, r.path, "application/json", r.body)
		isLink := strings.HasPrefix(r.path, "/api/sms/links")
		switch {
		case isLink && rec.Code != http.StatusServiceUnavailable:
			t.Errorf("sms %s %s: %d %s, want 503 (not configured)", r.method, r.path, rec.Code, rec.Body.String())
		case isLink && !strings.Contains(rec.Body.String(), f.h.app.i18n.T("sms.notConfigured")):
			t.Errorf("sms %s %s: body %s lacks the not-configured notice", r.method, r.path, rec.Body.String())
		case !isLink && rec.Code >= 300 && rec.Code != http.StatusNotFound:
			t.Errorf("sms %s %s: %d %s", r.method, r.path, rec.Code, rec.Body.String())
		}
	}
}

type smsReport struct {
	Rows []struct {
		ID           int     `json:"id"`
		Code         string  `json:"code"`
		Brand        *string `json:"brand"`
		Orders       int     `json:"orders"`
		Buyers       int     `json:"buyers"`
		FirstOrderAt *string `json:"first_order_at"`
		LastOrderAt  *string `json:"last_order_at"`
		CreatedBy    string  `json:"created_by"`
	} `json:"rows"`
	Feed struct {
		OldestOrderedAt *string `json:"oldest_ordered_at"`
		LastReceivedAt  *string `json:"last_received_at"`
		Stale           bool    `json:"stale"`
	} `json:"feed"`
	Known []struct {
		Value  string `json:"value"`
		Orders int    `json:"orders"`
	} `json:"known"`
}

func (f *smsDBFixture) report(t *testing.T) smsReport {
	t.Helper()
	rec := f.sms.do(http.MethodGet, "/api/sms/promo-codes", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("report: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data smsReport `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

// buy writes one feed row as the upsert does (value_lc = LOWER(value)), at a fixed instant.
func (f *smsDBFixture) buy(order, kind, value, hash, at string) {
	f.h.db.MustExec(`INSERT INTO promo_purchases (order_id, kind, value, value_lc, email_hash, ordered_at)
		VALUES ($1, $2, $3, LOWER($3), $4, $5::TIMESTAMPTZ)`, order, kind, value, hash, at)
}

// I8 -- the report's counts by the feed's promotion comparison.
func TestSMSPromoReport(t *testing.T) {
	f := newSMSDBFixture(t)
	h1, h2 := strings.Repeat("a", 64), strings.Repeat("b", 64)

	// An empty feed: a marked code lists with 0 and NULL dates; the feed has no extent.
	f.h.db.MustExec(`INSERT INTO sms_promo_codes (code, brand, created_by) VALUES ('BFCM-Deal-2', 'thirstygirl', 'listmonk:m')`)
	r := f.report(t)
	if len(r.Rows) != 1 || r.Rows[0].Orders != 0 || r.Rows[0].Buyers != 0 || r.Rows[0].FirstOrderAt != nil || r.Rows[0].LastOrderAt != nil {
		t.Fatalf("empty feed rows %+v", r.Rows)
	}
	if r.Feed.OldestOrderedAt != nil || r.Feed.LastReceivedAt != nil || len(r.Known) != 0 {
		t.Fatalf("empty feed %+v known %+v", r.Feed, r.Known)
	}

	f.h.db.MustExec(`INSERT INTO sms_promo_codes (code, created_by) VALUES ('newCustomerSMS', 'listmonk:m')`)
	f.buy("o1", "promotion", "newCustomerSMS", h1, "2026-11-01T10:00:00Z")
	f.buy("o2", "promotion", "NEWCUSTOMERSMS", h1, "2026-11-05T10:00:00Z") // the same buyer, another spelling
	f.buy("o2", "promotion", "newcustomersms", h1, "2026-11-05T10:00:00Z") // the same order carrying it twice
	f.buy("o3", "promotion", "newCustomerSMS", h2, "2026-11-03T10:00:00Z")
	f.buy("o4", "sku", "newCustomerSMS", h2, "2026-11-20T10:00:00Z")              // a SKU of the same text: never counted
	f.buy("o5", "promotion", "newCustomerSMS-x", h2, "2026-11-21T10:00:00Z")      // a different value: equality, never a pattern
	f.buy("o6", "promotion", "newCustomer%", h2, "2026-11-22T10:00:00Z")          // a value that would match as a LIKE pattern
	f.buy("o7", "promotion", "komigobox", h1, "2026-10-01T10:00:00Z")             // known: the most frequent spelling wins
	f.buy("o8", "promotion", "komigobox", h2, "2026-10-02T10:00:00Z")
	f.buy("o9", "promotion", "KomigoBox", h2, "2026-10-03T10:00:00Z")

	r = f.report(t)
	if len(r.Rows) != 2 {
		t.Fatalf("rows %+v", r.Rows)
	}
	got := r.Rows[0]
	if got.Code != "newCustomerSMS" || got.Orders != 3 || got.Buyers != 2 || got.Brand != nil {
		t.Fatalf("newCustomerSMS row %+v (want 3 orders o1/o2/o3, 2 buyers, All brands)", got)
	}
	if got.FirstOrderAt == nil || !strings.HasPrefix(*got.FirstOrderAt, "2026-11-01T10:00:00") ||
		got.LastOrderAt == nil || !strings.HasPrefix(*got.LastOrderAt, "2026-11-05T10:00:00") {
		t.Fatalf("first/last %v %v", got.FirstOrderAt, got.LastOrderAt)
	}
	if z := r.Rows[1]; z.Code != "BFCM-Deal-2" || z.Orders != 0 || z.LastOrderAt != nil || z.Brand == nil || *z.Brand != "thirstygirl" {
		t.Fatalf("zero-order row last (NULLS LAST) %+v", z)
	}
	if r.Feed.OldestOrderedAt == nil || !strings.HasPrefix(*r.Feed.OldestOrderedAt, "2026-10-01T10:00:00") || r.Feed.LastReceivedAt == nil || r.Feed.Stale {
		t.Fatalf("feed %+v", r.Feed)
	}
	known := []string{}
	for _, k := range r.Known {
		known = append(known, fmt.Sprintf("%s:%d", k.Value, k.Orders))
	}
	// The marked code is out in every spelling; komigobox's three orders under its majority spelling.
	if strings.Join(known, " ") != "komigobox:3 newCustomer%:1 newCustomerSMS-x:1" {
		t.Fatalf("known %v", known)
	}

	// known is at most 200.
	f.h.db.MustExec(`INSERT INTO promo_purchases (order_id, kind, value, value_lc, email_hash, ordered_at)
		SELECT 'bulk' || g, 'promotion', 'bulk-' || g, 'bulk-' || g, $1, NOW() - INTERVAL '1 day' FROM generate_series(1, 250) g`, h1)
	if r = f.report(t); len(r.Known) != 200 || r.Known[0].Value != "komigobox" {
		t.Fatalf("known has %d entries (first %+v), want 200 led by komigobox", len(r.Known), r.Known[0])
	}

	// A stale feed says so.
	f.h.db.MustExec(`UPDATE promo_purchases SET received_at = NOW() - INTERVAL '3 days'`)
	if r = f.report(t); !r.Feed.Stale {
		t.Fatal("a 3-day-old feed is not stale")
	}
}

// I9 -- marking and unmarking.
func TestSMSPromoMarking(t *testing.T) {
	f := newSMSDBFixture(t)
	f.buy("o1", "promotion", "newCustomerSMS", strings.Repeat("a", 64), "2026-11-01T10:00:00Z")

	post := func(body string) (int, map[string]any, string) {
		rec := f.sms.do(http.MethodPost, "/api/sms/promo-codes", "application/json", body)
		var out struct {
			Data map[string]any `json:"data"`
		}
		json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out.Data, rec.Body.String()
	}

	code, row, body := post(`{"code": "  newCustomerSMS  ", "created_by": "forged"}`)
	if code != http.StatusCreated || row["code"] != "newCustomerSMS" || row["created_by"] != f.smsWho || row["brand"] != nil {
		t.Fatalf("mark: %d %s", code, body)
	}
	id := int(row["id"].(float64))

	if code, _, body := post(`{"code": "NEWCUSTOMERSMS"}`); code != http.StatusConflict || !strings.Contains(body, f.h.app.i18n.T("sms.codeExists")) {
		t.Fatalf("case-insensitive duplicate: %d %s", code, body)
	}
	for name, b := range map[string]string{
		"empty":     `{"code": "   "}`,
		"absent":    `{}`,
		"too long":  fmt.Sprintf(`{"code": %q}`, strings.Repeat("é", 129)),
		"control":   `{"code": "a\u0007b"}`,
		"NUL":       `{"code": "a\u0000b"}`,
		"not json":  `nope`,
		"bad brand": `{"code": "BFCM-Deal-2", "brand": "nope"}`,
	} {
		if code, _, body := post(b); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, code, body)
		}
	}
	if code, row, body := post(`{"code": "BFCM-Deal-2", "brand": "thirstygirl"}`); code != http.StatusCreated || row["brand"] != "thirstygirl" {
		t.Fatalf("mark with a brand: %d %s", code, body)
	}
	if code, _, body := post(fmt.Sprintf(`{"code": %q}`, strings.Repeat("é", 128))); code != http.StatusCreated {
		t.Fatalf("a 128-character code: %d %s", code, body)
	}
	var stored []string
	f.h.db.Select(&stored, `SELECT code || '|' || COALESCE(brand, '-') || '|' || created_by FROM sms_promo_codes WHERE LENGTH(code) < 50 ORDER BY id`)
	if strings.Join(stored, " ") != "newCustomerSMS|-|"+f.smsWho+" BFCM-Deal-2|thirstygirl|"+f.smsWho {
		t.Fatalf("stored %v", stored)
	}

	// Unmark: absent id 404; present id 200, the feed untouched.
	if rec := f.sms.do(http.MethodDelete, "/api/sms/promo-codes/99999", "", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unmark absent: %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.sms.do(http.MethodDelete, fmt.Sprintf("/api/sms/promo-codes/%d", id), "", ""); rec.Code != http.StatusOK {
		t.Fatalf("unmark: %d %s", rec.Code, rec.Body.String())
	}
	var marked, feed int
	f.h.db.Get(&marked, `SELECT COUNT(*) FROM sms_promo_codes WHERE id = $1`, id)
	f.h.db.Get(&feed, `SELECT COUNT(*) FROM promo_purchases`)
	if marked != 0 || feed != 1 {
		t.Fatalf("after unmark: %d marking rows for the id, %d feed rows (want 0, 1)", marked, feed)
	}
	// Unmarked, the value is a suggestion again.
	if r := f.report(t); len(r.Known) != 1 || r.Known[0].Value != "newCustomerSMS" {
		t.Fatalf("known after unmark %+v", r.Known)
	}

	// The brand FK: a deleted brand leaves the marking as All brands.
	f.h.db.MustExec(`DELETE FROM brands WHERE slug = 'thirstygirl'`)
	var brand *string
	f.h.db.Get(&brand, `SELECT brand FROM sms_promo_codes WHERE code = 'BFCM-Deal-2'`)
	if brand != nil {
		t.Fatalf("brand after the brand row's delete: %v", *brand)
	}
}
