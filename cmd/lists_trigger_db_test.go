package main

// Fork (list join triggers) -- integrations LIST-JOIN-TRIGGER-SPEC D2-D6, invariants I3-I9:
// PUT /api/lists/:id carries the tri-state join_trigger and transactional fields (validated and
// permission-checked BEFORE anything is applied), the audit stamps move only on a real change, a
// join_trigger change nulls trigger_sync, POST ignores both fields, and PUT
// /api/lists/:id/trigger-sync stores the reconciler's write-back verbatim after its shape check.
// Shares newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go). Fixture names are
// example.test only.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/paginator"
	"github.com/labstack/echo/v4"
)

type triggerHarness struct {
	t *testing.T
	h *linkHarness
}

func perms(name string, p ...string) auth.User {
	m := map[string]struct{}{}
	for _, x := range p {
		m[x] = struct{}{}
	}
	return auth.User{Username: name, PermissionsMap: m}
}

// call runs a handler and returns the status code and, on success, the decoded data.
func (th *triggerHarness) call(fn echo.HandlerFunc, method, path string, id int, body string, user auth.User) (int, map[string]json.RawMessage) {
	th.t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if id > 0 {
		c.Set("id", id)
	}
	c.Set(auth.UserHTTPCtxKey, user)
	if err := fn(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, nil
		}
		th.t.Fatalf("%s %s: %v", method, path, err)
	}
	var resp struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp.Data
}

func (th *triggerHarness) put(id int, body string, user auth.User) (int, map[string]json.RawMessage) {
	return th.call(th.h.app.UpdateList, http.MethodPut, "/api/lists/"+itoa(id), id, body, user)
}

func (th *triggerHarness) sync(id int, body string, user auth.User) int {
	code, _ := th.call(th.h.app.PutListTriggerSync, http.MethodPut, "/api/lists/"+itoa(id)+"/trigger-sync", id, body, user)
	return code
}

// get reads one list through GET /api/lists/:id.
func (th *triggerHarness) get(id int) map[string]json.RawMessage {
	th.t.Helper()
	code, data := th.call(th.h.app.GetList, http.MethodGet, "/api/lists/"+itoa(id), id, "", perms("reader", auth.PermListGetAll))
	if code != http.StatusOK {
		th.t.Fatalf("GetList %d: %d", id, code)
	}
	return data
}

// row reads the stored columns directly.
type triggerRow struct {
	Name        string  `db:"name"`
	JoinTrigger *string `db:"join_trigger"`
	Trans       bool    `db:"transactional"`
	SetBy       *string `db:"trigger_set_by"`
	SetAt       *string `db:"trigger_set_at"`
	Sync        *string `db:"trigger_sync"`
	UpdatedAt   string  `db:"updated_at"`
}

func (th *triggerHarness) row(id int) triggerRow {
	th.t.Helper()
	var r triggerRow
	if err := th.h.db.Get(&r, `SELECT name, join_trigger::TEXT AS join_trigger, transactional, trigger_set_by,
		trigger_set_at::TEXT AS trigger_set_at, trigger_sync::TEXT AS trigger_sync, updated_at::TEXT AS updated_at
		FROM lists WHERE id = $1`, id); err != nil {
		th.t.Fatalf("row %d: %v", id, err)
	}
	return r
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func newTriggerHarness(t *testing.T) (*triggerHarness, int, int) {
	h := newLinkHarness(t)
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})
	h.db.MustExec(`INSERT INTO brands (slug, from_email) VALUES ('acme', 'Acme <hello@acme.test>')`)
	var branded, untagged int
	h.db.Get(&branded, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme buyers', 'private', 'single',
		'{brand:acme,"from:Acme <hello@acme.test>"}') RETURNING id`)
	h.db.Get(&untagged, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Plain', 'private', 'single') RETURNING id`)
	return &triggerHarness{t: t, h: h}, branded, untagged
}

var (
	marketer = perms("marketer", auth.PermListGetAll, auth.PermListManageAll)
	admin    = perms("admin", auth.PermListGetAll, auth.PermListManageAll, auth.PermListTransactional)
)

