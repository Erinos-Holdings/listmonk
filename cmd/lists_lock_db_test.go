package main

// Fork (brand picker) -- integrations BRAND-PICKER-SPEC I9 through the real router (bpFixture,
// brands_db_test.go): the render catalog list (models.LockedListNames, the constant the
// integrations repo's catalog-list-name test pins) refuses update and delete -- single, bulk by
// id, bulk by query -- with 409; it may be created once and not while one exists; a rename onto
// the name is refused while it exists; and the lock is evaluated AFTER the permission check, so a
// caller without access gets the permission error. LISTMONK_TEST_PG opt-in.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
)

func TestListLocked(t *testing.T) {
	f := newBPFixture(t)
	locked := models.LockedListNames[0]
	if locked != "Render catalog (never send)" {
		t.Fatalf("fixture: LockedListNames[0] = %q", locked)
	}
	lists := func() int { return f.count(`SELECT COUNT(*) FROM lists`) }

	// Created once (untagged, as the sync script creates it), then refused while it exists.
	var l listResp
	json.Unmarshal(f.ok("create locked", f.admin.json(http.MethodPost, "/api/lists",
		map[string]any{"name": locked, "type": "private", "optin": "single", "tags": []string{}, "brand": ""})), &l)
	cat := l.ID
	n := lists()
	f.wantErr("second copy", f.admin.json(http.MethodPost, "/api/lists", map[string]any{"name": locked, "brand": ""}),
		http.StatusConflict, "lists.lockedList", "name", locked)
	if lists() != n {
		t.Fatal("a second locked list was created")
	}

	other := f.sqlList("Other list")
	spare := f.sqlList("Spare list")

	// Update and single delete.
	f.wantErr("update", f.admin.json(http.MethodPut, "/api/lists/"+itoa(cat), map[string]any{"name": "Renamed catalog"}),
		http.StatusConflict, "lists.lockedList", "name", locked)
	f.wantErr("delete", f.admin.json(http.MethodDelete, "/api/lists/"+itoa(cat), nil),
		http.StatusConflict, "lists.lockedList", "name", locked)

	// Renaming another list onto the locked name, while it exists.
	f.wantErr("rename onto", f.admin.json(http.MethodPut, "/api/lists/"+itoa(other), map[string]any{"name": locked}),
		http.StatusConflict, "lists.lockedList", "name", locked)

	// Bulk by id: refused whole -- the other list survives.
	f.wantErr("bulk by id", f.admin.json(http.MethodDelete, "/api/lists?id="+itoa(other)+"&id="+itoa(cat), nil),
		http.StatusConflict, "lists.lockedList", "name", locked)

	// Bulk by query: all=true, and a query matching the catalog list, are refused whole.
	f.wantErr("bulk all", f.admin.json(http.MethodDelete, "/api/lists?all=true", nil),
		http.StatusConflict, "lists.lockedList", "name", locked)
	f.wantErr("bulk query", f.admin.json(http.MethodDelete, "/api/lists?query="+url.QueryEscape("catalog"), nil),
		http.StatusConflict, "lists.lockedList", "name", locked)
	if lists() != n+2 || f.count(`SELECT COUNT(*) FROM lists WHERE name = $1`, locked) != 1 {
		t.Fatalf("a refused delete removed lists: %d, want %d", lists(), n+2)
	}

	// A by-query delete that does not match the catalog list still works, and removes only its match.
	f.ok("query without the catalog", f.admin.json(http.MethodDelete, "/api/lists?query="+url.QueryEscape("Spare"), nil))
	if f.count(`SELECT COUNT(*) FROM lists WHERE id = $1`, spare) != 0 || lists() != n+1 {
		t.Fatalf("by-query delete of Spare: spare left %d, lists %d", f.count(`SELECT COUNT(*) FROM lists WHERE id = $1`, spare), lists())
	}
	// And an ordinary update and delete of an unlocked list are unaffected.
	f.ok("update other", f.admin.json(http.MethodPut, "/api/lists/"+itoa(other), map[string]any{"name": "Other renamed"}))
	f.ok("delete other", f.admin.json(http.MethodDelete, "/api/lists/"+itoa(other), nil))

	// Permission first: a per-list manager of another list gets the permission error on the
	// locked list, never the lock (which would reveal the list's name).
	mine := f.sqlList("Mine")
	cl := f.perListUser("manager", []string{"campaigns:get"}, []string{auth.PermListGet, auth.PermListManage}, mine)
	for _, c := range []struct{ what, method, target string }{
		{"update", http.MethodPut, "/api/lists/" + itoa(cat)},
		{"delete", http.MethodDelete, "/api/lists/" + itoa(cat)},
		{"bulk by id", http.MethodDelete, "/api/lists?id=" + itoa(cat)},
	} {
		rec := cl.json(c.method, c.target, map[string]any{"name": "x"})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("unpermitted %s: %d %s, want 403 (permission before lock)", c.what, rec.Code, rec.Body.String())
		}
	}
	// Their by-query delete sees only their own list, so it is not refused by the lock.
	f.ok("unpermitted bulk all", cl.json(http.MethodDelete, "/api/lists?all=true", nil))
	if f.count(`SELECT COUNT(*) FROM lists WHERE id = $1`, mine) != 0 || f.count(`SELECT COUNT(*) FROM lists WHERE id = $1`, cat) != 1 {
		t.Fatal("the per-list manager's all=true delete must remove only their own list")
	}
	// A list-creating caller without lists:manage_all gets the route's permission error, not the lock.
	if rec := cl.json(http.MethodPost, "/api/lists", map[string]any{"name": locked}); rec.Code != http.StatusForbidden {
		t.Fatalf("unpermitted create of the locked name: %d, want 403", rec.Code)
	}
}
