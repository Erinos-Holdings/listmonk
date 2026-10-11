package main

// Fork (global brand) -- integrations GLOBAL-BRAND-SPEC I11 against a real database, through
// GET /api/lists: nobrand=true returns exactly the lists with no brand tag (list_brand_tag NULL --
// no tags, or free tags only), tag=brand:<slug>
// exactly that brand's lists, and neither widens the permission scoping. Shares newLinkHarness
// (LISTMONK_TEST_PG opt-in). Fixture names are example.test only.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/paginator"
	"github.com/labstack/echo/v4"
)

func TestListsNoBrand(t *testing.T) {
	h := newLinkHarness(t)
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})

	mk := func(name, tags string) int {
		t.Helper()
		var id int
		if err := h.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), $1, 'private', 'single', $2::VARCHAR(100)[]) RETURNING id`, name, tags); err != nil {
			t.Fatal(err)
		}
		return id
	}
	none := mk("no tags", "{}")
	free := mk("free tags", "{holiday,repermission:3}")
	acme := mk("acme", "{brand:acme,site:https://acme.example.test}")
	acme2 := mk("acme two", "{brand:acme,other}")
	beta := mk("beta", "{brand:beta}")

	get := func(u auth.User, q string) []int {
		t.Helper()
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/lists?per_page=all"+q, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(auth.UserHTTPCtxKey, u)
		if err := h.app.GetLists(c); err != nil {
			t.Fatalf("GetLists %s: %v", q, err)
		}
		var resp struct {
			Data struct {
				Results []struct {
					ID    int    `json:"id"`
					Brand string `json:"brand"`
				} `json:"results"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode %s: %v: %s", q, err, rec.Body.String())
		}
		ids := []int{}
		for _, r := range resp.Data.Results {
			ids = append(ids, r.ID)
		}
		sort.Ints(ids)
		return ids
	}

	all := auth.User{PermissionsMap: map[string]struct{}{auth.PermListGetAll: {}}}
	cases := []struct {
		name string
		u    auth.User
		q    string
		want []int
	}{
		{"no filter", all, "", sortedInts(none, free, acme, acme2, beta)},
		{"nobrand=true", all, "&nobrand=true", sortedInts(none, free)},
		{"nobrand=false is no filter", all, "&nobrand=false", sortedInts(none, free, acme, acme2, beta)},
		{"tag=brand:acme", all, "&tag=brand:acme", sortedInts(acme, acme2)},
		{"tag=brand:beta", all, "&tag=brand:beta", []int{beta}},
		{"tag=brand:ac is exact, not a prefix", all, "&tag=brand:ac", []int{}},
		// Both compose with the permission scoping and never widen it.
		{"per-list user, nobrand", baUser(nil, free, acme), "&nobrand=true", []int{free}},
		{"per-list user, tag=brand:beta", baUser(nil, free, acme), "&tag=brand:beta", []int{}},
	}
	for _, tc := range cases {
		if got := get(tc.u, tc.q); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: ids %v, want %v", tc.name, got, tc.want)
		}
	}
}
