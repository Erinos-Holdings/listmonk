package main

// Fork (campaign matrix) -- integrations listmonk/CAMPAIGN-MATRIX-SPEC I1-I5 against a real
// database, through the handler wrapped in a.auth.Perm exactly as handlers.go registers it
// (LISTMONK_TEST_PG opt-in; plain go test skips, the suite's documented shape), in BOTH tracking
// modes (the statement prepared through prepareQueries as cmd/init.go does at boot).
//
// I1: the matrix never returns a campaign the caller could not open in Analytics (every id is in the
// picker's own answer for that user); list_id only narrows; analytics-only users see started
// campaigns only; GP is null for a campaigns:get_analytics-only caller and present with brands:get.
// I2: broadcasts in/out by started_at (and sent > 0), journeys by campaign_send_hours with Sent the
// window sum, the kind filter.
// I3: Views = distinct subscribers; the journey first-view-in-window rule; Clicks likewise.
// I4: hard / soft / complaint bounces by type; conversions NULL when not covered, window-limited for
// journeys.
// I5: GP send day -- the newest brand_health row holding the day (with any Gmail rate) wins, with
// that row's thresholds; no numeric spamRate, an absent day or several brands -> null.
// MATRIX-POLISH-SPEC I7: the shop-link kinds -- the frozen send-time kind wins, the current kind is
// the fallback, the click-pair rule (a journey clicker whose first click is before t0 counts 0 in
// Clicks and in the kinds), NULL for a campaign whose body holds no shop link, 0 for an unclicked
// one; the D4 tier fields (view_clients, client_floor) are gone from the response.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/paginator"
	"github.com/labstack/echo/v4"
)

type cmMeasure struct {
	Orders   int     `json:"orders"`
	SalesUSD float64 `json:"sales_usd"`
}

type cmRow struct {
	ID           int             `json:"id"`
	Evergreen    bool            `json:"evergreen"`
	SendDay      *string         `json:"send_day"`
	LastSent     *string         `json:"last_sent"`
	Brand        *string         `json:"brand"`
	Sent         *int            `json:"sent"`
	Views        int             `json:"views"`
	Clicks       int             `json:"clicks"`
	ShopFounder  *int            `json:"shop_founder"`
	ShopSelf     *int            `json:"shop_self"`
	ShopCreator  *int            `json:"shop_creator"`
	ShopCorp     *int            `json:"shop_corp"`
	HardBounces  int             `json:"hard_bounces"`
	SoftBounces  int             `json:"soft_bounces"`
	Complaints   int             `json:"complaints"`
	Tracked      *cmMeasure      `json:"tracked"`
	Matched      *cmMeasure      `json:"matched"`
	GPRate       *float64        `json:"gp_rate"`
	GPThresholds json.RawMessage `json:"gp_thresholds"`
}

type cmResp struct {
	GP   bool    `json:"gp"`
	Rows []cmRow `json:"rows"`
}

// TestCampaignMatrixPerm is D10's gate: the route is registered behind campaigns:get_analytics
// through the pm wrapper (the picker's permission).
func TestCampaignMatrixPerm(t *testing.T) {
	b, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`g\.GET\("/api/analytics/campaigns/matrix", pm\(a\.GetCampaignMatrix, "campaigns:get_analytics"\)\)`).Match(b) {
		t.Fatal(`cmd/handlers.go does not register /api/analytics/campaigns/matrix as pm(a.GetCampaignMatrix, "campaigns:get_analytics")`)
	}

	reached := false
	next := func(c echo.Context) error { reached = true; return nil }
	gate := (&auth.Auth{}).Perm(next, "campaigns:get_analytics")
	code, _ := baCall(t, baUser([]string{auth.PermListGetAll, "brands:get"}), "/api/analytics/campaigns/matrix", gate)
	if code != http.StatusForbidden || reached {
		t.Fatalf("without campaigns:get_analytics: %d (handler reached %v), want 403 before the handler", code, reached)
	}
}

