package main

// Fork (global language filter) -- integrations GLOBAL-LANG-SPEC I8, I9, I10 and I13 against a real
// database, through the handlers (newLinkHarness, LISTMONK_TEST_PG opt-in; a plain go test skips).
//   I8  TestDashboardLangPartition -- for messages, views, clicks, bounces, complaints, hard bounces,
//       conversions and the campaign totals, the five languages' values plus the no-language
//       campaigns' equal the All value; a no-language campaign is counted under no language.
//   I9  TestLangSendsSource -- under lang, the charts' and the brand matrix's sends come only from
//       campaign_send_hours of live_camps ∩ the lang campaigns over the whole window, every point
//       live, zero-filled (live_on kept), sends_source "listmonk"; without lang the SES splice is
//       unchanged ("ses"). A list-scoped campaigns:get_all user's out-of-scope campaign's sends are
//       never counted.
//   I10 TestBrandMatrixLang -- subscriber segments by send language (EN includes none), C and
//       Tracked/Matched by the campaign rule; GP, the health chip, usage and thresholds unchanged.
//   I13 TestDashboardLangScope -- lang never widens a scope: a list-scoped user's counts, clients,
//       countries and brand matrix under lang return only in-scope campaigns and brands.
// Every expected value is computed in Go from the fixture table below, never read back from SQL.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
)

type langCamp struct {
	name      string
	lang      string // "" = no language
	list      string // "A", "B" or "" (no list)
	status    string
	sent      int
	views     []string // subscriber keys, one view each
	clicks    []string
	bounces   []string // bounce types, one row each on the first view subscriber
	tracked   float64  // a tracked conversion's sale (0 = none)
	sendHours map[int]int
}

type langFixture struct {
	h       *linkHarness
	listA   int
	listA2  int
	listB   int
	camps   []langCamp
	campIDs map[string]int
	day     func(n int) string
	start   func(n int) time.Time
}

