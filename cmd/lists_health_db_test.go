package main

// Fork (brands UX) -- integrations BRANDS-UX-SPEC I6 (fork half): GET /api/lists carries the
// health row's facts.logoUrl as health.logo_url -- the latest DEFAULT row's for an untagged list,
// JSON null when the document has no logoUrl (or no facts at all). Shares newLinkHarness
// (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go). Fixture names are example.test only.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/paginator"
	"github.com/labstack/echo/v4"
)

func TestListsHealthLogo(t *testing.T) {
	h := newLinkHarness(t)
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})
	db := h.db

	var untagged, tagged, bare int
	db.Get(&untagged, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'untagged', 'private', 'single') RETURNING id`)
	db.Get(&tagged, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'tagged', 'private', 'single', '{brand:acme}') RETURNING id`)
	db.Get(&bare, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'bare', 'private', 'single', '{brand:bare}') RETURNING id`)
	if untagged == 0 || tagged == 0 || bare == 0 {
		t.Fatal("lists not created")
	}

	ins := func(brand, day string, isDefault bool, doc string) {
		t.Helper()
		db.MustExec(`INSERT INTO brand_health (brand, day, status, is_default, doc) VALUES ($1, $2, 'ok', $3, $4::JSONB)
			ON CONFLICT (brand, day) DO UPDATE SET doc = EXCLUDED.doc`, brand, day, isDefault, doc)
	}
	// Two default rows: the NEWEST one's mark is the one an untagged list carries.
	ins("dflt", "2026-09-21", true, `{"v":1,"facts":{"logoUrl":"https://media.example.test/old.jpg"}}`)
	ins("dflt", "2026-09-22", true, `{"v":1,"facts":{"displayName":"Default","logoUrl":"https://media.example.test/logo.jpg"}}`)
	// A tagged brand whose facts carry no logoUrl, and one with facts null (an unmapped brand).
	ins("acme", "2026-09-22", false, `{"v":1,"facts":{"displayName":"Acme"}}`)
	ins("bare", "2026-09-22", false, `{"v":1,"facts":null}`)

	get := func() map[int]map[string]any {
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
				Results []struct {
					ID     int             `json:"id"`
					Health json.RawMessage `json:"health"`
				} `json:"results"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v: %s", err, rec.Body.String())
		}
		out := map[int]map[string]any{}
		for _, r := range resp.Data.Results {
			var hm map[string]any
			if err := json.Unmarshal(r.Health, &hm); err != nil {
				t.Fatalf("list %d health %s: %v", r.ID, r.Health, err)
			}
			out[r.ID] = hm
		}
		return out
	}

	lists := get()
	u := lists[untagged]
	if u == nil || u["default"] != true || u["brand"] != "dflt" {
		t.Fatalf("untagged list must carry the default row: %v", u)
	}
	if u["logo_url"] != "https://media.example.test/logo.jpg" {
		t.Fatalf("untagged logo_url = %v, want the newest default row's facts.logoUrl", u["logo_url"])
	}
	for _, id := range []int{tagged, bare} {
		hm := lists[id]
		v, ok := hm["logo_url"]
		if hm == nil || !ok || v != nil {
			t.Fatalf("list %d: logo_url must be present and JSON null when the document has none: %v", id, hm)
		}
	}

	// The default document loses its mark: the untagged list's logo_url is null again.
	ins("dflt", "2026-09-22", true, `{"v":1,"facts":{"displayName":"Default"}}`)
	if v, ok := get()[untagged]["logo_url"]; !ok || v != nil {
		t.Fatalf("untagged logo_url after the field is cleared = %v (present %v), want null", v, ok)
	}
}