func TestCampaignMatrixDB(t *testing.T) {
	h := newLinkHarness(t)
	t.Cleanup(func() { ko.Set("privacy.individual_tracking", true) })

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
	noon := 12 * time.Hour
	dayAt := func(n int) time.Time { return start(n).Add(noon) }
	day := func(n int) string { return start(n).Format("2006-01-02") }
	ts := func(t time.Time) string { return t.Format(time.RFC3339) }

	// Lists: alpha (public + a private derivative), bravo, and an untagged list.
	listA := h.baList("Alpha", "public", "single")
	listA2 := h.baList("Alpha buyers", "private", "single")
	listB := h.baList("Bravo", "public", "single")
	listN := h.baList("No brand", "public", "single")
	h.db.MustExec(`UPDATE lists SET tags = '{brand:alpha}' WHERE id IN ($1, $2)`, listA, listA2)
	h.db.MustExec(`UPDATE lists SET tags = '{brand:bravo}' WHERE id = $1`, listB)

	camp := func(name string, evergreen bool, startedAt *time.Time, sent int, lists ...int) int {
		id := h.baCamp(name, "finished", sent, lists...)
		var st any
		if startedAt != nil {
			st = ts(*startedAt)
		}
		h.db.MustExec(`UPDATE campaigns SET evergreen = $2, started_at = $3::TIMESTAMPTZ WHERE id = $1`, id, evergreen, st)
		return id
	}
	at := func(n int) *time.Time { v := dayAt(n); return &v }

	bIn := camp("Broadcast in", false, at(10), 100, listA, listN) // the untagged list forms no brand
	bOut := camp("Broadcast before", false, at(65), 50, listA)
	bToday := camp("Broadcast today", false, at(0), 50, listA)
	bZero := camp("Broadcast zero", false, at(9), 0, listA)
	bB := camp("Bravo broadcast", false, at(20), 30, listB)
	bAB := camp("Two brands", false, at(15), 40, listA, listB)
	bLow := camp("Alpha buyers", false, at(20), 20, listA2)
	draft := h.baCamp("Draft", "draft", 0, listA)
	jIn := camp("Journey", true, at(90), 0, listA)
	jOut := camp("Journey old", true, at(90), 0, listA)
	jNull := camp("Journey unstarted", true, nil, 0, listA)
	_ = draft

	hours := func(c, n int, at time.Time) {
		h.db.MustExec(`INSERT INTO campaign_send_hours (campaign_id, hour, n) VALUES ($1, DATE_TRUNC('hour', $2::TIMESTAMPTZ), $3)`, c, ts(at), n)
	}
	hours(jIn, 5, dayAt(70))  // before the window
	hours(jIn, 7, dayAt(30))  // in
	hours(jIn, 4, dayAt(3))   // in, the last sent
	hours(jIn, 9, dayAt(0))   // Denver-today (out)
	hours(jOut, 5, dayAt(70)) // only before the window: no row
	hours(jNull, 2, dayAt(4))
	hours(bIn, 100, dayAt(10)) // a broadcast's hours never matter: its Sent is campaigns.sent

	s1 := h.baSub("s1@x", "enabled", map[int]string{listA: "confirmed"})
	s2 := h.baSub("s2@x", "enabled", map[int]string{listA: "confirmed"})
	s3 := h.baSub("s3@x", "enabled", map[int]string{listA: "confirmed"})

	view := func(c, sub int, client string, at time.Time) {
		h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id, client, created_at) VALUES ($1, NULLIF($2, 0), NULLIF($3, ''), $4::TIMESTAMPTZ)`,
			c, sub, client, ts(at))
	}
	// bIn: s1 three times from one client and once from another, s2 once, a NULL subscriber, an
	// Unknown-client view, and one Denver-today view (at or after t1: out).
	for i := 0; i < 3; i++ {
		view(bIn, s1, "outlook-windows", dayAt(9))
	}
	view(bIn, s1, "apple-mail", dayAt(8))
	view(bIn, s2, "apple-mail", dayAt(8))
	view(bIn, s2, "", dayAt(7))
	view(bIn, 0, "gmail-proxy", dayAt(8))
	view(bIn, s3, "outlook-windows", dayAt(0))
	// jIn: s1's first view is before the window (never recounted), s2's is in it, s3's is today.
	view(jIn, s1, "automation", dayAt(70))
	view(jIn, s1, "automation", dayAt(5))
	view(jIn, s2, "yahoo", dayAt(5))
	view(jIn, s3, "yahoo", dayAt(0))

	var link, shopLink int
	if err := h.db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://cm.test') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	// MATRIX-POLISH I7 -- a shop link (the stored url holds the unexpanded expression) and the bodies
	// that carry one: bIn and jIn (clicked) and bLow (never clicked). bB's body ('b') holds none.
	if err := h.db.Get(&shopLink, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), '{{ .Subscriber.Attribs.shop_links.alpha.url }}') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	h.db.MustExec(`UPDATE campaigns SET body = '<a href="{{ TrackLink .Subscriber.Attribs.shop_links.alpha.url }}">Shop</a>' WHERE id IN ($1, $2, $3)`, bIn, jIn, bLow)
	// Kinds held now: s1 creator, s2 self, s3 corporate. s1's send-time kind on bIn was founder.
	h.db.MustExec(`UPDATE subscribers SET attribs = '{"shop_links": {"alpha": {"kind": "creator"}}}' WHERE id = $1`, s1)
	h.db.MustExec(`UPDATE subscribers SET attribs = '{"shop_links": {"alpha": {"kind": "self"}}}' WHERE id = $1`, s2)
	h.db.MustExec(`UPDATE subscribers SET attribs = '{"shop_links": {"alpha": {"kind": "corporate"}}}' WHERE id = $1`, s3)
	h.db.MustExec(`INSERT INTO campaign_shop_link_recipients (campaign_id, subscriber_id, brand, kind) VALUES ($1, $2, 'alpha', 'founder')`, bIn, s1)
	clickOn := func(l, c, sub int, at time.Time) {
		h.db.MustExec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, created_at) VALUES ($1, NULLIF($2, 0), $3, $4::TIMESTAMPTZ)`,
			c, sub, l, ts(at))
	}
	clickOn(shopLink, bIn, s1, dayAt(9)) // frozen founder
	clickOn(shopLink, bIn, s1, dayAt(8))
	clickOn(shopLink, bIn, s2, dayAt(8))  // no frozen row: self, held now
	clickOn(shopLink, bIn, s3, dayAt(0))  // today: out
	clickOn(shopLink, jIn, s1, dayAt(70)) // s1's first click is before the window: never counted (TRUE)
	clickOn(shopLink, jIn, s1, dayAt(5))
	clickOn(link, jIn, s2, dayAt(5))
	clickOn(shopLink, jIn, s3, dayAt(4)) // first click in the window: corporate (s3 has no other jIn click)

	bounce := func(c int, typ string, at time.Time) {
		h.db.MustExec(`INSERT INTO bounces (campaign_id, subscriber_id, source, type, meta, created_at) VALUES ($1, $2, 'test', $3::bounce_type, '{}', $4::TIMESTAMPTZ)`,
			c, s3, typ, ts(at))
	}
	bounce(bIn, "hard", dayAt(9))
	bounce(bIn, "hard", dayAt(9))
	bounce(bIn, "soft", dayAt(9))
	bounce(bIn, "complaint", dayAt(8))
	bounce(bIn, "hard", dayAt(0)) // today: out
	bounce(jIn, "hard", dayAt(70))
	bounce(jIn, "soft", dayAt(5))

	// Conversions: the latest doc covers bIn for both measures and jIn for Tracked only; an older doc
	// covering everything is never read.
	sysHealth := func(n int, doc any) {
		b, _ := json.Marshal(doc)
		h.db.MustExec(`INSERT INTO system_health (kind, day, status, doc) VALUES ('conversions', $1::DATE, 'ok', $2::JSONB)`, day(n), string(b))
	}
	sysHealth(3, map[string]any{"asOf": "old", "trackedCampaigns": []int{bIn, jIn, bB}, "matchedCampaigns": []int{bIn, jIn, bB}})
	sysHealth(1, map[string]any{"asOf": day(1), "trackedFloor": "2026-01-01", "matchedFloor": "2026-01-01",
		"trackedCampaigns": []int{bIn, jIn}, "matchedCampaigns": []int{bIn}})
	order := 0
	conv := func(measure string, c int, at time.Time, usd float64) {
		order++
		h.db.MustExec(`INSERT INTO campaign_conversions (measure, order_id, campaign_id, brand, purchased_at, sales_usd) VALUES ($1, $2, $3, 'alpha', $4::TIMESTAMPTZ, $5)`,
			measure, fmt.Sprintf("o%d", order), c, ts(at), usd)
	}
	conv("tracked", bIn, dayAt(9), 10)
	conv("tracked", bIn, dayAt(8), 5.5)
	conv("matched", bIn, dayAt(8), 7)
	conv("tracked", bIn, dayAt(0), 99)  // today: out
	conv("tracked", jIn, dayAt(70), 50) // before the window: a journey counts only in-window orders
	conv("tracked", jIn, dayAt(5), 20)
	conv("matched", jIn, dayAt(5), 20) // not covered by the latest doc: null, not 20
	conv("tracked", bB, dayAt(19), 3)  // not covered: null

	// GP (I5). bIn's send day is day(10), bLow's day(20).
	bh := func(n int, series []map[string]any, th map[string]any) {
		b, _ := json.Marshal(map[string]any{"inputs": map[string]any{"spamRate": map[string]any{
			"status": "ok", "detail": map[string]any{"series": series, "thresholds": th}}}})
		h.db.MustExec(`INSERT INTO brand_health (brand, day, status, doc) VALUES ('alpha', $1::DATE, 'ok', $2::JSONB)`, day(n), string(b))
	}
	thA := map[string]any{"warn": 0.001, "issues": 0.003}
	thB := map[string]any{"warn": 0.002, "issues": 0.003}
	bh(25, []map[string]any{{"day": day(10), "spamRate": 0.9}}, thA)                                     // before the send day: never read
	bh(9, []map[string]any{{"day": day(10), "spamRate": 0.001}}, thA)                                    // older holder
	bh(8, []map[string]any{{"day": day(10), "spamRate": 0.004}, {"day": day(20), "spamRate": 0.3}}, thB) // newest holder of day(10) with a rate
	bh(7, []map[string]any{{"day": day(20), "spf": 0.99}}, thA)                                          // newest holder of day(20): no spamRate
	bh(5, []map[string]any{{"day": day(20), "spamRate": 0.5}, {"day": day(10)}}, thA)                    // day(20)+15: out; a bare day(10) point holds no rate
	h.db.MustExec(`INSERT INTO brand_health (brand, day, status, doc) VALUES ('bravo', $1::DATE, 'ok', $2::JSONB)`, day(14),
		fmt.Sprintf(`{"inputs": {"spamRate": {"detail": {"series": [{"day": %q, "spamRate": 0.2}, {"day": %q, "spamRate": 0.2}]}}}}`, day(15), day(20)))

	matrix := func(u auth.User, q url.Values) (int, cmResp, string) {
		t.Helper()
		code, body := baCall(t, u, "/api/analytics/campaigns/matrix?"+q.Encode(), h.app.auth.Perm(h.app.GetCampaignMatrix, "campaigns:get_analytics"))
		var resp struct {
			Data cmResp `json:"data"`
		}
		if code == http.StatusOK {
			if err := json.Unmarshal([]byte(body), &resp); err != nil {
				t.Fatalf("matrix %v: %s: %v", q, body, err)
			}
		}
		return code, resp.Data, body
	}
	ids := func(r cmResp) []int {
		out := []int{}
		for _, x := range r.Rows {
			out = append(out, x.ID)
		}
		sort.Ints(out)
		return out
	}
	byID := func(r cmResp) map[int]cmRow {
		out := map[int]cmRow{}
		for _, x := range r.Rows {
			out[x.ID] = x
		}
		return out
	}
	sortedInts := func(v ...int) []int { sort.Ints(v); return v }

	analytics := []string{auth.PermCampaignsGetAnalytics}
	admin := baUser([]string{auth.PermCampaignsGetAll, auth.PermCampaignsGetAnalytics, auth.PermListGetAll, "brands:get"})
	adminNoGP := baUser([]string{auth.PermCampaignsGetAll, auth.PermCampaignsGetAnalytics, auth.PermListGetAll})
	alphaOnly := baUser(analytics, listA)                                                      // Brand Analytics creator
	alphaGP := baUser([]string{auth.PermCampaignsGetAnalytics, "brands:get"}, listA)           // + brands:get
	alphaGet := baUser([]string{auth.PermCampaignsGet, auth.PermCampaignsGetAnalytics}, listA) // not analytics-only

	allIn := sortedInts(bIn, bB, bAB, bLow, jIn, jNull)
	alphaIn := sortedInts(bIn, bAB, jIn) // listA's campaigns, started only
	_ = bOut
	_ = bToday
	_ = bZero

	str := func(s string) *string { return &s }
	ip := func(n int) *int { return &n }
	fp := func(f float64) *float64 { return &f }

	for _, tracking := range []bool{true, false} {
		ko.Set("privacy.individual_tracking", tracking)
		h.app.core = core.New(&core.Opt{Queries: prepareQueries(loadQueryMap(t), h.db, ko), DB: h.db, I18n: h.app.i18n, Log: h.app.log}, &core.Hooks{})
		name := func(s string) string { return fmt.Sprintf("tracking=%v %s", tracking, s) }

		// I1 / I2 -- scoping, list_id narrowing, started-only, the kind filter.
		cases := []struct {
			name string
			u    auth.User
			q    url.Values
			want []int
		}{
			{"admin, all kinds", admin, url.Values{}, allIn},
			{"admin, kind=all", admin, url.Values{"kind": {"all"}}, allIn},
			{"admin, broadcasts", admin, url.Values{"kind": {"broadcasts"}}, sortedInts(bIn, bB, bAB, bLow)},
			{"admin, automations", admin, url.Values{"kind": {"automations"}}, sortedInts(jIn, jNull)},
			{"admin, list_id = bravo", admin, url.Values{"list_id": {fmt.Sprint(listB)}}, sortedInts(bB, bAB)},
			{"alpha analytics-only", alphaOnly, url.Values{}, alphaIn},
			{"alpha analytics-only, list_id = bravo narrows, never widens", alphaOnly, url.Values{"list_id": {fmt.Sprint(listB)}}, sortedInts(bAB)},
			{"alpha campaigns:get sees the unstarted journey", alphaGet, url.Values{}, sortedInts(bIn, bAB, jIn, jNull)},
		}
		for _, tc := range cases {
			code, got, body := matrix(tc.u, tc.q)
			if code != http.StatusOK {
				t.Fatalf("%s: %d %s", name(tc.name), code, body)
			}
			if !reflect.DeepEqual(ids(got), tc.want) {
				t.Fatalf("%s: ids %v, want %v", name(tc.name), ids(got), tc.want)
			}
			// I1 -- every row is a campaign the same user's picker offers (the analytics endpoint's
			// own scoping), for the same list_id scope.
			pq := url.Values{"per_page": {"50"}}
			for _, l := range tc.q["list_id"] {
				pq.Add("list_id", l)
			}
			picker := pickerIDs(t, h, name(tc.name), tc.u, pq)
			allowed := map[int]bool{}
			for _, id := range picker {
				allowed[id] = true
			}
			for _, id := range ids(got) {
				if !allowed[id] {
					t.Fatalf("%s: campaign %d is not in the user's picker %v", name(tc.name), id, picker)
				}
			}
		}
		if code, _, _ := matrix(admin, url.Values{"kind": {"bogus"}}); code != http.StatusBadRequest {
			t.Fatalf("%s: kind=bogus: %d, want 400", name("kind"), code)
		}

		// I1 -- GP only with brands:get.
		for _, c := range []struct {
			u    auth.User
			want bool
		}{{admin, true}, {adminNoGP, false}, {alphaOnly, false}, {alphaGP, true}} {
			_, got, _ := matrix(c.u, url.Values{})
			if got.GP != c.want {
				t.Fatalf("%s: gp %v, want %v", name("gp flag"), got.GP, c.want)
			}
			r := byID(got)[bIn]
			if (r.GPRate != nil) != c.want || (len(r.GPThresholds) > 0 && string(r.GPThresholds) != "null") != c.want {
				t.Fatalf("%s: bIn gp_rate %v thresholds %s with gp=%v", name("gp fields"), r.GPRate, r.GPThresholds, c.want)
			}
		}

		_, got, _ := matrix(admin, url.Values{})
		rows := byID(got)

		// I2 / I3 / I4 / I5 -- the broadcast row.
		wantViews, wantClicks, wantFounder := 2, 2, 1
		if !tracking {
			wantViews, wantClicks, wantFounder = 7, 3, 2
		}
		wantB := cmRow{ID: bIn, SendDay: str(day(10)), Brand: str("alpha"), Sent: ip(100),
			Views: wantViews, Clicks: wantClicks, HardBounces: 2, SoftBounces: 1, Complaints: 1,
			ShopFounder: ip(wantFounder), ShopSelf: ip(1), ShopCreator: ip(0), ShopCorp: ip(0),
			Tracked: &cmMeasure{2, 15.5}, Matched: &cmMeasure{1, 7}, GPRate: fp(0.004),
			GPThresholds: json.RawMessage(`{"issues":0.003,"warn":0.002}`)} // compactJSON sorts keys
		gotB := rows[bIn]
		gotB.GPThresholds = compactJSON(t, gotB.GPThresholds)
		if !reflect.DeepEqual(gotB, wantB) {
			t.Fatalf("%s: broadcast row\n got  %s\n want %s", name("bIn"), baJSON(gotB), baJSON(wantB))
		}

		// The journey row: Sent = the window's send hours, the first-view rule, journeys carry no GP.
		// I7 -- the pair rule: s1's first jIn click is before t0, so (TRUE) s1 counts in neither
		// Clicks nor a kind; s3's in-window shop click is corporate. FALSE counts raw in-window rows.
		wantViews, wantClicks = 1, 2
		wantCreator := 0
		if !tracking {
			wantViews, wantClicks, wantCreator = 2, 3, 1
		}
		wantJ := cmRow{ID: jIn, Evergreen: true, LastSent: str(day(3)), Brand: str("alpha"), Sent: ip(11),
			Views: wantViews, Clicks: wantClicks, SoftBounces: 1,
			ShopFounder: ip(0), ShopSelf: ip(0), ShopCreator: ip(wantCreator), ShopCorp: ip(1),
			Tracked: &cmMeasure{1, 20}}
		gotJ := rows[jIn]
		gotJ.GPThresholds = compactJSON(t, gotJ.GPThresholds)
		if !reflect.DeepEqual(gotJ, wantJ) {
			t.Fatalf("%s: journey row\n got  %s\n want %s", name("jIn"), baJSON(gotJ), baJSON(wantJ))
		}

		// I5 -- no numeric spamRate on the newest holder -> null; no brand_health for bravo's day
		// -> null; two brands -> brand and GP null. I7 -- an unclicked shop link is 0, no shop link
		// in the body is null in all four.
		zero := func(r cmRow) bool {
			return r.ShopFounder != nil && *r.ShopFounder == 0 && r.ShopSelf != nil && *r.ShopSelf == 0 &&
				r.ShopCreator != nil && *r.ShopCreator == 0 && r.ShopCorp != nil && *r.ShopCorp == 0
		}
		absent := func(r cmRow) bool {
			return r.ShopFounder == nil && r.ShopSelf == nil && r.ShopCreator == nil && r.ShopCorp == nil
		}
		if r := rows[bLow]; r.GPRate != nil || r.Brand == nil || *r.Brand != "alpha" || !zero(r) {
			t.Fatalf("%s: bLow %s, want alpha, gp null, shop kinds 0", name("bLow"), baJSON(r))
		}
		if r := rows[bB]; r.GPRate == nil || *r.GPRate != 0.2 || r.Tracked != nil || r.Matched != nil || !absent(r) {
			t.Fatalf("%s: bB %s, want gp 0.2 (thresholds absent), conversions null, shop kinds null", name("bB"), baJSON(r))
		}
		if r := rows[bAB]; r.Brand != nil || r.GPRate != nil {
			t.Fatalf("%s: bAB %s, want brand and gp null (two brands)", name("bAB"), baJSON(r))
		}
		if r := rows[jNull]; r.Sent == nil || *r.Sent != 2 {
			t.Fatalf("%s: jNull %s, want sent 2", name("jNull"), baJSON(r))
		}

		// I7 -- the D4 tier fields are gone from the response.
		_, _, rawBody := matrix(admin, url.Values{})
		for _, k := range []string{`"view_clients"`, `"client_floor"`} {
			if regexp.MustCompile(regexp.QuoteMeta(k)).MatchString(rawBody) {
				t.Fatalf("%s: the response still carries %s", name("tiers gone"), k)
			}
		}

		// GP for a brands:get creator is the same value the admin sees.
		_, gpGot, _ := matrix(alphaGP, url.Values{})
		if r := byID(gpGot)[bIn]; r.GPRate == nil || *r.GPRate != 0.004 {
			t.Fatalf("%s: alpha creator with brands:get: bIn %s", name("alphaGP"), baJSON(r))
		}
	}
}