func newLangFixture(t *testing.T) *langFixture {
	h := newLinkHarness(t)
	f := &langFixture{h: h, campIDs: map[string]int{}}

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
	f.start = func(n int) time.Time { return time.Date(ty, tm, td-n, 0, 0, 0, 0, den) }
	f.day = func(n int) string { return f.start(n).Format("2006-01-02") }

	f.listA = h.baList("Alpha", "public", "single")
	f.listA2 = h.baList("Alpha buyers", "private", "single")
	f.listB = h.baList("Bravo", "public", "single")
	h.db.MustExec(`UPDATE lists SET tags = '{brand:alpha}' WHERE id IN ($1, $2)`, f.listA, f.listA2)
	h.db.MustExec(`UPDATE lists SET tags = '{brand:bravo}' WHERE id = $1`, f.listB)
	lists := map[string]int{"A": f.listA, "B": f.listB}

	// Subscribers and their stored language ("" = none). a* on Alpha's public list, b* on Bravo's.
	subLang := map[string]string{"a1": "en", "a2": "", "a3": "fr", "a4": "es", "a5": "pt", "a6": "fr-CA", "b1": "es", "b2": "EN"}
	subs := map[string]int{}
	for _, k := range []string{"a1", "a2", "a3", "a4", "a5", "a6", "b1", "b2"} {
		l := f.listA
		if k[0] == 'b' {
			l = f.listB
		}
		subs[k] = h.baSub(k+"@lang.test", "enabled", map[int]string{l: "confirmed"})
		if subLang[k] != "" {
			h.db.MustExec(`UPDATE subscribers SET attribs = JSONB_BUILD_OBJECT('lang', $2::TEXT) WHERE id = $1`, subs[k], subLang[k])
		}
	}

	f.camps = []langCamp{
		{name: "EN on A", lang: "en", list: "A", status: "finished", sent: 10, views: []string{"a1"}, clicks: []string{"a1"}, bounces: []string{"hard"}, tracked: 10, sendHours: map[int]int{3: 100}},
		{name: "FR on A", lang: "fr", list: "A", status: "finished", sent: 20, views: []string{"a3", "a6"}, clicks: []string{"a3", "a6"}, bounces: []string{"complaint"}, tracked: 20, sendHours: map[int]int{3: 200, 0: 9}},
		{name: "None on A", lang: "", list: "A", status: "finished", sent: 5, views: []string{"a1", "a2", "a5"}, clicks: []string{"a2"}, bounces: []string{"soft", "hard"}, tracked: 5, sendHours: map[int]int{2: 50}},
		{name: "ES on B", lang: "es", list: "B", status: "running", sent: 40, views: []string{"b1"}, bounces: []string{"hard"}, tracked: 40, sendHours: map[int]int{1: 400}},
		{name: "FR on B", lang: "fr", list: "B", status: "paused", sent: 7, views: []string{"b2"}, clicks: []string{"b2"}},
		{name: "FR on no list", lang: "fr", list: "", status: "finished", sent: 3, views: []string{"a1"}, clicks: []string{"a1"}, sendHours: map[int]int{2: 70}},
		{name: "DE draft on A", lang: "de", list: "A", status: "draft"},
	}

	var link int
	if err := h.db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://lang.test') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	yesterdayNoon := f.start(1).Add(12 * time.Hour).Format(time.RFC3339)
	for i, c := range f.camps {
		var ls []int
		if c.list != "" {
			ls = append(ls, lists[c.list])
		}
		id := h.baCamp(c.name, c.status, c.sent, ls...)
		f.campIDs[c.name] = id
		if c.lang != "" {
			h.db.MustExec(`UPDATE campaigns SET attribs = JSONB_BUILD_OBJECT('lang', $2::TEXT) WHERE id = $1`, id, c.lang)
		}
		for _, s := range c.views {
			h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id, created_at, client, country) VALUES ($1, $2, $3::TIMESTAMPTZ, 'gmail', 'US')`, id, subs[s], yesterdayNoon)
		}
		for _, s := range c.clicks {
			h.db.MustExec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, created_at, client, country) VALUES ($1, $2, $3, $4::TIMESTAMPTZ, 'gmail', 'US')`, id, subs[s], link, yesterdayNoon)
		}
		for _, bt := range c.bounces {
			h.db.MustExec(`INSERT INTO bounces (subscriber_id, campaign_id, type, created_at) VALUES ($1, $2, $3::bounce_type, $4::TIMESTAMPTZ)`, subs[c.views[0]], id, bt, yesterdayNoon)
		}
		if c.tracked > 0 {
			brand := map[string]string{"A": "alpha", "B": "bravo"}[c.list]
			h.db.MustExec(`INSERT INTO campaign_conversions (measure, order_id, campaign_id, brand, purchased_at, sales_usd) VALUES ('tracked', $1, $2, $3, $4::TIMESTAMPTZ, $5)`,
				fmt.Sprintf("o%d", i), id, brand, yesterdayNoon, c.tracked)
		}
		for n, v := range c.sendHours {
			h.db.MustExec(`INSERT INTO campaign_send_hours (campaign_id, hour, n) VALUES ($1, $2::TIMESTAMPTZ, $3)`, id, f.start(n).Add(time.Hour).Format(time.RFC3339), v)
		}
	}

	// A SES document whose daily series covers every day before today, for both brands, so any
	// read that used SES would carry live:false points and SES counts.
	type sesDay struct {
		Day   string `json:"day"`
		Sends int    `json:"sends"`
	}
	daily := func(base int) []sesDay {
		var out []sesDay
		for n := 60; n >= 1; n-- {
			out = append(out, sesDay{f.day(n), base + n})
		}
		return out
	}
	doc, _ := json.Marshal(map[string]any{"v": 1, "kind": "ses", "brands": []map[string]any{
		{"brand": "alpha", "sends": 1, "daily": daily(10000)},
		{"brand": "bravo", "sends": 1, "daily": daily(20000)},
	}})
	h.db.MustExec(`INSERT INTO system_health (kind, day, status, doc) VALUES ('ses', $1::DATE, 'ok', $2::JSONB)`, f.day(0), string(doc))

	// Alpha's health document: GP and the chip, usage and thresholds (D6: unchanged under a language).
	hdoc := `{"inputs": {"spamRate": {"status": "warn", "detail": {"value": 0.002}}}, "usage": {"score": 50}, "thresholds": {"clickRate": {"warn": 0.01}}}`
	h.db.MustExec(`INSERT INTO brand_health (brand, day, status, doc) VALUES ('alpha', $1::DATE, 'warn', $2::JSONB)`, f.day(0), hdoc)
	return f
}

