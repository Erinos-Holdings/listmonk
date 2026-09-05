package migrations

// Fork (lists page sort) -- pins that query-lists sorts the WHOLE result set before
// paginating. Upstream applied OFFSET/LIMIT inside the ls CTE with no ORDER BY, so a
// sort only re-ordered the rows already on the page. Same opt-in harness as
// evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func TestQueryListsSortsBeforePaginating(t *testing.T) {
	h := newEvergreenHarness(t)

	q, ok := h.qs["query-lists"]
	if !ok {
		t.Fatal("query query-lists not found")
	}

	// Five lists whose created_at order coincides with id order in NEITHER
	// direction (ages in hours by insertion: 2,0,4,1,3), so a page picked by physical
	// or id order can never match a page picked by created_at, ascending or descending.
	ages := []int{2, 0, 4, 1, 3}
	var ids []int
	for i, age := range ages {
		var id int
		if err := h.db.Get(&id, fmt.Sprintf(`INSERT INTO lists (uuid, name, type, optin, created_at)
			VALUES (gen_random_uuid(), 'sort-%d', 'private', 'single', NOW() - INTERVAL '%d hours') RETURNING id`, i, age)); err != nil {
			t.Fatalf("insert list: %v", err)
		}
		ids = append(ids, id)
	}
	// Newest first by created_at.
	newest := []int{ids[1], ids[3], ids[0], ids[4], ids[2]}
	h.db.MustExec(`DELETE FROM lists WHERE id <> ALL($1)`, pq.Array(ids))
	h.db.MustExec(`REFRESH MATERIALIZED VIEW mat_list_subscriber_stats`)

	page := func(order string, offset, limit int) (got []int, total int) {
		stmt, err := h.db.Preparex(strings.ReplaceAll(q.Query, "%order%", order))
		if err != nil {
			t.Fatalf("prepare query-lists: %v", err)
		}
		defer stmt.Close()
		rows, err := stmt.Queryx(0, "", "", "", "", "", pq.StringArray{}, true, pq.Array([]int{}), offset, limit)
		if err != nil {
			t.Fatalf("query-lists: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			m := map[string]interface{}{}
			if err := rows.MapScan(m); err != nil {
				t.Fatal(err)
			}
			got = append(got, int(m["id"].(int64)))
			total = int(m["total"].(int64))
		}
		return got, total
	}

	// created_at DESC, two per page, walks newest exactly.
	got, total := page("created_at DESC", 0, 2)
	eq(t, "created_at desc page 1", got, newest[0:2])
	if total != 5 {
		t.Fatalf("total: got %d want 5", total)
	}
	got, _ = page("created_at DESC", 2, 2)
	eq(t, "created_at desc page 2", got, newest[2:4])
	got, _ = page("created_at DESC", 4, 2)
	eq(t, "created_at desc page 3", got, newest[4:5])

	// created_at ASC: oldest first, the reverse walk.
	got, _ = page("created_at ASC", 0, 2)
	eq(t, "created_at asc page 1", got, []int{newest[4], newest[3]})

	// A sort key that comes from the join, not the lists table.
	sub := h.subscriber("sort@example.com")
	h.join(sub, ids[2], "confirmed")
	h.db.MustExec(`REFRESH MATERIALIZED VIEW mat_list_subscriber_stats`)
	got, _ = page("subscriber_count DESC, id ASC", 0, 1)
	eq(t, "subscriber_count desc page 1", got, []int{ids[2]})

	// limit < 1 means no limit: every row, in order.
	got, _ = page("created_at DESC", 0, 0)
	eq(t, "unlimited", got, newest)

	// Ties: give every list the same created_at and page one row at a time. With
	// no total order Postgres may hand back a list twice and another never; the
	// ls.id tiebreaker makes the walk exactly the id order, no gap, no repeat. The
	// two extra UPDATEs move those rows to the end of the heap, so a heap-order walk
	// (what an untied sort degenerates to) would NOT be id order.
	h.db.MustExec(`UPDATE lists SET created_at = '2026-01-01'`)
	h.db.MustExec(`UPDATE lists SET name = name WHERE id = $1`, ids[0])
	h.db.MustExec(`UPDATE lists SET name = name WHERE id = $1`, ids[2])
	var walk []int
	for off := 0; off < len(ids); off++ {
		got, _ = page("created_at DESC", off, 1)
		walk = append(walk, got...)
	}
	eq(t, "tied created_at walk", walk, sorted(ids))
}
