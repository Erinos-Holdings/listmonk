package main

// Fork (list grid) -- integrations LIST-GRID-SPEC I2, I3, I4, I5 and I12 against a real database.
// Shares newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go), whose schema.sql
// carries the v6.2.11 functions and view. The fixture and every EXPECTED value come from
// internal/testfixtures/listgrid -- an independent Go statement of the spec's D1/D2 tables,
// decoded from each subscriber's e-mail -- so no assertion compares the SQL with itself.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/testfixtures/listgrid"
	"github.com/knadh/listmonk/models"
	"github.com/knadh/paginator"
	"github.com/labstack/echo/v4"
)

type gridFixture struct {
	*linkHarness
	single, double int
	rows           []listgrid.Row
}

func newGridFixture(t *testing.T) *gridFixture {
	t.Helper()
	h := newLinkHarness(t)
	f := &gridFixture{linkHarness: h}
	// The GET handlers paginate. Same options as main.go.
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})
	var err error
	if f.single, f.double, err = listgrid.Load(h.db); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if f.rows, err = listgrid.Rows(h.db); err != nil || len(f.rows) == 0 {
		t.Fatalf("fixture rows: %d, %v", len(f.rows), err)
	}
	h.db.MustExec(`REFRESH MATERIALIZED VIEW mat_list_subscriber_stats`)
	return f
}

// want returns the sorted fixture subscriber ids in a segment ("" = any) and lang filter value
// ("" = any; en = the en and none buckets; none; a code; other) on a single/double list.
func (f *gridFixture) want(double bool, segment, lang string) []int {
	var out []int
	for _, r := range f.rows {
		if segment != "" && r.Segment(double) != segment {
			continue
		}
		if lang == "none" && r.Bucket() != "none" {
			continue
		}
		if lang != "" && lang != "none" && r.SendLang() != lang {
			continue
		}
		out = append(out, r.ID)
	}
	sort.Ints(out)
	return out
}

func subIDs(subs models.Subscribers) []int {
	out := make([]int, 0, len(subs))
	for _, s := range subs {
		out = append(out, s.ID)
	}
	sort.Ints(out)
	return out
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var (
	// adminUser holds every list and the sql_query permission; plainUser everything but sql_query.
	adminUser = auth.User{PermissionsMap: map[string]struct{}{
		auth.PermListGetAll: {}, auth.PermListManageAll: {}, auth.PermSubscribersGetAll: {}, auth.PermSubscribersSqlQuery: {},
	}}
	plainUser = auth.User{PermissionsMap: map[string]struct{}{
		auth.PermListGetAll: {}, auth.PermListManageAll: {}, auth.PermSubscribersGetAll: {},
	}}
)

// call runs one handler as user and returns the HTTP status (an echo.HTTPError's code included).
func (f *gridFixture) call(handler echo.HandlerFunc, method, target, body string, user auth.User) (int, string) {
	f.t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.UserHTTPCtxKey, user)
	if err := handler(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		f.t.Fatalf("%s %s: %v", method, target, err)
	}
	return rec.Code, rec.Body.String()
}

func (f *gridFixture) grid(listID int) models.ListGrid {
	f.t.Helper()
	l, err := f.app.core.GetList(listID, "")
	if err != nil {
		f.t.Fatal(err)
	}
	return l.SubscriberGrid
}

func cell(r models.ListGridRow, segment string) int {
	switch segment {
	case "active":
		return r.Active
	case "held":
		return r.Held
	case "unsubscribed":
		return r.Unsubscribed
	case "pending":
		return r.Pending
	case "blocked":
		return r.Blocked
	}
	return r.Total
}