// langData GETs a dashboard endpoint as u and returns its data object.
func (f *langFixture) langData(t *testing.T, u auth.User, path, q string) map[string]any {
	t.Helper()
	handlers := map[string]func(auth.User, string) (int, string){
		"counts":       func(u auth.User, p string) (int, string) { return baCall(t, u, p, f.h.app.GetDashboardCounts) },
		"charts":       func(u auth.User, p string) (int, string) { return baCall(t, u, p, f.h.app.GetDashboardCharts) },
		"brand-matrix": func(u auth.User, p string) (int, string) { return baCall(t, u, p, f.h.app.GetDashboardBrandMatrix) },
		"clients":      func(u auth.User, p string) (int, string) { return baCall(t, u, p, f.h.app.GetDashboardClients) },
		"countries":    func(u auth.User, p string) (int, string) { return baCall(t, u, p, f.h.app.GetDashboardCountries) },
	}
	code, body := handlers[path](u, "/api/dashboard/"+path+q)
	if code != http.StatusOK {
		t.Fatalf("%s%s: %d %s", path, q, code, body)
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("%s%s: %s: %v", path, q, body, err)
	}
	return resp.Data
}

func langNum(m map[string]any, path ...string) float64 {
	var cur any = m
	for _, k := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return -1
		}
		cur = mm[k]
	}
	n, ok := cur.(float64)
	if !ok {
		return -1
	}
	return n
}

// countsOf is the Go reference: the campaign-level numbers of the campaigns pick selects.
type langCounts struct {
	campaigns, messages, views, clicks, bounces, complaints, hard, tracked float64
}

func (f *langFixture) expect(pick func(langCamp) bool) langCounts {
	var o langCounts
	for _, c := range f.camps {
		if !pick(c) {
			continue
		}
		o.campaigns++
		o.messages += float64(c.sent)
		o.views += float64(len(c.views))
		o.clicks += float64(len(c.clicks))
		for _, b := range c.bounces {
			o.bounces++
			if b == "complaint" {
				o.complaints++
			}
			if b == "hard" {
				o.hard++
			}
		}
		if c.tracked > 0 {
			o.tracked++
		}
	}
	return o
}

func countsFrom(d map[string]any) langCounts {
	return langCounts{
		campaigns:  langNum(d, "campaigns", "total"),
		messages:   langNum(d, "messages"),
		views:      langNum(d, "engagement", "views"),
		clicks:     langNum(d, "engagement", "clicks"),
		bounces:    langNum(d, "engagement", "bounces"),
		complaints: langNum(d, "engagement", "complaints"),
		hard:       langNum(d, "engagement", "hard_bounces"),
		tracked:    langNum(d, "conversions", "tracked", "orders"),
	}
}

func (a langCounts) plus(b langCounts) langCounts {
	return langCounts{a.campaigns + b.campaigns, a.messages + b.messages, a.views + b.views, a.clicks + b.clicks,
		a.bounces + b.bounces, a.complaints + b.complaints, a.hard + b.hard, a.tracked + b.tracked}
}

var langCodes = []string{"en", "es", "fr", "de", "it"}