// I4 -- tri-state join_trigger with normalization and refusals; GET returns the five fields.
func TestListTriggerTriState(t *testing.T) {
	th, list, _ := newTriggerHarness(t)

	// Nothing stored: transactional always present (false), the other four absent.
	g := th.get(list)
	if string(g["transactional"]) != "false" {
		t.Fatalf("transactional = %s", g["transactional"])
	}
	for _, k := range []string{"join_trigger", "trigger_set_by", "trigger_set_at", "trigger_sync"} {
		if _, ok := g[k]; ok {
			t.Fatalf("%s present before any write: %s", k, g[k])
		}
	}

	// An object is validated, normalized (trim, lower-case, de-dup in order) and stored.
	code, data := th.put(list, `{"name":"Acme buyers","join_trigger":{"kind":"skus","skus":[" Widget-A ","widget-a","B-2"]}}`, marketer)
	if code != http.StatusOK {
		t.Fatalf("valid skus PUT: %d", code)
	}
	var jt map[string]any
	json.Unmarshal(data["join_trigger"], &jt)
	if skus, _ := jt["skus"].([]any); jt["kind"] != "skus" || len(skus) != 2 || skus[0] != "widget-a" || skus[1] != "b-2" {
		t.Fatalf("response join_trigger = %s", data["join_trigger"])
	}
	r := th.row(list)
	if str(r.SetBy) != "marketer" || r.SetAt == nil {
		t.Fatalf("stamps after a change: by %s at %s", str(r.SetBy), str(r.SetAt))
	}
	g = th.get(list)
	for _, k := range []string{"join_trigger", "trigger_set_by", "trigger_set_at"} {
		if _, ok := g[k]; !ok {
			t.Fatalf("GET lacks %s after a write", k)
		}
	}

	// Absent keeps.
	if code, _ := th.put(list, `{"name":"Acme buyers 2"}`, marketer); code != http.StatusOK {
		t.Fatalf("rename: %d", code)
	}
	if r := th.row(list); r.JoinTrigger == nil || !strings.Contains(*r.JoinTrigger, "widget-a") || r.Name != "Acme buyers 2" {
		t.Fatalf("absent join_trigger changed it: %s (%s)", str(r.JoinTrigger), r.Name)
	}

	// first_order.
	if code, _ := th.put(list, `{"name":"Acme buyers 2","join_trigger":{"kind":"first_order"}}`, marketer); code != http.StatusOK {
		t.Fatalf("first_order: %d", code)
	}
	if r := th.row(list); str(r.JoinTrigger) != `{"kind": "first_order"}` {
		t.Fatalf("first_order stored as %s", str(r.JoinTrigger))
	}

	// The refusals: 400, nothing stored (the name in the same request included -- F13).
	long := strings.Repeat("x", 101)
	many := make([]string, 201)
	for i := range many {
		many[i] = `"s` + itoa(i) + `"`
	}
	for name, jtBody := range map[string]string{
		"not an object":     `"first_order"`,
		"array":             `[1]`,
		"no kind":           `{"skus":["a"]}`,
		"unknown kind":      `{"kind":"last_order"}`,
		"kind not string":   `{"kind":1}`,
		"first_order extra": `{"kind":"first_order","skus":["a"]}`,
		"skus extra key":    `{"kind":"skus","skus":["a"],"brand":"acme"}`,
		"skus missing":      `{"kind":"skus"}`,
		"skus not array":    `{"kind":"skus","skus":"a"}`,
		"skus empty":        `{"kind":"skus","skus":[]}`,
		"sku blank":         `{"kind":"skus","skus":["a","  "]}`,
		"sku not string":    `{"kind":"skus","skus":["a",3]}`,
		"sku too long":      `{"kind":"skus","skus":["` + long + `"]}`,
		"too many skus":     `{"kind":"skus","skus":[` + strings.Join(many, ",") + `]}`,
		// NO-PURCHASE-STEPS-SPEC I1 -- the no_purchase kind's refusals.
		"days missing":    `{"kind":"no_purchase"}`,
		"days null":       `{"kind":"no_purchase","days":null}`,
		"days float":      `{"kind":"no_purchase","days":7.5}`,
		"days 7.0":        `{"kind":"no_purchase","days":7.0}`,
		"days exponent":   `{"kind":"no_purchase","days":7e0}`,
		"days string":     `{"kind":"no_purchase","days":"7"}`,
		"days 0":          `{"kind":"no_purchase","days":0}`,
		"days negative":   `{"kind":"no_purchase","days":-3}`,
		"days 366":        `{"kind":"no_purchase","days":366}`,
		"days huge":       `{"kind":"no_purchase","days":99999999}`,
		"no_purchase key": `{"kind":"no_purchase","days":7,"skus":["a"]}`,
		// LIFECYCLE-SERIES-SPEC I12 -- the post_purchase kind's refusals (same shape as no_purchase).
		"post days missing": `{"kind":"post_purchase"}`,
		"post days float":   `{"kind":"post_purchase","days":5.5}`,
		"post days string":  `{"kind":"post_purchase","days":"5"}`,
		"post days 0":       `{"kind":"post_purchase","days":0}`,
		"post days 366":     `{"kind":"post_purchase","days":366}`,
		"post extra key":    `{"kind":"post_purchase","days":5,"skus":["a"]}`,
	} {
		if code, _ := th.put(list, `{"name":"Must not apply","join_trigger":`+jtBody+`}`, marketer); code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d", name, code)
		}
	}
	if r := th.row(list); r.Name != "Acme buyers 2" || str(r.JoinTrigger) != `{"kind": "first_order"}` {
		t.Fatalf("a refused request applied something: %q %s", r.Name, str(r.JoinTrigger))
	}
	// 100 characters and 200 SKUs (after de-dup) are the limits, not refusals.
	exact := make([]string, 200)
	for i := range exact {
		exact[i] = `"s` + itoa(i) + `"`
	}
	if code, _ := th.put(list, `{"name":"Acme buyers 2","join_trigger":{"kind":"skus","skus":["`+strings.Repeat("y", 100)+`",`+strings.Join(exact[:199], ",")+`,"S0"]}}`, marketer); code != http.StatusOK {
		t.Fatalf("at the limits: %d", code)
	}

	// NO-PURCHASE-STEPS-SPEC I1 -- no_purchase: days 1 and 365 are the limits; stored normalized.
	for _, days := range []string{"1", "365", " 7 "} {
		if code, _ := th.put(list, `{"name":"Acme buyers 2","join_trigger":{ "days" : `+days+`, "kind":"no_purchase" }}`, marketer); code != http.StatusOK {
			t.Fatalf("no_purchase days %s: %d", days, code)
		}
		want := `{"days": ` + strings.TrimSpace(days) + `, "kind": "no_purchase"}`
		if r := th.row(list); str(r.JoinTrigger) != want {
			t.Fatalf("no_purchase days %s stored as %s, want %s", days, str(r.JoinTrigger), want)
		}
	}
	// LIFECYCLE-SERIES-SPEC I12 -- post_purchase is stored normalized too.
	if code, _ := th.put(list, `{"name":"Acme buyers 2","join_trigger":{"days":14,"kind":"post_purchase"}}`, marketer); code != http.StatusOK {
		t.Fatalf("post_purchase days 14: %d", code)
	}
	if r := th.row(list); str(r.JoinTrigger) != `{"days": 14, "kind": "post_purchase"}` {
		t.Fatalf("post_purchase stored as %s", str(r.JoinTrigger))
	}
	// The pure validator's normalized output (key order kind, days; the other kinds unchanged).
	for in, want := range map[string]string{
		`{"days":30,"kind":"no_purchase"}`:   `{"kind":"no_purchase","days":30}`,
		`{"days":30,"kind":"post_purchase"}`: `{"kind":"post_purchase","days":30}`,
		`{"kind":"first_order"}`:           `{"kind":"first_order"}`,
		`{"kind":"skus","skus":[" A "]}`:   `{"kind":"skus","skus":["a"]}`,
	} {
		got, err := validateJoinTrigger([]byte(in))
		if err != nil || string(got) != want {
			t.Fatalf("validateJoinTrigger(%s) = %s, %v; want %s", in, got, err, want)
		}
	}

	// null clears.
	if code, _ := th.put(list, `{"name":"Acme buyers 2","join_trigger":null}`, marketer); code != http.StatusOK {
		t.Fatalf("null: %d", code)
	}
	if r := th.row(list); r.JoinTrigger != nil {
		t.Fatalf("null did not clear: %s", str(r.JoinTrigger))
	}
	if _, ok := th.get(list)["join_trigger"]; ok {
		t.Fatal("GET carries join_trigger after a clear")
	}

	// The list endpoints (full and minimal) carry the fields too.
	th.h.db.MustExec(`UPDATE lists SET join_trigger = '{"kind":"first_order"}', trigger_sync = '{"state":"live","slug":"x","synced_at":"2026-10-14T05:05:12Z"}' WHERE id = $1`, list)
	for _, q := range []string{"/api/lists?per_page=all", "/api/lists?minimal=true&per_page=all"} {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, q, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(auth.UserHTTPCtxKey, perms("reader", auth.PermListGetAll))
		if err := th.h.app.GetLists(c); err != nil {
			t.Fatalf("GetLists %s: %v", q, err)
		}
		var resp struct {
			Data struct {
				Results []map[string]json.RawMessage `json:"results"`
			} `json:"data"`
		}
		json.Unmarshal(rec.Body.Bytes(), &resp)
		found := false
		for _, l := range resp.Data.Results {
			var id int
			json.Unmarshal(l["id"], &id)
			if id != list {
				continue
			}
			found = true
			for _, k := range []string{"join_trigger", "transactional", "trigger_set_by", "trigger_set_at", "trigger_sync"} {
				if _, ok := l[k]; !ok {
					t.Fatalf("%s: list lacks %s", q, k)
				}
			}
		}
		if !found {
			t.Fatalf("%s: list %d missing", q, list)
		}
	}
}

