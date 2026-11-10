package main

// Fork (journey lists) -- integrations listmonk/JOURNEY-LISTS-SPEC D2/D3, invariants I5 and I6 (I15 is
// I5 + I6 together): PUT /api/lists/:id/active turns on only with a running automation on the
// list, off always, refuses an untriggered or archived list and a malformed body, writes nothing
// on the same value; UpdateList/CreateList never write the flag; a change of the trigger, the
// brand (a brand-only PUT included) or transactional turns an active list off, any other field
// does not. Real router, real database (lockFixture; LISTMONK_TEST_PG opt-in).

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lib/pq"
)

// activeRow is comparable: a NULL stamp reads as "" (At/By are pointers to keep the nil checks
// below readable; key() is what the equality checks compare).
type activeRow struct {
	Active    bool    `db:"trigger_active"`
	At        *string `db:"trigger_active_at"`
	By        *string `db:"trigger_active_by"`
	UpdatedAt string  `db:"updated_at"`
}

func (r activeRow) key() string {
	return str(r.At) + "|" + str(r.By) + "|" + r.UpdatedAt + "|" + map[bool]string{true: "on", false: "off"}[r.Active]
}

func (l *lockFixture) active(id int) activeRow {
	l.h.t.Helper()
	var r activeRow
	if err := l.h.db.Get(&r, `SELECT trigger_active, trigger_active_at::TEXT AS trigger_active_at, trigger_active_by, updated_at::TEXT AS updated_at FROM lists WHERE id = $1`, id); err != nil {
		l.h.t.Fatal(err)
	}
	return r
}

// automation inserts an evergreen campaign on the list with the given status.
func (l *lockFixture) automation(list int, status string) int {
	l.h.t.Helper()
	var id int
	if err := l.h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, evergreen, status, template_id)
		VALUES (gen_random_uuid(), 'auto', 's', 'f@acme.test', 'b', 'email', true, $1, (SELECT id FROM templates ORDER BY id LIMIT 1)) RETURNING id`, status); err != nil {
		l.h.t.Fatal(err)
	}
	l.h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'x')`, id, list)
	return id
}

func (l *lockFixture) putActive(id int, body string) (int, map[string]json.RawMessage) {
	l.h.t.Helper()
	rec := l.mkt.do(http.MethodPut, "/api/lists/"+itoa(id)+"/active", "application/json", body)
	var resp struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp.Data
}

func TestActiveSwitch(t *testing.T) {
	l := newLockFixture(t)

	// 1 -- no trigger, or archived: 400, nothing written.
	if code, _ := l.putActive(l.plain, `{"active":true}`); code != http.StatusBadRequest {
		t.Fatalf("untriggered list: %d", code)
	}
	archived := l.sqlList("Acme archived", "brand:acme", "from:"+pfBrandFrom)
	l.juTrigger(archived, `{"kind":"skus","skus":["z"]}`)
	l.h.db.MustExec(`UPDATE lists SET status = 'archived' WHERE id = $1`, archived)
	l.automation(archived, "running")
	if code, _ := l.putActive(archived, `{"active":true}`); code != http.StatusBadRequest {
		t.Fatalf("archived list: %d", code)
	}
	if l.active(archived).Active || l.active(l.plain).Active {
		t.Fatal("a refused switch wrote the flag")
	}

	// Malformed bodies: 400.
	for _, b := range []string{`{}`, `{"active":"true"}`, `{"active":1}`, `{"active":true,"x":1}`, `[true]`, `{"active":null}`} {
		if code, _ := l.putActive(l.fo, b); code != http.StatusBadRequest {
			t.Fatalf("body %s: %d", b, code)
		}
	}

	// 3 -- on without a running automation: 400 activeNeedsRunning, nothing written; a draft or
	// paused automation, or a running BROADCAST, is not one.
	before := l.active(l.fo)
	l.automation(l.fo, "draft")
	l.automation(l.fo, "paused")
	var bc int
	l.h.db.Get(&bc, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, evergreen, status, template_id)
		VALUES (gen_random_uuid(), 'bc', 's', 'f@acme.test', 'b', 'email', false, 'running', (SELECT id FROM templates ORDER BY id LIMIT 1)) RETURNING id`)
	l.h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'x')`, bc, l.fo)
	rec := l.mkt.do(http.MethodPut, "/api/lists/"+itoa(l.fo)+"/active", "application/json", `{"active":true}`)
	l.wantErr("on without a running automation", rec, http.StatusBadRequest, "lists.activeNeedsRunning")
	if got := l.active(l.fo); got.key() != before.key() {
		t.Fatalf("a refused switch wrote: %+v -> %+v", before, got)
	}
	g := l.ok("GET", l.mkt.json(http.MethodGet, "/api/lists/"+itoa(l.fo), nil))
	var gl struct {
		Running *bool `json:"running_automation"`
		Active  *bool `json:"trigger_active"`
	}
	json.Unmarshal(g, &gl)
	if gl.Running == nil || *gl.Running || gl.Active == nil || *gl.Active {
		t.Fatalf("GET before: %s", g)
	}

	// 3/4 -- with a running automation: 200 (Marketing, no lists:journeys, on a locked list), the
	// flag and both stamps written, updated_at untouched.
	l.automation(l.fo, "running")
	code, data := l.putActive(l.fo, `{"active":true}`)
	if code != http.StatusOK || string(data["trigger_active"]) != "true" || string(data["running_automation"]) != "true" {
		t.Fatalf("on: %d %v", code, data)
	}
	on := l.active(l.fo)
	if !on.Active || on.At == nil || on.By == nil || *on.By != "marketer" || on.UpdatedAt != before.UpdatedAt {
		t.Fatalf("on stored: %+v (before %+v)", on, before)
	}

	// 2 -- same value: 200, stamps unchanged.
	l.h.db.MustExec(`UPDATE lists SET trigger_active_at = '2026-01-01T00:00:00Z' WHERE id = $1`, l.fo)
	pinned := l.active(l.fo)
	if code, _ := l.putActive(l.fo, `{"active":true}`); code != http.StatusOK {
		t.Fatalf("same value: %d", code)
	}
	if got := l.active(l.fo); got.key() != pinned.key() {
		t.Fatalf("same value moved the stamps: %+v -> %+v", pinned, got)
	}

	// Off is always allowed, the automation still running; stamps move.
	if code, _ := l.putActive(l.fo, `{"active":false}`); code != http.StatusOK {
		t.Fatalf("off: %d", code)
	}
	if off := l.active(l.fo); off.Active || off.At == nil || *off.At == *pinned.At {
		t.Fatalf("off stored: %+v", off)
	}

	// Without the list's manage permission: refused.
	var bmRole int
	l.h.db.Get(&bmRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Brand Manager', $1) RETURNING id`, pq.StringArray([]string{"campaigns:get"}))
	bm := l.h.user("brandmgr", bmRole)
	if _, err := cacheUsers(l.h.app.core, l.h.app.auth); err != nil {
		t.Fatal(err)
	}
	if rec := l.h.sessionClient(bm).do(http.MethodPut, "/api/lists/"+itoa(l.fo)+"/active", "application/json", `{"active":false}`); rec.Code != http.StatusForbidden {
		t.Fatalf("no list permission: %d", rec.Code)
	}

	// The switch has one route: UpdateList and CreateList ignore trigger_active*.
	l.automation(l.sku, "running")
	l.ok("PUT carrying trigger_active", l.mkt.json(http.MethodPut, "/api/lists/"+itoa(l.sku),
		map[string]any{"name": "Acme widget buyers", "trigger_active": true, "trigger_active_by": "x"}))
	if r := l.active(l.sku); r.Active || r.By != nil {
		t.Fatalf("UpdateList wrote the switch: %+v", r)
	}
	var created struct {
		ID int `json:"id"`
	}
	json.Unmarshal(l.ok("POST carrying trigger_active", l.mkt.json(http.MethodPost, "/api/lists",
		map[string]any{"name": "Acme new", "type": "private", "optin": "single", "trigger_active": true})), &created)
	if r := l.active(created.ID); r.Active || r.At != nil {
		t.Fatalf("CreateList wrote the switch (D2b: new lists default inactive): %+v", r)
	}
}