func TestDashboardLangPartition(t *testing.T) {
	f := newLangFixture(t)
	admin := baUser([]string{auth.PermListGetAll, auth.PermCampaignsGetAll, auth.PermCampaignsGet})

	scopes := []struct {
		name string
		q    string
		in   func(langCamp) bool
	}{
		{"unscoped", "", func(langCamp) bool { return true }},
		{"list_id=A", fmt.Sprintf("?list_id=%d", f.listA), func(c langCamp) bool { return c.list == "A" }},
		{"list_id=A,B", fmt.Sprintf("?list_id=%d&list_id=%d", f.listA, f.listB), func(c langCamp) bool { return c.list != "" }},
	}
	for _, sc := range scopes {
		sep := "?"
		if sc.q != "" {
			sep = "&"
		}
		all := countsFrom(f.langData(t, admin, "counts", sc.q))
		if want := f.expect(sc.in); all != want {
			t.Fatalf("%s All: got %+v, want %+v", sc.name, all, want)
		}
		var sum langCounts
		for _, l := range langCodes {
			l := l
			got := countsFrom(f.langData(t, admin, "counts", sc.q+sep+"lang="+l))
			want := f.expect(func(c langCamp) bool { return sc.in(c) && c.lang == l })
			if got != want {
				t.Fatalf("%s lang=%s: got %+v, want %+v", sc.name, l, got, want)
			}
			sum = sum.plus(got)
		}
		none := f.expect(func(c langCamp) bool { return sc.in(c) && c.lang == "" })
		if none.messages == 0 || none.views == 0 {
			t.Fatalf("%s: fixture has no no-language campaign in scope", sc.name)
		}
		if got := sum.plus(none); got != all {
			t.Fatalf("%s: five languages + no-language = %+v, All = %+v", sc.name, got, all)
		}
		// lang= (empty) is All.
		if got := countsFrom(f.langData(t, admin, "counts", sc.q+sep+"lang=")); got != all {
			t.Fatalf("%s lang=: got %+v, want All %+v", sc.name, got, all)
		}
	}

	// The subscriber grid and the lists counts are unchanged by lang (the client reads the line).
	a := f.langData(t, admin, "counts", "")
	fr := f.langData(t, admin, "counts", "?lang=fr")
	for _, k := range []string{"subscribers", "lists"} {
		if !reflect.DeepEqual(a[k], fr[k]) {
			t.Fatalf("%s changed under lang=fr:\n all %s\n fr  %s", k, baJSON(a[k]), baJSON(fr[k]))
		}
	}
	if a["scoped"] != fr["scoped"] {
		t.Fatal("scoped changed under lang")
	}
}