// I5, I6 -- the write runs only on a real change; a join_trigger change nulls trigger_sync, a
// transactional change alone does not.
func TestListTriggerStampsAndSyncReset(t *testing.T) {
	th, list, _ := newTriggerHarness(t)

	if code, _ := th.put(list, `{"name":"Acme buyers","join_trigger":{"kind":"first_order"}}`, marketer); code != http.StatusOK {
		t.Fatalf("set: %d", code)
	}
	// Pin the stamps to a known past value, then send the same value (in another form) twice.
	th.h.db.MustExec(`UPDATE lists SET trigger_set_at = '2026-01-01T00:00:00Z', trigger_set_by = 'earlier',
		trigger_sync = '{"synced_at":"2026-10-14T05:05:12Z","state":"live","slug":"acme-first-order"}' WHERE id = $1`, list)
	before := th.row(list)
	for i := 0; i < 2; i++ {
		if code, _ := th.put(list, `{"name":"Acme buyers","join_trigger":{ "kind" : "first_order" },"transactional":false}`, admin); code != http.StatusOK {
			t.Fatalf("same value %d: %d", i, code)
		}
	}
	after := th.row(list)
	if str(after.SetBy) != "earlier" || str(after.SetAt) != str(before.SetAt) || str(after.Sync) != str(before.Sync) {
		t.Fatalf("an unchanged value moved the stamps or the sync: %+v -> %+v", before, after)
	}

	// transactional alone changes: stamped, trigger_sync kept (I6).
	if code, _ := th.put(list, `{"name":"Acme buyers","transactional":true}`, admin); code != http.StatusOK {
		t.Fatalf("transactional: %d", code)
	}
	r := th.row(list)
	if !r.Trans || str(r.SetBy) != "admin" || str(r.SetAt) == str(before.SetAt) {
		t.Fatalf("transactional change not stored/stamped: %+v", r)
	}
	if str(r.Sync) != str(before.Sync) {
		t.Fatalf("a transactional-only change touched trigger_sync: %s", str(r.Sync))
	}

	// join_trigger changes: trigger_sync nulled (the badge reads Pending sync).
	if code, _ := th.put(list, `{"name":"Acme buyers","join_trigger":{"kind":"skus","skus":["a"]}}`, admin); code != http.StatusOK {
		t.Fatalf("change: %d", code)
	}
	if r := th.row(list); r.Sync != nil {
		t.Fatalf("a join_trigger change kept trigger_sync: %s", str(r.Sync))
	}

	// A plain rename leaves the trigger, the stamps and trigger_sync alone.
	th.h.db.MustExec(`UPDATE lists SET trigger_sync = '{"synced_at":"2026-10-14T05:10:00Z","state":"live","slug":"acme-skus-1"}' WHERE id = $1`, list)
	before = th.row(list)
	if code, _ := th.put(list, `{"name":"Acme renamed"}`, marketer); code != http.StatusOK {
		t.Fatalf("rename: %d", code)
	}
	after = th.row(list)
	if after.Name != "Acme renamed" || str(after.Sync) != str(before.Sync) || str(after.SetAt) != str(before.SetAt) || str(after.JoinTrigger) != str(before.JoinTrigger) {
		t.Fatalf("a rename touched the trigger: %+v -> %+v", before, after)
	}
}

