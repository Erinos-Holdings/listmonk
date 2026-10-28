package main

// Fork (campaign conversions) -- integrations CAMPAIGN-CONVERSIONS-SPEC against a real database,
// through the handlers and the same a.auth.Perm wrapper handlers.go registers. Shares newLinkHarness
// (LISTMONK_TEST_PG opt-in; plain go test skips) and the brand-analytics helpers.
//   T6 (I6)  PUT /api/campaigns/conversions stores an order at most once per measure and replaces one
//            measure only -- a duplicate order id, an unknown campaign, a negative sale, an unknown
//            measure or an incomplete row is a 400 with nothing written; a PUT of matched leaves the
//            tracked rows intact.
//   T7 (I7)  the click export is gated on subscribers:get_all, never on campaigns:get_analytics
//            (route registration asserted from handlers.go, and the wrapper exercised).
//   T8 (I8)  type=conversions applies the analytics campaign-scope check (a list-scoped user asking
//            for another brand's campaign is refused exactly as type=links is), and returns NULL for
//            an uncovered measure, 0 for a covered campaign with no rows.
//   T9 (I9, I12) the Brand Matrix's tracked/matched = the sum of the brand's campaigns' rows in the
//            60 Denver days ending yesterday (from the statement's one zone CTE -- a row at 23:30
//            Denver on the window's last day counts, 00:30 today does not), the Overview's are the
//            lifetime sum over the scope's campaigns, and All = the sum of the brands.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
)

// convCall runs handler as u with a method and JSON body; an HTTPError's message is the body.
func convCall(t *testing.T, u auth.User, method, path, body string, handler echo.HandlerFunc) (int, string) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.UserHTTPCtxKey, u)
	if err := handler(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return rec.Code, rec.Body.String()
}

func (h *linkHarness) convRows(measure string) []string {
	var rows []struct {
		OrderID    string  `db:"order_id"`
		CampaignID int     `db:"campaign_id"`
		Sales      float64 `db:"sales_usd"`
	}
	if err := h.db.Select(&rows, `SELECT order_id, campaign_id, sales_usd FROM campaign_conversions WHERE measure = $1 ORDER BY order_id`, measure); err != nil {
		h.t.Fatal(err)
	}
	out := []string{}
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s>%d=%.2f", r.OrderID, r.CampaignID, r.Sales))
	}
	return out
}

func convRow(order string, camp int, sales float64) string {
	return fmt.Sprintf(`{"order_id":%q,"campaign_id":%d,"brand":"alpha","purchased_at":"2026-10-20T10:00:00Z","sales_usd":%v}`, order, camp, sales)
}

func convBody(measure string, rows ...string) string {
	return fmt.Sprintf(`{"measure":%q,"rows":[%s]}`, measure, joinComma(rows))
}

func joinComma(s []string) string {
	var b bytes.Buffer
	for i, x := range s {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(x)
	}
	return b.String()
}

