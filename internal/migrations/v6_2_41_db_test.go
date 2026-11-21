package migrations

// Fork (promo series) -- integrations listmonk/PROMO-SERIES-SPEC §4 (the migration test) and I17:
// v6.2.41 brings a v6.2.40 database to exactly schema.sql's shape (the promo_purchases table, its
// index and the six functions -- the 60-day window and the 24-hour staleness limit, one literal each,
// among them); a second run changes nothing and keeps the rows. schema.sql declares the DDL
// verbatim. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG). The readers are
// cmd/promo_db_test.go.

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lib/pq"
)

var promoFunctions = []string{
	"promo_email_hash", "promo_applicable_tags", "promo_campaign_tags",
	"promo_matching_orders", "promo_feed_stale", "promo_purchasers",
}

func (h *evergreenHarness) promoShape() string {
	var fn []string
	if err := h.db.Select(&fn, `SELECT pg_get_functiondef(p.oid) FROM pg_proc p
		WHERE p.proname = ANY($1) ORDER BY p.proname`, pq.StringArray(promoFunctions)); err != nil {
		h.t.Fatal(err)
	}
	var cols []string
	h.db.Select(&cols, `SELECT column_name || ' ' || data_type || ' ' || is_nullable || ' ' || COALESCE(column_default, '-')
		FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'promo_purchases'
		ORDER BY ordinal_position`)
	var idx []string
	h.db.Select(&idx, `SELECT indexdef FROM pg_indexes WHERE tablename = 'promo_purchases'
		OR indexname = 'idx_subs_promo_email_hash' ORDER BY indexname`)
	return strings.Join(cols, ", ") + " | " + strings.Join(idx, "; ") + " | " + strings.Join(fn, "; ")
}

func TestPromoSeriesMigration_V6_2_41(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(promoSeriesDDL)) {
		t.Fatal("promoSeriesDDL is not in schema.sql verbatim")
	}

	fresh := h.promoShape()
	for _, want := range []string{
		"order_id text NO -", "value_lc text NO -", "email_hash text NO -", "received_at timestamp with time zone NO now()",
		"promo_purchases_pkey", "(order_id, kind, value)", "idx_promo_purchases_ordered_at", "(ordered_at)",
		"promo_email_hash(email text)", "promo_applicable_tags(p_type text, p_evergreen boolean, p_code jsonb, p_step jsonb, p_tags jsonb)",
		"promo_campaign_tags(camp_id integer)", "promo_matching_orders(p_tags text[])", "promo_feed_stale()",
		"promo_purchasers(p_camp_id integer, p_tags text[])", "IMMUTABLE",
		// Stage 4 V1 -- the expression index the purchaser lookup probes.
		"CREATE INDEX idx_subs_promo_email_hash ON public.subscribers USING btree (promo_email_hash(email))",
	} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh schema lacks %q: %s", want, fresh)
		}
	}
	// I17 -- the window and the staleness limit are ONE literal each, in their one function.
	if n := strings.Count(fresh, "INTERVAL '60 days'"); n != 1 {
		t.Fatalf("the 60-day window literal appears %d times across the promo functions, want 1", n)
	}
	if n := strings.Count(fresh, "INTERVAL '24 hours'"); n != 1 {
		t.Fatalf("the 24-hour staleness literal appears %d times across the promo functions, want 1", n)
	}
	// No CHECK constraint on the feed (kind is validated by the API).
	var checks int
	h.db.Get(&checks, `SELECT COUNT(*) FROM pg_constraint WHERE conrelid = 'promo_purchases'::regclass AND contype = 'c'`)
	if checks != 0 {
		t.Fatalf("promo_purchases carries %d CHECK constraint(s)", checks)
	}

	// A v6.2.40 database: no table, no functions, no index.
	h.db.MustExec(`DROP INDEX idx_subs_promo_email_hash`)
	h.db.MustExec(`DROP FUNCTION promo_purchasers(INT, TEXT[])`)
	h.db.MustExec(`DROP FUNCTION promo_feed_stale()`)
	h.db.MustExec(`DROP FUNCTION promo_matching_orders(TEXT[])`)
	h.db.MustExec(`DROP FUNCTION promo_campaign_tags(INT)`)
	h.db.MustExec(`DROP FUNCTION promo_applicable_tags(TEXT, BOOLEAN, JSONB, JSONB, JSONB)`)
	h.db.MustExec(`DROP FUNCTION promo_email_hash(TEXT)`)
	h.db.MustExec(`DROP TABLE promo_purchases`)

	// PROMO-SERIES-SPEC G6 -- the migration ANALYZEs subscribers, so the new expression index has
	// planner statistics from the start (without them production read every subscriber per lookup).
	// Rows must exist for statistics to exist.
	h.db.MustExec(`INSERT INTO subscribers (uuid, email, name)
		SELECT gen_random_uuid(), 'g6-stats-' || g || '@example.com', 'g6' FROM generate_series(1, 50) g`)
	var statsBefore int
	h.db.Get(&statsBefore, `SELECT COUNT(*) FROM pg_stats WHERE tablename = 'idx_subs_promo_email_hash'`)
	if statsBefore != 0 {
		t.Fatalf("the dropped index still has %d statistics row(s)", statsBefore)
	}

	if err := V6_2_41(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_41 run 1: %v", err)
	}
	if got := h.promoShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}
	var statsAfter int
	h.db.Get(&statsAfter, `SELECT COUNT(*) FROM pg_stats WHERE tablename = 'idx_subs_promo_email_hash'`)
	if statsAfter != 1 {
		t.Fatalf("after the migration the expression index has %d statistics row(s), want 1 (ANALYZE subscribers missing?)", statsAfter)
	}
	h.db.MustExec(`INSERT INTO promo_purchases (order_id, kind, value, value_lc, email_hash, ordered_at) VALUES ('o1', 'sku', 'X', 'x', 'h', NOW())`)
	if err := V6_2_41(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_41 run 2: %v", err)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM promo_purchases`)
	if n != 1 {
		t.Fatalf("second run left %d rows, want 1", n)
	}
	if got := h.promoShape(); got != fresh {
		t.Fatalf("second run shape differs")
	}
	// The index exists after both runs, exactly once, and is valid.
	var idxN int
	h.db.Get(&idxN, `SELECT COUNT(*) FROM pg_index i JOIN pg_class c ON (c.oid = i.indexrelid)
		WHERE c.relname = 'idx_subs_promo_email_hash' AND i.indisvalid`)
	if idxN != 1 {
		t.Fatalf("idx_subs_promo_email_hash: %d valid index(es) after two runs, want 1", idxN)
	}
}