// TestSegmentFilterMatchesGrid -- I2. For every cell of both lists, three INDEPENDENT numbers
// agree: the grid (the cached view, after a refresh), the live filter (len(rows) of an
// unpaginated QuerySubscribers AND its reported total), and the fixture's expectation. The ids
// are compared too, so a cell that counts the right number of the wrong people fails.
func TestSegmentFilterMatchesGrid(t *testing.T) {
	f := newGridFixture(t)

	for _, li := range []struct {
		id     int
		double bool
	}{{f.single, false}, {f.double, true}} {
		grid := f.grid(li.id)
		for _, key := range []string{"all", "en", "fr", "es", "de", "it", "other", "none"} {
			lang := key
			if key == "all" {
				lang = ""
			}
			for _, seg := range append([]string{""}, listgrid.Segments...) {
				want := f.want(li.double, seg, lang)
				if seg == "" && lang == "" {
					continue // no filter -- the plain list page, I5's business.
				}
				subs, total, err := f.app.core.QuerySubscribers("", "", []int{li.id}, "",
					models.SubscriberFilter{Segment: seg, Lang: lang}, "", "", 0, 0)
				if err != nil {
					t.Fatalf("list %d %s/%s: %v", li.id, key, seg, err)
				}
				if got := subIDs(subs); !sameInts(got, want) || total != len(want) {
					t.Fatalf("list %d filter %s/%q: %d rows, total %d, want %d", li.id, key, seg, len(got), total, len(want))
				}
				if got := cell(grid[key], seg); got != len(want) {
					t.Fatalf("list %d grid %s/%q = %d, the link shows %d", li.id, key, seg, got, len(want))
				}
			}
		}
	}

	// lang= alone on the all-subscribers page (no list) returns rows -- a view-sourced count
	// would read the list_id = 0 row's NULL lang as 0 and answer "no results" (review H2).
	for _, lang := range []string{"fr", "en", "none", "other"} {
		want := f.want(false, "", lang)
		subs, total, err := f.app.core.QuerySubscribers("", "", nil, "", models.SubscriberFilter{Lang: lang}, "", "", 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(want) == 0 || !sameInts(subIDs(subs), want) || total != len(want) {
			t.Fatalf("all-subscribers lang=%s: %d rows, total %d, want %d", lang, len(subs), total, len(want))
		}
	}

	// lang=en_only (the Subscribers picker's "English") is stored en alone: with lang=none it
	// partitions lang=en, on a list under every segment and on the all-subscribers page.
	for _, ids := range [][]int{{f.single}, {f.double}, nil} {
		segs := append([]string{""}, listgrid.Segments...)
		if ids == nil {
			segs = []string{""}
		}
		for _, seg := range segs {
			count := func(lang string) []int {
				subs, total, err := f.app.core.QuerySubscribers("", "", ids, "",
					models.SubscriberFilter{Segment: seg, Lang: lang}, "", "", 0, 0)
				if err != nil {
					t.Fatalf("lists %v %s/%q: %v", ids, lang, seg, err)
				}
				if total != len(subs) {
					t.Fatalf("lists %v %s/%q: total %d, %d rows", ids, lang, seg, total, len(subs))
				}
				return subIDs(subs)
			}
			en, only, none := count("en"), count("en_only"), count("none")

			// Fixture-side expectation, independent of the SQL under test: stored en = the en
			// send language minus the none bucket. The union check below already forces this
			// given the earlier loops pin en and none to the fixture; asserting it directly
			// keeps en_only pinned even if those loops are ever edited.
			double := len(ids) == 1 && ids[0] == f.double
			inNone := map[int]bool{}
			for _, id := range f.want(double, seg, "none") {
				inNone[id] = true
			}
			wantOnly := []int{}
			for _, id := range f.want(double, seg, "en") {
				if !inNone[id] {
					wantOnly = append(wantOnly, id)
				}
			}
			if !sameInts(only, wantOnly) {
				t.Fatalf("lists %v seg %q: en_only = %d rows, fixture says %d", ids, seg, len(only), len(wantOnly))
			}
			if seg == "" && (len(only) == 0 || len(none) == 0) {
				t.Fatalf("lists %v: fixture has no stored-en or no none rows (%d, %d)", ids, len(only), len(none))
			}
			union := append(append([]int{}, only...), none...)
			sort.Ints(union)
			if !sameInts(union, en) {
				t.Fatalf("lists %v seg %q: en_only (%d) + none (%d) != en (%d)", ids, seg, len(only), len(none), len(en))
			}
		}
	}

	// The filter composes with subscription_status and with a search.
	subs, total, err := f.app.core.QuerySubscribers("upperfr-enabled", "", []int{f.double}, "unconfirmed",
		models.SubscriberFilter{Segment: "pending", Lang: "fr"}, "", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range f.rows {
		if r.Variant == "upperfr" && r.SStatus == "enabled" && r.SLStatus == "unconfirmed" {
			n++
		}
	}
	if n == 0 || len(subs) != n || total != n {
		t.Fatalf("search + status + filter: %d rows, total %d, want %d", len(subs), total, n)
	}
}

// TestActiveMatchesCampaignRecipients -- I3, the drift guard for NOT refactoring the send
// queries onto subscription_segment()/subscriber_lang(). For a regular broadcast in each of the
// five languages, what the REAL send path fetches (store.NextSubscribers, i.e.
// get-running-campaign + next-campaign-subscribers) is exactly the active filter for that send
// language -- en and none for an en campaign -- and the five together are active minus other.
// A legacy language-less campaign (written by SQL) still reaches all of active, other included.
func TestActiveMatchesCampaignRecipients(t *testing.T) {
	f := newGridFixture(t)
	st := &store{queries: f.q, core: f.app.core}

	recipients := func(listID int, lang string) []int {
		c := newLangCampaign(t, f.linkHarness, listID, models.JSON{"lang": "en"})
		if lang == "" {
			f.db.MustExec(`UPDATE campaigns SET attribs = attribs - 'lang' WHERE id = $1`, c.ID)
		} else {
			f.db.MustExec(`UPDATE campaigns SET attribs = JSONB_SET(attribs, '{lang}', TO_JSONB($2::TEXT)) WHERE id = $1`, c.ID, lang)
		}
		f.db.MustExec(`UPDATE campaigns SET status = 'running', last_subscriber_id = 0,
			max_subscriber_id = (SELECT MAX(id) FROM subscribers) WHERE id = $1`, c.ID)
		var out []int
		for {
			batch, err := st.NextSubscribers(c.ID, 97)
			if err != nil {
				t.Fatalf("NextSubscribers: %v", err)
			}
			if len(batch) == 0 {
				break
			}
			for _, s := range batch {
				out = append(out, s.ID)
			}
		}
		sort.Ints(out)
		return out
	}
	active := func(listID int, lang string) []int {
		subs, _, err := f.app.core.QuerySubscribers("", "", []int{listID}, "",
			models.SubscriberFilter{Segment: "active", Lang: lang}, "", "", 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		return subIDs(subs)
	}

	for _, li := range []struct {
		id     int
		double bool
	}{{f.single, false}, {f.double, true}} {
		union := map[int]bool{}
		for _, lang := range models.CampaignLangs {
			sent, filt := recipients(li.id, lang), active(li.id, lang)
			if len(sent) == 0 || !sameInts(sent, filt) {
				t.Fatalf("list %d lang %s: the campaign reaches %d, the active filter shows %d", li.id, lang, len(sent), len(filt))
			}
			if !sameInts(sent, f.want(li.double, "active", lang)) {
				t.Fatalf("list %d lang %s: recipients differ from the fixture's expectation", li.id, lang)
			}
			for _, id := range sent {
				if union[id] {
					t.Fatalf("list %d: subscriber %d is reached by two languages", li.id, id)
				}
				union[id] = true
			}
		}
		// en includes none -- the fixture has active none rows, or this proves nothing.
		if len(f.want(li.double, "active", "none")) == 0 {
			t.Fatal("fixture: no active no-language rows")
		}

		allActive, other := active(li.id, ""), active(li.id, "other")
		if len(other) == 0 || len(union) != len(allActive)-len(other) {
			t.Fatalf("list %d: five languages reach %d, active %d - other %d", li.id, len(union), len(allActive), len(other))
		}
		for _, id := range other {
			if union[id] {
				t.Fatalf("list %d: other subscriber %d is reached by a language-scoped campaign", li.id, id)
			}
		}

		if legacy := recipients(li.id, ""); !sameInts(legacy, allActive) {
			t.Fatalf("list %d: a language-less campaign reaches %d, active is %d", li.id, len(legacy), len(allActive))
		}
	}
}

// survivors returns the sorted ids of the fixture subscribers that still exist.
func (f *gridFixture) survivors() []int {
	var out []int
	if err := f.db.Select(&out, `SELECT id FROM subscribers WHERE email LIKE '%@grid.test' ORDER BY id`); err != nil {
		f.t.Fatal(err)
	}
	return out
}

func minus(all, remove []int) []int {
	rm := map[int]bool{}
	for _, id := range remove {
		rm[id] = true
	}
	var out []int
	for _, id := range all {
		if !rm[id] {
			out = append(out, id)
		}
	}
	return out
}

// TestSegmentByQueryScoped -- I4. Every by-query write with a segment touches that segment's
// rows and nothing else. The named cases are the ones that failed in review: all=true + segment
// on delete AND on blocklist (C1), a user "a OR b" + segment (H4), and a blocklist carrying only
// a segment (M8).
func TestSegmentByQueryScoped(t *testing.T) {
	all := func(f *gridFixture) []int {
		out := make([]int, 0, len(f.rows))
		for _, r := range f.rows {
			out = append(out, r.ID)
		}
		sort.Ints(out)
		return out
	}

	t.Run("delete all=true + segment (C1)", func(t *testing.T) {
		f := newGridFixture(t)
		seg := f.want(false, "unsubscribed", "")
		// search and query ride along. all=true blanks them in the handler and must NOT blank
		// the segment.
		body := fmt.Sprintf(`{"all": true, "search": "zzz", "list_ids": [%d], "segment": "unsubscribed"}`, f.single)
		if code, msg := f.call(f.app.DeleteSubscribersByQuery, http.MethodPost, "/api/subscribers/query/delete", body, plainUser); code != http.StatusOK {
			t.Fatalf("delete: %d %s", code, msg)
		}
		if got, want := f.survivors(), minus(all(f), seg); len(seg) == 0 || !sameInts(got, want) {
			t.Fatalf("delete all+segment: %d survivors, want %d (segment %d of %d)", len(got), len(want), len(seg), len(f.rows))
		}
	})

	t.Run("delete all=true + segment + lang", func(t *testing.T) {
		f := newGridFixture(t)
		seg := f.want(true, "pending", "fr")
		body := fmt.Sprintf(`{"all": true, "list_ids": [%d], "segment": "pending", "lang": "fr"}`, f.double)
		if code, msg := f.call(f.app.DeleteSubscribersByQuery, http.MethodPost, "/", body, plainUser); code != http.StatusOK {
			t.Fatalf("delete: %d %s", code, msg)
		}
		if got := f.survivors(); len(seg) == 0 || !sameInts(got, minus(all(f), seg)) {
			t.Fatalf("delete all+segment+lang: %d survivors, segment %d of %d", len(got), len(seg), len(f.rows))
		}
	})

	blocklisted := func(f *gridFixture) []int {
		var out []int
		f.db.Select(&out, `SELECT id FROM subscribers WHERE email LIKE '%@grid.test' AND status = 'blocklisted' ORDER BY id`)
		return out
	}

	t.Run("blocklist all=true + segment (C1)", func(t *testing.T) {
		f := newGridFixture(t)
		before, seg := blocklisted(f), f.want(false, "held", "")
		body := fmt.Sprintf(`{"all": true, "query": "TRUE", "list_ids": [%d], "segment": "held"}`, f.single)
		if code, msg := f.call(f.app.BlocklistSubscribersByQuery, http.MethodPut, "/", body, plainUser); code != http.StatusOK {
			t.Fatalf("blocklist: %d %s", code, msg)
		}
		want := append(append([]int{}, before...), seg...)
		sort.Ints(want)
		if got := blocklisted(f); len(seg) == 0 || !sameInts(got, want) {
			t.Fatalf("blocklist all+segment: %d blocklisted, want %d + %d", len(got), len(before), len(seg))
		}
	})

	t.Run("blocklist with a segment and nothing else (M8)", func(t *testing.T) {
		f := newGridFixture(t)
		before, seg := blocklisted(f), f.want(true, "pending", "")
		body := fmt.Sprintf(`{"list_ids": [%d], "segment": "pending"}`, f.double)
		if code, msg := f.call(f.app.BlocklistSubscribersByQuery, http.MethodPut, "/", body, plainUser); code != http.StatusOK {
			t.Fatalf("a segment must satisfy the search-or-query check: %d %s", code, msg)
		}
		want := append(append([]int{}, before...), seg...)
		sort.Ints(want)
		if got := blocklisted(f); len(seg) == 0 || !sameInts(got, want) {
			t.Fatalf("blocklist segment-only: %d blocklisted, want %d + %d", len(got), len(before), len(seg))
		}
		// Still refused with no filter of any kind.
		if code, _ := f.call(f.app.BlocklistSubscribersByQuery, http.MethodPut, "/", fmt.Sprintf(`{"list_ids": [%d]}`, f.double), plainUser); code != http.StatusBadRequest {
			t.Fatalf("no search, query, all, segment or lang: want 400, got %d", code)
		}
	})

	t.Run("user query a OR b stays inside the segment (H4)", func(t *testing.T) {
		f := newGridFixture(t)
		seg := f.want(false, "unsubscribed", "")
		// Unparenthesised, "AND <segment> AND a OR b" is "(... AND a) OR b", and b is every row.
		body := fmt.Sprintf(`{"query": "subscribers.email = 'nobody@nowhere.test' OR subscribers.id > 0", "list_ids": [%d], "segment": "unsubscribed"}`, f.single)
		if code, msg := f.call(f.app.DeleteSubscribersByQuery, http.MethodPost, "/", body, adminUser); code != http.StatusOK {
			t.Fatalf("delete: %d %s", code, msg)
		}
		if got := f.survivors(); len(seg) == 0 || !sameInts(got, minus(all(f), seg)) {
			t.Fatalf("a OR b escaped the segment: %d survivors, want %d", len(got), len(f.rows)-len(seg))
		}
	})

	t.Run("user query a OR b stays inside the segment, read path (H4)", func(t *testing.T) {
		f := newGridFixture(t)
		seg := f.want(false, "unsubscribed", "")
		subs, total, err := f.app.core.QuerySubscribers("", "subscribers.email = 'nobody@nowhere.test' OR subscribers.id > 0", []int{f.single}, "",
			models.SubscriberFilter{Segment: "unsubscribed"}, "", "", 0, 0)
		if err != nil || total != len(seg) || len(subs) != len(seg) {
			t.Fatalf("read path a OR b: %d rows, total %d, want %d (%v)", len(subs), total, len(seg), err)
		}
	})

	slRows := func(f *gridFixture, list int) []int {
		var out []int
		f.db.Select(&out, `SELECT subscriber_id FROM subscriber_lists WHERE list_id = $1 ORDER BY subscriber_id`, list)
		return out
	}

	t.Run("manage remove + segment", func(t *testing.T) {
		f := newGridFixture(t)
		seg := f.want(false, "unsubscribed", "")
		doubleBefore := slRows(f, f.double)
		body := fmt.Sprintf(`{"action": "remove", "list_ids": [%d], "target_list_ids": [%d], "segment": "unsubscribed"}`, f.single, f.single)
		if code, msg := f.call(f.app.ManageSubscriberListsByQuery, http.MethodPut, "/", body, plainUser); code != http.StatusOK {
			t.Fatalf("remove: %d %s", code, msg)
		}
		if got := slRows(f, f.single); len(seg) == 0 || !sameInts(got, minus(all(f), seg)) {
			t.Fatalf("remove + segment: %d rows left on the list, want %d", len(got), len(f.rows)-len(seg))
		}
		if !sameInts(slRows(f, f.double), doubleBefore) || len(f.survivors()) != len(f.rows) {
			t.Fatal("remove + segment touched another list or deleted a subscriber")
		}
	})

	t.Run("manage unsubscribe + segment + lang", func(t *testing.T) {
		f := newGridFixture(t)
		seg := f.want(true, "active", "other")
		var before []int
		f.db.Select(&before, `SELECT subscriber_id FROM subscriber_lists WHERE list_id = $1 AND status = 'unsubscribed' ORDER BY 1`, f.double)
		body := fmt.Sprintf(`{"action": "unsubscribe", "list_ids": [%d], "target_list_ids": [%d], "segment": "active", "lang": "other"}`, f.double, f.double)
		if code, msg := f.call(f.app.ManageSubscriberListsByQuery, http.MethodPut, "/", body, plainUser); code != http.StatusOK {
			t.Fatalf("unsubscribe: %d %s", code, msg)
		}
		var after []int
		f.db.Select(&after, `SELECT subscriber_id FROM subscriber_lists WHERE list_id = $1 AND status = 'unsubscribed' ORDER BY 1`, f.double)
		want := append(append([]int{}, before...), seg...)
		sort.Ints(want)
		if len(seg) == 0 || !sameInts(after, want) {
			t.Fatalf("unsubscribe + segment + lang: %d unsubscribed, want %d + %d", len(after), len(before), len(seg))
		}
	})

	t.Run("manage add + segment", func(t *testing.T) {
		f := newGridFixture(t)
		var target int
		f.db.Get(&target, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'target', 'private', 'single') RETURNING id`)
		seg := f.want(false, "held", "")
		body := fmt.Sprintf(`{"action": "add", "list_ids": [%d], "target_list_ids": [%d], "segment": "held", "status": "confirmed"}`, f.single, target)
		if code, msg := f.call(f.app.ManageSubscriberListsByQuery, http.MethodPut, "/", body, plainUser); code != http.StatusOK {
			t.Fatalf("add: %d %s", code, msg)
		}
		if got := slRows(f, target); len(seg) == 0 || !sameInts(got, seg) {
			t.Fatalf("add + segment: %d rows on the target, want %d", len(got), len(seg))
		}
	})

	t.Run("export + segment", func(t *testing.T) {
		f := newGridFixture(t)
		seg := f.want(true, "pending", "")
		code, body := f.call(f.app.ExportSubscribers, http.MethodGet,
			fmt.Sprintf("/api/subscribers/export?list_id=%d&segment=pending", f.double), "", plainUser)
		if code != http.StatusOK {
			t.Fatalf("export: %d %s", code, body)
		}
		// One CSV line per subscriber after the header.
		if got := strings.Count(strings.TrimSpace(body), "\n"); len(seg) == 0 || got != len(seg) {
			t.Fatalf("export + segment: %d rows, want %d", got, len(seg))
		}
	})
}

// TestSegmentParamValidation -- I4. Unknown values and a segment without exactly one PERMITTED
// list are 400 on all five paths, before anything is written; neither param needs sql_query.
func TestSegmentParamValidation(t *testing.T) {
	f := newGridFixture(t)
	nSubs := len(f.survivors())

	type path struct {
		name    string
		handler echo.HandlerFunc
		method  string
		// build renders the request for a list-id set and a segment/lang pair.
		build func(lists []int, segment, lang string) (target, body string)
	}
	ids := func(lists []int) string {
		b, _ := json.Marshal(lists)
		if lists == nil {
			return "[]"
		}
		return string(b)
	}
	qs := func(lists []int, segment, lang string) string {
		v := url.Values{}
		for _, l := range lists {
			v.Add("list_id", fmt.Sprint(l))
		}
		if segment != "" {
			v.Set("segment", segment)
		}
		if lang != "" {
			v.Set("lang", lang)
		}
		return v.Encode()
	}
	byQuery := func(extra string) func([]int, string, string) (string, string) {
		return func(lists []int, segment, lang string) (string, string) {
			return "/", fmt.Sprintf(`{%s"list_ids": %s, "segment": %q, "lang": %q}`, extra, ids(lists), segment, lang)
		}
	}
	paths := []path{
		{"query", f.app.QuerySubscribers, http.MethodGet, func(l []int, s, g string) (string, string) { return "/api/subscribers?" + qs(l, s, g), "" }},
		{"export", f.app.ExportSubscribers, http.MethodGet, func(l []int, s, g string) (string, string) {
			return "/api/subscribers/export?" + qs(l, s, g), ""
		}},
		{"delete", f.app.DeleteSubscribersByQuery, http.MethodPost, byQuery(`"all": true, `)},
		{"blocklist", f.app.BlocklistSubscribersByQuery, http.MethodPut, byQuery(`"all": true, `)},
		{"manage", f.app.ManageSubscriberListsByQuery, http.MethodPut, byQuery(fmt.Sprintf(`"action": "remove", "target_list_ids": [%d], `, f.single))},
	}

	// A list-role user permitted on BOTH fixture lists and nothing else: a non-permitted list id
	// is replaced by the user's own lists (auth.User.GetPermittedListIDs), so ONE requested id
	// becomes TWO lists -- the count must be taken after that substitution (review M5).
	perm := map[string]struct{}{auth.PermListGet: {}, auth.PermListManage: {}}
	listUser := auth.User{
		PermissionsMap:     map[string]struct{}{},
		ListPermissionsMap: map[int]map[string]struct{}{f.single: perm, f.double: perm},
		GetListIDs:         []int{f.single, f.double},
		ManageListIDs:      []int{f.single, f.double},
	}

	for _, p := range paths {
		cases := []struct {
			name         string
			lists        []int
			segment, lng string
			user         auth.User
		}{
			{"unknown segment", []int{f.single}, "mailable", "", plainUser},
			{"sql in segment", []int{f.single}, "active') OR ('1'='1", "", plainUser},
			{"unknown lang", []int{f.single}, "", "pt", plainUser},
			{"sql in lang", []int{f.single}, "active", "en' OR '1'='1", plainUser},
			{"segment with no list", nil, "unsubscribed", "", plainUser},
			{"segment with two lists", []int{f.single, f.double}, "unsubscribed", "", plainUser},
			{"non-permitted list expanding to several (M5)", []int{99999}, "unsubscribed", "", listUser},
		}
		for _, c := range cases {
			target, body := p.build(c.lists, c.segment, c.lng)
			if code, msg := f.call(p.handler, p.method, target, body, c.user); code != http.StatusBadRequest {
				t.Fatalf("%s / %s: want 400, got %d %s", p.name, c.name, code, msg)
			}
		}
	}
	if got := len(f.survivors()); got != nSubs {
		t.Fatalf("a refused request wrote: %d subscribers, was %d", got, nSubs)
	}
	var blocked, wantBlocked int
	f.db.Get(&blocked, `SELECT COUNT(*) FROM subscribers WHERE email LIKE '%@grid.test' AND status = 'blocklisted'`)
	for _, r := range f.rows {
		if r.SStatus == "blocklisted" {
			wantBlocked++
		}
	}
	if blocked != wantBlocked {
		t.Fatalf("a refused request blocklisted: %d, was %d", blocked, wantBlocked)
	}

	// Neither param needs subscribers:sql_query (plainUser lacks it) -- and a query still does.
	for _, target := range []string{
		fmt.Sprintf("/api/subscribers?list_id=%d&segment=held", f.single),
		"/api/subscribers?lang=fr",
		fmt.Sprintf("/api/subscribers?list_id=%d&segment=held&lang=none", f.single),
	} {
		if code, msg := f.call(f.app.QuerySubscribers, http.MethodGet, target, "", plainUser); code != http.StatusOK {
			t.Fatalf("%s without sql_query: want 200, got %d %s", target, code, msg)
		}
	}
	if code, _ := f.call(f.app.QuerySubscribers, http.MethodGet,
		fmt.Sprintf("/api/subscribers?list_id=%d&segment=held&query=TRUE", f.single), "", plainUser); code != http.StatusForbidden {
		t.Fatalf("query= without sql_query: want 403, got %d", code)
	}

	// lang= alone, no list, is allowed; the permitted single list works for a list-role user.
	if code, msg := f.call(f.app.QuerySubscribers, http.MethodGet,
		fmt.Sprintf("/api/subscribers?list_id=%d&segment=active", f.single), "", listUser); code != http.StatusOK {
		t.Fatalf("list-role user on a permitted list: %d %s", code, msg)
	}
}

// TestListStatsBackCompat -- I5. subscriber_count, subscriber_statuses (held included) and
// ?subscription_status= are what they were before the view was regrouped. The fixture is
// multi-language with blocklisted members, so each status spans many view rows: without the
// pre-aggregation JSONB_OBJECT_AGG keeps ONE of them (review H1) and this fails.
func TestListStatsBackCompat(t *testing.T) {
	f := newGridFixture(t)

	var viewRows int
	f.db.Get(&viewRows, `SELECT COUNT(*) FROM mat_list_subscriber_stats WHERE list_id = $1 AND status = 'confirmed'`, f.single)
	if viewRows < 2 {
		t.Fatalf("fixture: status confirmed must span several view rows to be able to fail, got %d", viewRows)
	}

	want := map[string]int{}
	for _, r := range f.rows {
		want[r.LegacyStatus()]++
	}

	lists, _, err := f.app.core.QueryLists("grid-", "", "", "", nil, "id", "asc", true, nil, 0, 0)
	if err != nil || len(lists) != 2 {
		t.Fatalf("QueryLists: %d, %v", len(lists), err)
	}
	for _, l := range lists {
		if l.SubscriberCount != len(f.rows) {
			t.Fatalf("list %s subscriber_count = %d, want %d", l.Name, l.SubscriberCount, len(f.rows))
		}
		if len(l.SubscriberCounts) != len(want) {
			t.Fatalf("list %s subscriber_statuses = %v, want %v", l.Name, l.SubscriberCounts, want)
		}
		for k, n := range want {
			if l.SubscriberCounts[k] != n {
				t.Fatalf("list %s subscriber_statuses[%s] = %d, want %d", l.Name, k, l.SubscriberCounts[k], n)
			}
		}

		// query-subscribers-count-all (unchanged SQL) over the finer grouping, every status.
		for k, n := range want {
			var got int
			if err := f.q.QuerySubscribersCountAll.Get(&got, fmt.Sprintf("{%d}", l.ID), k); err != nil || got != n {
				t.Fatalf("count-all list %s status %s = %d, want %d (%v)", l.Name, k, got, n, err)
			}
		}
		var got int
		if err := f.q.QuerySubscribersCountAll.Get(&got, fmt.Sprintf("{%d}", l.ID), ""); err != nil || got != len(f.rows) {
			t.Fatalf("count-all list %s = %d, want %d (%v)", l.Name, got, len(f.rows), err)
		}
	}
	var total int
	if err := f.q.QuerySubscribersCountAll.Get(&total, "{}", ""); err != nil || total != len(f.rows) {
		t.Fatalf("count-all, all subscribers = %d, want %d (%v)", total, len(f.rows), err)
	}

	// ?subscription_status= is the RAW subscription status, held rows included, as before.
	for _, status := range []string{"unconfirmed", "confirmed", "unsubscribed"} {
		n := 0
		for _, r := range f.rows {
			if r.SLStatus == status {
				n++
			}
		}
		subs, got, err := f.app.core.QuerySubscribers("", "", []int{f.double}, status, models.SubscriberFilter{}, "", "", 0, 0)
		if err != nil || got != n || len(subs) != n {
			t.Fatalf("subscription_status=%s: %d rows, total %d, want %d (%v)", status, len(subs), got, n, err)
		}
	}
}

// TestListGridShape -- I12 (D5). Keys: all always; the send-language rows and other only where
// total > 0, en already holding none; none only where total > 0. all = the language rows summed
// and none <= en cell by cell. The JSON carries subscriber_grid and not the sort-key columns.
func TestListGridShape(t *testing.T) {
	f := newGridFixture(t)

	for _, id := range []int{f.single, f.double} {
		g := f.grid(id)
		if len(g) != 8 {
			t.Fatalf("list %d: want keys all en fr es de it other none, got %v", id, g)
		}
		var sum models.ListGridRow
		for _, k := range []string{"en", "fr", "es", "de", "it", "other"} {
			r, ok := g[k]
			if !ok || r.Total == 0 {
				t.Fatalf("list %d: key %s missing or empty", id, k)
			}
			if r.Total != r.Active+r.Held+r.Unsubscribed+r.Pending+r.Blocked {
				t.Fatalf("list %d row %s does not partition: %+v", id, k, r)
			}
			sum.Active, sum.Held, sum.Unsubscribed = sum.Active+r.Active, sum.Held+r.Held, sum.Unsubscribed+r.Unsubscribed
			sum.Pending, sum.Blocked, sum.Total = sum.Pending+r.Pending, sum.Blocked+r.Blocked, sum.Total+r.Total
		}
		if g["all"] != sum || g["all"].Total != len(f.rows) {
			t.Fatalf("list %d: all %+v != the language rows %+v", id, g["all"], sum)
		}
		for _, s := range append([]string{"total"}, listgrid.Segments...) {
			if cell(g["none"], s) > cell(g["en"], s) || cell(g["none"], "total") == 0 {
				t.Fatalf("list %d: none %+v must be a non-empty subset of en %+v", id, g["none"], g["en"])
			}
		}
	}
	if f.grid(f.single)["all"].Pending != 0 || f.grid(f.double)["all"].Pending == 0 {
		t.Fatal("pending exists on the double opt-in list only")
	}

	// A list of explicit-en subscribers only: all and en, no none, no other. An empty list: all.
	var enOnly, empty int
	f.db.Get(&enOnly, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'en-only', 'private', 'single') RETURNING id`)
	f.db.Get(&empty, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'empty', 'private', 'single') RETURNING id`)
	f.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status)
		SELECT id, $1, 'confirmed' FROM subscribers WHERE email LIKE 'upperen-enabled-%'`, enOnly)
	f.db.MustExec(`REFRESH MATERIALIZED VIEW mat_list_subscriber_stats`)
	if g := f.grid(enOnly); len(g) != 2 || g["en"].Active == 0 || g["en"] != g["all"] {
		t.Fatalf("en-only list: %v", g)
	}
	if g := f.grid(empty); len(g) != 1 || g["all"] != (models.ListGridRow{}) {
		t.Fatalf("empty list: %v", g)
	}

	l, err := f.app.core.GetList(enOnly, "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(l)
	if !strings.Contains(string(b), `"subscriber_grid":{`) || strings.Contains(string(b), "active_count") {
		t.Fatalf("list JSON: %s", b)
	}
}
