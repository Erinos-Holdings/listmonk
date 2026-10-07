package main

// Fork (brand analytics) -- integrations BRAND-ANALYTICS-SPEC I1-I6 against a real database,
// through the handlers: the Campaign Analytics picker (GET /api/analytics/campaigns) is scoped by
// the ANY-list rule and gated on campaigns:get_analytics through the same a.auth.Perm wrapper
// handlers.go uses; the analytics endpoint stays all-or-nothing over several ids; the Dashboard
// counts and charts are live and scoped for list-scoped users, byte-for-byte the materialized views
// for everyone else, and null-free. Shares newLinkHarness (LISTMONK_TEST_PG opt-in; see
// link_redirect_db_test.go).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// baUser builds a request user: user-role permissions plus per-list list:get on getLists.
func baUser(perms []string, getLists ...int) auth.User {
	u := auth.User{PermissionsMap: map[string]struct{}{}, ListPermissionsMap: map[int]map[string]struct{}{}}
	for _, p := range perms {
		u.PermissionsMap[p] = struct{}{}
	}
	for _, id := range getLists {
		u.GetListIDs = append(u.GetListIDs, id)
		u.ListPermissionsMap[id] = map[string]struct{}{auth.PermListGet: {}}
	}
	return u
}

// baCall runs handler as u and returns the status and body (an HTTPError's message as the body).
func baCall(t *testing.T, u auth.User, path string, handler echo.HandlerFunc, params ...string) (int, string) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if len(params) > 0 {
		c.SetParamNames(params[0])
		c.SetParamValues(params[1])
	}
	c.Set(auth.UserHTTPCtxKey, u)
	if err := handler(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("%s: %v", path, err)
	}
	return rec.Code, rec.Body.String()
}

type baFixture struct {
	listA, listB                 int
	campA, campAB, campB, draftA int
	summerB                      int
}

func (h *linkHarness) baList(name, typ, optin string) int {
	var id int
	if err := h.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), $1, $2, $3, '{}') RETURNING id`, name, typ, optin); err != nil {
		h.t.Fatal(err)
	}
	return id
}

// baCamp inserts a campaign on lists with the given status and sent count. Any status but draft
// and scheduled gets a started_at.
func (h *linkHarness) baCamp(name, status string, sent int, lists ...int) int {
	var id int
	if err := h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id, status, sent, started_at)
		VALUES (gen_random_uuid(), $1, 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1), $2::campaign_status, $3,
		CASE WHEN $2 IN ('draft', 'scheduled') THEN NULL ELSE NOW() END) RETURNING id`, name, status, sent); err != nil {
		h.t.Fatal(err)
	}
	for _, l := range lists {
		h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'x')`, id, l)
	}
	return id
}

func (h *linkHarness) baSub(email, status string, lists map[int]string) int {
	var id int
	if err := h.db.Get(&id, `INSERT INTO subscribers (uuid, email, name, status) VALUES (gen_random_uuid(), $1, 'S', $2::subscriber_status) RETURNING id`, email, status); err != nil {
		h.t.Fatal(err)
	}
	for l, st := range lists {
		h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, $3::subscription_status)`, id, l, st)
	}
	return id
}

func newBAFixture(h *linkHarness) baFixture {
	var f baFixture
	f.listA = h.baList("A", "public", "single")
	f.listB = h.baList("B", "private", "double")
	f.campA = h.baCamp("Alpha", "finished", 10, f.listA)
	f.campAB = h.baCamp("Both", "running", 5, f.listA, f.listB)
	f.campB = h.baCamp("Bravo", "paused", 7, f.listB)
	f.draftA = h.baCamp("Draft on A", "draft", 0, f.listA)
	f.summerB = h.baCamp("Summer Sale", "finished", 3, f.listB)
	return f
}

// pickerReq calls GET /api/analytics/campaigns wrapped in a.auth.Perm exactly as handlers.go
// registers it -- calling the handler bare would skip the permission gate.
func pickerReq(t *testing.T, h *linkHarness, u auth.User, q url.Values) (int, string) {
	t.Helper()
	return baCall(t, u, "/api/analytics/campaigns?"+q.Encode(), h.app.auth.Perm(h.app.GetAnalyticsCampaigns, "campaigns:get_analytics"))
}

