package main

// Fork (campaign mode) -- integrations listmonk/CAMPAIGN-MODE-SPEC I1, I3, I4, I6-I10 against a real
// database, through the handlers (LISTMONK_TEST_PG opt-in; plain go test skips). Shares
// newLinkHarness (link_redirect_db_test.go), baUser / baCall / baList / pickerReq
// (brand_analytics_db_test.go). Fixture names are acme.test / example.test only.
//
// I1: core.NameKeySQL through Postgres agrees with every pair of frontend/src/testdata/campaign-groups.json
// (the same file campaignGroups.test.mjs reads for nameKey), and the database's UTF-8 case folding.
// I3: partition -- with no query the grouped list's members are exactly the ungrouped list's ids, each once.
// I4: the keys -- brand, code and name key; an evergreen's Code forms no series.
// I6: an order is stored at most once per measure.
// I7: whole-group search, no-hit search, paging and sort over top-level rows.
// I8: a list-scoped user sees only permitted members (list, picker, matrix).
// I9: the Matrix's brand and keys agree with the list's grouping.
// I10: the grouped picker.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/paginator"
)

type cgSet struct {
	Name string `json:"name"`
	Step *int   `json:"step"`
	IDs  []int  `json:"ids"`
}

type cgRow struct {
	Kind      string            `json:"kind"`
	Brand     *string           `json:"brand"`
	Code      *string           `json:"code"`
	Step      *int              `json:"step"`
	Name      string            `json:"name"`
	IDs       []int             `json:"ids"`
	Count     int               `json:"count"`
	Sets      []cgSet           `json:"sets"`
	Campaigns []json.RawMessage `json:"campaigns"`
}

func (r cgRow) memberIDs(t *testing.T) []int {
	t.Helper()
	out := []int{}
	for _, raw := range r.Campaigns {
		var c struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		out = append(out, c.ID)
	}
	return out
}

type cgFixture struct {
	h                    *linkHarness
	listA, listA2, listB int
	listN                int
}

// camp inserts a campaign. lang "" = no language; code "" = no Code; step 0 = no Step.
type cgCamp struct {
	name, subject, status, lang, code string
	step, sent                        int
	evergreen                         bool
	created                           time.Time
	lists                             []int
}

func (f cgFixture) add(c cgCamp) int {
	f.h.t.Helper()
	attribs := map[string]any{}
	if c.lang != "" {
		attribs["lang"] = c.lang
	}
	if c.code != "" {
		attribs["promo_code"] = c.code
	}
	if c.step != 0 {
		attribs["promo_step"] = c.step
	}
	ab, _ := json.Marshal(attribs)
	if c.status == "" {
		c.status = "finished"
	}
	if c.subject == "" {
		c.subject = "s"
	}
	if c.created.IsZero() {
		c.created = time.Now()
	}
	var id int
	if err := f.h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id, status, sent, evergreen, attribs, created_at, updated_at, started_at)
		VALUES (gen_random_uuid(), $1, $2, 'Acme <hello@acme.test>', '<p>b</p>', 'email', (SELECT id FROM templates LIMIT 1), $3::campaign_status, $4, $5, $6::JSONB, $7, $7,
		CASE WHEN $3 IN ('draft', 'scheduled') THEN NULL ELSE NOW() - INTERVAL '1 day' END) RETURNING id`,
		c.name, c.subject, c.status, c.sent, c.evergreen, string(ab), c.created); err != nil {
		f.h.t.Fatal(err)
	}
	for _, l := range c.lists {
		f.h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'x')`, id, l)
	}
	return id
}

func newCGFixture(t *testing.T) cgFixture {
	h := newLinkHarness(t)
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})
	f := cgFixture{h: h}
	f.listA = h.baList("Alpha", "public", "single")
	f.listA2 = h.baList("Alpha buyers", "private", "single")
	f.listB = h.baList("Bravo", "public", "single")
	f.listN = h.baList("No brand", "public", "single")
	h.db.MustExec(`UPDATE lists SET tags = '{brand:alpha}' WHERE id IN ($1, $2)`, f.listA, f.listA2)
	h.db.MustExec(`UPDATE lists SET tags = '{brand:bravo}' WHERE id = $1`, f.listB)
	return f
}