// I6 -- a change of join_trigger, brand (a brand-only PUT included) or transactional turns an active
// list off; name, description, tags, opt-in and a round-trip do not.
func TestTriggerChangeTurnsOff(t *testing.T) {
	l := newLockFixture(t)
	l.brandRow("beta", "Beta <hello@beta.test>", "")
	// The transactional change needs lists:transactional (the admin's role gets it here).
	l.h.db.MustExec(`UPDATE roles SET permissions = permissions || '{lists:transactional}' WHERE id = 1`)
	if _, err := cacheUsers(l.h.app.core, l.h.app.auth); err != nil {
		t.Fatal(err)
	}
	on := func() {
		l.h.db.MustExec(`UPDATE lists SET trigger_active = true, trigger_active_at = '2026-01-01T00:00:00Z', trigger_active_by = 'earlier' WHERE id = $1`, l.sku)
	}
	turnsOff := func(what string, body map[string]any, by string) {
		t.Helper()
		on()
		cl := l.mkt
		if by == "admin" {
			cl = l.admin
		}
		l.ok(what, cl.json(http.MethodPut, "/api/lists/"+itoa(l.sku), body))
		r := l.active(l.sku)
		if r.Active || r.At == nil || *r.At == "2026-01-01 00:00:00+00" || r.By == nil || *r.By == "earlier" {
			t.Fatalf("%s did not turn Active off with a stamp: %+v", what, r)
		}
	}
	keeps := func(what string, body map[string]any) {
		t.Helper()
		on()
		before := l.active(l.sku)
		l.ok(what, l.mkt.json(http.MethodPut, "/api/lists/"+itoa(l.sku), body))
		r := l.active(l.sku)
		if !r.Active || *r.At != *before.At || *r.By != "earlier" {
			t.Fatalf("%s touched Active: %+v -> %+v", what, before, r)
		}
	}

	keeps("rename", map[string]any{"name": "Acme widget buyers 2"})
	keeps("description", map[string]any{"name": "Acme widget buyers 2", "description": "d"})
	keeps("tags", map[string]any{"name": "Acme widget buyers 2", "tags": []string{"holiday"}})
	keeps("opt-in", map[string]any{"name": "Acme widget buyers 2", "optin": "double"})
	keeps("trigger round-trip", map[string]any{"name": "Acme widget buyers 2", "join_trigger": json.RawMessage(`{"kind":"skus","skus":["WIDGET "]}`)})
	keeps("brand round-trip", map[string]any{"name": "Acme widget buyers 2", "brand": "acme"})

	turnsOff("trigger change", map[string]any{"name": "Acme widget buyers 2", "join_trigger": json.RawMessage(`{"kind":"skus","skus":["other"]}`)}, "")
	turnsOff("brand-only change", map[string]any{"name": "Acme widget buyers 2", "brand": "beta"}, "")
	turnsOff("transactional change", map[string]any{"name": "Acme widget buyers 2", "transactional": true}, "admin")
	turnsOff("trigger cleared", map[string]any{"name": "Acme widget buyers 2", "join_trigger": nil}, "admin")
	if l.juStoredTrigger(l.sku) != "<nil>" || l.active(l.sku).Active {
		t.Fatal("a cleared trigger left the list Active")
	}
}
