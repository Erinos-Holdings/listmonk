package main

// Fork (brand matrix) -- integrations BRAND-MATRIX-SPEC against a real database, through the
// handlers (LISTMONK_TEST_PG opt-in; plain go test skips, the suite's documented shape), plus I4's
// permission gate, which needs no database.
//
// I1: counts.engagement.bounces keeps the every-bounce-row definition and hard_bounces is an added
// field -- hard, soft AND complaint bounces are seeded, so bounces = hard + soft + complaints and
// hard_bounces = hard (an expression leaking soft rows fails).
// D3/D5/D7: the matrix rows -- one per brand of the scope's ACTIVE lists, the public lists'
// segments, clicks over the brand's campaigns and the 60 Denver days ending yesterday in BOTH
// tracking modes (each statement prepared through prepareQueries as cmd/init.go does at boot),
// hard bounces / the block class (the classPredicate("hard","block") mirror, I2's fork half) /
// complaints in the window, sent from the latest ses document's in-window daily rows, and the
// brand's own latest brand_health row (status, spamRate.detail.value) with no default fallback;
// GP-SPAM-VISIBILITY-SPEC I2: that row's spamRate.status and the NEEDS_WORK USER_REPORTED_SPAM_RATE
// compliance flag projected verbatim (spam_status, spam_flagged), absent when not stored.
// I4: GET /api/dashboard/brand-matrix is refused (403) without brands:get.
// BRAND-USAGE-SPEC I1: the brand's latest doc's `usage` comes back on its row equal to the stored
// value (after the transport's JSON compaction); a doc without it, or no doc, yields no usage key.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/labstack/echo/v4"
)

type matrixSubs struct {
	Total        int `json:"total"`
	Active       int `json:"active"`
	Held         int `json:"held"`
	Unsubscribed int `json:"unsubscribed"`
	Blocked      int `json:"blocked"`
}

type matrixHealth struct {
	Status      string   `json:"status,omitempty"`
	SpamRate    *float64 `json:"spam_rate,omitempty"`
	SpamStatus  string   `json:"spam_status,omitempty"`
	SpamFlagged *bool    `json:"spam_flagged,omitempty"`
}

type matrixRow struct {
	Brand       string       `json:"brand"`
	Subscribers matrixSubs   `json:"subscribers"`
	Sent        *int         `json:"sent"`
	Clicks      int          `json:"clicks"`
	HardBounces int          `json:"hard_bounces"`
	Blocks      int          `json:"blocks"`
	Complaints  int          `json:"complaints"`
	Health      matrixHealth `json:"health"`
	// BRAND-USAGE-SPEC I1 -- the stored doc's usage verbatim; nil when the key is absent.
	Usage json.RawMessage `json:"usage,omitempty"`
}

type matrixResp struct {
	Rows   []matrixRow `json:"rows"`
	Scoped bool        `json:"scoped"`
}

// TestDashboardBrandMatrixPerm is I4: the route is registered behind brands:get through the pm
// wrapper, and that wrapper refuses a user without it before the handler runs.
func TestDashboardBrandMatrixPerm(t *testing.T) {
	b, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`g\.GET\("/api/dashboard/brand-matrix", pm\(a\.GetDashboardBrandMatrix, "brands:get"\)\)`).Match(b) {
		t.Fatal(`cmd/handlers.go does not register /api/dashboard/brand-matrix as pm(a.GetDashboardBrandMatrix, "brands:get")`)
	}

	reached := false
	next := func(c echo.Context) error { reached = true; return nil }
	gate := (&auth.Auth{}).Perm(next, "brands:get")
	u := baUser([]string{auth.PermListGetAll, auth.PermCampaignsGetAll, auth.PermCampaignsGetAnalytics})
	code, _ := baCall(t, u, "/api/dashboard/brand-matrix", gate)
	if code != http.StatusForbidden || reached {
		t.Fatalf("without brands:get: %d (handler reached %v), want 403 before the handler", code, reached)
	}
	code, _ = baCall(t, baUser([]string{"brands:get"}), "/api/dashboard/brand-matrix", gate)
	if code != http.StatusOK || !reached {
		t.Fatalf("with brands:get: %d (handler reached %v), want the handler", code, reached)
	}
}

