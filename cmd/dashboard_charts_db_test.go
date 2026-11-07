package main

// Fork (dashboard sends) -- integrations DASHBOARD-SENDS-SPEC I3 and I4 against a real database,
// through the handler, as revision 2 (R-I4, R-I5) has them. I3/R-I4: the Clicks series is
// link_clicks of the scope's campaigns C per America/Denver day over [today-60, today-1] (Denver
// dates), 60 zero-filled points; under individual tracking each (campaign, subscriber) pair counts
// once on its first-click day (a pair first clicked before the window is not counted, NULL
// subscribers never), raw rows when OFF -- each mode's statement prepared through prepareQueries as
// cmd/init.go does at boot (TestDashboardCountsDB's pattern). The session time zone is set far from
// both UTC and Denver so a session-zone bucket would land boundary clicks on the wrong day. I4/R-I5:
// the Sends series is the latest ses system_health document's brands[].daily summed per day --
// every row unscoped (unattributed included), only the scope lists' brand-tag rows scoped, never
// unattributed, [] for a scope with no branded list or with no ses document; days outside the
// window are dropped, a row with daily null or no daily contributes nothing. Shares newLinkHarness
// and the brand-analytics fixtures (LISTMONK_TEST_PG opt-in).
//
// Integrations TODAY-SO-FAR-SPEC I1/I2 amend both. I1: the window is [today-60, today] (Denver), so
// Clicks has 61 zero-filled points ending today (a today click counts, day -61 and tomorrow do not)
// and the response carries today. I2 (the splice): SES points (live false) only for days up to
// ses_last, the latest document's newest in-window day before today over ANY brands row; the days
// after it through today are campaign_send_hours (live true, zero-filled) of every campaign
// unscoped, else of the campaigns on a branded (never unattributed) scope list -- [] for a scope
// with no branded list; a document ending day -2 makes day -1 live; no document leaves only today,
// live. The conversions half of I1 is TestCampaignConversionsDashboards.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
)

type chartPoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type sendPoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
	Live  bool   `json:"live"`
}

type chartsResp struct {
	Sends  []sendPoint  `json:"sends"`
	Clicks []chartPoint `json:"clicks"`
	Today  string       `json:"today"`
	Scoped bool         `json:"scoped"`
}