// I3 -- a non-null RESULTING join_trigger needs a RESULTING brand, both ways.
func TestListTriggerNeedsBrand(t *testing.T) {
	th, branded, untagged := newTriggerHarness(t)

	// Set on an untagged list: 400, nothing applied.
	if code, _ := th.put(untagged, `{"name":"Plain renamed","join_trigger":{"kind":"first_order"}}`, marketer); code != http.StatusBadRequest {
		t.Fatalf("trigger on untagged: want 400, got %d", code)
	}
	if r := th.row(untagged); r.JoinTrigger != nil || r.Name != "Plain" {
		t.Fatalf("refused request applied: %+v", r)
	}

	// Clear the brand of a list carrying a trigger: 400, brand kept.
	if code, _ := th.put(branded, `{"name":"Acme buyers","join_trigger":{"kind":"first_order"}}`, marketer); code != http.StatusOK {
		t.Fatalf("set: %d", code)
	}
	if code, _ := th.put(branded, `{"name":"Acme buyers","brand":""}`, marketer); code != http.StatusBadRequest {
		t.Fatalf("clear brand under a trigger: want 400, got %d", code)
	}
	if b := string(th.get(branded)["brand"]); b != `"acme"` {
		t.Fatalf("brand changed by a refused request: %s", b)
	}
	// Clearing both together is fine.
	if code, _ := th.put(branded, `{"name":"Acme buyers","brand":"","join_trigger":null}`, marketer); code != http.StatusOK {
		t.Fatalf("clear both: %d", code)
	}

	// Brand and trigger set in the same request: checked on the post-projection brand.
	if code, _ := th.put(untagged, `{"name":"Plain","brand":"acme","join_trigger":{"kind":"first_order"}}`, marketer); code != http.StatusOK {
		t.Fatalf("brand + trigger together: %d", code)
	}
	if r := th.row(untagged); r.JoinTrigger == nil {
		t.Fatal("brand + trigger together not stored")
	}
}

