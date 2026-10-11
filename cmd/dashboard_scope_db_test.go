package main

// Fork (global brand) -- integrations GLOBAL-BRAND-SPEC I1 against a real database, through the
// handlers: under a list_id a blanket-access, campaigns:get_all user's Dashboard counts its
// campaigns as GET /api/campaigns?list_id= does over the same ids (allCampaigns is false), its
// subscribers and lists as the scope's, and every read is scoped; a scope outside a list-scoped
// user's permission, or an empty one, is the zero shape. With no list_id the blanket-access
// Dashboard is still the materialized views (I7). Shares newLinkHarness (LISTMONK_TEST_PG opt-in).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/paginator"
)

func TestDashboardListScopeDB(t *testing.T) {
	h := newLinkHarness(t)
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})
	f := newBAFixture(h)
	listC := h.baList("C", "private", "single")
	h.baCamp("Unlisted", "finished", 100)
	h.baCamp("On C", "cancelled", 1000, listC)

	h.baSub("a@x", "enabled", map[int]string{f.listA: "confirmed"})
	h.baSub("ab@x", "enabled", map[int]string{f.listA: "confirmed", f.listB: "unconfirmed"})
	h.baSub("b@x", "enabled", map[int]string{f.listB: "confirmed"})
	h.baSub("blockb@x", "blocklisted", map[int]string{f.listB: "unsubscribed"})
	h.baSub("c@x", "enabled", map[int]string{listC: "confirmed"})

	admin := baUser([]string{auth.PermListGetAll, auth.PermCampaignsGetAll, auth.PermCampaignsGet})

	// campaignsTotal is GET /api/campaigns?list_id=... 's total for the same user.
	campaignsTotal := func(ids ...int) int {
		t.Helper()
		q := "/api/campaigns?per_page=all"
		for _, id := range ids {
			q += fmt.Sprintf("&list_id=%d", id)
		}
		code, body := baCall(t, admin, q, h.app.GetCampaigns)
		if code != http.StatusOK {
			t.Fatalf("GetCampaigns: %d %s", code, body)
		}
		var resp struct {
			Data struct {
				Total int `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("GetCampaigns: %s: %v", body, err)
		}
		return resp.Data.Total
	}

	counts := func(u auth.User, q string) map[string]any {
		t.Helper()
		code, body := baCall(t, u, "/api/dashboard/counts"+q, h.app.GetDashboardCounts)
		if code != http.StatusOK {
			t.Fatalf("counts %s: %d %s", q, code, body)
		}
		var resp struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("counts %s: %s: %v", q, body, err)
		}
		return resp.Data
	}
	num := func(m map[string]any, k1, k2 string) int {
		return int(m[k1].(map[string]any)[k2].(float64))
	}

	// list_id = B: B's campaigns only (A+B, Bravo, Summer Sale), never every campaign -- get_all
	// does not widen a brand. Equal to the campaigns listing over the same ids.
	onB := counts(admin, fmt.Sprintf("?list_id=%d", f.listB))
	if onB["scoped"] != true {
		t.Fatalf("list_id=B: not scoped: %s", baJSON(onB))
	}
	if got, want := num(onB, "campaigns", "total"), campaignsTotal(f.listB); got != want || got != 3 {
		t.Fatalf("list_id=B: campaigns.total %d, campaigns listing %d, want both 3", got, want)
	}
	// B's subscribers in any subscription status (ab, b, blockb), one blocklisted; one list.
	if got := num(onB, "subscribers", "total"); got != 3 {
		t.Fatalf("list_id=B: subscribers.total %d, want 3", got)
	}
	if got := num(onB, "subscribers", "blocklisted"); got != 1 {
		t.Fatalf("list_id=B: subscribers.blocklisted %d, want 1", got)
	}
	if got := num(onB, "lists", "total"); got != 1 {
		t.Fatalf("list_id=B: lists.total %d, want 1", got)
	}

	// list_id = A and C together.
	onAC := counts(admin, fmt.Sprintf("?list_id=%d&list_id=%d", f.listA, listC))
	if got, want := num(onAC, "campaigns", "total"), campaignsTotal(f.listA, listC); got != want || got != 4 {
		t.Fatalf("list_id=A,C: campaigns.total %d, campaigns listing %d, want both 4", got, want)
	}

	// An empty set: a list no campaign or subscriber is on, and a list-scoped user's list_id
	// outside their permission -- the exact zero shape, scoped.
	wantZero := `{"campaigns":{"by_status":{},"total":0},"lists":{"optin_double":0,"optin_single":0,"private":0,"public":0,"total":0},"messages":0,"scoped":true,"subscribers":{"blocklisted":0,"orphans":0,"total":0}}`
	if got := baJSON(counts(admin, "?list_id=999999")); got != wantZero {
		t.Fatalf("list_id=999999\n got  %s\n want %s", got, wantZero)
	}
	scopedA := baUser([]string{auth.PermCampaignsGetAnalytics, auth.PermCampaignsGetAll}, f.listA)
	if got := baJSON(counts(scopedA, fmt.Sprintf("?list_id=%d", f.listB))); got != wantZero {
		t.Fatalf("scoped on A with list_id=B\n got  %s\n want %s", got, wantZero)
	}

	// The charts are scoped under a list_id too.
	code, body := baCall(t, admin, fmt.Sprintf("/api/dashboard/charts?list_id=%d", f.listB), h.app.GetDashboardCharts)
	if code != http.StatusOK {
		t.Fatalf("charts list_id=B: %d %s", code, body)
	}
	var charts struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &charts); err != nil || charts.Data["scoped"] != true {
		t.Fatalf("charts list_id=B: not scoped: %s (%v)", body, err)
	}

	// I7: no list_id -- the materialized view, no scoped key.
	got := counts(admin, "")
	if want := matView(t, h, "mat_dashboard_counts"); !reflect.DeepEqual(got, want) {
		t.Fatalf("no list_id\n got  %s\n want %s", baJSON(got), baJSON(want))
	}
	if _, ok := got["scoped"]; ok {
		t.Fatal("no list_id: counts carry a scoped key")
	}

	// A malformed list_id is a 400, never the unscoped numbers.
	if code, body := baCall(t, admin, "/api/dashboard/counts?list_id=x", h.app.GetDashboardCounts); code != http.StatusBadRequest {
		t.Fatalf("list_id=x: %d %s, want 400", code, body)
	}
}
