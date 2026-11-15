package main

// Fork (integrations SUBSCRIBERS-FILTERS-2-SPEC I5, I6, I7) against a real database. Shares the
// list-grid fixture (newGridFixture; LISTMONK_TEST_PG opt-in, see link_redirect_db_test.go): every
// subscriber sits on BOTH grid lists, and each row's language bucket is decoded from its e-mail
// (listgrid.Row.Bucket), never from the SQL under test.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/knadh/listmonk/internal/testfixtures/listgrid"
)

// I5 -- minimal GET /api/lists carries has_other_lang, has_broadcasts and has_automations with
// the D2/D6 meaning, false for a list with no such rows; the full listing omits them.
func TestListsKindAndOtherLangFlags(t *testing.T) {
	f := newGridFixture(t)

	mkList := func(name string) int {
		t.Helper()
		var id int
		if err := f.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), $1, 'private', 'single') RETURNING id`, name); err != nil {
			t.Fatal(err)
		}
		return id
	}
	mkCampaign := func(name string, evergreen bool, status string, listID int) {
		t.Helper()
		var id int
		if err := f.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, status, evergreen)
			VALUES (gen_random_uuid(), $1, 's', 'f@example.test', 'b', 'email', $2::campaign_status, $3) RETURNING id`, name, status, evergreen); err != nil {
			t.Fatal(err)
		}
		f.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'x')`, id, listID)
	}

	empty := mkList("f2-empty")
	enOnly := mkList("f2-en-only")
	// One stored-English member: a list with members but no 'other' bucket.
	f.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status)
		SELECT id, $1, 'confirmed' FROM subscribers WHERE email LIKE 'upperen-%@grid.test' LIMIT 1`, enOnly)
	// The grid's "pt" variant is the 'other' bucket on both grid lists; an unsubscribed member
	// counts too (the view counts every membership row).
	f.db.MustExec(`REFRESH MATERIALIZED VIEW mat_list_subscriber_stats`)

	// Broadcasts on single (a cancelled one -- any status counts), an automation on double, both
	// on enOnly; nothing on empty.
	mkCampaign("f2-bc", false, "cancelled", f.single)
	mkCampaign("f2-ev", true, "draft", f.double)
	mkCampaign("f2-bc2", false, "draft", enOnly)
	mkCampaign("f2-ev2", true, "paused", enOnly)

	type flags struct {
		ID             int   `json:"id"`
		HasOtherLang   *bool `json:"has_other_lang"`
		HasBroadcasts  *bool `json:"has_broadcasts"`
		HasAutomations *bool `json:"has_automations"`
	}
	read := func(q string) map[int]flags {
		t.Helper()
		code, body := f.call(f.app.GetLists, http.MethodGet, "/api/lists?per_page=all"+q, "", adminUser)
		if code != http.StatusOK {
			t.Fatalf("GetLists %s: %d %s", q, code, body)
		}
		var resp struct {
			Data struct {
				Results []flags `json:"results"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("decode: %v: %s", err, body)
		}
		out := map[int]flags{}
		for _, r := range resp.Data.Results {
			out[r.ID] = r
		}
		return out
	}

	b := func(p *bool) string {
		if p == nil {
			return "absent"
		}
		return fmt.Sprint(*p)
	}
	minimal := read("&minimal=true")
	want := map[int][3]string{
		f.single: {"true", "true", "false"},
		f.double: {"true", "false", "true"},
		enOnly:   {"false", "true", "true"},
		empty:    {"false", "false", "false"},
	}
	for id, w := range want {
		r, ok := minimal[id]
		if !ok {
			t.Fatalf("list %d missing from the minimal listing", id)
		}
		if got := [3]string{b(r.HasOtherLang), b(r.HasBroadcasts), b(r.HasAutomations)}; got != w {
			t.Errorf("list %d: has_other_lang/has_broadcasts/has_automations = %v, want %v", id, got, w)
		}
	}

	// S2-5 -- the full listing does not select them: absent, never a false false.
	for id, r := range read("") {
		if r.HasOtherLang != nil || r.HasBroadcasts != nil || r.HasAutomations != nil {
			t.Errorf("full listing list %d carries a minimal-only flag", id)
		}
	}
}

// bucketKey is the expected Lang sort key of a fixture row: the bucket, none as "" (D3).
func bucketKey(r listgrid.Row) string {
	if b := r.Bucket(); b != "none" {
		return b
	}
	return ""
}

// I6 -- order_by=lang sorts by the bucket, no language first ascending (last descending), with the
// id tiebreak, and pages stably: the concatenated pages equal the unpaged result row for row --
// with one list and with two (one row per membership).
func TestSubscribersLangSortPagesStably(t *testing.T) {
	f := newGridFixture(t)
	byID := map[int]listgrid.Row{}
	for _, r := range f.rows {
		byID[r.ID] = r
	}

	type page struct {
		Data struct {
			Results []struct {
				ID int `json:"id"`
			} `json:"results"`
			Total int `json:"total"`
		} `json:"data"`
	}
	get := func(q url.Values) page {
		t.Helper()
		code, body := f.call(f.app.QuerySubscribers, http.MethodGet, "/api/subscribers?"+q.Encode(), "", adminUser)
		if code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", q.Encode(), code, body)
		}
		var p page
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	for _, lists := range [][]int{{f.single}, {f.single, f.double}} {
		for _, order := range []string{"asc", "desc"} {
			base := url.Values{"order_by": {"lang"}, "order": {order}}
			for _, id := range lists {
				base.Add("list_id", fmt.Sprint(id))
			}
			all := url.Values{}
			for k, v := range base {
				all[k] = v
			}
			all.Set("per_page", "all")
			full := get(all)
			if len(full.Data.Results) != len(f.rows)*len(lists) || full.Data.Total != len(full.Data.Results) {
				t.Fatalf("lists %v %s: %d rows, total %d, want %d", lists, order, len(full.Data.Results), full.Data.Total, len(f.rows)*len(lists))
			}

			// The order: bucket key monotonic (no language first asc), id monotonic within a key.
			for i := 1; i < len(full.Data.Results); i++ {
				pa, pb := byID[full.Data.Results[i-1].ID], byID[full.Data.Results[i].ID]
				ka, kb := bucketKey(pa), bucketKey(pb)
				bad := false
				if order == "asc" {
					bad = ka > kb || (ka == kb && pa.ID > pb.ID)
				} else {
					bad = ka < kb || (ka == kb && pa.ID < pb.ID)
				}
				if bad {
					t.Fatalf("lists %v %s: row %d (%d %q) before row %d (%d %q)", lists, order, i-1, pa.ID, ka, i, pb.ID, kb)
				}
			}
			first, last := bucketKey(byID[full.Data.Results[0].ID]), bucketKey(byID[full.Data.Results[len(full.Data.Results)-1].ID])
			if order == "asc" && first != "" || order == "desc" && last != "" {
				t.Fatalf("lists %v %s: no language not at the %s end (first %q, last %q)", lists, order, map[string]string{"asc": "start", "desc": "end"}[order], first, last)
			}

			// Paging: small pages concatenated equal the unpaged sequence, row for row.
			var paged []int
			for pg := 1; ; pg++ {
				q := url.Values{}
				for k, v := range base {
					q[k] = v
				}
				q.Set("per_page", "7")
				q.Set("page", fmt.Sprint(pg))
				p := get(q)
				if len(p.Data.Results) == 0 {
					break
				}
				for _, r := range p.Data.Results {
					paged = append(paged, r.ID)
				}
				if pg > 1000 {
					t.Fatal("paging did not end")
				}
			}
			if len(paged) != len(full.Data.Results) {
				t.Fatalf("lists %v %s: paged %d rows, unpaged %d", lists, order, len(paged), len(full.Data.Results))
			}
			for i, id := range paged {
				if id != full.Data.Results[i].ID {
					t.Fatalf("lists %v %s: paged row %d is %d, unpaged %d", lists, order, i, id, full.Data.Results[i].ID)
				}
			}
		}
	}
}

// I7 -- lang_none_total is present iff the request's lang is en (0 included, on the zero-total path
// too), equals the count of the same filter narrowed to no language, and total - lang_none_total
// equals the lang=en_only total of the same filter -- with a search, a list, a segment and an
// advanced query applied, and with two lists.
func TestSubscribersLangNoneTotal(t *testing.T) {
	f := newGridFixture(t)

	type resp struct {
		Data struct {
			Total         int  `json:"total"`
			LangNoneTotal *int `json:"lang_none_total"`
		} `json:"data"`
	}
	get := func(q url.Values) resp {
		t.Helper()
		code, body := f.call(f.app.QuerySubscribers, http.MethodGet, "/api/subscribers?"+q.Encode(), "", adminUser)
		if code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", q.Encode(), code, body)
		}
		var r resp
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	with := func(base url.Values, k, v string) url.Values {
		q := url.Values{}
		for kk, vv := range base {
			q[kk] = append([]string(nil), vv...)
		}
		if v == "" {
			q.Del(k)
		} else {
			q.Set(k, v)
		}
		return q
	}

	cases := []struct {
		name string
		q    url.Values
	}{
		{"no filter", url.Values{}},
		{"one list", url.Values{"list_id": {fmt.Sprint(f.single)}}},
		{"list + segment", url.Values{"list_id": {fmt.Sprint(f.double)}, "segment": {"active"}}},
		{"list + segment + search", url.Values{"list_id": {fmt.Sprint(f.single)}, "segment": {"unsubscribed"}, "search": {"enabled"}}},
		{"list + advanced query", url.Values{"list_id": {fmt.Sprint(f.single)}, "query": {"subscribers.status = 'enabled'"}}},
		{"two lists", url.Values{"list_id": {fmt.Sprint(f.single), fmt.Sprint(f.double)}}},
		{"two lists + search", url.Values{"list_id": {fmt.Sprint(f.single), fmt.Sprint(f.double)}, "search": {"blocklisted"}}},
	}
	for _, tc := range cases {
		en := get(with(tc.q, "lang", "en"))
		if en.Data.LangNoneTotal == nil {
			t.Fatalf("%s: lang=en without lang_none_total", tc.name)
		}
		none := get(with(tc.q, "lang", "none"))
		only := get(with(tc.q, "lang", "en_only"))
		if *en.Data.LangNoneTotal != none.Data.Total {
			t.Errorf("%s: lang_none_total %d, lang=none total %d", tc.name, *en.Data.LangNoneTotal, none.Data.Total)
		}
		if en.Data.Total-*en.Data.LangNoneTotal != only.Data.Total {
			t.Errorf("%s: total %d - lang_none_total %d != lang=en_only total %d", tc.name, en.Data.Total, *en.Data.LangNoneTotal, only.Data.Total)
		}
		if *en.Data.LangNoneTotal == 0 || only.Data.Total == 0 {
			t.Errorf("%s: a degenerate case (none %d, en_only %d) proves nothing", tc.name, *en.Data.LangNoneTotal, only.Data.Total)
		}
		// Absent on every other request.
		for _, l := range []string{"", "fr", "none", "en_only", "other"} {
			if r := get(with(tc.q, "lang", l)); r.Data.LangNoneTotal != nil {
				t.Errorf("%s: lang=%q carries lang_none_total %d", tc.name, l, *r.Data.LangNoneTotal)
			}
		}
	}

	// The independent count: the fixture's own decode on one list (every row is on both lists).
	wantNone := len(f.want(false, "", "none"))
	if r := get(url.Values{"list_id": {fmt.Sprint(f.single)}, "lang": {"en"}}); r.Data.LangNoneTotal == nil || *r.Data.LangNoneTotal != wantNone {
		t.Errorf("one list: lang_none_total %v, fixture decode %d", r.Data.LangNoneTotal, wantNone)
	}

	// S2-4 -- the zero-total path still answers 0 on the wire.
	r := get(url.Values{"lang": {"en"}, "search": {"no-such-subscriber-xyz"}})
	if r.Data.Total != 0 || r.Data.LangNoneTotal == nil || *r.Data.LangNoneTotal != 0 {
		t.Errorf("zero total: total %d, lang_none_total %v (want present 0)", r.Data.Total, r.Data.LangNoneTotal)
	}
}