// I7, I8 -- the permission matrix (gate G3). The 403 runs before anything is applied.
func TestListTriggerTransactionalPermission(t *testing.T) {
	th, list, _ := newTriggerHarness(t)

	// Without the permission, a differing transactional is 403 and the rename in the same request
	// is NOT stored (the order is the invariant).
	if code, _ := th.put(list, `{"name":"Renamed","transactional":true}`, marketer); code != http.StatusForbidden {
		t.Fatalf("unpermitted transactional: want 403, got %d", code)
	}
	if r := th.row(list); r.Name != "Acme buyers" || r.Trans {
		t.Fatalf("a 403 applied something: %+v", r)
	}
	// The stored value round-tripped (a whole GET row) is ignored: 200, no stamp.
	if code, _ := th.put(list, `{"name":"Renamed","transactional":false}`, marketer); code != http.StatusOK {
		t.Fatalf("round-tripped transactional: %d", code)
	}
	if r := th.row(list); r.Name != "Renamed" || r.SetBy != nil {
		t.Fatalf("round-trip: %+v", r)
	}
	// A per-list manager (list:manage on this list only) may switch a journey on (D5).
	perList := auth.User{Username: "creator", PermissionsMap: map[string]struct{}{},
		ListPermissionsMap: map[int]map[string]struct{}{list: {auth.PermListGet: {}, auth.PermListManage: {}}}}
	if code, _ := th.put(list, `{"name":"Renamed","join_trigger":{"kind":"first_order"}}`, perList); code != http.StatusOK {
		t.Fatalf("per-list manager sets a trigger: %d", code)
	}
	if code, _ := th.put(list, `{"name":"Renamed","transactional":true}`, perList); code != http.StatusForbidden {
		t.Fatalf("per-list manager sets transactional: want 403, got %d", code)
	}

	// With the permission: 200, stored, stamped with that user (I8).
	if code, _ := th.put(list, `{"name":"Renamed","transactional":true}`, admin); code != http.StatusOK {
		t.Fatalf("permitted transactional: %d", code)
	}
	if r := th.row(list); !r.Trans || str(r.SetBy) != "admin" {
		t.Fatalf("permitted: %+v", r)
	}

	// The pairing is locked (F6): on a transactional list, a join_trigger change (set, change or
	// clear) or a brand change without the permission is 403 and applies nothing.
	th.h.db.MustExec(`INSERT INTO brands (slug, from_email) VALUES ('other', 'Other <hello@acme.test>')`)
	for name, body := range map[string]string{
		"change trigger":  `{"name":"Locked rename","join_trigger":{"kind":"skus","skus":["a"]}}`,
		"clear trigger":   `{"name":"Locked rename","join_trigger":null}`,
		"invalid trigger": `{"name":"Locked rename","join_trigger":{"kind":"nope"}}`,
		"change brand":    `{"name":"Locked rename","brand":"other"}`,
		"clear brand":     `{"name":"Locked rename","brand":""}`,
	} {
		if code, _ := th.put(list, body, marketer); code != http.StatusForbidden {
			t.Fatalf("%s on a transactional list: want 403, got %d", name, code)
		}
	}
	if r := th.row(list); r.Name != "Renamed" || str(r.JoinTrigger) != `{"kind": "first_order"}` {
		t.Fatalf("a pairing 403 applied something: %+v", r)
	}
	// A whole GET row round-tripped (same trigger, same brand, same transactional) is fine.
	if code, _ := th.put(list, `{"name":"Renamed again","brand":"acme","join_trigger":{"kind":"first_order"},"transactional":true}`, marketer); code != http.StatusOK {
		t.Fatalf("round-trip on a transactional list: %d", code)
	}
	// The admin may change the pairing.
	if code, _ := th.put(list, `{"name":"Renamed again","join_trigger":{"kind":"skus","skus":["a"]}}`, admin); code != http.StatusOK {
		t.Fatalf("admin changes the trigger of a transactional list: %d", code)
	}

	// A JSON null transactional is absent (unchanged), even without the permission.
	if code, _ := th.put(list, `{"name":"Renamed again","transactional":null}`, marketer); code != http.StatusOK {
		t.Fatalf("null transactional: %d", code)
	}
	if r := th.row(list); !r.Trans {
		t.Fatal("a null transactional cleared the flag")
	}
}