func TestDashboardBrandMatrixDB(t *testing.T) {
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
	dayAt := func(n int, clock time.Duration) time.Time { return start(n).Add(clock) }
	endOf := func(n int) time.Time { return start(n - 1).Add(-30 * time.Second) }
	day := func(n int) string { return start(n).Format("2006-01-02") }
	noon := 12 * time.Hour

	// Lists: alpha (a public list, a private derivative, an ARCHIVED public list), bravo (public,
	// double opt-in), delta (a private list, nothing else), a list tagged with the unattributed key
	// and an untagged list.
	pubA := h.baList("Alpha", "public", "single")
	privA := h.baList("Alpha buyers", "private", "single")
	arcA := h.baList("Alpha old", "public", "single")
	pubB := h.baList("Bravo", "public", "double")
	listD := h.baList("Delta", "private", "single")
	listU := h.baList("Odd", "private", "single")
	listN := h.baList("No brand", "public", "single")
	h.db.MustExec(`UPDATE lists SET tags = '{brand:alpha,from:alpha <a@x>}' WHERE id IN ($1, $2)`, pubA, privA)
	h.db.MustExec(`UPDATE lists SET tags = '{brand:alpha}', status = 'archived' WHERE id = $1`, arcA)
	h.db.MustExec(`UPDATE lists SET tags = '{brand:bravo}' WHERE id = $1`, pubB)
	h.db.MustExec(`UPDATE lists SET tags = '{brand:delta}' WHERE id = $1`, listD)
	h.db.MustExec(`UPDATE lists SET tags = '{brand:unattributed}' WHERE id = $1`, listU)

	// Subscribers: on alpha's public list one per shown segment; bravo one active and one pending
	// (pending is in total, not a column -- stage 2 F4); a private-only, an archived-only and an
	// untagged-only member count nowhere.
	s1 := h.baSub("s1@x", "enabled", map[int]string{pubA: "confirmed", pubB: "confirmed", privA: "confirmed", listN: "confirmed"})
	sHeld := h.baSub("held@x", "enabled", map[int]string{pubA: "unsubscribed"})
	h.db.MustExec(`UPDATE subscriber_lists SET meta = '{"hold": {"reason": "sunset"}}' WHERE subscriber_id = $1`, sHeld)
	h.baSub("unsub@x", "enabled", map[int]string{pubA: "unsubscribed"})
	sBlock := h.baSub("block@x", "blocklisted", map[int]string{pubA: "unsubscribed"})
	sPend := h.baSub("pend@x", "enabled", map[int]string{pubB: "unconfirmed"})
	s2 := h.baSub("s2@x", "enabled", map[int]string{privA: "confirmed"})
	s3 := h.baSub("s3@x", "enabled", map[int]string{arcA: "confirmed"})

	cA := h.baCamp("On A", "finished", 10, pubA)
	cPriv := h.baCamp("On A buyers", "finished", 4, privA)
	cArc := h.baCamp("On A old", "finished", 3, arcA)
	cB := h.baCamp("On B", "finished", 6, pubB)
	cNone := h.baCamp("No list", "finished", 2)
	cN := h.baCamp("On no brand", "finished", 2, listN)

	var link int
	if err := h.db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://matrix.test') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	click := func(camp, sub int, at time.Time, times int) {
		for i := 0; i < times; i++ {
			h.db.MustExec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, created_at) VALUES ($1, NULLIF($2, 0), $3, $4::TIMESTAMPTZ)`,
				camp, sub, link, at.Format(time.RFC3339))
		}
	}
	click(cA, s1, dayAt(5, noon), 3)    // one pair, three rows
	click(cA, s2, dayAt(65, noon), 1)   // first click before the window...
	click(cA, s2, dayAt(10, noon), 1)   // ...so this in-window row is no first click
	click(cA, 0, dayAt(5, noon), 2)     // NULL subscriber: raw rows only
	click(cPriv, s1, dayAt(2, noon), 1) // the private derivative's campaign is the brand's
	click(cArc, s3, dayAt(3, noon), 1)  // an archived list forms no brand
	click(cB, s3, dayAt(0, noon), 1)    // Denver-today (out)...
	click(cB, s3, endOf(1), 1)          // ...but yesterday's last moment is in, and the first click
	click(cB, s1, endOf(61), 1)         // day -61 (out)
	click(cNone, s1, dayAt(5, noon), 1) // a campaign on no list
	click(cN, s1, dayAt(5, noon), 1)    // a campaign on an untagged list
	wantClicks := map[bool]map[string]int{
		true:  {"alpha": 2, "bravo": 1},
		false: {"alpha": 7, "bravo": 1},
	}

	// Bounces. meta carries the SES shape the block predicate reads.
	type bRow struct {
		camp        int // 0 = NULL
		typ         string
		status, sub string
		at          time.Time
	}
	in := dayAt(4, noon)
	bounces := []bRow{
		{cA, "hard", "5.7.1", "General", in},                     // block
		{cA, "hard", "5.7.1", "Suppressed", in},                  // ses-suppressed wins (first match)
		{cA, "hard", "5.1.1", "General", in},                     // address
		{cA, "hard", "", "", in},                                 // no meta at all
		{cA, "soft", "5.7.1", "General", in},                     // soft, never counted as hard
		{cA, "complaint", "", "", in},                            // complaint
		{cA, "hard", "5.7.1", "General", dayAt(65, noon)},        // before the window
		{cPriv, "hard", "5.7.1", "OnAccountSuppressionList", in}, // ses-suppressed
		{cB, "hard", "5.7.0", "General", in},                     // block
		{cB, "complaint", "", "", dayAt(0, noon)},                // Denver-today (out)
		{cNone, "hard", "5.7.1", "General", in},                  // no list: no brand
		{0, "hard", "5.7.1", "General", in},                      // deleted campaign
	}
	for _, r := range bounces {
		meta := `{}`
		if r.status != "" || r.sub != "" {
			b, _ := json.Marshal(map[string]any{"bounce": map[string]any{
				"bounceSubType": r.sub, "bouncedRecipients": []map[string]any{{"status": r.status}},
			}})
			meta = string(b)
		}
		h.db.MustExec(`INSERT INTO bounces (campaign_id, subscriber_id, source, type, meta, created_at)
			VALUES (NULLIF($1, 0), $2, 'test', $3::bounce_type, $4::JSONB, $5::TIMESTAMPTZ)`,
			r.camp, sBlock, r.typ, meta, r.at.Format(time.RFC3339))
	}

	// ses documents: an older one that must never be read and the latest; days outside the window
	// in every daily series.
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
	sysHealth := func(kind string, n int, doc any) {
		b, _ := json.Marshal(doc)
		h.db.MustExec(`INSERT INTO system_health (kind, day, status, doc) VALUES ($1, $2::DATE, 'ok', $3::JSONB)`, kind, day(n), string(b))
	}
	sysHealth("ses", 2, map[string]any{"brands": []map[string]any{{"brand": "alpha", "sends": 1, "daily": daily(50000)}}})
	sysHealth("ses", 0, map[string]any{"brands": []map[string]any{
		{"brand": "alpha", "sends": 1, "daily": daily(100)},
		{"brand": "bravo", "sends": 1, "daily": daily(200)[3:10]}, // days 58..52 only
		{"brand": "unattributed", "sends": 1, "daily": daily(400)},
	}})
	alphaSent, bravoSent := 0, 0
	for n := 1; n <= 60; n++ {
		alphaSent += 100 + n
	}
	for n := 52; n <= 58; n++ {
		bravoSent += 200 + n
	}

	// brand_health: alpha's older row must lose to its newest; bravo's newest carries no spam value;
	// a default-sender row for another brand must never reach delta (no default fallback).
	// GP-SPAM-VISIBILITY-SPEC I2: spam_status is inputs.spamRate.status verbatim (here deliberately
	// different from the row status, so a projection of the wrong field fails), absent when the doc
	// lacks it; spam_flagged is true only for a NEEDS_WORK USER_REPORTED_SPAM_RATE compliance row
	// (bravo's newest carries each half on a different row -- no match), absent otherwise, and only
	// the latest doc counts (bravo's older row is flagged).
	type cfgRow struct {
		Requirement string `json:"requirement"`
		Status      string `json:"status"`
	}
	flaggedRows := []cfgRow{{"USER_REPORTED_SPAM_RATE", "NEEDS_WORK"}, {"DMARC_POLICY", "COMPLIANT"}}
	brandHealth := func(brand string, n int, status string, isDefault bool, spam any, spamStatus string, cfg []cfgRow) {
		sr := map[string]any{"detail": map[string]any{"value": spam}}
		if spamStatus != "" {
			sr["status"] = spamStatus
		}
		inputs := map[string]any{"spamRate": sr}
		if cfg != nil {
			inputs["config"] = map[string]any{"status": "warn", "detail": map[string]any{"rows": cfg}}
		}
		b, _ := json.Marshal(map[string]any{"brand": brand, "status": status, "inputs": inputs})
		h.db.MustExec(`INSERT INTO brand_health (brand, day, status, is_default, doc) VALUES ($1, $2::DATE, $3, $4, $5::JSONB)`,
			brand, day(n), status, isDefault, string(b))
	}
	brandHealth("alpha", 3, "warn", false, 0.5, "warn", nil)
	brandHealth("alpha", 1, "ok", false, 0.0012, "issues", flaggedRows)
	brandHealth("bravo", 3, "issues", false, 0.9, "issues", flaggedRows)
	brandHealth("bravo", 1, "unknown", false, nil, "", []cfgRow{{"USER_REPORTED_SPAM_RATE", "COMPLIANT"}, {"SPF", "NEEDS_WORK"}})
	brandHealth("curated", 1, "issues", true, 0.9, "issues", flaggedRows)

	// BRAND-USAGE-SPEC I1: alpha's newest doc carries a usage object (nulls, nested arrays,
	// fractional numbers, a non-ASCII string) and its older doc a different one -- only the newest
	// is projected, verbatim; bravo's newest carries none (its older one does) and delta has no doc,
	// so neither row has a usage key.
	usageNew := `{"channel": "listmonk", "welcome": {"state": "active", "listId": 7, "campaigns": [{"id": 3, "status": "running"}]},
		"purchase": {"state": "idle", "rows": [{"slug": "alpha-first-order", "listId": null, "error": "trigger is missing — “x”", "evergreen": "none"}]},
		"noPurchase": {"state": "none", "rows": []}, "sunset": true, "attribution": false, "journeys": {"running": 1, "idle": 0},
		"broadcasts": {"lastAt": "2026-10-20T12:00:00.000Z", "windowCount": 4, "cadenceDays": 12.5, "daysSince": 4.1458333333, "ratio": 0.33166666666, "state": "ok"}}`
	setUsage := func(brand string, n int, usage string) {
		h.db.MustExec(`UPDATE brand_health SET doc = doc || JSONB_BUILD_OBJECT('usage', $3::JSONB) WHERE brand = $1 AND day = $2::DATE`, brand, day(n), usage)
	}
	setUsage("alpha", 1, usageNew)
	setUsage("alpha", 3, `{"channel": "zma"}`)
	setUsage("bravo", 3, `{"channel": "zma"}`)
	var storedUsage string
	if err := h.db.Get(&storedUsage, `SELECT (doc->'usage')::TEXT FROM brand_health WHERE brand = 'alpha' AND day = $1::DATE`, day(1)); err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(storedUsage)); err != nil {
		t.Fatal(err)
	}
	wantUsage := json.RawMessage(compact.Bytes())

	spam := 0.0012
	flagged := true
	alpha := func(tracking bool) matrixRow {
		return matrixRow{Brand: "alpha", Subscribers: matrixSubs{Total: 4, Active: 1, Held: 1, Unsubscribed: 1, Blocked: 1},
			Sent: &alphaSent, Clicks: wantClicks[tracking]["alpha"], HardBounces: 5, Blocks: 1, Complaints: 1,
			Health: matrixHealth{Status: "ok", SpamRate: &spam, SpamStatus: "issues", SpamFlagged: &flagged},
			Usage:  wantUsage}
	}
	bravo := func(tracking bool) matrixRow {
		return matrixRow{Brand: "bravo", Subscribers: matrixSubs{Total: 2, Active: 1},
			Sent: &bravoSent, Clicks: wantClicks[tracking]["bravo"], HardBounces: 1, Blocks: 1,
			Health: matrixHealth{Status: "unknown"}}
	}
	delta := matrixRow{Brand: "delta"}

	matrix := func(u auth.User, q string) matrixResp {
		t.Helper()
		code, body := baCall(t, u, "/api/dashboard/brand-matrix"+q, h.app.GetDashboardBrandMatrix)
		if code != http.StatusOK {
			t.Fatalf("matrix %q: %d %s", q, code, body)
		}
		var resp struct {
			Data matrixResp `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("matrix %q: %s: %v", q, body, err)
		}
		return resp.Data
	}

	admin := baUser([]string{auth.PermListGetAll, auth.PermCampaignsGetAll, auth.PermCampaignsGet, "brands:get"})
	scopedB := baUser([]string{"brands:get", auth.PermCampaignsGetAll}, pubB)
	for _, tracking := range []bool{true, false} {
		ko.Set("privacy.individual_tracking", tracking)
		q := prepareQueries(loadQueryMap(t), h.db, ko)
		h.app.core = core.New(&core.Opt{Queries: q, DB: h.db, I18n: h.app.i18n, Log: h.app.log}, &core.Hooks{})

		cases := []struct {
			name   string
			u      auth.User
			q      string
			want   []matrixRow
			scoped bool
		}{
			{"no list_id", admin, "", []matrixRow{alpha(tracking), bravo(tracking), delta}, false},
			{"list_id = alpha", admin, fmt.Sprintf("?list_id=%d&list_id=%d", pubA, privA), []matrixRow{alpha(tracking)}, true},
			// The archived list alone forms no brand; the unattributed and untagged lists no row.
			{"list_id = archived alpha", admin, fmt.Sprintf("?list_id=%d", arcA), []matrixRow{}, true},
			{"list_id = unattributed, no brand", admin, fmt.Sprintf("?list_id=%d&list_id=%d", listU, listN), []matrixRow{}, true},
			// A list-scoped user: their permitted lists only, campaigns get_all notwithstanding.
			{"list-scoped user", scopedB, "", []matrixRow{bravo(tracking)}, true},
			// A list_id outside the user's permission contributes nothing.
			{"list-scoped user, foreign list_id", scopedB, fmt.Sprintf("?list_id=%d", pubA), []matrixRow{}, true},
		}
		for _, tc := range cases {
			name := fmt.Sprintf("tracking=%v %s", tracking, tc.name)
			got := matrix(tc.u, tc.q)
			if got.Scoped != tc.scoped {
				t.Fatalf("%s: scoped %v, want %v", name, got.Scoped, tc.scoped)
			}
			if got.Rows == nil {
				got.Rows = []matrixRow{}
			}
			if !reflect.DeepEqual(got.Rows, tc.want) {
				t.Fatalf("%s: rows\n got  %s\n want %s", name, baJSON(got.Rows), baJSON(tc.want))
			}
		}

		// I1 -- the Overview's engagement over every campaign (unscoped): bounces keeps every row
		// (the NULL-campaign row is on no campaign), hard_bounces only the hard ones.
		hard, soft, complaints := 0, 0, 0
		for _, r := range bounces {
			if r.camp == 0 {
				continue
			}
			switch r.typ {
			case "hard":
				hard++
			case "soft":
				soft++
			case "complaint":
				complaints++
			}
		}
		code, body := baCall(t, admin, "/api/dashboard/counts", h.app.GetDashboardCounts)
		if code != http.StatusOK {
			t.Fatalf("counts: %d %s", code, body)
		}
		var cr struct {
			Data struct {
				Engagement struct {
					Bounces     int `json:"bounces"`
					HardBounces int `json:"hard_bounces"`
					Complaints  int `json:"complaints"`
				} `json:"engagement"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &cr); err != nil {
			t.Fatal(err)
		}
		e := cr.Data.Engagement
		if hard == 0 || soft == 0 || complaints == 0 {
			t.Fatal("fixture: seed hard, soft and complaint bounces")
		}
		if e.Bounces != hard+soft+complaints || e.HardBounces != hard || e.Complaints != complaints {
			t.Fatalf("tracking=%v I1: engagement %+v, want bounces %d = hard %d + soft %d + complaints %d",
				tracking, e, hard+soft+complaints, hard, soft, complaints)
		}
	}

	// The fixture means something: the two modes differ, and pending counts in bravo's total.
	if wantClicks[true]["alpha"] == wantClicks[false]["alpha"] || bravo(true).Subscribers.Total == bravo(true).Subscribers.Active {
		t.Fatal("fixture: modes or pending not exercised")
	}
	_ = sPend

	// No ses document with a daily series: sent is absent on every row, the rest unaffected.
	h.db.MustExec(`DELETE FROM system_health WHERE kind = 'ses'`)
	got := matrix(admin, "")
	if len(got.Rows) != 3 || got.Rows[0].Sent != nil || got.Rows[1].Sent != nil || got.Rows[0].Clicks != wantClicks[false]["alpha"] {
		t.Fatalf("no ses document: %s", baJSON(got))
	}
	ko.Set("privacy.individual_tracking", true)
}
