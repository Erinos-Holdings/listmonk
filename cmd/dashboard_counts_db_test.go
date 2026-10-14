package main

// Fork (dashboard counts) -- integrations DASHBOARD-COUNTS-SPEC I1 and I2 against a real database,
// through the handlers. I1: the Dashboard's Subscribers block is the sum over the scope's PUBLIC
// lists of GET /api/lists' subscriber_grid, per key and segment (a private list in scope
// contributes nothing); unscoped = every public list with orphans account-wide, scoped orphans = 0;
// total = grid.all.total and blocklisted = grid.all.blocked. I2: engagement.views/clicks/bounces =
// get-campaign-stats summed over the scope's campaigns C, in BOTH individual-tracking modes, each
// statement prepared through prepareQueries as cmd/init.go does at boot (TestCampaignStatsCounting's
// pattern). Shares newLinkHarness and the brand-analytics fixtures (LISTMONK_TEST_PG opt-in).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/models"
	"github.com/knadh/paginator"
	"github.com/lib/pq"
)

func TestDashboardCountsDB(t *testing.T) {
	h := newLinkHarness(t)
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})

	// Brand alpha: a public list (its master opt-in list) and a private derivative. Brand bravo: a
	// public double opt-in list.
	pubA := h.baList("Alpha", "public", "single")
	privA := h.baList("Alpha buyers", "private", "single")
	pubB := h.baList("Bravo", "public", "double")
	h.db.MustExec(`UPDATE lists SET tags = '{brand:alpha}' WHERE id = ANY($1)`, pq.Array([]int{pubA, privA}))
	h.db.MustExec(`UPDATE lists SET tags = '{brand:bravo}' WHERE id = $1`, pubB)
	// An ARCHIVED public list on alpha (Stage 4 F1): it keeps its subscriptions and counts nowhere
	// -- the brand scope never loads it, and the unscoped read must agree with the brands' sum.
	arcA := h.baList("Alpha old", "public", "single")
	h.db.MustExec(`UPDATE lists SET tags = '{brand:alpha}', status = 'archived' WHERE id = $1`, arcA)

	lang := func(sub int, l string) {
		h.db.MustExec(`UPDATE subscribers SET attribs = JSONB_BUILD_OBJECT('lang', $2::TEXT) WHERE id = $1`, sub, l)
	}

	// Segments and languages: en active on A (and its private list), a held en on A, a blocklisted
	// en on A, fr active on A, a no-language active on A and B, a plain unsubscribe on A, an
	// unrecognised-language pending on B, a private-only member, and an orphan.
	sEn := h.baSub("en@x", "enabled", map[int]string{pubA: "confirmed", privA: "confirmed"})
	lang(sEn, "en")
	sHeld := h.baSub("held@x", "enabled", map[int]string{pubA: "unsubscribed"})
	lang(sHeld, "en")
	h.db.MustExec(`UPDATE subscriber_lists SET meta = '{"hold": {"reason": "sunset"}}' WHERE subscriber_id = $1`, sHeld)
	sBlock := h.baSub("block@x", "blocklisted", map[int]string{pubA: "unsubscribed"})
	lang(sBlock, "en")
	sFr := h.baSub("fr@x", "enabled", map[int]string{pubA: "confirmed"})
	lang(sFr, "fr")
	sNone := h.baSub("none@x", "enabled", map[int]string{pubA: "confirmed", pubB: "confirmed"})
	h.baSub("unsub@x", "enabled", map[int]string{pubA: "unsubscribed"})
	sOther := h.baSub("other@x", "enabled", map[int]string{pubB: "unconfirmed"})
	lang(sOther, "xx")
	sPriv := h.baSub("priv@x", "enabled", map[int]string{privA: "confirmed"})
	h.baSub("orphan@x", "enabled", nil)
	// On the archived public list only: excluded from every scope's grid (F1), never an orphan.
	h.baSub("archived@x", "enabled", map[int]string{arcA: "confirmed"})

	// Campaigns: one per list and one on no list.
	cA := h.baCamp("On A", "finished", 10, pubA)
	cPriv := h.baCamp("On A buyers", "finished", 4, privA)
	cB := h.baCamp("On B", "finished", 6, pubB)
	cNone := h.baCamp("No list", "finished", 2)
	allCamps := []int{cA, cPriv, cB, cNone}

	// Engagement: repeated rows per subscriber and NULL-subscriber rows, so the two tracking modes
	// differ; bounces raw.
	var link int
	if err := h.db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://dc.test') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	view := func(camp, sub, times int) {
		for i := 0; i < times; i++ {
			h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id) VALUES ($1, NULLIF($2, 0))`, camp, sub)
		}
	}
	click := func(camp, sub, times int) {
		for i := 0; i < times; i++ {
			h.db.MustExec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id) VALUES ($1, NULLIF($2, 0), $3)`, camp, sub, link)
		}
	}
	bounce := func(camp, sub, times int) {
		for i := 0; i < times; i++ {
			h.db.MustExec(`INSERT INTO bounces (campaign_id, subscriber_id, source) VALUES ($1, $2, 'test')`, camp, sub)
		}
	}
	view(cA, sEn, 2)
	view(cA, sFr, 1)
	view(cA, 0, 1)
	click(cA, sEn, 2)
	click(cA, sFr, 2)
	click(cA, 0, 1)
	bounce(cA, sEn, 2)
	view(cPriv, sPriv, 3)
	click(cPriv, sPriv, 2)
	view(cB, sNone, 1)
	click(cB, sOther, 2)
	bounce(cB, sNone, 1)
	view(cNone, sEn, 1)
	bounce(cNone, sFr, 1)

	admin := baUser([]string{auth.PermListGetAll, auth.PermCampaignsGetAll, auth.PermCampaignsGet})

	// The lists' grids from GET /api/lists (every list, admin), keyed by id, with each list's type.
	type listRow struct {
		ID   int             `json:"id"`
		Type string          `json:"type"`
		Grid models.ListGrid `json:"subscriber_grid"`
	}
	listGrids := func() map[int]listRow {
		t.Helper()
		code, body := baCall(t, admin, "/api/lists?per_page=all", h.app.GetLists)
		if code != http.StatusOK {
			t.Fatalf("GetLists: %d %s", code, body)
		}
		var resp struct {
			Data struct {
				Results []listRow `json:"results"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("GetLists: %s: %v", body, err)
		}
		out := map[int]listRow{}
		for _, r := range resp.Data.Results {
			out[r.ID] = r
		}
		return out
	}
	lists := listGrids()

	// sumPublic is the Lists grids of the PUBLIC lists among ids, summed per key and segment.
	sumPublic := func(ids ...int) models.ListGrid {
		out := models.ListGrid{"all": {}}
		for _, id := range ids {
			l, ok := lists[id]
			if !ok {
				t.Fatalf("list %d missing from GET /api/lists", id)
			}
			if l.Type != "public" {
				continue
			}
			for k, r := range l.Grid {
				s := out[k]
				s.Active += r.Active
				s.Held += r.Held
				s.Unsubscribed += r.Unsubscribed
				s.Pending += r.Pending
				s.Blocked += r.Blocked
				s.Total += r.Total
				out[k] = s
			}
		}
		return out
	}

	// stats is get-campaign-stats summed over ids, through q.
	stats := func(q *models.Queries, ids []int) [3]int {
		t.Helper()
		var rows []models.CampaignMeta
		if err := q.GetCampaignStats.Select(&rows, pq.Array(ids)); err != nil {
			t.Fatal(err)
		}
		var out [3]int
		for _, r := range rows {
			out[0] += r.Views
			out[1] += r.Clicks
			out[2] += r.Bounces
		}
		return out
	}

	type countsResp struct {
		Subscribers struct {
			Total       int             `json:"total"`
			Blocklisted int             `json:"blocklisted"`
			Orphans     int             `json:"orphans"`
			Grid        models.ListGrid `json:"grid"`
		} `json:"subscribers"`
		Engagement struct {
			Views   int `json:"views"`
			Clicks  int `json:"clicks"`
			Bounces int `json:"bounces"`
		} `json:"engagement"`
		Scoped bool `json:"scoped"`
	}
	counts := func(u auth.User, q string) countsResp {
		t.Helper()
		code, body := baCall(t, u, "/api/dashboard/counts"+q, h.app.GetDashboardCounts)
		if code != http.StatusOK {
			t.Fatalf("counts %q: %d %s", q, code, body)
		}
		var resp struct {
			Data countsResp `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("counts %q: %s: %v", q, body, err)
		}
		return resp.Data
	}

	listScoped := baUser([]string{auth.PermCampaignsGetAnalytics}, privA, pubB)
	cases := []struct {
		name     string
		u        auth.User
		q        string
		lists    []int
		camps    []int
		scoped   bool
		orphans  int
		nonEmpty bool
	}{
		// Unscoped: every public list, every campaign, orphans account-wide.
		{"no list_id", admin, "", []int{pubA, privA, pubB}, allCamps, false, 1, true},
		// The brand's effective set (public + private): only the public list counts; C = the
		// campaigns on either list.
		{"list_id = alpha", admin, fmt.Sprintf("?list_id=%d&list_id=%d", pubA, privA), []int{pubA, privA}, []int{cA, cPriv}, true, 0, true},
		{"list_id = bravo", admin, fmt.Sprintf("?list_id=%d", pubB), []int{pubB}, []int{cB}, true, 0, true},
		// A private-only scope: a zero grid, its campaign still counted.
		{"list_id = private only", admin, fmt.Sprintf("?list_id=%d", privA), []int{privA}, []int{cPriv}, true, 0, false},
		// A list-scoped user with no list_id: their permitted lists.
		{"list-scoped user", listScoped, "", []int{privA, pubB}, []int{cPriv, cB}, true, 0, true},
	}

	for _, tracking := range []bool{true, false} {
		ko.Set("privacy.individual_tracking", tracking)
		q := prepareQueries(loadQueryMap(t), h.db, ko)
		h.app.core = core.New(&core.Opt{Queries: q, DB: h.db, I18n: h.app.i18n, Log: h.app.log}, &core.Hooks{})

		for _, tc := range cases {
			name := fmt.Sprintf("tracking=%v %s", tracking, tc.name)
			got := counts(tc.u, tc.q)

			// I1.
			want := sumPublic(tc.lists...)
			if !reflect.DeepEqual(got.Subscribers.Grid, want) {
				t.Fatalf("%s: grid\n got  %s\n want %s", name, baJSON(got.Subscribers.Grid), baJSON(want))
			}
			if got.Subscribers.Total != want["all"].Total || got.Subscribers.Blocklisted != want["all"].Blocked {
				t.Fatalf("%s: total %d blocklisted %d, want grid.all %+v", name, got.Subscribers.Total, got.Subscribers.Blocklisted, want["all"])
			}
			if got.Subscribers.Orphans != tc.orphans || got.Scoped != tc.scoped {
				t.Fatalf("%s: orphans %d scoped %v, want %d %v", name, got.Subscribers.Orphans, got.Scoped, tc.orphans, tc.scoped)
			}
			if tc.nonEmpty != (want["all"].Total > 0) {
				t.Fatalf("%s: fixture expectation broken: grid.all %+v", name, want["all"])
			}

			// I2.
			if e, w := [3]int{got.Engagement.Views, got.Engagement.Clicks, got.Engagement.Bounces}, stats(q, tc.camps); e != w {
				t.Fatalf("%s: engagement %v, want get-campaign-stats summed %v", name, e, w)
			}
		}
	}
	ko.Set("privacy.individual_tracking", true)

	// The fixture means something: the unscoped grid carries every segment and the language keys,
	// a private list's member is nowhere in it, and the two tracking modes differ.
	un := sumPublic(pubA, privA, pubB)
	if a := un["all"]; a.Active != 4 || a.Held != 1 || a.Blocked != 1 || a.Unsubscribed != 1 || a.Pending != 1 || a.Total != 8 {
		t.Fatalf("fixture: unscoped grid.all %+v", a)
	}
	for _, k := range []string{"en", "fr", "none", "other"} {
		if un[k].Total == 0 {
			t.Fatalf("fixture: unscoped grid has no %s key: %s", k, baJSON(un))
		}
	}
	on := prepareQueries(loadQueryMap(t), h.db, ko)
	ko.Set("privacy.individual_tracking", false)
	off := prepareQueries(loadQueryMap(t), h.db, ko)
	ko.Set("privacy.individual_tracking", true)
	if stats(on, allCamps) == stats(off, allCamps) {
		t.Fatal("fixture: the two tracking modes count alike")
	}
}