// I8 -- POST /api/lists ignores both fields.
func TestListTriggerCreateIgnoresFields(t *testing.T) {
	th, _, _ := newTriggerHarness(t)
	code, data := th.call(th.h.app.CreateList, http.MethodPost, "/api/lists", 0,
		`{"name":"Created","type":"private","optin":"single","brand":"acme","join_trigger":{"kind":"first_order"},"transactional":true}`, admin)
	if code != http.StatusOK {
		t.Fatalf("create: %d", code)
	}
	var id int
	json.Unmarshal(data["id"], &id)
	r := th.row(id)
	if r.JoinTrigger != nil || r.Trans || r.SetBy != nil || r.SetAt != nil {
		t.Fatalf("POST stored trigger fields: %+v", r)
	}
}

// I9 -- the write-back endpoint.
func TestListTriggerSyncEndpoint(t *testing.T) {
	th, list, _ := newTriggerHarness(t)
	updatedBefore := th.row(list).UpdatedAt

	live := `{"synced_at":"2026-10-14T05:05:12Z","state":"live","slug":"acme-first-order"}`
	if code := th.sync(list, live, marketer); code != http.StatusOK {
		t.Fatalf("live: %d", code)
	}
	var stored map[string]any
	json.Unmarshal(th.get(list)["trigger_sync"], &stored)
	if stored["state"] != "live" || stored["slug"] != "acme-first-order" || stored["synced_at"] != "2026-10-14T05:05:12Z" || len(stored) != 3 {
		t.Fatalf("stored %v", stored)
	}
	invalid := `{"synced_at":"2026-10-14T05:10:00Z","state":"invalid","error":"acme already has a first-order trigger on list 7"}`
	if code := th.sync(list, invalid, marketer); code != http.StatusOK {
		t.Fatalf("invalid: %d", code)
	}
	if r := th.row(list); r.UpdatedAt != updatedBefore {
		t.Fatalf("updated_at moved: %s -> %s", updatedBefore, r.UpdatedAt)
	}

	before := str(th.row(list).Sync)
	for name, body := range map[string]string{
		"not json":          `nope`,
		"array":             `[1]`,
		"no synced_at":      `{"state":"live","slug":"x"}`,
		"bad synced_at":     `{"synced_at":"yesterday","state":"live","slug":"x"}`,
		"no state":          `{"synced_at":"2026-10-14T05:05:12Z","slug":"x"}`,
		"unknown state":     `{"synced_at":"2026-10-14T05:05:12Z","state":"pending"}`,
		"live without slug": `{"synced_at":"2026-10-14T05:05:12Z","state":"live"}`,
		"live slug number":  `{"synced_at":"2026-10-14T05:05:12Z","state":"live","slug":3}`,
		"live with error":   `{"synced_at":"2026-10-14T05:05:12Z","state":"live","slug":"x","error":"e"}`,
		"invalid no error":  `{"synced_at":"2026-10-14T05:05:12Z","state":"invalid"}`,
		"invalid empty":     `{"synced_at":"2026-10-14T05:05:12Z","state":"invalid","error":" "}`,
		"invalid with slug": `{"synced_at":"2026-10-14T05:05:12Z","state":"invalid","error":"e","slug":"x"}`,
		"error too long":    `{"synced_at":"2026-10-14T05:05:12Z","state":"invalid","error":"` + strings.Repeat("e", 501) + `"}`,
		"unknown key":       `{"synced_at":"2026-10-14T05:05:12Z","state":"live","slug":"x","note":"n"}`,
		"two objects":       live + `{}`,
		"body over the cap": `{"synced_at":"2026-10-14T05:05:12Z","state":"live","slug":"` + strings.Repeat("s", 4100) + `"}`,
	} {
		if code := th.sync(list, body, marketer); code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d", name, code)
		}
	}
	if str(th.row(list).Sync) != before {
		t.Fatal("a refused write changed the stored document")
	}
	// 500 characters is the limit, not a refusal.
	if code := th.sync(list, `{"synced_at":"2026-10-14T05:05:12Z","state":"invalid","error":"`+strings.Repeat("e", 500)+`"}`, marketer); code != http.StatusOK {
		t.Fatalf("500-char error: %d", code)
	}

	// lists:manage_all only: a per-list manager and a get-only user are refused; a missing list 404.
	perList := auth.User{Username: "creator", PermissionsMap: map[string]struct{}{},
		ListPermissionsMap: map[int]map[string]struct{}{list: {auth.PermListGet: {}, auth.PermListManage: {}}}}
	if code := th.sync(list, live, perList); code != http.StatusForbidden {
		t.Fatalf("per-list manager: want 403, got %d", code)
	}
	if code := th.sync(list, live, perms("reader", auth.PermListGetAll)); code != http.StatusForbidden {
		t.Fatalf("get-only: want 403, got %d", code)
	}
	if code := th.sync(999999, live, marketer); code != http.StatusNotFound {
		t.Fatalf("missing list: want 404, got %d", code)
	}
}