func TestLangSendsSource(t *testing.T) {
	f := newLangFixture(t)
	admin := baUser([]string{auth.PermListGetAll, auth.PermCampaignsGetAll, auth.PermCampaignsGet})
	// A list-scoped user on Alpha holding campaigns:get_all: C widens to every campaign (counts),
	// but the sends must stay inside the scope (live_camps, never camps).
	scopedGetAll := baUser([]string{auth.PermCampaignsGetAnalytics, auth.PermCampaignsGetAll}, f.listA)

	type point struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
		Live  bool   `json:"live"`
	}
	sends := func(d map[string]any) []point {
		var out []point
		b, _ := json.Marshal(d["sends"])
		_ = json.Unmarshal(b, &out)
		return out
	}
	// want: 61 live zero-filled days of the send hours of the campaigns pick selects.
	want := func(pick func(langCamp) bool) []point {
		per := map[int]int{}
		for _, c := range f.camps {
			if pick(c) {
				for n, v := range c.sendHours {
					per[n] += v
				}
			}
		}
		var out []point
		for n := 60; n >= 0; n-- {
			out = append(out, point{f.day(n), per[n], true})
		}
		return out
	}

	// Without lang: the SES splice, sends_source "ses", SES days present (live false).
	all := f.langData(t, admin, "charts", "")
	if all["sends_source"] != "ses" {
		t.Fatalf("no lang: sends_source %v, want ses", all["sends_source"])
	}
	sesDays := 0
	for _, p := range sends(all) {
		if !p.Live {
			sesDays++
		}
	}
	if sesDays != 60 {
		t.Fatalf("no lang: %d SES days, want 60 (the splice unchanged)", sesDays)
	}

	cases := []struct {
		name string
		u    auth.User
		q    string
		pick func(langCamp) bool
	}{
		{"unscoped fr", admin, "?lang=fr", func(c langCamp) bool { return c.lang == "fr" }},
		{"unscoped en", admin, "?lang=en", func(c langCamp) bool { return c.lang == "en" }},
		{"list_id=A fr", admin, fmt.Sprintf("?list_id=%d&lang=fr", f.listA), func(c langCamp) bool { return c.lang == "fr" && c.list == "A" }},
		{"list_id=B fr (no hours: zero-filled)", admin, fmt.Sprintf("?list_id=%d&lang=fr", f.listB), func(c langCamp) bool { return c.lang == "fr" && c.list == "B" }},
		// The get_all user's out-of-scope "FR on no list" (70 on day 2) is never counted.
		{"scoped get_all fr", scopedGetAll, "?lang=fr", func(c langCamp) bool { return c.lang == "fr" && c.list == "A" }},
	}
	for _, tc := range cases {
		d := f.langData(t, tc.u, "charts", tc.q)
		if d["sends_source"] != "listmonk" {
			t.Fatalf("%s: sends_source %v, want listmonk", tc.name, d["sends_source"])
		}
		if got, w := sends(d), want(tc.pick); !reflect.DeepEqual(got, w) {
			t.Fatalf("%s: sends\n got  %v\n want %v", tc.name, got, w)
		}
	}
	// The scoped get_all case really is widened for counts -- the sends rule is what keeps it in.
	if got := langNum(f.langData(t, scopedGetAll, "counts", "?lang=fr"), "messages"); got != 30 {
		t.Fatalf("scoped get_all counts lang=fr: messages %v, want 30 (20 + 7 + 3: get_all widens C)", got)
	}

	// Live_on kept: a scope with no branded list has no live points at all, under lang too.
	listN := f.h.baList("No brand", "private", "single")
	if got := sends(f.langData(t, admin, "charts", fmt.Sprintf("?list_id=%d&lang=fr", listN))); len(got) != 0 {
		t.Fatalf("unbranded scope lang=fr: %d points, want none (live_on)", len(got))
	}

	// The brand matrix: sent under lang = the brand's lang campaigns' send hours, never SES.
	m := f.langData(t, admin, "brand-matrix", "?lang=fr")
	if m["sends_source"] != "listmonk" {
		t.Fatalf("matrix lang=fr: sends_source %v", m["sends_source"])
	}
	if got := f.langData(t, admin, "brand-matrix", "")["sends_source"]; got != "ses" {
		t.Fatalf("matrix no lang: sends_source %v", got)
	}
	sent := map[string]any{}
	for _, r := range m["rows"].([]any) {
		row := r.(map[string]any)
		sent[row["brand"].(string)] = row["sent"]
	}
	if sent["alpha"] != float64(209) {
		t.Fatalf("matrix lang=fr alpha sent %v, want 209 (FR on A's hours, no SES day)", sent["alpha"])
	}
	if sent["bravo"] != nil {
		t.Fatalf("matrix lang=fr bravo sent %v, want null (no hours, SES never used)", sent["bravo"])
	}
}