// pickerIDs returns the ids of the picker's rows, sorted, and fails on a non-200.
func pickerIDs(t *testing.T, h *linkHarness, name string, u auth.User, q url.Values) []int {
	t.Helper()
	code, body := pickerReq(t, h, u, q)
	if code != http.StatusOK {
		t.Fatalf("%s: %d %s", name, code, body)
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("%s: %s: %v", name, body, err)
	}
	if resp.Data == nil {
		t.Fatalf("%s: data is null, want an array: %s", name, body)
	}
	ids := []int{}
	for _, r := range resp.Data {
		ids = append(ids, int(r["id"].(float64)))
	}
	sort.Ints(ids)
	return ids
}

func sortedInts(in ...int) []int {
	out := append([]int{}, in...)
	sort.Ints(out)
	return out
}

// TestBrandAnalyticsPicker is I1 and I2.
func TestBrandAnalyticsPicker(t *testing.T) {
	h := newLinkHarness(t)
	h.app.auth = &auth.Auth{}
	f := newBAFixture(h)

	analyticsA := baUser([]string{auth.PermCampaignsGetAnalytics}, f.listA)
	campaignsGetAll := baUser([]string{auth.PermCampaignsGetAnalytics, auth.PermCampaignsGetAll}, f.listA)
	listsGetAll := baUser([]string{auth.PermCampaignsGetAnalytics, auth.PermCampaignsGet, auth.PermListGetAll})
	listsGetAllAnalyticsOnly := baUser([]string{auth.PermCampaignsGetAnalytics, auth.PermListGetAll})
	noLists := baUser([]string{auth.PermCampaignsGetAnalytics})
	noAnalytics := baUser([]string{auth.PermCampaignsGet, auth.PermCampaignsGetAll, auth.PermListGetAll})
	superAdmin := baUser(nil)
	superAdmin.UserRoleID = auth.SuperAdminRoleID
	superAdmin.UserRole.ID = auth.SuperAdminRoleID
	// Super Admin holding only get_analytics is still never analytics-only (sees drafts).
	superAdminAnalytics := baUser([]string{auth.PermCampaignsGetAnalytics})
	superAdminAnalytics.UserRoleID = auth.SuperAdminRoleID
	superAdminAnalytics.UserRole.ID = auth.SuperAdminRoleID

	all := sortedInts(f.campA, f.campAB, f.campB, f.draftA, f.summerB)
	startedAll := sortedInts(f.campA, f.campAB, f.campB, f.summerB)
	cases := []struct {
		name string
		u    auth.User
		q    url.Values
		want []int
	}{
		// D2: A and A+B, never B-only; D11: no draft for an analytics-only user.
		{"analytics-only on A", analyticsA, url.Values{}, sortedInts(f.campA, f.campAB)},
		{"campaigns:get_all sees every status", campaignsGetAll, url.Values{}, all},
		{"lists:get_all without campaigns:get_all sees all", listsGetAll, url.Values{}, all},
		{"lists:get_all analytics-only sees all started", listsGetAllAnalyticsOnly, url.Values{}, startedAll},
		{"super admin sees all", superAdmin, url.Values{}, all},
		{"super admin with only get_analytics sees all", superAdminAnalytics, url.Values{}, all},
		{"scoped user with no lists sees none", noLists, url.Values{}, []int{}},
		// id= : exactly the permitted subset, unknown and unpermitted dropped silently, query and
		// per_page ignored.
		{"id= drops unpermitted/unknown, ignores query/per_page", analyticsA,
			url.Values{"id": {fmt.Sprint(f.campA), fmt.Sprint(f.campAB), fmt.Sprint(f.campB), fmt.Sprint(f.draftA), "999999"},
				"query": {"zzzz-nomatch"}, "per_page": {"1"}},
			sortedInts(f.campA, f.campAB)},
		{"id= for get_all", campaignsGetAll, url.Values{"id": {fmt.Sprint(f.campB), fmt.Sprint(f.draftA)}}, sortedInts(f.campB, f.draftA)},
		{"id= only unpermitted", analyticsA, url.Values{"id": {fmt.Sprint(f.campB)}}, []int{}},
		// query: name match and exact numeric id.
		{"query matches name", campaignsGetAll, url.Values{"query": {"summer"}}, []int{f.summerB}},
		{"query matches numeric id", campaignsGetAll, url.Values{"query": {fmt.Sprint(f.campB)}}, []int{f.campB}},
		{"numeric id outside scope", analyticsA, url.Values{"query": {fmt.Sprint(f.campB)}}, []int{}},
		{"numeric id inside scope", analyticsA, url.Values{"query": {fmt.Sprint(f.campAB)}}, []int{f.campAB}},
		{"name outside scope", analyticsA, url.Values{"query": {"summer"}}, []int{}},
		// Typed per keystroke: a number beyond int32 and tsquery syntax search as text, never 500.
		{"numeric query beyond int32", analyticsA, url.Values{"query": {"99999999999"}}, []int{}},
		{"tsquery syntax characters", analyticsA, url.Values{"query": {"a:b) !"}}, []int{}},
	}
	for _, tc := range cases {
		if got := pickerIDs(t, h, tc.name, tc.u, tc.q); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: ids %v, want %v", tc.name, got, tc.want)
		}
	}

	// No get_analytics -> 403 through the Perm wrapper, whatever else the user holds.
	if code, body := pickerReq(t, h, noAnalytics, url.Values{}); code != http.StatusForbidden {
		t.Fatalf("no get_analytics: %d %s, want 403", code, body)
	}

	// I2: exactly the D3 key set, on every row, in both modes.
	for _, q := range []url.Values{{}, {"id": {fmt.Sprint(f.campA)}}} {
		_, body := pickerReq(t, h, campaignsGetAll, q)
		var resp struct {
			Data []map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil || len(resp.Data) == 0 {
			t.Fatalf("I2 %v: %s (%v)", q, body, err)
		}
		want := []string{"created_at", "evergreen", "id", "name", "sent", "started_at", "status"}
		for _, r := range resp.Data {
			keys := []string{}
			for k := range r {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			if !reflect.DeepEqual(keys, want) {
				t.Fatalf("I2: row keys %v, want %v", keys, want)
			}
		}
	}

	// per_page: default 20, capped at 50.
	for i := 0; i < 55; i++ {
		h.baCamp(fmt.Sprintf("Bulk %d", i), "finished", 0, f.listA)
	}
	for _, tc := range []struct {
		perPage string
		want    int
	}{{"", 20}, {"0", 20}, {"junk", 20}, {"30", 30}, {"50", 50}, {"500", 50}} {
		q := url.Values{}
		if tc.perPage != "" {
			q.Set("per_page", tc.perPage)
		}
		if got := len(pickerIDs(t, h, "per_page "+tc.perPage, analyticsA, q)); got != tc.want {
			t.Errorf("per_page=%q: %d rows, want %d", tc.perPage, got, tc.want)
		}
	}
}

// analyticsMultiReq calls GetCampaignViewAnalytics for several ids.
func analyticsMultiReq(t *testing.T, h *linkHarness, u auth.User, typ string, ids ...int) (int, string) {
	t.Helper()
	q := url.Values{}
	for _, id := range ids {
		q.Add("id", fmt.Sprint(id))
	}
	q.Set("from", time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339))
	q.Set("to", time.Now().UTC().Add(48*time.Hour).Format(time.RFC3339))
	return baCall(t, u, "/api/campaigns/analytics/"+typ+"?"+q.Encode(),
		h.app.auth.Perm(h.app.GetCampaignViewAnalytics, "campaigns:get_analytics"), "type", typ)
}

