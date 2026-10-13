package main

// Fork (sunset v2) -- integrations SUNSET-V2-SPEC D8 / §4.4: PUT /api/lists/:id/sunset stores the
// queue countdown verbatim after a shape check (list manage permission), and GET /api/lists
// carries it as `sunset` -- absent on every list that has none. Shares newLinkHarness
// (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go). Fixture names are example.test only.

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

func TestListSunsetEndpoint(t *testing.T) {
	h := newLinkHarness(t)
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})
	db := h.db

	var queue, plain int
	db.Get(&queue, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme re-engagement 2', 'private', 'single', '{reengage:1:2}') RETURNING id`)
	db.Get(&plain, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'single') RETURNING id`)
	var updatedBefore string
	db.Get(&updatedBefore, `SELECT updated_at::TEXT FROM lists WHERE id = $1`, queue)

	manageAll := auth.User{PermissionsMap: map[string]struct{}{auth.PermListGetAll: {}, auth.PermListManageAll: {}}}
	put := func(id int, body string, user auth.User) int {
		t.Helper()
		e := echo.New()
		req := httptest.NewRequest(http.MethodPut, "/api/lists/"+itoa(id)+"/sunset", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("id", id)
		c.Set(auth.UserHTTPCtxKey, user)
		if err := h.app.PutListSunset(c); err != nil {
			if he, ok := err.(*echo.HTTPError); ok {
				return he.Code
			}
			t.Fatalf("PutListSunset: %v", err)
		}
		return rec.Code
	}
	get := func() map[int]json.RawMessage {
		t.Helper()
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/lists?per_page=all", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(auth.UserHTTPCtxKey, auth.User{PermissionsMap: map[string]struct{}{auth.PermListGetAll: {}}})
		if err := h.app.GetLists(c); err != nil {
			t.Fatalf("GetLists: %v", err)
		}
		var resp struct {
			Data struct {
				Results []map[string]json.RawMessage `json:"results"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		out := map[int]json.RawMessage{}
		for _, r := range resp.Data.Results {
			var id int
			json.Unmarshal(r["id"], &id)
			out[id] = r["sunset"]
		}
		return out
	}

	// Nothing stored: the key is absent on every list.
	if s := get(); s[queue] != nil || s[plain] != nil {
		t.Fatalf("sunset present before any write: %s / %s", s[queue], s[plain])
	}

	doc := `{"oldestDays":12,"archiveInDays":65,"asOf":"2026-11-01T15:00:00Z"}`
	if code := put(queue, doc, manageAll); code != http.StatusOK {
		t.Fatalf("valid PUT: %d", code)
	}
	var stored map[string]any
	if err := json.Unmarshal(get()[queue], &stored); err != nil {
		t.Fatal(err)
	}
	if stored["oldestDays"] != float64(12) || stored["archiveInDays"] != float64(65) || stored["asOf"] != "2026-11-01T15:00:00Z" {
		t.Fatalf("stored %v", stored)
	}
	if get()[plain] != nil {
		t.Fatal("another list gained a sunset")
	}
	// Queue 1's document: archiveInDays null is valid and stored as null.
	if code := put(queue, `{"oldestDays":0,"archiveInDays":null,"asOf":"2026-11-02T15:00:00Z"}`, manageAll); code != http.StatusOK {
		t.Fatalf("null archiveInDays: %d", code)
	}
	if !strings.Contains(string(get()[queue]), `"archiveInDays": null`) && !strings.Contains(string(get()[queue]), `"archiveInDays":null`) {
		t.Fatalf("archiveInDays not null: %s", get()[queue])
	}
	// A countdown write is not an edit of the list.
	var updatedAfter string
	db.Get(&updatedAfter, `SELECT updated_at::TEXT FROM lists WHERE id = $1`, queue)
	if updatedAfter != updatedBefore {
		t.Fatalf("updated_at moved: %s -> %s", updatedBefore, updatedAfter)
	}

	// The shape check refuses everything else, and leaves the stored document alone.
	before := string(get()[queue])
	for name, body := range map[string]string{
		"not json":          `nope`,
		"array":             `[1]`,
		"no oldestDays":     `{"archiveInDays":3,"asOf":"2026-11-01T15:00:00Z"}`,
		"negative":          `{"oldestDays":-1,"asOf":"2026-11-01T15:00:00Z"}`,
		"fraction":          `{"oldestDays":1.5,"asOf":"2026-11-01T15:00:00Z"}`,
		"string days":       `{"oldestDays":"3","asOf":"2026-11-01T15:00:00Z"}`,
		"bad archive":       `{"oldestDays":3,"archiveInDays":"x","asOf":"2026-11-01T15:00:00Z"}`,
		"no asOf":           `{"oldestDays":3}`,
		"bad asOf":          `{"oldestDays":3,"asOf":"yesterday"}`,
		"unknown key":       `{"oldestDays":3,"asOf":"2026-11-01T15:00:00Z","note":"x"}`,
		"two objects":       `{"oldestDays":3,"asOf":"2026-11-01T15:00:00Z"}{}`,
	} {
		if code := put(queue, body, manageAll); code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d", name, code)
		}
	}
	if string(get()[queue]) != before {
		t.Fatal("a refused write changed the stored document")
	}

	// Permission: list manage on THIS list (a get-only user is refused); a missing list is a 404.
	if code := put(queue, doc, auth.User{PermissionsMap: map[string]struct{}{auth.PermListGetAll: {}}}); code != http.StatusForbidden {
		t.Fatalf("get-only user: want 403, got %d", code)
	}
	if code := put(999999, doc, manageAll); code != http.StatusNotFound {
		t.Fatalf("missing list: want 404, got %d", code)
	}
}
