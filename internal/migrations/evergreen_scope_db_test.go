package migrations

// Fork (erinos evergreen campaigns) — DB harness for the Broadcasts | Automations
// scope on query-campaigns and delete-campaigns (the `evergreen` param, bound as
// NULL / false / true). Shares newEvergreenHarness (see evergreen_db_test.go for
// the LISTMONK_TEST_PG opt-in). query-campaigns carries %order%, which
// makeSearchQuery replaces textually, so the harness does the same before Preparex.

import (
	"sort"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func TestEvergreenScopeQueries(t *testing.T) {
	h := newEvergreenHarness(t)

	qc, ok := h.qs["query-campaigns"]
	if !ok {
		t.Fatal("query-campaigns not found")
	}
	query, err := h.db.Preparex(strings.ReplaceAll(qc.Query, "%order%", "created_at DESC"))
	if err != nil {
		t.Fatalf("prepare query-campaigns: %v", err)
	}
	dc, ok := h.qs["delete-campaigns"]
	if !ok {
		t.Fatal("delete-campaigns not found")
	}
	del, err := h.db.Preparex(dc.Query)
	if err != nil {
		t.Fatalf("prepare delete-campaigns: %v", err)
	}

	b1 := h.campaign("broadcast-1", false, 0, h.listA)
	b2 := h.campaign("broadcast-2", false, 0, h.listA)
	e1 := h.campaign("automation-1", true, 0, h.listA)

	list := func(evergreen *bool) ([]int, int) {
		rows, err := query.Queryx(0, pq.StringArray{}, pq.StringArray{}, "", true, pq.Array([]int{}), 0, 0, evergreen)
		if err != nil {
			t.Fatalf("query-campaigns: %v", err)
		}
		defer rows.Close()
		var ids []int
		total := 0
		for rows.Next() {
			m := map[string]interface{}{}
			if err := rows.MapScan(m); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, int(m["id"].(int64)))
			total = int(m["total"].(int64))
		}
		sort.Ints(ids)
		return ids, total
	}
	bp := func(b bool) *bool { return &b }

	// query-campaigns with NULL / false / true.
	for _, tc := range []struct {
		name  string
		scope *bool
		want  []int
	}{
		{"query NULL", nil, []int{b1, b2, e1}},
		{"query false", bp(false), []int{b1, b2}},
		{"query true", bp(true), []int{e1}},
	} {
		ids, total := list(tc.scope)
		eq(t, tc.name, ids, tc.want)
		if total != len(tc.want) {
			t.Errorf("%s: total = %d, want %d", tc.name, total, len(tc.want))
		}
	}

	// delete-campaigns by query (no ids), scoped true, then false, then NULL.
	execDel := func(name string, scope *bool, wantLeft []int) {
		if _, err := del.Exec(pq.Array([]int{}), "", true, pq.Array([]int{}), scope); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		ids, _ := list(nil)
		eq(t, name, ids, wantLeft)
	}
	// An explicit id list is never scoped (the by-ids branch is unaffected by the param).
	if _, err := del.Exec(pq.Array([]int{b2}), "", true, pq.Array([]int{}), bp(true)); err != nil {
		t.Fatalf("delete by id with mismatched scope: %v", err)
	}
	ids, _ := list(nil)
	eq(t, "delete by id ignores scope", ids, []int{b1, e1})
	b2 = h.campaign("broadcast-2", false, 0, h.listA)

	execDel("delete true leaves broadcasts", bp(true), []int{b1, b2})
	e2 := h.campaign("automation-2", true, 0, h.listA)
	execDel("delete false leaves automations", bp(false), []int{e2})
	b3 := h.campaign("broadcast-3", false, 0, h.listA)
	_ = b3
	execDel("delete NULL removes everything", nil, nil)
}
