package main

// Fork (campaign lists) -- integrations listmonk/CAMPAIGN-LISTS-SPEC I5 and the DB half of I2: the
// query-lists hide_system predicate (the SQL statement of D6) agrees with cmd isHiddenSystemList
// over every I1 case (systemListCases, lists_system_test.go), total and pagination follow the
// filter, a hidden list is still readable by id, and system_hidden rides on the listing and on GET
// /api/lists/:id. Through the real router (bpFixture). LISTMONK_TEST_PG opt-in (newLinkHarness
// skips without it).

import (
	"encoding/json"
	"net/http"
	"sort"
	"testing"
)

type systemListResp struct {
	ID            int  `json:"id"`
	SystemHidden  bool `json:"system_hidden"`
	JourneyLocked bool `json:"journey_locked"`
}

func TestQueryListsHideSystemAgreesWithGo(t *testing.T) {
	f := newBPFixture(t)

	ids := map[int]systemListCase{}
	for _, c := range systemListCases {
		id := f.sqlList("sys "+c.name, c.tags...)
		if c.trigger != "" {
			f.h.db.MustExec(`UPDATE lists SET join_trigger = $2::JSONB WHERE id = $1`, id, c.trigger)
		}
		ids[id] = c
	}

	// The Go function over the rows the database holds (every list, the harness's own included).
	all, total, err := f.h.app.core.QueryLists("", "", "", "", nil, "id", "asc", true, nil, false, false, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != len(all) || len(all) < len(systemListCases) {
		t.Fatalf("unfiltered: %d rows, total %d", len(all), total)
	}
	var want []int
	for _, l := range all {
		if c, ok := ids[l.ID]; ok && isHiddenSystemList(l) != c.hidden {
			t.Fatalf("%s: the stored row reads hidden=%v, the case says %v", c.name, isHiddenSystemList(l), c.hidden)
		}
		if !isHiddenSystemList(l) {
			want = append(want, l.ID)
		}
	}
	shown, shownTotal, err := f.h.app.core.QueryLists("", "", "", "", nil, "id", "asc", true, nil, false, true, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, l := range shown {
		got = append(got, l.ID)
	}
	if !sameIntSet(got, want) {
		t.Fatalf("hide_system ids %v, Go says %v", got, want)
	}
	if shownTotal != len(want) {
		t.Fatalf("hide_system total %d want %d", shownTotal, len(want))
	}

	// Through the API: the parameter filters, total follows, pagination follows, and every row
	// carries system_hidden equal to the Go function.
	type page struct {
		Results []systemListResp `json:"results"`
		Total   int              `json:"total"`
	}
	var full, filtered, paged page
	json.Unmarshal(f.ok("GET lists", f.admin.json(http.MethodGet, "/api/lists?per_page=all", nil)), &full)
	json.Unmarshal(f.ok("GET lists hide_system", f.admin.json(http.MethodGet, "/api/lists?per_page=all&hide_system=true", nil)), &filtered)
	json.Unmarshal(f.ok("GET lists hide_system page", f.admin.json(http.MethodGet, "/api/lists?per_page=2&page=1&hide_system=true", nil)), &paged)
	if full.Total != len(all) || len(full.Results) != len(all) {
		t.Fatalf("API unfiltered total %d rows %d, want %d", full.Total, len(full.Results), len(all))
	}
	var apiGot []int
	for _, r := range filtered.Results {
		apiGot = append(apiGot, r.ID)
	}
	if !sameIntSet(apiGot, want) || filtered.Total != len(want) {
		t.Fatalf("API hide_system ids %v total %d, want %v", apiGot, filtered.Total, want)
	}
	if paged.Total != len(want) || len(paged.Results) != 2 {
		t.Fatalf("API hide_system page: total %d rows %d", paged.Total, len(paged.Results))
	}
	for _, r := range full.Results {
		if c, ok := ids[r.ID]; ok && r.SystemHidden != c.hidden {
			t.Fatalf("%s: listing system_hidden %v want %v", c.name, r.SystemHidden, c.hidden)
		}
	}
	// The minimal listing carries it too.
	var minimal page
	json.Unmarshal(f.ok("GET lists minimal", f.admin.json(http.MethodGet, "/api/lists?minimal=true", nil)), &minimal)
	for _, r := range minimal.Results {
		if c, ok := ids[r.ID]; ok && r.SystemHidden != c.hidden {
			t.Fatalf("%s: minimal system_hidden %v want %v", c.name, r.SystemHidden, c.hidden)
		}
	}

	// A hidden list is always readable by id: core.GetList and GET /api/lists/:id, the latter
	// carrying system_hidden (and journey_locked) like the listing.
	for id, c := range ids {
		l, err := f.h.app.core.GetList(id, "")
		if err != nil || l.ID != id {
			t.Fatalf("%s: GetList %d: %v", c.name, id, err)
		}
		var one systemListResp
		json.Unmarshal(f.ok("GET list "+c.name, f.admin.json(http.MethodGet, "/api/lists/"+itoa(id), nil)), &one)
		if one.ID != id || one.SystemHidden != c.hidden || one.JourneyLocked != isJourneyLockedList(l) {
			t.Fatalf("%s: GET /api/lists/%d = %+v want hidden %v", c.name, id, one, c.hidden)
		}
	}
}

// sameIntSet compares two id lists as sets (order ignored).
func sameIntSet(a, b []int) bool {
	a = append([]int(nil), a...)
	b = append([]int(nil), b...)
	sort.Ints(a)
	sort.Ints(b)
	return sameInts(a, b)
}