// TestBrandAnalyticsAggregation is I3: several permitted ids aggregate; one unpermitted id in the
// set refuses the whole request, for every analytics type.
func TestBrandAnalyticsAggregation(t *testing.T) {
	h := newLinkHarness(t)
	h.app.auth = &auth.Auth{}
	f := newBAFixture(h)
	var link int
	h.db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://agg.test') RETURNING id`)
	// One subscriber per campaign, so the unique-mode link count (distinct subscribers per URL)
	// shows the aggregation too.
	for i, c := range []int{f.campA, f.campAB, f.campB} {
		s := h.baSub(fmt.Sprintf("s%d@x", i), "enabled", map[int]string{f.listA: "confirmed"})
		h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id, country) VALUES ($1, $2, 'US')`, c, s)
		h.db.MustExec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, country) VALUES ($1, $2, $3, 'US')`, c, s, link)
		h.db.MustExec(`INSERT INTO bounces (subscriber_id, campaign_id) VALUES ($1, $2)`, s, c)
	}

	member := baUser([]string{auth.PermCampaignsGetAnalytics}, f.listA)
	for _, typ := range []string{"views", "clicks", "bounces", "links", "countries"} {
		// Several permitted ids -> 200, the data of both campaigns.
		code, body := analyticsMultiReq(t, h, member, typ, f.campA, f.campAB)
		if code != http.StatusOK {
			t.Fatalf("%s permitted: %d %s", typ, code, body)
		}
		var resp struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("%s: %s: %v", typ, body, err)
		}
		switch typ {
		case "links":
			if len(resp.Data) != 1 || resp.Data[0]["count"].(float64) != 2 {
				t.Errorf("links aggregated: %s, want one url counted 2", body)
			}
		case "countries":
			if len(resp.Data) != 1 || resp.Data[0]["views"].(float64) != 2 || resp.Data[0]["clicks"].(float64) != 2 {
				t.Errorf("countries aggregated: %s, want US 2 views 2 clicks", body)
			}
		default:
			camps := map[int]float64{}
			for _, r := range resp.Data {
				camps[int(r["campaign_id"].(float64))] += r["count"].(float64)
			}
			if want := map[int]float64{f.campA: 1, f.campAB: 1}; !reflect.DeepEqual(camps, want) {
				t.Errorf("%s aggregated: %v, want %v", typ, camps, want)
			}
		}

		// One unpermitted id anywhere in the set -> 403, no data.
		for _, ids := range [][]int{{f.campA, f.campB}, {f.campB, f.campA, f.campAB}} {
			code, body := analyticsMultiReq(t, h, member, typ, ids...)
			if code != http.StatusForbidden {
				t.Errorf("%s with unpermitted id %v: %d %s, want 403", typ, ids, code, body)
			}
		}
	}

	// Without get_analytics the endpoint is 403 through the wrapper.
	if code, body := analyticsMultiReq(t, h, baUser(nil, f.listA), "views", f.campA); code != http.StatusForbidden {
		t.Fatalf("no get_analytics: %d %s, want 403", code, body)
	}
}