func TestCampaignConversionsPut(t *testing.T) {
	h := newLinkHarness(t)
	h.app.auth = &auth.Auth{}
	f := newBAFixture(h)
	writer := baUser([]string{"brands:manage"})
	put := func(body string) (int, string) {
		return convCall(t, writer, http.MethodPut, "/api/campaigns/conversions", body, h.app.auth.Perm(h.app.PutCampaignConversions, "brands:manage"))
	}
	eq := func(name string, got, want []string) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
	}

	if code, body := put(convBody("matched", convRow("o1", f.campA, 12.5), convRow("o2", f.campB, 3))); code != http.StatusOK {
		t.Fatalf("matched PUT: %d %s", code, body)
	}
	// The same order may appear once in EACH measure.
	if code, body := put(convBody("tracked", convRow("o1", f.campA, 10))); code != http.StatusOK {
		t.Fatalf("tracked PUT: %d %s", code, body)
	}
	eq("matched", h.convRows("matched"), []string{fmt.Sprintf("o1>%d=12.50", f.campA), fmt.Sprintf("o2>%d=3.00", f.campB)})
	eq("tracked", h.convRows("tracked"), []string{fmt.Sprintf("o1>%d=10.00", f.campA)})

	// Every refusal is a 400 with nothing written.
	for _, tc := range []struct{ name, body string }{
		{"duplicate order id", convBody("matched", convRow("o3", f.campA, 1), convRow("o3", f.campB, 2))},
		{"unknown campaign", convBody("matched", convRow("o4", 999999, 1))},
		{"negative sale", convBody("matched", convRow("o5", f.campA, -0.01))},
		{"unknown measure", convBody("estimated", convRow("o6", f.campA, 1))},
		{"incomplete row", `{"measure":"matched","rows":[{"order_id":"o7","campaign_id":1,"brand":"alpha","sales_usd":1}]}`},
		{"empty order id", convBody("matched", convRow("", f.campA, 1))},
		{"bad timestamp", `{"measure":"matched","rows":[{"order_id":"o8","campaign_id":1,"brand":"alpha","purchased_at":"yesterday","sales_usd":1}]}`},
		{"no rows key", `{"measure":"matched"}`},
	} {
		if code, body := put(tc.body); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s, want 400", tc.name, code, body)
		}
		eq(tc.name+" left matched", h.convRows("matched"), []string{fmt.Sprintf("o1>%d=12.50", f.campA), fmt.Sprintf("o2>%d=3.00", f.campB)})
	}

	// A PUT of matched replaces matched only; tracked is intact.
	if code, body := put(convBody("matched", convRow("o9", f.campB, 7))); code != http.StatusOK {
		t.Fatalf("matched replace: %d %s", code, body)
	}
	eq("matched replaced", h.convRows("matched"), []string{fmt.Sprintf("o9>%d=7.00", f.campB)})
	eq("tracked intact", h.convRows("tracked"), []string{fmt.Sprintf("o1>%d=10.00", f.campA)})

	// An empty list empties the measure (and only it).
	if code, body := put(`{"measure":"matched","rows":[]}`); code != http.StatusOK {
		t.Fatalf("empty matched: %d %s", code, body)
	}
	eq("matched empty", h.convRows("matched"), []string{})
	eq("tracked intact after empty", h.convRows("tracked"), []string{fmt.Sprintf("o1>%d=10.00", f.campA)})

	// Without brands:manage -> 403 through the wrapper.
	code, _ := convCall(t, baUser([]string{auth.PermCampaignsGetAnalytics, "brands:get"}), http.MethodPut, "/api/campaigns/conversions",
		convBody("matched"), h.app.auth.Perm(h.app.PutCampaignConversions, "brands:manage"))
	if code != http.StatusForbidden {
		t.Fatalf("without brands:manage: %d, want 403", code)
	}

	// The routes are registered with exactly these permissions.
	b, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`g\.PUT\("/api/campaigns/conversions", pm\(a\.PutCampaignConversions, "brands:manage"\)\)`).Match(b) {
		t.Fatal(`handlers.go does not register PUT /api/campaigns/conversions as pm(a.PutCampaignConversions, "brands:manage")`)
	}
}

func TestCampaignConversionsClickExport(t *testing.T) {
	h := newLinkHarness(t)
	h.app.auth = &auth.Auth{}
	f := newBAFixture(h)

	// I7 -- the registration: subscribers:get_all and nothing else (never campaigns:get_analytics).
	b, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`g\.GET\("/api/campaigns/analytics/clicks/export", pm\(a\.ExportConversionClicks, "subscribers:get_all"\)\)`).Match(b) {
		t.Fatal(`handlers.go does not register the click export as pm(a.ExportConversionClicks, "subscribers:get_all")`)
	}

	var link int
	h.db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://conv.test') RETURNING id`)
	s1 := h.baSub("one@x", "enabled", nil)
	s2 := h.baSub("Two.Dots@Gmail.com", "enabled", nil)
	ins := func(camp, sub int, at string) {
		h.db.MustExec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, created_at) VALUES (NULLIF($1, 0), NULLIF($2, 0), $3, $4::TIMESTAMPTZ)`, camp, sub, link, at)
	}
	ins(f.campA, s1, "2026-08-30T12:00:00Z") // before since: out
	ins(f.campA, s1, "2026-09-01T12:00:00Z")
	ins(f.campA, 0, "2026-09-02T12:00:00Z") // no subscriber: out
	ins(0, s1, "2026-09-03T12:00:00Z")      // no campaign: out
	ins(f.campB, s2, "2026-09-04T12:00:00Z")
	ins(f.campAB, s1, "2026-09-05T12:00:00Z")

	export := func(u auth.User, q string) (int, string) {
		return convCall(t, u, http.MethodGet, "/api/campaigns/analytics/clicks/export?"+q, "",
			h.app.auth.Perm(h.app.ExportConversionClicks, "subscribers:get_all"))
	}
	since := "since=2026-08-31T00:00:00Z"

	// Analytics alone (the listmonk_bi shape) -> 403.
	if code, body := export(baUser([]string{auth.PermCampaignsGetAnalytics, auth.PermCampaignsGetAll}, f.listA, f.listB), since); code != http.StatusForbidden {
		t.Fatalf("analytics only: %d %s, want 403", code, body)
	}

	reader := baUser([]string{auth.PermSubscribersGetAll})
	type row struct {
		ID         int64     `json:"id"`
		CampaignID int       `json:"campaign_id"`
		Email      string    `json:"email"`
		CreatedAt  time.Time `json:"created_at"`
	}
	page := func(q string) []row {
		t.Helper()
		code, body := export(reader, q)
		if code != http.StatusOK {
			t.Fatalf("export %s: %d %s", q, code, body)
		}
		var resp struct {
			Data []row `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.Data == nil {
			t.Fatalf("export %s: %s (%v)", q, body, err)
		}
		return resp.Data
	}
	all := page(since)
	if len(all) != 3 || all[0].CampaignID != f.campA || all[1].Email != "Two.Dots@Gmail.com" || all[2].CampaignID != f.campAB {
		t.Fatalf("export rows: %+v", all)
	}
	// Keyset: per_page 2, then after the last id.
	p1 := page(since + "&per_page=2")
	p2 := page(fmt.Sprintf("%s&per_page=2&after_id=%d", since, p1[len(p1)-1].ID))
	if len(p1) != 2 || len(p2) != 1 || p2[0].ID != all[2].ID {
		t.Fatalf("keyset pages: %+v / %+v", p1, p2)
	}
	for _, q := range []string{"", "since=yesterday", since + "&per_page=10001", since + "&after_id=-1"} {
		if code, _ := export(reader, q); code != http.StatusBadRequest {
			t.Fatalf("export %q: %d, want 400", q, code)
		}
	}
}