func TestBrandMatrixLang(t *testing.T) {
	f := newLangFixture(t)
	admin := baUser([]string{auth.PermListGetAll, auth.PermCampaignsGetAll, auth.PermCampaignsGet})

	rows := func(q string) map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, r := range f.langData(t, admin, "brand-matrix", q)["rows"].([]any) {
			row := r.(map[string]any)
			out[row["brand"].(string)] = row
		}
		return out
	}
	all := rows("")

	// Subscribers by send language: Alpha's public list a1 en, a2 none, a3 fr, a4 es, a5 pt (other),
	// a6 fr-CA (fr); Bravo's b1 es, b2 EN (en).
	wantSubs := map[string]map[string]float64{
		"en": {"alpha": 2, "bravo": 1}, // EN includes no-language
		"fr": {"alpha": 2, "bravo": 0},
		"es": {"alpha": 1, "bravo": 1},
		"de": {"alpha": 0, "bravo": 0},
	}
	for l, per := range wantSubs {
		r := rows("?lang=" + l)
		for b, n := range per {
			if got := langNum(r[b], "subscribers", "total"); got != n {
				t.Fatalf("lang=%s %s subscribers.total %v, want %v", l, b, got, n)
			}
		}
		// C, Tracked by the campaign rule.
		for b, list := range map[string]string{"alpha": "A", "bravo": "B"} {
			w := f.expect(func(c langCamp) bool { return c.list == list && c.lang == l })
			if got := langNum(r[b], "views"); got != w.views {
				t.Fatalf("lang=%s %s views %v, want %v", l, b, got, w.views)
			}
			if got := langNum(r[b], "clicks"); got != w.clicks {
				t.Fatalf("lang=%s %s clicks %v, want %v", l, b, got, w.clicks)
			}
			if got := langNum(r[b], "complaints"); got != w.complaints {
				t.Fatalf("lang=%s %s complaints %v, want %v", l, b, got, w.complaints)
			}
			if got := langNum(r[b], "hard_bounces"); got != w.hard {
				t.Fatalf("lang=%s %s hard_bounces %v, want %v", l, b, got, w.hard)
			}
			if got := langNum(r[b], "tracked", "orders"); got != w.tracked {
				t.Fatalf("lang=%s %s tracked %v, want %v", l, b, got, w.tracked)
			}
		}
		// GP, the chip, usage and thresholds are brand-level: unchanged.
		for _, k := range []string{"health", "usage", "thresholds"} {
			if !reflect.DeepEqual(r["alpha"][k], all["alpha"][k]) {
				t.Fatalf("lang=%s alpha %s changed: %s vs %s", l, k, baJSON(r["alpha"][k]), baJSON(all["alpha"][k]))
			}
		}
		if all["alpha"]["health"] == nil || all["alpha"]["usage"] == nil {
			t.Fatal("fixture: alpha has no health / usage")
		}
	}
	// The no-language campaign counts under All only.
	wAll := f.expect(func(c langCamp) bool { return c.list == "A" })
	if got := langNum(all["alpha"], "views"); got != wAll.views {
		t.Fatalf("All alpha views %v, want %v", got, wAll.views)
	}
}

func TestDashboardLangScope(t *testing.T) {
	f := newLangFixture(t)
	scopedA := baUser([]string{auth.PermCampaignsGetAnalytics}, f.listA)

	// Counts: only the in-scope FR campaign (FR on B and FR on no list are out of scope).
	fr := countsFrom(f.langData(t, scopedA, "counts", "?lang=fr"))
	if want := f.expect(func(c langCamp) bool { return c.list == "A" && c.lang == "fr" }); fr != want {
		t.Fatalf("scoped A lang=fr: got %+v, want %+v", fr, want)
	}
	// A list_id outside the permission stays empty under lang.
	out := countsFrom(f.langData(t, scopedA, "counts", fmt.Sprintf("?list_id=%d&lang=es", f.listB)))
	if out != (langCounts{}) {
		t.Fatalf("scoped A, list_id=B lang=es: got %+v, want zero", out)
	}

	// Clients and countries: only the in-scope FR views (2).
	for _, p := range []string{"clients", "countries"} {
		d := f.langData(t, scopedA, p, "?lang=fr")
		key := map[string]string{"clients": "clients", "countries": "countries"}[p]
		var views float64
		for _, r := range d[key].([]any) {
			views += langNum(r.(map[string]any), "views")
		}
		if views != 2 || d["scoped"] != true {
			t.Fatalf("scoped A %s lang=fr: %v views (scoped %v), want 2", p, views, d["scoped"])
		}
	}

	// Brand matrix: only alpha's row, under every language.
	for _, l := range langCodes {
		d := f.langData(t, scopedA, "brand-matrix", "?lang="+l)
		var brands []string
		for _, r := range d["rows"].([]any) {
			brands = append(brands, r.(map[string]any)["brand"].(string))
		}
		if !reflect.DeepEqual(brands, []string{"alpha"}) {
			t.Fatalf("scoped A brand-matrix lang=%s: brands %v, want [alpha]", l, brands)
		}
	}
}
