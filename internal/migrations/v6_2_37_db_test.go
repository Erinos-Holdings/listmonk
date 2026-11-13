package migrations

// Fork (join link) -- integrations listmonk/JOIN-LINK-SPEC D3/D4/D6 (the migration half): v6.2.37
// brings a v6.2.36 database to exactly schema.sql's shape (the brand_master_list_id,
// join_refusal and join_plan function bodies) and is idempotent; TestBrandMasterListID is I6's
// master choice. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG). The behaviour
// of the plan and the write is cmd/join_db_test.go.

import (
	"database/sql"
	"log"
	"os"
	"strings"
	"testing"
)

func (h *evergreenHarness) joinLinkShape() string {
	var fn []string
	if err := h.db.Select(&fn, `SELECT pg_get_functiondef(p.oid) FROM pg_proc p
		WHERE p.proname IN ('brand_master_list_id', 'join_refusal', 'join_plan') ORDER BY p.proname`); err != nil {
		h.t.Fatal(err)
	}
	return strings.Join(fn, "; ")
}

func TestJoinLinkMigration_V6_2_37(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	fresh := h.joinLinkShape()
	for _, want := range []string{
		"brand_master_list_id(p_brand text)", "join_refusal(p_sub_id integer, p_brand text)",
		"join_plan(p_sub_uuid uuid, p_camp_uuid uuid)", "IS DISTINCT FROM 'never-engaged'",
		"brand_marketing_list_ids(p_brand)", "STABLE",
	} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh schema lacks %q: %s", want, fresh)
		}
	}

	// A v6.2.36 database: none of the functions exists.
	h.db.MustExec(`DROP FUNCTION join_plan(UUID, UUID)`)
	h.db.MustExec(`DROP FUNCTION join_refusal(INT, TEXT)`)
	h.db.MustExec(`DROP FUNCTION brand_master_list_id(TEXT)`)
	if got := h.joinLinkShape(); got != "" {
		t.Fatalf("functions survived the drop: %s", got)
	}

	for i := 0; i < 2; i++ {
		if err := V6_2_37(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_37 run %d: %v", i+1, err)
		}
	}
	if got := h.joinLinkShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}
}

// TestBrandMasterListID -- I6 (the master choice): the single public, non-transactional, active
// list carrying the brand tag; NULL for no such list, for two (the liyora case), for an unknown
// or NULL brand. A private list, a transactional public list and an archived public list never
// count, so each leaves the real master the only candidate.
func TestBrandMasterListID(t *testing.T) {
	h := newEvergreenHarness(t)

	master := h.list("acme news", "public", []string{"brand:acme"}, "")
	h.list("acme step", "private", []string{"brand:acme"}, "")
	txn := h.list("acme receipts", "public", []string{"brand:acme"}, "")
	h.db.MustExec(`UPDATE lists SET transactional = TRUE WHERE id = $1`, txn)
	arch := h.list("acme old news", "public", []string{"brand:acme"}, "")
	h.db.MustExec(`UPDATE lists SET status = 'archived' WHERE id = $1`, arch)
	h.list("liyora a", "public", []string{"brand:liyora"}, "")
	h.list("liyora b", "public", []string{"brand:liyora"}, "")
	h.list("bare private", "private", []string{"brand:bare"}, "")
	h.list("untagged", "public", []string{}, "")

	get := func(brand any) sql.NullInt64 {
		var v sql.NullInt64
		if err := h.db.Get(&v, `SELECT brand_master_list_id($1)`, brand); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if v := get("acme"); !v.Valid || int(v.Int64) != master {
		t.Fatalf("acme master = %v, want %d", v, master)
	}
	for _, b := range []any{"liyora", "bare", "nope", nil} {
		if v := get(b); v.Valid {
			t.Fatalf("brand_master_list_id(%v) = %d, want NULL", b, v.Int64)
		}
	}
}