func TestDashboardChartsDB(t *testing.T) {
	h := newLinkHarness(t)

	// The session time zone 14 hours ahead of UTC: a click at 23:59 UTC is the next local day.
	h.db.MustExec(`ALTER DATABASE ` + linkTestDB + ` SET TimeZone TO 'Pacific/Kiritimati'`)
	h.db.SetMaxIdleConns(0)
	h.db.SetMaxIdleConns(2)
	var tz string
	if err := h.db.Get(&tz, `SHOW TimeZone`); err != nil || tz != "Pacific/Kiritimati" {
		t.Fatalf("session time zone %q (%v), want Pacific/Kiritimati", tz, err)
	}

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
	// start is Denver midnight of today-n (DST-aware); dayAt adds a wall-clock offset that never
	// crosses a DST change; endOf is the last 30 s of Denver day today-n.
	start := func(n int) time.Time { return time.Date(ty, tm, td-n, 0, 0, 0, 0, den) }
	dayAt := func(n int, clock time.Duration) time.Time { return start(n).Add(clock) }
	endOf := func(n int) time.Time { return start(n - 1).Add(-30 * time.Second) }
	day := func(n int) string { return start(n).Format("2006-01-02") }
	t0, t1 := start(60), start(-1) // the window ends at tomorrow's Denver midnight (I1)

	// Lists: alpha (two lists, one brand), bravo, a list tagged with the unattributed key, and a list
	// with no brand.
	listA := h.baList("Alpha", "public", "single")
	listA2 := h.baList("Alpha buyers", "private", "single")
	listB := h.baList("Bravo", "public", "single")
	listU := h.baList("Odd", "private", "single")
	listN := h.baList("No brand", "private", "single")
	h.db.MustExec(`UPDATE lists SET tags = '{brand:alpha,from:alpha <a@x>}' WHERE id IN ($1, $2)`, listA, listA2)
	h.db.MustExec(`UPDATE lists SET tags = '{brand:bravo}' WHERE id = $1`, listB)
	h.db.MustExec(`UPDATE lists SET tags = '{brand:unattributed}' WHERE id = $1`, listU)

	cA := h.baCamp("On A", "finished", 10, listA)
	cB := h.baCamp("On B", "finished", 10, listB)
	cNone := h.baCamp("No list", "finished", 1)
	cU := h.baCamp("On odd", "finished", 1, listU)
	cN := h.baCamp("On no brand", "finished", 1, listN)
	allCamps := []int{cA, cB, cNone, cU, cN}

	s1 := h.baSub("s1@x", "enabled", map[int]string{listA: "confirmed", listB: "confirmed"})
	s2 := h.baSub("s2@x", "enabled", map[int]string{listA: "confirmed", listB: "confirmed"})
	s3 := h.baSub("s3@x", "enabled", map[int]string{listA: "confirmed", listB: "confirmed"})

	var link int
	if err := h.db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://charts.test') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	type clickRow struct {
		camp, sub int // sub 0 = NULL
		at        time.Time
	}
	var rows []clickRow
	click := func(camp, sub int, at time.Time, times int) {
		for i := 0; i < times; i++ {
			h.db.MustExec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, created_at) VALUES ($1, NULLIF($2, 0), $3, $4::TIMESTAMPTZ)`,
				camp, sub, link, at.Format(time.RFC3339))
			rows = append(rows, clickRow{camp, sub, at})
		}
	}
	noon := 12 * time.Hour
	click(cA, s1, dayAt(5, noon), 3)            // a repeat clicker: one pair, three rows
	click(cA, s1, dayAt(3, noon), 1)            // the same pair later: never a second first click
	click(cA, s2, dayAt(65, noon), 1)           // first click before the window...
	click(cA, s2, dayAt(10, noon), 1)           // ...so its in-window click is not a first click
	click(cA, 0, dayAt(5, noon), 2)             // NULL subscriber: raw rows only
	click(cA, s3, dayAt(60, 30*time.Second), 1) // day -60's first moment (in)
	click(cB, s1, dayAt(5, noon), 1)            // the same subscriber on another campaign: its own pair
	click(cB, s3, endOf(61), 1)                 // day -61's last moment (out)
	click(cB, s3, endOf(1), 1)                  // Denver-yesterday's last moment (in; UTC and the session say today+)
	click(cB, s2, dayAt(0, 30*time.Second), 1)  // Denver-today (in, I1)
	click(cA, s3, start(-1), 1)                 // tomorrow's first moment (out; a raw row only)
	click(cNone, s1, dayAt(2, noon), 1)         // a campaign on no list
	// 05:30Z on the UTC date of day -4 is the evening of Denver day -5 (UTC-6/-7).
	click(cNone, s2, time.Date(ty, tm, td-4, 5, 30, 0, 0, time.UTC), 1)

	// wantClicks is the reference count over C in a mode, as 61 zero-filled Denver days.
	wantClicks := func(camps []int, unique bool) []chartPoint {
		inC := map[int]bool{}
		for _, c := range camps {
			inC[c] = true
		}
		in := func(at time.Time) bool { return !at.Before(t0) && at.Before(t1) }
		per := map[string]int{}
		if unique {
			first := map[[2]int]time.Time{}
			for _, r := range rows {
				if !inC[r.camp] || r.sub == 0 {
					continue
				}
				k := [2]int{r.camp, r.sub}
				if f, ok := first[k]; !ok || r.at.Before(f) {
					first[k] = r.at
				}
			}
			for _, f := range first {
				if in(f) {
					per[f.In(den).Format("2006-01-02")]++
				}
			}
		} else {
			for _, r := range rows {
				if inC[r.camp] && in(r.at) {
					per[r.at.In(den).Format("2006-01-02")]++
				}
			}
		}
		out := []chartPoint{}
		for n := 60; n >= 0; n-- {
			out = append(out, chartPoint{day(n), per[day(n)]})
		}
		return out
	}

	// campaign_send_hours (I2) -- today's hours on every kind of campaign, day -1's hours (SES's
	// while the document covers day -1, live once it ends day -2), and tomorrow (never).
	type hourRow struct {
		camp int
		at   time.Time
		n    int
	}
	var hours []hourRow
	sendHour := func(camp int, at time.Time, n int) {
		h.db.MustExec(`INSERT INTO campaign_send_hours (campaign_id, hour, n) VALUES ($1, $2::TIMESTAMPTZ, $3)`, camp, at.Format(time.RFC3339), n)
		hours = append(hours, hourRow{camp, at, n})
	}
	sendHour(cA, start(0), 5)                  // today's first hour
	sendHour(cA, dayAt(0, noon), 7)            // (a future hour on a morning run: still today)
	sendHour(cB, dayAt(0, 2*time.Hour), 11)    // bravo today
	sendHour(cNone, dayAt(0, 3*time.Hour), 13) // on no list: unscoped only
	sendHour(cU, dayAt(0, 4*time.Hour), 19)    // on the unattributed list: unscoped only
	sendHour(cN, dayAt(0, 5*time.Hour), 23)    // on the untagged list: unscoped only
	sendHour(cA, start(0).Add(-time.Hour), 17) // day -1's last hour
	sendHour(cA, dayAt(1, noon), 1000)         // day -1
	sendHour(cA, start(-1), 99)                // tomorrow's first hour (out)
	sendHour(cB, dayAt(61, noon), 5000)        // before the window (never)

	// The ses documents: an older one that must never be read, the latest, and a newer document of
	// another kind. Each daily list carries a day before and a day after the window.
	type sesDay struct {
		Day   string `json:"day"`
		Sends int    `json:"sends"`
	}
	daily := func(base int) []sesDay {
		out := []sesDay{{day(61), 9000}}
		for n := 60; n >= 1; n-- {
			out = append(out, sesDay{day(n), base + n})
		}
		return append(out, sesDay{day(0), 9000})
	}
	latest := map[string]any{"v": 1, "kind": "ses", "brands": []map[string]any{
		{"brand": "alpha", "sends": 1, "daily": daily(100)},
		{"brand": "bravo", "sends": 1, "daily": daily(200)[3:10]}, // only some days
		{"brand": "charlie", "sends": 1, "daily": daily(300)},     // no list in the fixture
		{"brand": "unattributed", "sends": 1, "daily": daily(400)},
		{"brand": "delta", "sends": nil, "daily": nil},
		{"brand": "echo", "sends": nil},
	}}
	older := map[string]any{"v": 1, "kind": "ses", "brands": []map[string]any{
		{"brand": "alpha", "sends": 1, "daily": daily(50000)},
	}}
	health := func(kind string, n int, doc any) {
		b, _ := json.Marshal(doc)
		h.db.MustExec(`INSERT INTO system_health (kind, day, status, doc) VALUES ($1, $2::DATE, 'ok', $3::JSONB)`, kind, day(n), string(b))
	}
	health("ses", 2, older)
	health("ses", 0, latest)
	health("other", -1, map[string]any{"brands": []map[string]any{{"brand": "alpha", "daily": daily(70000)}}})

	// wantSends is the splice (I2): doc's rows for brands summed per day over [day 60, day lf+1]
	// (lf = live_from in days ago), no zero fill, live false; then, when liveOn, the send hours of
	// liveCamps per Denver day over [day lf, today], zero-filled, live true.
	type wantLive struct {
		camps []int
		on    bool
	}
	wantSends := func(doc map[string]any, lf int, lv wantLive, brands ...string) []sendPoint {
		per := map[string]int{}
		var rowsOf []map[string]any
		if doc != nil {
			rowsOf = doc["brands"].([]map[string]any)
		}
		for _, r := range rowsOf {
			want := false
			for _, b := range brands {
				want = want || r["brand"] == b
			}
			d, ok := r["daily"].([]sesDay)
			if !want || !ok {
				continue
			}
			for _, x := range d {
				per[x.Day] += x.Sends
			}
		}
		out := []sendPoint{}
		for n := 60; n > lf; n-- {
			if c, ok := per[day(n)]; ok {
				out = append(out, sendPoint{day(n), c, false})
			}
		}
		if !lv.on {
			return out
		}
		inC := map[int]bool{}
		for _, c := range lv.camps {
			inC[c] = true
		}
		livePer := map[string]int{}
		for _, hr := range hours {
			if inC[hr.camp] && !hr.at.Before(start(lf)) && hr.at.Before(t1) {
				livePer[hr.at.In(den).Format("2006-01-02")] += hr.n
			}
		}
		for n := lf; n >= 0; n-- {
			out = append(out, sendPoint{day(n), livePer[day(n)], true})
		}
		return out
	}

	charts := func(u auth.User, q string) chartsResp {
		t.Helper()
		code, body := baCall(t, u, "/api/dashboard/charts"+q, h.app.GetDashboardCharts)
		if code != http.StatusOK {
			t.Fatalf("charts %q: %d %s", q, code, body)
		}
		var resp struct {
			Data chartsResp `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("charts %q: %s: %v", q, body, err)
		}
		return resp.Data
	}

	admin := baUser([]string{auth.PermListGetAll, auth.PermCampaignsGetAll, auth.PermCampaignsGet})
	scopedAll := baUser([]string{auth.PermCampaignsGetAnalytics, auth.PermCampaignsGetAll}, listA)
	scopedA := baUser([]string{auth.PermCampaignsGetAnalytics}, listA)
	// live = the D3 campaigns whose send hours fill the live days, and whether they are zero-filled.
	cases := []struct {
		name   string
		u      auth.User
		q      string
		camps  []int
		brands []string
		live   wantLive
		scoped bool
	}{
		{"no list_id", admin, "", allCamps, []string{"alpha", "bravo", "charlie", "unattributed"}, wantLive{allCamps, true}, false},
		{"list_id = alpha", admin, fmt.Sprintf("?list_id=%d&list_id=%d", listA, listA2), []int{cA}, []string{"alpha"}, wantLive{[]int{cA}, true}, true},
		{"list_id = bravo", admin, fmt.Sprintf("?list_id=%d", listB), []int{cB}, []string{"bravo"}, wantLive{[]int{cB}, true}, true},
		{"list_id = unattributed-tagged list", admin, fmt.Sprintf("?list_id=%d", listU), []int{cU}, nil, wantLive{}, true},
		{"list_id = no brand", admin, fmt.Sprintf("?list_id=%d", listN), []int{cN}, nil, wantLive{}, true},
		{"list-scoped user", scopedA, "", []int{cA}, []string{"alpha"}, wantLive{[]int{cA}, true}, true},
		// campaigns:get_all widens C to every campaign; the sends -- SES rows and live hours alike --
		// stay the scope's brand (D3).
		{"list-scoped user, get_all", scopedAll, "", allCamps, []string{"alpha"}, wantLive{[]int{cA}, true}, true},
	}

	results := map[bool]map[string]chartsResp{}
	for _, tracking := range []bool{true, false} {
		ko.Set("privacy.individual_tracking", tracking)
		q := prepareQueries(loadQueryMap(t), h.db, ko)
		h.app.core = core.New(&core.Opt{Queries: q, DB: h.db, I18n: h.app.i18n, Log: h.app.log}, &core.Hooks{})
		results[tracking] = map[string]chartsResp{}

		for _, tc := range cases {
			name := fmt.Sprintf("tracking=%v %s", tracking, tc.name)
			got := charts(tc.u, tc.q)
			results[tracking][tc.name] = got
			if got.Scoped != tc.scoped {
				t.Fatalf("%s: scoped %v, want %v", name, got.Scoped, tc.scoped)
			}
			// I3 / TODAY-SO-FAR I1.
			if w := wantClicks(tc.camps, tracking); !reflect.DeepEqual(got.Clicks, w) {
				t.Fatalf("%s: clicks\n got  %s\n want %s", name, baJSON(got.Clicks), baJSON(w))
			}
			if got.Today != day(0) {
				t.Fatalf("%s: today %q, want %s", name, got.Today, day(0))
			}
			// I4 / TODAY-SO-FAR I2 -- the latest document ends day -1 (alpha's), so only today is live.
			if w := wantSends(latest, 0, tc.live, tc.brands...); !reflect.DeepEqual(got.Sends, w) {
				t.Fatalf("%s: sends\n got  %s\n want %s", name, baJSON(got.Sends), baJSON(w))
			}
		}
	}

	// The fixture means something.
	on, off := results[true]["no list_id"], results[false]["no list_id"]
	at := func(pts []chartPoint, d string) int {
		for _, p := range pts {
			if p.Date == d {
				return p.Count
			}
		}
		t.Fatalf("no point for %s in %s", d, baJSON(pts))
		return 0
	}
	atS := func(pts []sendPoint, d string) sendPoint {
		for _, p := range pts {
			if p.Date == d {
				return p
			}
		}
		t.Fatalf("no sends point for %s in %s", d, baJSON(pts))
		return sendPoint{}
	}
	if len(on.Clicks) != 61 || on.Clicks[0].Date != day(60) || on.Clicks[60].Date != day(0) {
		t.Fatalf("fixture: clicks window %s", baJSON(on.Clicks))
	}
	// TODAY-SO-FAR I1: s2's first click on cB is Denver-today (both modes); the tomorrow row never.
	if at(on.Clicks, day(0)) != 1 || at(off.Clicks, day(0)) != 1 {
		t.Fatalf("fixture: today's clicks on %s off %s", baJSON(on.Clicks), baJSON(off.Clicks))
	}
	// ON: s1 on cA and on cB and s2 on cNone (05:30Z, the previous Denver day) at day 5 (3 pairs),
	// s3 at day 60, s1 on cNone at day 2; s2 on cA (first click before the window), s3 on cB (first
	// click day 61) and the NULL rows not at all.
	if at(on.Clicks, day(5)) != 3 || at(on.Clicks, day(4)) != 0 || at(on.Clicks, day(3)) != 0 || at(on.Clicks, day(10)) != 0 ||
		at(on.Clicks, day(60)) != 1 || at(on.Clicks, day(1)) != 0 || at(on.Clicks, day(2)) != 1 {
		t.Fatalf("fixture: unique clicks %s", baJSON(on.Clicks))
	}
	// OFF: day 5 = 3 + 2 NULL + 1 + the 05:30Z row; s3's day-1 click is a raw row in Denver-yesterday
	// (UTC and the UTC+14 session would put it on a later day).
	if at(off.Clicks, day(5)) != 7 || at(off.Clicks, day(4)) != 0 || at(off.Clicks, day(3)) != 1 || at(off.Clicks, day(10)) != 1 ||
		at(off.Clicks, day(60)) != 1 || at(off.Clicks, day(1)) != 1 || at(off.Clicks, day(2)) != 1 {
		t.Fatalf("fixture: raw clicks %s", baJSON(off.Clicks))
	}
	un := results[true]["no list_id"].Sends
	// R-I5: day -61 is dropped and so is the document's day 0 (today is live, TODAY-SO-FAR I2);
	// bravo's daily is only days 58..52: day 55 carries it, day 1 does not. Today = every campaign's
	// today hours (5+7 cA, 11 cB, 13 no list, 19 unattributed list, 23 untagged list).
	todayAll := 5 + 7 + 11 + 13 + 19 + 23
	if len(un) != 61 || atS(un, day(1)) != (sendPoint{day(1), 101 + 301 + 401, false}) || atS(un, day(55)).Count != 155+255+355+455 ||
		atS(un, day(60)).Count != 160+360+460 || un[60] != (sendPoint{day(0), todayAll, true}) {
		t.Fatalf("fixture: unscoped sends %s", baJSON(un))
	}
	if s := results[true]["list_id = alpha"].Sends; len(s) != 61 || atS(s, day(1)).Count != 101 || s[60] != (sendPoint{day(0), 12, true}) {
		t.Fatalf("fixture: alpha sends %s", baJSON(s))
	}
	// Document-wide ses_last: bravo's own rows end at day 52, yet days 51..1 are neither SES nor live.
	if s := results[true]["list_id = bravo"].Sends; len(s) != 8 || s[7] != (sendPoint{day(0), 11, true}) {
		t.Fatalf("fixture: bravo sends %s", baJSON(s))
	}

	// A newer document from a failed SES read (every row's daily null) is skipped: the latest
	// document that carries a daily series is read (Stage 4 F1).
	health("ses", -1, map[string]any{"v": 1, "kind": "ses", "brands": []map[string]any{
		{"brand": "alpha", "sends": nil, "daily": nil},
		{"brand": "unattributed", "sends": nil, "daily": nil},
	}})
	if s := charts(admin, "").Sends; len(s) != 61 || atS(s, day(1)).Count != 101+301+401 {
		t.Fatalf("failed-read document: sends %s", baJSON(s))
	}

	// TODAY-SO-FAR I2 -- a document a day behind (its newest day is -2: the first hour after
	// Mountain midnight, or a failed SES read yesterday): day -1 is live too (cA's 1000 + 17), from
	// listmonk, never a gap; bravo's scope, with no row in this document, has the two live days only.
	behind := map[string]any{"v": 1, "kind": "ses", "brands": []map[string]any{
		{"brand": "alpha", "sends": 1, "daily": daily(100)[:60]}, // days 61..2
		{"brand": "unattributed", "sends": 1, "daily": daily(400)[:60]},
	}}
	health("ses", -2, behind)
	for _, tc := range cases {
		got := charts(tc.u, tc.q)
		if w := wantSends(behind, 1, tc.live, tc.brands...); !reflect.DeepEqual(got.Sends, w) {
			t.Fatalf("behind document, %s: sends\n got  %s\n want %s", tc.name, baJSON(got.Sends), baJSON(w))
		}
	}
	if s := charts(admin, ""); len(s.Sends) != 61 || s.Sends[59] != (sendPoint{day(1), 1017, true}) || s.Sends[58] != (sendPoint{day(2), 102 + 402, false}) {
		t.Fatalf("fixture: behind document sends %s", baJSON(s.Sends))
	}
	if s := charts(admin, fmt.Sprintf("?list_id=%d", listB)).Sends; !reflect.DeepEqual(s, []sendPoint{{day(1), 0, true}, {day(0), 11, true}}) {
		t.Fatalf("fixture: behind document, bravo sends %s", baJSON(s))
	}

	// No ses document at all: only today, live (and the clicks are unaffected); a scope with no
	// branded list still has no sends at all.
	h.db.MustExec(`DELETE FROM system_health WHERE kind = 'ses'`)
	got := charts(admin, "")
	if !reflect.DeepEqual(got.Sends, []sendPoint{{day(0), todayAll, true}}) || len(got.Clicks) != 61 {
		t.Fatalf("no ses document: %s", baJSON(got))
	}
	for _, q := range []string{fmt.Sprintf("?list_id=%d", listN), fmt.Sprintf("?list_id=%d", listU)} {
		if s := charts(admin, q).Sends; s == nil || len(s) != 0 {
			t.Fatalf("no ses document, %s: sends %s, want []", q, baJSON(s))
		}
	}
	ko.Set("privacy.individual_tracking", true)
}