func TestCampaignConversionsAnalytics(t *testing.T) {
	h := newLinkHarness(t)
	h.app.auth = &auth.Auth{}
	f := newBAFixture(h)

	ins := func(measure, order string, camp int, at string, sales float64) {
		h.db.MustExec(`INSERT INTO campaign_conversions (measure, order_id, campaign_id, brand, purchased_at, sales_usd) VALUES ($1, $2, $3, 'alpha', $4::TIMESTAMPTZ, $5)`,
			measure, order, camp, at, sales)
	}
	now := time.Now().UTC()
	in := now.Add(-time.Hour).Format(time.RFC3339)
	out := now.Add(-10 * 24 * time.Hour).Format(time.RFC3339) // outside analyticsMultiReq's 48 h range
	ins("tracked", "t1", f.campA, in, 10)
	ins("tracked", "t2", f.campA, in, 5.5)
	ins("tracked", "t3", f.campA, out, 100)
	ins("matched", "m1", f.campA, in, 7)

	member := baUser([]string{auth.PermCampaignsGetAnalytics}, f.listA)
	memberB := baUser([]string{auth.PermCampaignsGetAnalytics}, f.listB)

	type meas struct {
		Orders int     `json:"orders"`
		Sales  float64 `json:"sales_usd"`
	}
	type camp struct {
		CampaignID int   `json:"campaign_id"`
		Tracked    *meas `json:"tracked"`
		Matched    *meas `json:"matched"`
	}
	type resp struct {
		AsOf         *string `json:"as_of"`
		TrackedFloor *string `json:"tracked_floor"`
		MatchedFloor *string `json:"matched_floor"`
		TrackedStale *bool   `json:"tracked_stale"`
		Campaigns    []camp  `json:"campaigns"`
	}
	read := func(u auth.User, ids ...int) resp {
		t.Helper()
		code, body := analyticsMultiReq(t, h, u, "conversions", ids...)
		if code != http.StatusOK {
			t.Fatalf("conversions %v: %d %s", ids, code, body)
		}
		var r struct {
			Data resp `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		return r.Data
	}

	// No doc: every measure NULL (uncovered), the meta NULL.
	r := read(member, f.campA)
	if r.AsOf != nil || len(r.Campaigns) != 1 || r.Campaigns[0].Tracked != nil || r.Campaigns[0].Matched != nil {
		t.Fatalf("no doc: %+v", r)
	}

	// The doc covers A for both measures, AB for matched only.
	h.db.MustExec(`INSERT INTO system_health (kind, day, status, doc) VALUES ('conversions', CURRENT_DATE, 'ok', $1::JSONB)`,
		fmt.Sprintf(`{"v":1,"kind":"conversions","asOf":"2026-10-28T14:45:00Z","trackedFloor":"2026-04-01","matchedFloor":"2026-08-31T00:00:00Z","trackedStale":false,"trackedCampaigns":[%d],"matchedCampaigns":[%d,%d]}`,
			f.campA, f.campA, f.campAB))
	r = read(member, f.campAB, f.campA)
	if r.AsOf == nil || *r.AsOf != "2026-10-28T14:45:00Z" || r.TrackedFloor == nil || *r.TrackedFloor != "2026-04-01" || r.TrackedStale == nil || *r.TrackedStale {
		t.Fatalf("meta: %+v", r)
	}
	if len(r.Campaigns) != 2 {
		t.Fatalf("campaigns: %+v", r.Campaigns)
	}
	a, ab := r.Campaigns[0], r.Campaigns[1]
	if a.CampaignID != f.campA {
		a, ab = ab, a
	}
	// A: the in-range rows only (t3 is outside the date range); covered both.
	if a.Tracked == nil || a.Tracked.Orders != 2 || a.Tracked.Sales != 15.5 || a.Matched == nil || a.Matched.Orders != 1 || a.Matched.Sales != 7 {
		t.Fatalf("A: %+v %+v", a.Tracked, a.Matched)
	}
	// AB: tracked uncovered -> NULL; matched covered with no rows -> a real 0.
	if ab.Tracked != nil || ab.Matched == nil || ab.Matched.Orders != 0 || ab.Matched.Sales != 0 {
		t.Fatalf("AB: %+v %+v", ab.Tracked, ab.Matched)
	}

	// I8 -- the scope check: refused exactly where type=links is.
	for _, tc := range []struct {
		name string
		u    auth.User
		ids  []int
		want int
	}{
		{"member A on A", member, []int{f.campA}, http.StatusOK},
		{"member B on A (another brand)", memberB, []int{f.campA}, http.StatusForbidden},
		{"member A on A+B (one unpermitted)", member, []int{f.campA, f.campB}, http.StatusForbidden},
		{"no analytics permission", baUser(nil, f.listA), []int{f.campA}, http.StatusForbidden},
	} {
		lc, _ := analyticsMultiReq(t, h, tc.u, "links", tc.ids...)
		cc, cb := analyticsMultiReq(t, h, tc.u, "conversions", tc.ids...)
		if lc != tc.want || cc != lc {
			t.Fatalf("%s: links %d, conversions %d (%s), want both %d", tc.name, lc, cc, cb, tc.want)
		}
	}
}

func TestCampaignConversionsDashboards(t *testing.T) {
	h := newLinkHarness(t)

	den, err := time.LoadLocation("America/Denver")
	if err != nil {
		t.Fatal(err)
	}
	var todayS string
	if err := h.db.Get(&todayS, `SELECT TO_CHAR((NOW() AT TIME ZONE 'America/Denver')::DATE, 'YYYY-MM-DD')`); err != nil {
		t.Fatal(err)
	}
	today, err := time.ParseInLocation("2006-01-02", todayS, den)
	if err != nil {
		t.Fatal(err)
	}
	ty, tm, td := today.Date()
	start := func(n int) time.Time { return time.Date(ty, tm, td-n, 0, 0, 0, 0, den) }

	pubA := h.baList("Alpha", "public", "single")
	pubB := h.baList("Bravo", "public", "single")
	listN := h.baList("No brand", "public", "single")
	h.db.MustExec(`UPDATE lists SET tags = '{brand:alpha}' WHERE id = $1`, pubA)
	h.db.MustExec(`UPDATE lists SET tags = '{brand:bravo}' WHERE id = $1`, pubB)
	cA1 := h.baCamp("A1", "finished", 10, pubA)
	cA2 := h.baCamp("A2", "finished", 10, pubA)
	cB := h.baCamp("B", "finished", 10, pubB)
	h.baCamp("N", "finished", 10, listN)

	ins := func(measure, order string, camp int, brand string, at time.Time, sales float64) {
		h.db.MustExec(`INSERT INTO campaign_conversions (measure, order_id, campaign_id, brand, purchased_at, sales_usd) VALUES ($1, $2, $3, $4, $5::TIMESTAMPTZ, $6)`,
			measure, order, camp, brand, at.Format(time.RFC3339), sales)
	}
	lastDay := start(1).Add(23*time.Hour + 30*time.Minute) // 23:30 Denver on the window's last day: IN
	ins("tracked", "a1", cA1, "alpha", lastDay, 10)
	ins("tracked", "a2", cA2, "alpha", start(30), 20)
	ins("tracked", "a3", cA1, "alpha", start(0).Add(30*time.Minute), 400)  // 00:30 Denver today: OUT
	ins("tracked", "a4", cA1, "alpha", start(60), 1)                        // the window's first instant: IN
	ins("tracked", "a5", cA1, "alpha", start(60).Add(-time.Second), 800)    // one second before: OUT
	ins("matched", "a1", cA2, "alpha", lastDay, 3)
	ins("matched", "b1", cB, "bravo", start(5), 50)
	ins("matched", "b2", cB, "bravo", start(70), 9) // out of the window, in the lifetime

	type meas struct {
		Orders int     `json:"orders"`
		Sales  float64 `json:"sales_usd"`
	}
	type row struct {
		Brand   string `json:"brand"`
		Tracked meas   `json:"tracked"`
		Matched meas   `json:"matched"`
	}
	admin := baUser([]string{auth.PermListGetAll, auth.PermCampaignsGetAll, auth.PermCampaignsGet, "brands:get"})
	code, body := baCall(t, admin, "/api/dashboard/brand-matrix", h.app.GetDashboardBrandMatrix)
	if code != http.StatusOK {
		t.Fatalf("matrix: %d %s", code, body)
	}
	var mr struct {
		Data struct {
			Conversions json.RawMessage `json:"conversions"`
			Rows        []row            `json:"rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &mr); err != nil {
		t.Fatal(err)
	}
	got := map[string]row{}
	for _, r := range mr.Data.Rows {
		got[r.Brand] = r
	}
	if a := got["alpha"]; a.Tracked != (meas{3, 31}) || a.Matched != (meas{1, 3}) {
		t.Fatalf("alpha: %+v, want tracked {3 31} (a1+a2+a4), matched {1 3}", a)
	}
	if b := got["bravo"]; b.Tracked != (meas{0, 0}) || b.Matched != (meas{1, 50}) {
		t.Fatalf("bravo: %+v", b)
	}
	if string(mr.Data.Conversions) != "null" {
		t.Fatalf("no doc: conversions %s, want null", mr.Data.Conversions)
	}

	// Overview (lifetime over the scope's C): All = the sum of every row; one brand = its rows.
	counts := func(u auth.User, q string) (meas, meas, json.RawMessage) {
		t.Helper()
		code, body := baCall(t, u, "/api/dashboard/counts"+q, h.app.GetDashboardCounts)
		if code != http.StatusOK {
			t.Fatalf("counts %q: %d %s", q, code, body)
		}
		var r struct {
			Data struct {
				Conversions struct {
					Tracked meas            `json:"tracked"`
					Matched meas            `json:"matched"`
					Meta    json.RawMessage `json:"meta"`
				} `json:"conversions"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			t.Fatal(err)
		}
		return r.Data.Conversions.Tracked, r.Data.Conversions.Matched, r.Data.Conversions.Meta
	}
	tr, ma, _ := counts(admin, "")
	if tr != (meas{5, 1231}) || ma != (meas{3, 62}) {
		t.Fatalf("overview all: tracked %+v matched %+v", tr, ma)
	}
	tr, ma, _ = counts(admin, fmt.Sprintf("?list_id=%d", pubB))
	if tr != (meas{0, 0}) || ma != (meas{2, 59}) {
		t.Fatalf("overview bravo: tracked %+v matched %+v", tr, ma)
	}
	tr, ma, _ = counts(admin, fmt.Sprintf("?list_id=%d", listN))
	if tr != (meas{0, 0}) || ma != (meas{0, 0}) {
		t.Fatalf("overview no brand: tracked %+v matched %+v, want zeros", tr, ma)
	}

	// The doc's meta is projected verbatim on both.
	h.db.MustExec(`INSERT INTO system_health (kind, day, status, doc) VALUES ('conversions', CURRENT_DATE, 'warn', '{"asOf":"2026-10-28T02:45:00Z","trackedFloor":"2026-04-01","matchedFloor":"2026-08-31T00:00:00Z","trackedStale":true}'::JSONB)`)
	_, _, meta := counts(admin, "")
	var m struct {
		AsOf         string `json:"as_of"`
		TrackedFloor string `json:"tracked_floor"`
		MatchedFloor string `json:"matched_floor"`
		TrackedStale bool   `json:"tracked_stale"`
	}
	if err := json.Unmarshal(meta, &m); err != nil || m.AsOf != "2026-10-28T02:45:00Z" || !m.TrackedStale || m.MatchedFloor != "2026-08-31T00:00:00Z" {
		t.Fatalf("overview meta: %s (%v)", meta, err)
	}
	_, body = baCall(t, admin, "/api/dashboard/brand-matrix", h.app.GetDashboardBrandMatrix)
	if !bytes.Contains([]byte(body), []byte(`"tracked_stale":true`)) {
		t.Fatalf("matrix meta: %s", body)
	}
}