// TestCampaignsIDFilterDB is integrations MATRIX-POLISH-SPEC I4: GET /api/campaigns?id=... lists only
// those ids, ANDed with the permission scope and every other filter -- a list-scoped user cannot
// reach a foreign id through it, and no id is today's result.
func TestCampaignsIDFilterDB(t *testing.T) {
	h := newLinkHarness(t)
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})

	listA := h.baList("Alpha", "public", "single")
	listB := h.baList("Bravo", "public", "single")
	campA := h.baCamp("On A", "finished", 1, listA)
	campA2 := h.baCamp("Also on A", "draft", 0, listA)
	campB := h.baCamp("On B", "finished", 1, listB)

	list := func(u auth.User, q url.Values) []int {
		t.Helper()
		code, body := baCall(t, u, "/api/campaigns?"+q.Encode(), h.app.GetCampaigns)
		if code != http.StatusOK {
			t.Fatalf("campaigns %v: %d %s", q, code, body)
		}
		var resp struct {
			Data struct {
				Results []struct {
					ID int `json:"id"`
				} `json:"results"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("campaigns %v: %s: %v", q, body, err)
		}
		out := []int{}
		for _, r := range resp.Data.Results {
			out = append(out, r.ID)
		}
		sort.Ints(out)
		return out
	}
	ints := func(v ...int) []int { sort.Ints(v); return v }
	idq := func(ids ...int) url.Values {
		q := url.Values{"no_body": {"true"}}
		for _, id := range ids {
			q.Add("id", fmt.Sprint(id))
		}
		return q
	}

	admin := baUser([]string{auth.PermCampaignsGetAll, auth.PermListGetAll})
	alpha := baUser([]string{auth.PermCampaignsGet}, listA) // permittedLists = [A]

	cases := []struct {
		name string
		u    auth.User
		q    url.Values
		want []int
	}{
		{"list-scoped user, foreign id -> empty", alpha, idq(campB), []int{}},
		{"list-scoped user, own id -> it", alpha, idq(campA), ints(campA)},
		{"list-scoped user, both -> only the permitted one", alpha, idq(campA, campB), ints(campA)},
		{"list-scoped user, no id -> today's result", alpha, idq(), ints(campA, campA2)},
		{"admin, two ids -> exactly those", admin, idq(campA, campB), ints(campA, campB)},
		{"admin, no id -> today's result", admin, idq(), ints(campA, campA2, campB)},
		{"admin, id ANDed with status", admin, func() url.Values { q := idq(campA, campA2); q.Add("status", "draft"); return q }(), ints(campA2)},
		{"admin, id ANDed with list_id", admin, func() url.Values { q := idq(campA, campB); q.Add("list_id", fmt.Sprint(listB)); return q }(), ints(campB)},
	}
	for _, tc := range cases {
		if got := list(tc.u, tc.q); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: ids %v, want %v", tc.name, got, tc.want)
		}
	}

	if code, _ := baCall(t, admin, "/api/campaigns?id=x", h.app.GetCampaigns); code != http.StatusBadRequest {
		t.Fatalf("id=x: %d, want 400", code)
	}
}

// compactJSON is the raw JSON compacted (nil for a JSON null), so stored objects compare byte-wise.
func compactJSON(t *testing.T, b json.RawMessage) json.RawMessage {
	t.Helper()
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
