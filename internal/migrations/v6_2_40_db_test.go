package migrations

// Fork (60-day opens) -- integrations listmonk/SIXTY-DAY-OPENS-SPEC T11 (migration half) and I14:
// v6.2.40 brings a v6.2.39 database to exactly schema.sql's shape (campaign_smart_skips.reason and the
// two functions -- the window and floor literals among them), backfills every NULL reason by the one
// column test (lookahead where due_list_id is set, else lookback), never overwrites a stored reason,
// and a second run changes nothing. schema.sql declares the DDL verbatim. Same opt-in harness as
// evergreen_db_test.go (LISTMONK_TEST_PG). The readers are cmd/silent_skip_db_test.go.

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (h *evergreenHarness) silentShape() string {
	var fn []string
	if err := h.db.Select(&fn, `SELECT pg_get_functiondef(p.oid) FROM pg_proc p
		WHERE p.proname IN ('open_observable_provider', 'brand_silent_skips') ORDER BY p.proname`); err != nil {
		h.t.Fatal(err)
	}
	var col string
	h.db.Get(&col, `SELECT data_type || ' ' || is_nullable FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'campaign_smart_skips' AND column_name = 'reason'`)
	return col + " | " + strings.Join(fn, "; ")
}

func TestSilentSkipsMigration_V6_2_40(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(silentSkipsDDL)) {
		t.Fatal("silentSkipsDDL is not in schema.sql verbatim")
	}

	fresh := h.silentShape()
	for _, want := range []string{
		"text YES |", "open_observable_provider(p_email text)", "brand_silent_skips(camp_id integer, p_preview boolean)",
		"INTERVAL '60 days'", "3 AS floor", "'outlook-windows', 'outlook-mac', 'outlook-mobile', 'thunderbird'",
		`COLLATE "C"`, "'silent_skip'", "STABLE", "IMMUTABLE",
	} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh schema lacks %q: %s", want, fresh)
		}
	}
	// No CHECK constraint on the skip table (a raise inside the claim stalls every claim).
	var checks int
	h.db.Get(&checks, `SELECT COUNT(*) FROM pg_constraint WHERE conrelid = 'campaign_smart_skips'::regclass AND contype = 'c'`)
	if checks != 0 {
		t.Fatalf("campaign_smart_skips carries %d CHECK constraint(s)", checks)
	}

	// A v6.2.39 database: no column, no functions, rows written by the older claim.
	h.db.MustExec(`DROP FUNCTION brand_silent_skips(INT, BOOLEAN)`)
	h.db.MustExec(`DROP FUNCTION open_observable_provider(TEXT)`)
	h.db.MustExec(`ALTER TABLE campaign_smart_skips DROP COLUMN reason`)
	camp := h.campaign("broadcast", false, 0, h.listA)
	h.db.MustExec(`INSERT INTO campaign_smart_skips (campaign_id, subscriber_id, journey_campaign_id, journey_sent_at) VALUES ($1, 1, 9, NOW())`, camp)
	h.db.MustExec(`INSERT INTO campaign_smart_skips (campaign_id, subscriber_id, due_list_id, due_at) VALUES ($1, 2, 9, NOW())`, camp)

	if err := V6_2_40(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_40 run 1: %v", err)
	}
	if got := h.silentShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}
	reasons := func() string {
		var s string
		if err := h.db.Get(&s, `SELECT STRING_AGG(subscriber_id::TEXT || '=' || COALESCE(reason, '-'), ' ' ORDER BY subscriber_id) FROM campaign_smart_skips`); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if got := reasons(); got != "1=lookback 2=lookahead" {
		t.Fatalf("backfilled reasons %q", got)
	}

	// A stored reason is never overwritten; a second run changes nothing.
	h.db.MustExec(`INSERT INTO campaign_smart_skips (campaign_id, subscriber_id, reason) VALUES ($1, 3, 'silent')`, camp)
	before := reasons()
	if err := V6_2_40(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_40 run 2: %v", err)
	}
	if after := reasons(); after != before || after != "1=lookback 2=lookahead 3=silent" {
		t.Fatalf("second run: before %q after %q", before, after)
	}
	if got := h.silentShape(); got != fresh {
		t.Fatalf("second run shape differs")
	}
}