// groupedList calls GET /api/campaigns?grouped=true with q and returns the rows and total.
func (f cgFixture) groupedList(t *testing.T, u auth.User, q string) ([]cgRow, int) {
	t.Helper()
	code, body := baCall(t, u, "/api/campaigns?grouped=true&no_body=true&"+q, f.h.app.GetCampaigns)
	if code != http.StatusOK {
		t.Fatalf("grouped %q: %d %s", q, code, body)
	}
	var resp struct {
		Data struct {
			Total   int     `json:"total"`
			Results []cgRow `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("grouped %q: %v: %s", q, err, body)
	}
	if resp.Data.Results == nil {
		t.Fatalf("grouped %q: results null: %s", q, body)
	}
	return resp.Data.Results, resp.Data.Total
}

// flatList calls GET /api/campaigns (ungrouped) and returns the ids in order.
func (f cgFixture) flatList(t *testing.T, u auth.User, q string) []int {
	t.Helper()
	code, body := baCall(t, u, "/api/campaigns?no_body=true&per_page=all&"+q, f.h.app.GetCampaigns)
	if code != http.StatusOK {
		t.Fatalf("list %q: %d %s", q, code, body)
	}
	var resp struct {
		Data struct {
			Results []struct {
				ID int `json:"id"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	out := []int{}
	for _, r := range resp.Data.Results {
		out = append(out, r.ID)
	}
	return out
}

// groupOf returns the top-level row whose ids contain id.
func groupOf(rows []cgRow, id int) *cgRow {
	for i := range rows {
		for _, x := range rows[i].IDs {
			if x == id {
				return &rows[i]
			}
		}
	}
	return nil
}

func TestCampaignGroupNameKeyFixture(t *testing.T) {
	h := newLinkHarness(t)
	b, err := os.ReadFile(filepath.Join("..", "frontend", "src", "testdata", "campaign-groups.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Cases []struct {
			Name string `json:"name"`
			Key  string `json:"key"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	if len(fx.Cases) < 10 {
		t.Fatalf("fixture has %d cases", len(fx.Cases))
	}
	for _, c := range fx.Cases {
		var got string
		if err := h.db.Get(&got, "SELECT "+core.NameKeySQL("$1::TEXT"), c.Name); err != nil {
			t.Fatal(err)
		}
		if got != c.Key {
			t.Errorf("name key %q = %q, want %q", c.Name, got, c.Key)
		}
	}
	// D3 -- the case folding relies on the database's UTF-8 locale (prod probed 2026-11-23).
	var ok bool
	if err := h.db.Get(&ok, `SELECT LOWER('LIYORÁ É') = 'liyorá é'`); err != nil || !ok {
		t.Fatalf("UTF-8 case folding: %v %v", ok, err)
	}
}

func TestCampaignGroupKeys(t *testing.T) {
	f := newCGFixture(t)
	admin := baUser([]string{auth.PermCampaignsGetAll})

	setEN := f.add(cgCamp{name: "Spring Sale", lang: "en", lists: []int{f.listA}})
	setFR := f.add(cgCamp{name: "  spring\u00a0 SALE", lang: "fr", lists: []int{f.listA, f.listN}}) // the untagged list forms no brand
	evg := f.add(cgCamp{name: "Spring Sale", code: "BF", evergreen: true, lists: []int{f.listA}})   // an evergreen's Code forms no series
	bravo := f.add(cgCamp{name: "Spring Sale", lists: []int{f.listB}})
	noBrand := f.add(cgCamp{name: "Spring Sale", lists: []int{f.listN}})
	noBrand2 := f.add(cgCamp{name: "Spring Sale", lists: []int{f.listN}})
	twoBrands := f.add(cgCamp{name: "Spring Sale", lists: []int{f.listA, f.listB}})
	s1en := f.add(cgCamp{name: "Spring Sale", code: "BF", step: 1, lang: "en", lists: []int{f.listA}})
	s1fr := f.add(cgCamp{name: "Spring Sale", code: "bf ", step: 1, lang: "fr", lists: []int{f.listA2}})
	s2 := f.add(cgCamp{name: "Last call", code: "BF", step: 2, lists: []int{f.listA}})
	s3a := f.add(cgCamp{name: "Mixed steps", code: "BF", step: 3, lists: []int{f.listA}})
	s3b := f.add(cgCamp{name: "Mixed steps", code: "BF", step: 2, lists: []int{f.listA}})
	other := f.add(cgCamp{name: "Spring Sale", code: "CM", lists: []int{f.listA}})
	// A brand whose only list is archived is no brand.
	listArch := f.h.baList("Archived", "public", "single")
	f.h.db.MustExec(`UPDATE lists SET tags = '{brand:alpha}', status = 'archived' WHERE id = $1`, listArch)
	arch := f.add(cgCamp{name: "Spring Sale", lists: []int{listArch}})

	rows, total := f.groupedList(t, admin, "per_page=all")
	if total != len(rows) {
		t.Fatalf("total %d, rows %d", total, len(rows))
	}

	set := groupOf(rows, setEN)
	if set == nil || set.Kind != "set" || set.Brand == nil || *set.Brand != "alpha" ||
		!reflect.DeepEqual(sortedInts(set.IDs...), sortedInts(setEN, setFR, evg)) {
		t.Fatalf("different case/spacing names + an evergreen with a Code -> one alpha set: %+v", set)
	}
	if set.Sets != nil {
		t.Fatalf("a set carries no sets: %+v", set)
	}
	for _, id := range []int{bravo, noBrand, noBrand2, twoBrands, arch} {
		g := groupOf(rows, id)
		if g == nil || g.Kind != "campaign" || len(g.IDs) != 1 {
			t.Fatalf("campaign %d: want its own row, got %+v", id, g)
		}
	}
	if g := groupOf(rows, bravo); g.Brand == nil || *g.Brand != "bravo" {
		t.Fatalf("bravo singleton brand %+v", g)
	}

	// G5 bySend: the Campaigns list's row is the SEND -- each step of the BF series is its own row
	// (the series itself is the picker's and the Matrix's; TestAnalyticsCampaignsGrouped).
	for _, g := range rows {
		if g.Kind == "series" || g.Sets != nil {
			t.Fatalf("a series row in the Campaigns list: %v", g.IDs)
		}
	}
	step1 := groupOf(rows, s1en)
	if step1 == nil || step1.Kind != "set" || step1.Step == nil || *step1.Step != 1 || step1.Code == nil ||
		!strings.EqualFold(strings.TrimSpace(*step1.Code), "bf") || !reflect.DeepEqual(sortedInts(step1.IDs...), sortedInts(s1en, s1fr)) {
		t.Fatalf("BF step 1 (two languages, codes 'BF' and 'bf ') -> one set row: %v", step1 != nil && step1.Kind == "set")
	}
	if g := groupOf(rows, s2); g == nil || g.Kind != "campaign" || !reflect.DeepEqual(g.IDs, []int{s2}) {
		t.Fatalf("BF step 2 alone -> its own campaign row")
	}
	if g := groupOf(rows, s3a); g == nil || g.Kind != "set" || g.Step != nil || !reflect.DeepEqual(sortedInts(g.IDs...), sortedInts(s3a, s3b)) {
		t.Fatalf("one name, disagreeing steps -> one set row, no step")
	}
	if g := groupOf(rows, other); g == nil || g.Kind != "campaign" || len(g.IDs) != 1 {
		t.Fatalf("code CM alone -> a campaign row")
	}
}

func TestCampaignGroupsPartition(t *testing.T) {
	f := newCGFixture(t)
	f.add(cgCamp{name: "Spring Sale", lang: "en", lists: []int{f.listA}})
	f.add(cgCamp{name: "Spring Sale", lang: "fr", lists: []int{f.listA}, status: "draft"})
	f.add(cgCamp{name: "Spring Sale", lang: "de", lists: []int{f.listA2}, evergreen: true, status: "running"})
	f.add(cgCamp{name: "Promo", code: "BF", step: 1, lists: []int{f.listA}})
	f.add(cgCamp{name: "Promo two", code: "BF", step: 2, lists: []int{f.listA}, status: "paused"})
	f.add(cgCamp{name: "Bravo one", lists: []int{f.listB}})
	f.add(cgCamp{name: "Orphan", lists: nil})

	users := map[string]auth.User{
		"all":        baUser([]string{auth.PermCampaignsGetAll}),
		"restricted": baUser([]string{auth.PermCampaignsGet}, f.listA),
	}
	filters := []string{"", "status=finished", "status=draft&status=paused", "evergreen=false", "evergreen=true",
		"list_id=" + fmt.Sprint(f.listA), "lang=fr", "lang=en&evergreen=false"}
	nonEmpty := 0
	for un, u := range users {
		for _, q := range filters {
			rows, _ := f.groupedList(t, u, "per_page=all&"+q)
			got := []int{}
			for _, r := range rows {
				m := r.memberIDs(t)
				if !reflect.DeepEqual(sortedInts(m...), sortedInts(r.IDs...)) {
					t.Fatalf("%s %q: members %v, ids %v", un, q, m, r.IDs)
				}
				got = append(got, m...)
			}
			want := f.flatList(t, u, q)
			if !reflect.DeepEqual(sortedInts(got...), sortedInts(want...)) {
				t.Errorf("%s %q: grouped members %v, ungrouped %v", un, q, sortedInts(got...), sortedInts(want...))
			}
			if len(want) > 0 {
				nonEmpty++
			}
		}
	}
	if nonEmpty < 8 {
		t.Fatalf("I3 nearly vacuous: %d non-empty cases", nonEmpty)
	}
}

func TestCampaignGroupsMatchedOrderOnce(t *testing.T) {
	f := newCGFixture(t)
	c1 := f.add(cgCamp{name: "Spring Sale", lang: "en", lists: []int{f.listA}})
	c2 := f.add(cgCamp{name: "Spring Sale", lang: "fr", lists: []int{f.listA}})
	f.h.db.MustExec(`INSERT INTO campaign_conversions (measure, order_id, campaign_id, brand, purchased_at, sales_usd) VALUES ('matched', 'o1', $1, 'alpha', NOW(), 5)`, c1)
	_, err := f.h.db.Exec(`INSERT INTO campaign_conversions (measure, order_id, campaign_id, brand, purchased_at, sales_usd) VALUES ('matched', 'o1', $1, 'alpha', NOW(), 5)`, c2)
	if err == nil || !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("a second campaign's (matched, o1): %v, want a unique violation", err)
	}
	// The other measure is its own key.
	f.h.db.MustExec(`INSERT INTO campaign_conversions (measure, order_id, campaign_id, brand, purchased_at, sales_usd) VALUES ('tracked', 'o1', $1, 'alpha', NOW(), 5)`, c2)
}

func TestCampaignGroupsSearchWholeGroups(t *testing.T) {
	f := newCGFixture(t)
	admin := baUser([]string{auth.PermCampaignsGetAll})
	a := f.add(cgCamp{name: "Spring Sale", subject: "zebra crossing", lang: "en", lists: []int{f.listA}})
	b := f.add(cgCamp{name: "Spring Sale", subject: "plain", lang: "fr", lists: []int{f.listA}})
	c := f.add(cgCamp{name: "Spring Sale", subject: "plain", lang: "de", lists: []int{f.listA}, status: "draft"})
	f.add(cgCamp{name: "Other", subject: "plain", lists: []int{f.listA}})
	s1 := f.add(cgCamp{name: "Promo", subject: "zebra", code: "BF", step: 1, lists: []int{f.listA}})
	s2 := f.add(cgCamp{name: "Promo two", subject: "plain", code: "BF", step: 2, lists: []int{f.listA}})

	rows, total := f.groupedList(t, admin, "per_page=all&query=zebra")
	if total != 2 || len(rows) != 2 {
		t.Fatalf("zebra: %d rows, total %d: %+v", len(rows), total, rows)
	}
	if g := groupOf(rows, a); g == nil || !reflect.DeepEqual(sortedInts(g.memberIDs(t)...), sortedInts(a, b, c)) {
		t.Fatalf("whole set: %+v", g)
	}
	// G5 amendment (bySend): the list's row is the send -- the hit's own step, never its series.
	if g := groupOf(rows, s1); g == nil || !reflect.DeepEqual(g.memberIDs(t), []int{s1}) || groupOf(rows, s2) != nil {
		t.Fatalf("the hit's step only: %v", g != nil && g.Kind != "")
	}
	// Filters other than search narrow members: the draft member drops under status=finished.
	rows, _ = f.groupedList(t, admin, "per_page=all&query=zebra&status=finished")
	if g := groupOf(rows, a); g == nil || !reflect.DeepEqual(sortedInts(g.memberIDs(t)...), sortedInts(a, b)) {
		t.Fatalf("status narrows members: %+v", g)
	}
	// A search with no hits returns no rows.
	rows, total = f.groupedList(t, admin, "per_page=all&query=nothingmatchesthis")
	if total != 0 || len(rows) != 0 {
		t.Fatalf("no hits: %d rows total %d", len(rows), total)
	}
	// The hit must pass the filters too: a draft-only hit under status=finished is no hit.
	f.add(cgCamp{name: "Draftonly", subject: "yak", status: "draft", lists: []int{f.listA}})
	if rows, total = f.groupedList(t, admin, "per_page=all&query=yak&status=finished"); total != 0 || len(rows) != 0 {
		t.Fatalf("filtered-out hit: %+v", rows)
	}
}

func TestCampaignGroupsPaging(t *testing.T) {
	f := newCGFixture(t)
	admin := baUser([]string{auth.PermCampaignsGetAll})
	base := time.Now().Add(-100 * time.Hour)
	at := func(h int) time.Time { return base.Add(time.Duration(h) * time.Hour) }
	f.add(cgCamp{name: "Bravo set", lang: "en", lists: []int{f.listA}, created: at(1)})
	f.add(cgCamp{name: "bravo  SET", lang: "fr", lists: []int{f.listA}, created: at(9), status: "draft"})
	f.add(cgCamp{name: "Alpha one", lists: []int{f.listA}, created: at(5), status: "paused"})
	// One promo send in two languages (G5 bySend: a send, not a series, is the list's row).
	f.add(cgCamp{name: "Delta", code: "BF", lang: "en", lists: []int{f.listA}, created: at(3)})
	f.add(cgCamp{name: "Delta", code: "BF", lang: "fr", lists: []int{f.listA}, created: at(7), status: "running"})
	f.add(cgCamp{name: "Echo", lists: []int{f.listN}, created: at(2), status: "scheduled"})
	f.add(cgCamp{name: "Foxtrot", lists: []int{f.listB}, created: at(4), status: "cancelled"})

	all, total := f.groupedList(t, admin, "per_page=all")
	if total != 5 || len(all) != 5 {
		t.Fatalf("top-level rows %d total %d", len(all), total)
	}

	// Paging never splits a group; total counts top-level rows.
	seen := map[int]bool{}
	for page := 1; page <= 3; page++ {
		rows, tot := f.groupedList(t, admin, fmt.Sprintf("per_page=2&page=%d", page))
		if tot != 5 {
			t.Fatalf("page %d total %d", page, tot)
		}
		for _, r := range rows {
			m := r.memberIDs(t)
			if !reflect.DeepEqual(sortedInts(m...), sortedInts(groupOf(all, r.IDs[0]).IDs...)) {
				t.Fatalf("page %d: split group %v", page, m)
			}
			for _, id := range m {
				if seen[id] {
					t.Fatalf("id %d on two pages", id)
				}
				seen[id] = true
			}
		}
	}
	if len(seen) != 7 {
		t.Fatalf("paging covered %d of 7 campaigns", len(seen))
	}

	names := func(q string) []string {
		rows, _ := f.groupedList(t, admin, "per_page=all&"+q)
		out := []string{}
		for _, r := range rows {
			out = append(out, r.Name)
		}
		return out
	}
	cases := map[string][]string{
		// name -> the name key of the row's name (the newest member's: "bravo  SET").
		"order_by=name&order=asc":  {"Alpha one", "bravo  SET", "Delta", "Echo", "Foxtrot"},
		"order_by=name&order=desc": {"Foxtrot", "Echo", "Delta", "bravo  SET", "Alpha one"},
		// created_at -> the members' MAX (set 9, the BF send 7, alpha 5, foxtrot 4, echo 2).
		"order_by=created_at&order=desc": {"bravo  SET", "Delta", "Alpha one", "Foxtrot", "Echo"},
		"order_by=created_at&order=asc":  {"Echo", "Foxtrot", "Alpha one", "Delta", "bravo  SET"},
		"order_by=updated_at&order=asc":  {"Echo", "Foxtrot", "Alpha one", "Delta", "bravo  SET"},
		// status -> the members' MIN in the enum's order (draft, running, scheduled, paused, cancelled, finished).
		"order_by=status&order=asc":  {"bravo  SET", "Delta", "Echo", "Alpha one", "Foxtrot"},
		"order_by=status&order=desc": {"Foxtrot", "Alpha one", "Echo", "Delta", "bravo  SET"},
	}
	for q, want := range cases {
		if got := names(q); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v, want %v", q, got, want)
		}
	}
	// A sort field outside the whitelist falls back to created_at desc, as the list does.
	if got := names("order_by=subject;DROP&order=sideways"); !reflect.DeepEqual(got, cases["order_by=created_at&order=desc"]) {
		t.Errorf("bad sort: %v", got)
	}
}

// matrixRows is the Campaign Matrix's rows as raw maps.
func (f cgFixture) matrixRows(t *testing.T, u auth.User) []map[string]any {
	t.Helper()
	code, body := baCall(t, u, "/api/analytics/campaigns/matrix", f.h.app.auth.Perm(f.h.app.GetCampaignMatrix, "campaigns:get_analytics"))
	if code != http.StatusOK {
		t.Fatalf("matrix: %d %s", code, body)
	}
	var resp struct {
		Data struct {
			Rows []map[string]any `json:"rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Data.Rows
}

// pickerGroups calls the grouped picker.
func (f cgFixture) pickerGroups(t *testing.T, u auth.User, q url.Values) []cgRow {
	t.Helper()
	q.Set("grouped", "true")
	code, body := pickerReq(t, f.h, u, q)
	if code != http.StatusOK {
		t.Fatalf("picker %v: %d %s", q, code, body)
	}
	var resp struct {
		Data []cgRow `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data == nil {
		t.Fatalf("picker %v: data null: %s", q, body)
	}
	return resp.Data
}

func TestCampaignGroupsMatrixKeysAgree(t *testing.T) {
	f := newCGFixture(t)
	admin := baUser([]string{auth.PermCampaignsGetAll, auth.PermCampaignsGetAnalytics, auth.PermListGetAll})
	mk := func(c cgCamp) int { c.sent = 10; return f.add(c) }
	mk(cgCamp{name: "Spring Sale", lang: "en", lists: []int{f.listA}})
	mk(cgCamp{name: "spring  sale", lang: "fr", lists: []int{f.listA2}})
	mk(cgCamp{name: "Spring Sale", lists: []int{f.listB}})
	mk(cgCamp{name: "Spring Sale", lists: []int{f.listN}})
	mk(cgCamp{name: "Spring Sale", lists: []int{f.listA, f.listB}})
	mk(cgCamp{name: "Promo", code: "BF", step: 1, lang: "en", lists: []int{f.listA}})
	mk(cgCamp{name: "Promo", code: "BF", step: 1, lang: "fr", lists: []int{f.listA}})
	mk(cgCamp{name: "Promo two", code: "bf", step: 2, lists: []int{f.listA}})
	mk(cgCamp{name: "Promo two", code: "BF", lists: []int{f.listA}, evergreen: true})

	rows := f.matrixRows(t, admin)
	list, _ := f.groupedList(t, admin, "per_page=all")
	// The evergreen has no send hours, so it is not in the matrix window; every broadcast is.
	if len(rows) != 8 {
		t.Fatalf("matrix rows %d", len(rows))
	}
	setOf := map[string][]int{}
	seriesOf := map[string][]int{}
	for _, r := range rows {
		for _, k := range []string{"set_key", "series_key"} {
			if _, ok := r[k]; !ok {
				t.Fatalf("matrix row without %s: %v", k, r)
			}
		}
		id := int(r["id"].(float64))
		g := groupOf(list, id)
		if g == nil {
			t.Fatalf("matrix id %d not in the list", id)
		}
		// The displayed brand is the group brand.
		lb, mb := "", ""
		if g.Brand != nil {
			lb = *g.Brand
		}
		if s, ok := r["brand"].(string); ok {
			mb = s
		}
		if lb != mb {
			t.Fatalf("campaign %d: matrix brand %q, group brand %q", id, mb, lb)
		}
		// G5 bySend: the list never returns a series row; the matrix series is checked as the union
		// of its steps' list rows below.
		if g.Kind == "series" {
			t.Fatalf("campaign %d: a series row in the Campaigns list", id)
		}
		if sk, ok := r["series_key"].(string); ok {
			seriesOf[sk] = append(seriesOf[sk], id)
		}
		if sk, ok := r["set_key"].(string); ok {
			setOf[sk] = append(setOf[sk], id)
		} else if mb != "" {
			t.Fatalf("campaign %d: a brand but no set_key", id)
		}
	}
	// Every matrix set is exactly one list row's matrix members; every matrix series is exactly the
	// union of its members' list rows (one per step -- G5 bySend).
	inMatrix := map[int]bool{}
	for _, r := range rows {
		inMatrix[int(r["id"].(float64))] = true
	}
	rowMembers := func(ids []int) []int {
		seen := map[*cgRow]bool{}
		want := []int{}
		for _, id := range ids {
			g := groupOf(list, id)
			if seen[g] {
				continue
			}
			seen[g] = true
			for _, x := range g.IDs {
				if inMatrix[x] {
					want = append(want, x)
				}
			}
		}
		return want
	}
	for k, ids := range setOf {
		if want := rowMembers(ids[:1]); !reflect.DeepEqual(sortedInts(ids...), sortedInts(want...)) {
			t.Fatalf("set %q: matrix %v, list row %v", k, sortedInts(ids...), sortedInts(want...))
		}
	}
	for k, ids := range seriesOf {
		if want := rowMembers(ids); !reflect.DeepEqual(sortedInts(ids...), sortedInts(want...)) {
			t.Fatalf("series %q: matrix %v, list rows %v", k, sortedInts(ids...), sortedInts(want...))
		}
	}
	if len(seriesOf) != 1 || len(setOf) < 4 {
		t.Fatalf("fixture shape: series %v sets %v", seriesOf, setOf)
	}
}

func TestCampaignGroupsScopedUser(t *testing.T) {
	f := newCGFixture(t)
	mk := func(c cgCamp) int { c.sent = 10; return f.add(c) }
	ok1 := mk(cgCamp{name: "Spring Sale", lang: "en", lists: []int{f.listA}})
	ok2 := mk(cgCamp{name: "Spring Sale", lang: "fr", lists: []int{f.listA}})
	hidden := mk(cgCamp{name: "Spring Sale", lang: "de", lists: []int{f.listA2}})
	sOK := mk(cgCamp{name: "Promo", code: "BF", step: 1, lists: []int{f.listA}})
	sHidden := mk(cgCamp{name: "Promo two", code: "BF", step: 2, lists: []int{f.listA2}})
	bHidden := mk(cgCamp{name: "Bravo", lists: []int{f.listB}})
	forbidden := map[int]bool{hidden: true, sHidden: true, bHidden: true}

	assertScoped := func(name string, rows []cgRow) {
		t.Helper()
		seen := []int{}
		for _, r := range rows {
			ids := append([]int{}, r.IDs...)
			ids = append(ids, r.memberIDs(t)...)
			for _, s := range r.Sets {
				ids = append(ids, s.IDs...)
			}
			for _, id := range ids {
				if forbidden[id] {
					t.Fatalf("%s: unpermitted campaign %d in %+v", name, id, r)
				}
			}
			if r.Count != 0 && r.Count != len(r.Campaigns) {
				t.Fatalf("%s: count %d, members %d", name, r.Count, len(r.Campaigns))
			}
			seen = append(seen, r.memberIDs(t)...)
		}
		if !reflect.DeepEqual(sortedInts(seen...), sortedInts(ok1, ok2, sOK)) {
			t.Fatalf("%s: members %v", name, sortedInts(seen...))
		}
		if g := groupOf(rows, ok1); g == nil || g.Kind != "set" || len(g.Campaigns) != 2 {
			t.Fatalf("%s: the permitted set %+v", name, g)
		}
		// The picker keeps series; the list's row is the send (G5 bySend): the permitted step alone.
		wantKind, wantSets := "series", 1
		if name == "list" {
			wantKind, wantSets = "campaign", 0
		}
		if g := groupOf(rows, sOK); g == nil || g.Kind != wantKind || len(g.Campaigns) != 1 || len(g.Sets) != wantSets {
			t.Fatalf("%s: the permitted %s, got kind %v", name, wantKind, g != nil && g.Kind == wantKind)
		}
	}

	t.Run("list", func(t *testing.T) {
		u := baUser([]string{auth.PermCampaignsGet}, f.listA)
		rows, _ := f.groupedList(t, u, "per_page=all")
		assertScoped("list", rows)
		rows, _ = f.groupedList(t, u, "per_page=all&query=spring")
		if g := groupOf(rows, ok1); g == nil || len(g.Campaigns) != 2 {
			t.Fatalf("list search: %+v", rows)
		}
	})
	t.Run("picker", func(t *testing.T) {
		u := baUser([]string{auth.PermCampaignsGetAnalytics}, f.listA)
		assertScoped("picker", f.pickerGroups(t, u, url.Values{"per_page": {"50"}}))
		// The ids-only hit step (review S2) prepares, binds its eight parameters ($7 NULL) and
		// returns whole permitted groups for a search. (What is RETURNED is scoped by the groups and
		// members steps -- assertScoped above; hits only select among scoped groups, re-review T2.)
		rows := f.pickerGroups(t, u, url.Values{"query": {"spring"}})
		if len(rows) != 1 || !reflect.DeepEqual(sortedInts(rows[0].memberIDs(t)...), sortedInts(ok1, ok2)) {
			t.Fatalf("picker search: %+v", rows)
		}
		if rows := f.pickerGroups(t, u, url.Values{"query": {"promo two"}}); len(rows) != 0 {
			t.Fatalf("picker search of an unpermitted campaign: %+v", rows)
		}
	})
	t.Run("matrix", func(t *testing.T) {
		u := baUser([]string{auth.PermCampaignsGetAnalytics}, f.listA)
		rows := f.matrixRows(t, u)
		got := []int{}
		keys := map[string]int{}
		for _, r := range rows {
			id := int(r["id"].(float64))
			if forbidden[id] {
				t.Fatalf("matrix: unpermitted %d", id)
			}
			got = append(got, id)
			if k, ok := r["set_key"].(string); ok {
				keys[k]++
			}
		}
		if !reflect.DeepEqual(sortedInts(got...), sortedInts(ok1, ok2, sOK)) {
			t.Fatalf("matrix rows %v", got)
		}
		// The permitted set buckets to two members only.
		two := 0
		for _, n := range keys {
			if n == 2 {
				two++
			}
		}
		if two != 1 {
			t.Fatalf("matrix set keys %v", keys)
		}
	})
}

func TestAnalyticsCampaignsGrouped(t *testing.T) {
	f := newCGFixture(t)
	admin := baUser([]string{auth.PermCampaignsGetAll, auth.PermCampaignsGetAnalytics})
	base := time.Now().Add(-500 * time.Hour)
	// An old matching member, then many newer campaigns that would fill a hit cap.
	old := f.add(cgCamp{name: "Spring Sale", lang: "en", lists: []int{f.listA}, created: base})
	newer := f.add(cgCamp{name: "Spring Sale", lang: "fr", lists: []int{f.listA}, created: base.Add(400 * time.Hour)})
	for i := 0; i < 55; i++ {
		f.add(cgCamp{name: fmt.Sprintf("Spring filler %d", i), lists: []int{f.listB}, created: base.Add(time.Duration(10+i) * time.Hour)})
	}
	s1 := f.add(cgCamp{name: "Promo", code: "BF", step: 1, lists: []int{f.listA}})
	s2 := f.add(cgCamp{name: "Promo two", code: "BF", step: 2, lists: []int{f.listA}})

	// Whole groups; an older matching member is never cut by the hit cap (55 newer hits).
	rows := f.pickerGroups(t, admin, url.Values{"query": {"spring"}, "per_page": {"50"}})
	if len(rows) != 50 {
		t.Fatalf("per_page limits top-level rows: %d", len(rows))
	}
	// Top-level rows are newest first by their newest member: the set's (400h in) leads the fillers
	// (10-64h in), and it carries the old member the 50-campaign hit cap would have cut.
	if rows[0].Kind != "set" {
		t.Fatalf("newest first: %+v", rows[0])
	}
	if g := groupOf(rows, old); g == nil || g.Kind != "set" || !reflect.DeepEqual(sortedInts(g.memberIDs(t)...), sortedInts(old, newer)) || g.Count != 2 {
		t.Fatalf("whole set with the old member: %+v", g)
	}
	rows = f.pickerGroups(t, admin, url.Values{"query": {"promo two"}, "per_page": {"5"}})
	if len(rows) != 1 || rows[0].Kind != "series" || !reflect.DeepEqual(sortedInts(rows[0].memberIDs(t)...), sortedInts(s1, s2)) || len(rows[0].Sets) != 2 {
		t.Fatalf("series by one step's name: %+v", rows)
	}
	// A search with no hits returns no rows.
	if rows = f.pickerGroups(t, admin, url.Values{"query": {"nothingmatches"}}); len(rows) != 0 {
		t.Fatalf("no hits: %+v", rows)
	}
	// The id= prefill ignores grouped and returns campaigns.
	code, body := pickerReq(t, f.h, admin, url.Values{"grouped": {"true"}, "id": {fmt.Sprint(old)}})
	var flat struct {
		Data []map[string]any `json:"data"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &flat) != nil || len(flat.Data) != 1 || int(flat.Data[0]["id"].(float64)) != old {
		t.Fatalf("id prefill: %d %s", code, body)
	}
	if _, ok := flat.Data[0]["kind"]; ok {
		t.Fatalf("id prefill returned a group: %s", body)
	}
	// Its groups equal the list's for the same campaigns (no search, every row).
	pick := f.pickerGroups(t, admin, url.Values{"per_page": {"50"}})
	list, _ := f.groupedList(t, admin, "per_page=all")
	canon := func(rs []cgRow) []string {
		out := []string{}
		for _, r := range rs {
			out = append(out, fmt.Sprint(r.Kind, sortedInts(r.IDs...)))
		}
		sort.Strings(out)
		return out
	}
	if len(pick) != 50 {
		t.Fatalf("picker unsearched rows %d", len(pick))
	}
	listCanon := map[string]bool{}
	for _, s := range canon(list) {
		listCanon[s] = true
	}
	// G5 bySend: the picker keeps series, the list returns one row per send -- a picker series is
	// exactly its steps' list rows; every other picker row is a list row.
	expanded := []cgRow{}
	for _, r := range pick {
		if r.Kind != "series" {
			expanded = append(expanded, r)
			continue
		}
		for _, s := range r.Sets {
			kind := "campaign"
			if len(s.IDs) > 1 {
				kind = "set"
			}
			expanded = append(expanded, cgRow{Kind: kind, IDs: s.IDs})
		}
	}
	for _, s := range canon(expanded) {
		if !listCanon[s] {
			t.Fatalf("picker group %s is not a list group", s)
		}
	}
}
