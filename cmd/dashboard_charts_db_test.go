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

type chartsResp struct {
	Sends  []chartPoint `json:"sends"`
	Clicks []chartPoint `json:"clicks"`
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
	t0, t1 := start(60), start(0)

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
	allCamps := []int{cA, cB, cNone}

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
	click(cB, s2, dayAt(0, 30*time.Second), 1)  // Denver-today (out)
	click(cNone, s1, dayAt(2, noon), 1)         // a campaign on no list
	// 05:30Z on the UTC date of day -4 is the evening of Denver day -5 (UTC-6/-7).
	click(cNone, s2, time.Date(ty, tm, td-4, 5, 30, 0, 0, time.UTC), 1)

	// wantClicks is the reference count over C in a mode, as 60 zero-filled Denver days.
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
		for n := 60; n >= 1; n-- {
			out = append(out, chartPoint{day(n), per[day(n)]})
		}
		return out
	}

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

	// wantSends sums the latest document's rows for brands, inside the window, no zero fill.
	wantSends := func(brands ...string) []chartPoint {
		per := map[string]int{}
		for _, r := range latest["brands"].([]map[string]any) {
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
		out := []chartPoint{}
		for n := 60; n >= 1; n-- {
			if c, ok := per[day(n)]; ok {
				out = append(out, chartPoint{day(n), c})
			}
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
	cases := []struct {
		name   string
		u      auth.User
		q      string
		camps  []int
		brands []string
		scoped bool
	}{
		{"no list_id", admin, "", allCamps, []string{"alpha", "bravo", "charlie", "unattributed"}, false},
		{"list_id = alpha", admin, fmt.Sprintf("?list_id=%d&list_id=%d", listA, listA2), []int{cA}, []string{"alpha"}, true},
		{"list_id = bravo", admin, fmt.Sprintf("?list_id=%d", listB), []int{cB}, []string{"bravo"}, true},
		{"list_id = unattributed-tagged list", admin, fmt.Sprintf("?list_id=%d", listU), []int{}, nil, true},
		{"list_id = no brand", admin, fmt.Sprintf("?list_id=%d", listN), []int{}, nil, true},
		{"list-scoped user", scopedA, "", []int{cA}, []string{"alpha"}, true},
		// campaigns:get_all widens C to every campaign; the sends stay the scope's brand.
		{"list-scoped user, get_all", scopedAll, "", allCamps, []string{"alpha"}, true},
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
			// I3.
			if w := wantClicks(tc.camps, tracking); !reflect.DeepEqual(got.Clicks, w) {
				t.Fatalf("%s: clicks\n got  %s\n want %s", name, baJSON(got.Clicks), baJSON(w))
			}
			// I4.
			if w := wantSends(tc.brands...); !reflect.DeepEqual(got.Sends, w) {
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
	if len(on.Clicks) != 60 || on.Clicks[0].Date != day(60) || on.Clicks[59].Date != day(1) {
		t.Fatalf("fixture: clicks window %s", baJSON(on.Clicks))
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
	// R-I5: days -61 and 0 are dropped; bravo's daily is only days 58..52: day 55 carries it, day 1
	// does not.
	if len(un) != 60 || at(un, day(1)) != 101+301+401 || at(un, day(55)) != 155+255+355+455 || at(un, day(60)) != 160+360+460 {
		t.Fatalf("fixture: unscoped sends %s", baJSON(un))
	}
	if s := results[true]["list_id = alpha"].Sends; len(s) != 60 || at(s, day(1)) != 101 {
		t.Fatalf("fixture: alpha sends %s", baJSON(s))
	}

	// A newer document from a failed SES read (every row's daily null) is skipped: the latest
	// document that carries a daily series is read (Stage 4 F1).
	health("ses", -1, map[string]any{"v": 1, "kind": "ses", "brands": []map[string]any{
		{"brand": "alpha", "sends": nil, "daily": nil},
		{"brand": "unattributed", "sends": nil, "daily": nil},
	}})
	if s := charts(admin, "").Sends; len(s) != 60 || at(s, day(1)) != 101+301+401 {
		t.Fatalf("failed-read document: sends %s", baJSON(s))
	}

	// No ses document at all: sends [] (and the clicks are unaffected).
	h.db.MustExec(`DELETE FROM system_health WHERE kind = 'ses'`)
	got := charts(admin, "")
	if got.Sends == nil || len(got.Sends) != 0 || len(got.Clicks) != 60 {
		t.Fatalf("no ses document: %s", baJSON(got))
	}
	ko.Set("privacy.individual_tracking", true)
}