// dashJSON calls a dashboard handler as u and returns the decoded data object.
func dashJSON(t *testing.T, u auth.User, name string, handler echo.HandlerFunc) map[string]any {
	t.Helper()
	code, body := baCall(t, u, "/api/dashboard/"+name, handler)
	if code != http.StatusOK {
		t.Fatalf("dashboard %s: %d %s", name, code, body)
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("dashboard %s: %s: %v", name, body, err)
	}
	return resp.Data
}

func matView(t *testing.T, h *linkHarness, view string) map[string]any {
	t.Helper()
	var raw []byte
	if err := h.db.Get(&raw, `SELECT data::TEXT FROM `+view); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func baJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// TestBrandAnalyticsDashboardCounts is I4 and the counts half of I6.
func TestBrandAnalyticsDashboardCounts(t *testing.T) {
	h := newLinkHarness(t)
	f := newBAFixture(h)
	listC := h.baList("C", "private", "single")
	h.baCamp("Unlisted", "finished", 100)
	h.baCamp("On C", "cancelled", 1000, listC)

	h.baSub("a@x", "enabled", map[int]string{f.listA: "confirmed"})
	h.baSub("ab@x", "enabled", map[int]string{f.listA: "confirmed", f.listB: "unconfirmed"})
	h.baSub("b@x", "enabled", map[int]string{f.listB: "confirmed"})
	h.baSub("unsub@x", "enabled", map[int]string{f.listA: "unsubscribed"})
	h.baSub("block@x", "blocklisted", map[int]string{f.listA: "unsubscribed"})
	h.baSub("blockb@x", "blocklisted", map[int]string{f.listB: "unsubscribed"})
	h.baSub("orphan@x", "enabled", nil)
	h.baSub("c@x", "enabled", map[int]string{listC: "confirmed"})

	counts := h.app.GetDashboardCounts
	get := []string{auth.PermCampaignsGetAnalytics}

	// Scoped on A: A's subscribers in any subscription status (4, one blocklisted); A and A+B
	// campaigns (draft included -- the Dashboard counts statuses).
	onA := dashJSON(t, baUser(get, f.listA), "counts", counts)
	wantA := `{"campaigns":{"by_status":{"draft":1,"finished":1,"running":1},"total":3},"lists":{"optin_double":0,"optin_single":1,"private":0,"public":1,"total":1},"messages":15,"scoped":true,"subscribers":{"blocklisted":1,"orphans":0,"total":4}}`
	if got := baJSON(onA); got != wantA {
		t.Fatalf("scoped on A\n got  %s\n want %s", got, wantA)
	}

	// Scoped on A and B: the A+B subscriber counted once.
	onAB := dashJSON(t, baUser(get, f.listA, f.listB), "counts", counts)
	wantAB := `{"campaigns":{"by_status":{"draft":1,"finished":2,"paused":1,"running":1},"total":5},"lists":{"optin_double":1,"optin_single":1,"private":1,"public":1,"total":2},"messages":25,"scoped":true,"subscribers":{"blocklisted":2,"orphans":0,"total":6}}`
	if got := baJSON(onAB); got != wantAB {
		t.Fatalf("scoped on A+B\n got  %s\n want %s", got, wantAB)
	}

	// campaigns:get_all + list-scoped: every campaign, but only A's lists and subscribers.
	allCamps := dashJSON(t, baUser([]string{auth.PermCampaignsGetAnalytics, auth.PermCampaignsGetAll}, f.listA), "counts", counts)
	wantAll := `{"campaigns":{"by_status":{"cancelled":1,"draft":1,"finished":3,"paused":1,"running":1},"total":7},"lists":{"optin_double":0,"optin_single":1,"private":0,"public":1,"total":1},"messages":1125,"scoped":true,"subscribers":{"blocklisted":1,"orphans":0,"total":4}}`
	if got := baJSON(allCamps); got != wantAll {
		t.Fatalf("get_all + scoped on A\n got  %s\n want %s", got, wantAll)
	}

	// I6 counts: no permitted list -> the exact zero shape, never global numbers.
	none := dashJSON(t, baUser(get), "counts", counts)
	wantNone := `{"campaigns":{"by_status":{},"total":0},"lists":{"optin_double":0,"optin_single":0,"private":0,"public":0,"total":0},"messages":0,"scoped":true,"subscribers":{"blocklisted":0,"orphans":0,"total":0}}`
	if got := baJSON(none); got != wantNone {
		t.Fatalf("no lists\n got  %s\n want %s", got, wantNone)
	}

	// Unscoped (blanket list access, and Super Admin): exactly the materialized view, no scoped key.
	superAdmin := baUser(nil)
	superAdmin.UserRoleID = auth.SuperAdminRoleID
	for name, u := range map[string]auth.User{
		"lists:get_all":    baUser([]string{auth.PermListGetAll, auth.PermCampaignsGetAll}),
		"lists:manage_all": baUser([]string{auth.PermListManageAll}),
		"super admin":      superAdmin,
	} {
		got := dashJSON(t, u, "counts", counts)
		if want := matView(t, h, "mat_dashboard_counts"); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: counts\n got  %s\n want %s", name, baJSON(got), baJSON(want))
		}
		if _, ok := got["scoped"]; ok {
			t.Fatalf("%s: unscoped counts carry a scoped key", name)
		}
		if got["subscribers"].(map[string]any)["total"].(float64) != 8 {
			t.Fatalf("%s: global subscriber total %v, want 8", name, got["subscribers"])
		}
	}
}

