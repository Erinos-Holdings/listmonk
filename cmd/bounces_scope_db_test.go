package main

// Fork (global brand) -- integrations GLOBAL-BRAND-PHASE2-SPEC I1 (D5) against a real database,
// through GET /api/bounces: with list_id a bounce appears exactly when its subscriber has a
// subscriber_lists row on one of the ids, at any subscription status; with no list_id the rows
// are today's (every bounce, including one whose subscriber is on no list). QueryBounces with a
// nil scope (the subscriber bounces panel) returns every row, and GetBounce (which executes the
// same query) still resolves by id. Shares newLinkHarness (LISTMONK_TEST_PG opt-in). Fixture
// names are example.test only.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/knadh/paginator"
	"github.com/labstack/echo/v4"
)

func TestBouncesListScope(t *testing.T) {
	h := newLinkHarness(t)
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})

	mkList := func(name string) int {
		t.Helper()
		var id int
		if err := h.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), $1, 'private', 'single', '{}') RETURNING id`, name); err != nil {
			t.Fatal(err)
		}
		return id
	}
	mkSub := func(email string, lists map[int]string) int {
		t.Helper()
		var id int
		if err := h.db.Get(&id, `INSERT INTO subscribers (uuid, email, name) VALUES (gen_random_uuid(), $1, 'S') RETURNING id`, email); err != nil {
			t.Fatal(err)
		}
		for l, st := range lists {
			h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, $3::subscription_status)`, id, l, st)
		}
		return id
	}
	mkBounce := func(sub int) int {
		t.Helper()
		var id int
		if err := h.db.Get(&id, `INSERT INTO bounces (subscriber_id, type, source) VALUES ($1, 'hard', 'api') RETURNING id`, sub); err != nil {
			t.Fatal(err)
		}
		return id
	}

	listA := mkList("brand a")
	listB := mkList("brand b")
	confirmed := mkSub("confirmed@example.test", map[int]string{listA: "confirmed"})
	unsub := mkSub("unsub@example.test", map[int]string{listA: "unsubscribed"})
	onB := mkSub("onb@example.test", map[int]string{listB: "confirmed"})
	orphan := mkSub("orphan@example.test", nil)

	bConfirmed := mkBounce(confirmed)
	bUnsub := mkBounce(unsub)
	bOnB := mkBounce(onB)
	bOrphan := mkBounce(orphan)

	type page struct {
		ids   []int
		total int
	}
	get := func(q string) page {
		t.Helper()
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/bounces?per_page=all"+q, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := h.app.GetBounces(c); err != nil {
			t.Fatalf("GetBounces %s: %v", q, err)
		}
		var resp struct {
			Data struct {
				Results []struct {
					ID int `json:"id"`
				} `json:"results"`
				Total int `json:"total"`
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
		return page{ids: ids, total: resp.Data.Total}
	}

	a := "&list_id=" + strconv.Itoa(listA)
	b := "&list_id=" + strconv.Itoa(listB)
	cases := []struct {
		name string
		q    string
		want []int
	}{
		{"no list_id is today's rows", "", sortedInts(bConfirmed, bUnsub, bOnB, bOrphan)},
		{"list_id=A, any status", a, sortedInts(bConfirmed, bUnsub)},
		{"list_id=B", b, []int{bOnB}},
		{"list_id=A&list_id=B", a + b, sortedInts(bConfirmed, bUnsub, bOnB)},
		{"list_id composes with source", a + "&source=webhook", []int{}},
	}
	for _, tc := range cases {
		got := get(tc.q)
		if !reflect.DeepEqual(got.ids, tc.want) {
			t.Errorf("%s: ids %v, want %v", tc.name, got.ids, tc.want)
		}
		if len(tc.want) > 0 && got.total != len(tc.want) {
			t.Errorf("%s: total %d, want %d", tc.name, got.total, len(tc.want))
		}
	}

	// An invalid list_id is a 400, never an unscoped read.
	{
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/bounces?list_id=x", nil)
		c := e.NewContext(req, httptest.NewRecorder())
		if err := h.app.GetBounces(c); err == nil {
			t.Errorf("list_id=x: want an error")
		}
	}

	// QueryBounces with a nil scope (GetSubscriberBounces passes nil): pq.Array(nil) would be SQL
	// NULL and return nothing; it is normalised to an empty array, so every row comes back.
	all, total, err := h.app.core.QueryBounces(0, 0, "", "", "", 0, 1000, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 || total != 4 {
		t.Errorf("QueryBounces(nil): %d rows, total %d, want 4", len(all), total)
	}
	sub, _, err := h.app.core.QueryBounces(0, unsub, "", "", "", 0, 1000, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sub) != 1 || sub[0].ID != bUnsub {
		t.Errorf("QueryBounces(sub=%d, nil): %+v, want bounce %d", unsub, sub, bUnsub)
	}

	// GetBounce by id still resolves, the orphan included (the scope never applies to a record).
	for _, id := range []int{bConfirmed, bOrphan} {
		got, err := h.app.core.GetBounce(id)
		if err != nil {
			t.Fatalf("GetBounce(%d): %v", id, err)
		}
		if got.ID != id {
			t.Errorf("GetBounce(%d): id %d", id, got.ID)
		}
	}
	{
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/bounces/"+strconv.Itoa(bOrphan), nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("id", bOrphan) // what the hasID middleware stores
		if err := h.app.GetBounce(c); err != nil {
			t.Fatalf("GET /api/bounces/:id: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("GET /api/bounces/:id: %d", rec.Code)
		}
	}
}
