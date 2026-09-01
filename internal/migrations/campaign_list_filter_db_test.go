package migrations

// Fork (erinos campaigns list filter) -- DB harness for I18N-SPEC E1, the repeatable
// list_id filter on GET /api/campaigns, run through the REAL goyesql-parsed
// query-campaigns statement. Same opt-in harness as evergreen_db_test.go
// (LISTMONK_TEST_PG); see that file's header for the DSN.

import (
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// queryCampaigns prepares query-campaigns the way core.makeSearchQuery does (the
// %order% placeholder is substituted in Go, not bound) and returns the campaign ids
// it yields for the given list filter. listIDs is passed to the driver verbatim so a
// test can bind SQL NULL as well as an empty array.
func (h *evergreenHarness) queryCampaigns(t *testing.T, stmt *sqlx.Stmt, evergreen interface{}, listIDs interface{}) []int {
	t.Helper()

	rows, err := stmt.Queryx(0, pq.StringArray{}, pq.StringArray{}, "", true, pq.Array([]int{}), 0, 0, evergreen, listIDs)
	if err != nil {
		t.Fatalf("query-campaigns: %v", err)
	}
	defer rows.Close()

	var out []int
	for rows.Next() {
		m := map[string]interface{}{}
		if err := rows.MapScan(m); err != nil {
			t.Fatal(err)
		}
		out = append(out, int(m["id"].(int64)))
	}
	return sorted(out)
}

func TestCampaignListFilter(t *testing.T) {
	h := newEvergreenHarness(t)

	q, ok := h.qs["query-campaigns"]
	if !ok {
		t.Fatal("query query-campaigns not found")
	}
	stmt, err := h.db.Preparex(strings.ReplaceAll(q.Query, "%order%", "c.created_at DESC"))
	if err != nil {
		t.Fatalf("prepare query-campaigns: %v", err)
	}

	A := h.listA
	var B, C int
	h.db.Get(&B, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'B', 'private', 'single') RETURNING id`)
	h.db.Get(&C, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'C', 'private', 'single') RETURNING id`)

	onA := h.campaign("on-a", false, 0, A)
	onB := h.campaign("on-b", false, 0, B)
	onAB := h.campaign("on-ab", false, 0, A, B)
	autoA := h.campaign("auto-a", true, 0, A)
	noLists := h.campaign("no-lists", false, 0)

	all := sorted([]int{onA, onB, onAB, autoA, noLists})

	// An empty array applies no filter -- this is what the handler binds when the
	// caller sends no list_id, and it must return every campaign, including one with
	// no lists at all.
	eq(t, "no filter", h.queryCampaigns(t, stmt, nil, pq.Array([]int{})), all)

	// A single list.
	eq(t, "list A", h.queryCampaigns(t, stmt, nil, pq.Array([]int{A})), sorted([]int{onA, onAB, autoA}))
	eq(t, "list B", h.queryCampaigns(t, stmt, nil, pq.Array([]int{B})), sorted([]int{onB, onAB}))

	// Repeatable -- the union, and a campaign on both lists appears exactly once.
	eq(t, "lists A+B", h.queryCampaigns(t, stmt, nil, pq.Array([]int{A, B})), sorted([]int{onA, onB, onAB, autoA}))

	// A list nothing targets.
	eq(t, "list C", h.queryCampaigns(t, stmt, nil, pq.Array([]int{C})), nil)

	// The filter composes with the evergreen scope rather than replacing it.
	eq(t, "list A broadcasts", h.queryCampaigns(t, stmt, false, pq.Array([]int{A})), sorted([]int{onA, onAB}))
	eq(t, "list A automations", h.queryCampaigns(t, stmt, true, pq.Array([]int{A})), []int{autoA})

	// THE HAZARD the CARDINALITY form exists for: binding SQL NULL instead of '{}'
	// makes CARDINALITY(NULL) NULL, so the predicate is NULL for every campaign not on
	// the (absent) list and the page comes up EMPTY. core.QueryCampaigns therefore
	// replaces a nil slice with an empty one; this pins what happens if that guard is
	// ever dropped, and why the predicate cannot be written as an IS NULL check.
	eq(t, "NULL bind", h.queryCampaigns(t, stmt, nil, nil), nil)

	// A deleted list nulls its campaign_lists.list_id (the rows survive for the name).
	// The campaign stays visible unfiltered and is simply unreachable by that list id.
	h.db.MustExec(`DELETE FROM lists WHERE id = $1`, B)
	eq(t, "deleted list id", h.queryCampaigns(t, stmt, nil, pq.Array([]int{B})), nil)
	eq(t, "unfiltered after delete", h.queryCampaigns(t, stmt, nil, pq.Array([]int{})), all)
	eq(t, "list A after delete", h.queryCampaigns(t, stmt, nil, pq.Array([]int{A})), sorted([]int{onA, onAB, autoA}))
}