// TestBrandAnalyticsDashboardCharts is I5 and the charts half of I6.
func TestBrandAnalyticsDashboardCharts(t *testing.T) {
	h := newLinkHarness(t)
	f := newBAFixture(h)
	var link int
	h.db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://ch.test') RETURNING id`)

	// day d at noon, in the session's time zone (the queries' ::DATE uses the same zone).
	view := func(camp int, day string, n int) {
		for i := 0; i < n; i++ {
			h.db.MustExec(`INSERT INTO campaign_views (campaign_id, created_at) VALUES ($1, ($2::DATE + TIME '12:00')::TIMESTAMP)`, camp, day)
		}
	}
	click := func(camp int, day string, n int) {
		for i := 0; i < n; i++ {
			h.db.MustExec(`INSERT INTO link_clicks (campaign_id, link_id, created_at) VALUES ($1, $2, ($3::DATE + TIME '12:00')::TIMESTAMP)`, camp, link, day)
		}
	}

	// Views of C (A and A+B): latest 2026-06-30 -> window 2026-05-31 .. 2026-06-30 (31 days); day
	// 32 (2026-05-30) is out. Clicks anchor independently on 2026-06-15.
	view(f.campA, "2026-06-30", 1)
	view(f.campAB, "2026-06-30", 2)
	view(f.campA, "2026-06-10", 1)
	view(f.campAB, "2026-05-31", 1)
	view(f.campA, "2026-05-30", 4)
	click(f.campA, "2026-06-15", 1)
	click(f.campAB, "2026-05-16", 2)
	click(f.campA, "2026-05-15", 3)
	// Another brand's campaign: events after C's latest (and inserted last, so the highest ids)
	// must neither count nor move the anchor; so must an older event with a higher id.
	view(f.campB, "2026-07-20", 5)
	click(f.campB, "2026-07-20", 5)
	view(f.campB, "2026-06-01", 1)

	charts := h.app.GetDashboardCharts
	onA := dashJSON(t, baUser([]string{auth.PermCampaignsGetAnalytics}, f.listA), "charts", charts)
	want := `{"campaign_views":[{"count":1,"date":"2026-05-31"},{"count":1,"date":"2026-06-10"},{"count":3,"date":"2026-06-30"}],"link_clicks":[{"count":2,"date":"2026-05-16"},{"count":1,"date":"2026-06-15"}],"scoped":true}`
	if got := baJSON(onA); got != want {
		t.Fatalf("scoped charts on A\n got  %s\n want %s", got, want)
	}

	// I6 charts: no permitted list -> empty arrays, never null, never global.
	none := dashJSON(t, baUser([]string{auth.PermCampaignsGetAnalytics}), "charts", charts)
	if got, want := baJSON(none), `{"campaign_views":[],"link_clicks":[],"scoped":true}`; got != want {
		t.Fatalf("no lists charts\n got  %s\n want %s", got, want)
	}

	// Unscoped: exactly the materialized view, no scoped key.
	got := dashJSON(t, baUser([]string{auth.PermListGetAll}), "charts", charts)
	if want := matView(t, h, "mat_dashboard_charts"); !reflect.DeepEqual(got, want) {
		t.Fatalf("unscoped charts\n got  %s\n want %s", baJSON(got), baJSON(want))
	}
	if _, ok := got["scoped"]; ok {
		t.Fatal("unscoped charts carry a scoped key")
	}
}

// Fork (client stats) -- integrations CLIENT-STATS-SPEC I5 (and D6/D7): GET /api/dashboard/clients.
// A list-scoped user's rollup covers only the campaigns on their permitted lists (get_all widens the
// campaigns, as for the counts), ?brand= is ignored for them; everyone else gets every campaign, or
// one brand's (list brand tag) campaigns with ?brand=; per table, the window is the charts' 31-day
// rule anchored on that set's latest event.
func TestDashboardClients(t *testing.T) {
	h := newLinkHarness(t)
	f := newBAFixture(h)
	h.db.MustExec(`UPDATE lists SET tags = '{brand:alpha}' WHERE id = $1`, f.listA)
	h.db.MustExec(`UPDATE lists SET tags = '{brand:bravo,other}' WHERE id = $1`, f.listB)
	var link int
	h.db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://cl.test') RETURNING id`)

	// client "" = NULL; day d at noon in the session's time zone.
	view := func(camp int, client, day string, n int) {
		for i := 0; i < n; i++ {
			h.db.MustExec(`INSERT INTO campaign_views (campaign_id, client, created_at) VALUES ($1, NULLIF($2, ''), ($3::DATE + TIME '12:00')::TIMESTAMP)`, camp, client, day)
		}
	}
	click := func(camp int, client, day string, n int) {
		for i := 0; i < n; i++ {
			h.db.MustExec(`INSERT INTO link_clicks (campaign_id, link_id, client, created_at) VALUES ($1, $2, NULLIF($3, ''), ($4::DATE + TIME '12:00')::TIMESTAMP)`, camp, link, client, day)
		}
	}

	// On A (A and A+B): views anchor on 2026-06-30 (05-30 is day 32, out), clicks on 2026-06-15
	// (05-15 out). B only: later events that must neither count nor move A's anchor.
	view(f.campA, "apple-mail", "2026-06-30", 2)
	view(f.campAB, "gmail-proxy", "2026-06-30", 1)
	view(f.campA, "", "2026-06-10", 1)
	view(f.campA, "apple-mail", "2026-05-30", 4)
	click(f.campA, "browser-ios", "2026-06-15", 1)
	click(f.campAB, "browser-ios", "2026-05-16", 2)
	click(f.campA, "browser-ios", "2026-05-15", 3)
	view(f.campB, "outlook-windows", "2026-07-20", 5)
	click(f.campB, "browser-windows", "2026-07-20", 5)

	type panel struct {
		Scoped  bool                             `json:"scoped"`
		Brand   string                           `json:"brand"`
		Brands  []string                         `json:"brands"`
		Clients []models.CampaignAnalyticsClient `json:"clients"`
	}
	call := func(name string, u auth.User, q string) panel {
		t.Helper()
		code, body := baCall(t, u, "/api/dashboard/clients"+q, h.app.GetDashboardClients)
		if code != http.StatusOK {
			t.Fatalf("%s: %d %s", name, code, body)
		}
		var resp struct {
			Data panel `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("%s: %s: %v", name, body, err)
		}
		if resp.Data.Clients == nil || resp.Data.Brands == nil {
			t.Fatalf("%s: null array in %s", name, body)
		}
		return resp.Data
	}
	rows := func(p panel) string {
		sort.Slice(p.Clients, func(i, j int) bool { return p.Clients[i].Client < p.Clients[j].Client })
		out := ""
		for _, r := range p.Clients {
			out += fmt.Sprintf("[%s %d %d]", r.Client, r.Views, r.Clicks)
		}
		return out
	}
	get := []string{auth.PermCampaignsGetAnalytics}
	onA := "[ 1 0][apple-mail 2 0][browser-ios 0 3][gmail-proxy 1 0]"

	// Scoped on A: only A's campaigns, A's own anchors; scoped, no brands.
	p := call("scoped on A", baUser(get, f.listA), "")
	if got := rows(p); got != onA || !p.Scoped || len(p.Brands) != 0 || p.Brand != "" {
		t.Fatalf("scoped on A: %s scoped=%v brands=%v brand=%q, want %s scoped, no brands", got, p.Scoped, p.Brands, p.Brand, onA)
	}

	// D7 / review F6: ?brand= is ignored for a list-scoped user -- B's rows never appear.
	p = call("scoped on A, brand=bravo", baUser(get, f.listA), "?brand=bravo")
	if got := rows(p); got != onA || p.Brand != "" {
		t.Fatalf("scoped on A with ?brand=bravo: %s brand=%q, want %s (brand ignored)", got, p.Brand, onA)
	}

	// No permitted list -> no rows, never the global numbers.
	if got := rows(call("no lists", baUser(get), "")); got != "" {
		t.Fatalf("no lists: %s, want no rows", got)
	}

	// campaigns:get_all + list-scoped: every campaign (anchors move to 07-20).
	wantAll := "[apple-mail 2 0][browser-windows 0 5][gmail-proxy 1 0][outlook-windows 5 0]"
	if got := rows(call("get_all scoped", baUser([]string{auth.PermCampaignsGetAnalytics, auth.PermCampaignsGetAll}, f.listA), "")); got != wantAll {
		t.Fatalf("get_all + scoped on A: %s, want %s", got, wantAll)
	}

	// Unscoped: every campaign, the picker's brands.
	admin := baUser([]string{auth.PermListGetAll})
	p = call("unscoped", admin, "")
	if got := rows(p); got != wantAll || p.Scoped || p.Brand != "" || strings.Join(p.Brands, ",") != "alpha,bravo" {
		t.Fatalf("unscoped: %s scoped=%v brand=%q brands=%v, want %s, alpha,bravo", got, p.Scoped, p.Brand, p.Brands, wantAll)
	}

	// ?brand= resolves to that brand's lists: alpha = A's campaigns (equal to the scoped result).
	p = call("brand alpha", admin, "?brand=alpha")
	if got := rows(p); got != onA || p.Brand != "alpha" {
		t.Fatalf("brand=alpha: %s brand=%q, want %s", got, p.Brand, onA)
	}
	// bravo = A+B and B: anchors on 07-20, so A+B's 06-30 view counts and its 05-16 clicks do not.
	if got, want := rows(call("brand bravo", admin, "?brand=bravo")), "[browser-windows 0 5][gmail-proxy 1 0][outlook-windows 5 0]"; got != want {
		t.Fatalf("brand=bravo: %s, want %s", got, want)
	}
	// A tag no list carries -> no rows.
	if got := rows(call("brand unknown", admin, "?brand=nope")); got != "" {
		t.Fatalf("brand=nope: %s, want no rows", got)
	}
}
