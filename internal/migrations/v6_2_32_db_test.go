package migrations

// Fork (smart sending look-ahead) -- integrations listmonk/SMART-SENDING-LOOKAHEAD-SPEC L8 (the
// migration half): v6.2.32 brings a v6.2.31 database to exactly schema.sql's shape (journey_due and
// its index, campaign_smart_skips' nullable journey pair and look-ahead pair, the four function
// bodies, the switch row inserted false), is idempotent, and leaves a pre-migration skip row
// readable. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG). The behaviour is
// cmd/smart_sending_db_test.go and cmd/journeys_due_db_test.go.

import (
	"log"
	"os"
	"strings"
	"testing"
)

func (h *evergreenHarness) lookaheadShape() string {
	var cols []string
	if err := h.db.Select(&cols, `SELECT table_name || '.' || column_name || ' ' || data_type || ' ' || is_nullable || ' ' || COALESCE(column_default, '')
		FROM information_schema.columns WHERE table_name IN ('campaign_smart_skips', 'journey_due') ORDER BY table_name, ordinal_position`); err != nil {
		h.t.Fatal(err)
	}
	var idx []string
	if err := h.db.Select(&idx, `SELECT indexdef FROM pg_indexes WHERE tablename IN ('campaign_smart_skips', 'journey_due') ORDER BY indexname`); err != nil {
		h.t.Fatal(err)
	}
	var cons []string
	if err := h.db.Select(&cons, `SELECT conrelid::regclass::text || ' ' || pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conrelid IN ('campaign_smart_skips'::regclass, 'journey_due'::regclass) ORDER BY conrelid::regclass::text, contype, conname`); err != nil {
		h.t.Fatal(err)
	}
	var fn []string
	if err := h.db.Select(&fn, `SELECT pg_get_functiondef(p.oid) FROM pg_proc p
		WHERE p.proname IN ('smart_sending_brand', 'brand_journey_sends_24h', 'brand_journey_due_24h', 'brand_journey_skips') ORDER BY p.proname`); err != nil {
		h.t.Fatal(err)
	}
	var sw []string
	if err := h.db.Select(&sw, `SELECT value::text FROM settings WHERE key = 'app.smart_sending_lookahead'`); err != nil {
		h.t.Fatal(err)
	}
	return strings.Join(cols, "; ") + " | " + strings.Join(idx, "; ") + " | " + strings.Join(cons, "; ") +
		" | " + strings.Join(fn, "; ") + " | switch " + strings.Join(sw, ",")
}

func TestSmartSendingLookaheadMigration_V6_2_32(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	fresh := h.lookaheadShape()
	for _, want := range []string{
		"PRIMARY KEY (source, list_id, subscriber_id, expected_at)", "idx_journey_due_expected_at",
		"campaign_smart_skips.journey_campaign_id integer YES", "campaign_smart_skips.journey_sent_at timestamp with time zone YES",
		"campaign_smart_skips.due_list_id integer YES", "campaign_smart_skips.due_at timestamp with time zone YES",
		"brand_journey_due_24h", "brand_journey_skips", "smart_sending_brand(camp_id)", "switch false",
	} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh schema lacks %q: %s", want, fresh)
		}
	}
	// S1 -- no CHECK on the skip record: a raise inside next-campaigns would stall every claim.
	if strings.Contains(fresh, "CHECK") {
		t.Fatalf("a CHECK constraint on the skip record or the feed: %s", fresh)
	}

	// A v6.2.31 database: no feed table, no new functions, no switch row, the skip record's journey
	// pair NOT NULL and no look-ahead pair, the v6.2.31 look-back body -- with a skip row.
	h.db.MustExec(`DROP FUNCTION brand_journey_skips(INT)`)
	h.db.MustExec(`DROP FUNCTION brand_journey_due_24h(INT)`)
	h.db.MustExec(`DROP TABLE journey_due`)
	h.db.MustExec(`DELETE FROM settings WHERE key = 'app.smart_sending_lookahead'`)
	h.db.MustExec(`ALTER TABLE campaign_smart_skips DROP COLUMN due_list_id, DROP COLUMN due_at`)
	h.db.MustExec(`ALTER TABLE campaign_smart_skips ALTER COLUMN journey_campaign_id SET NOT NULL, ALTER COLUMN journey_sent_at SET NOT NULL`)
	if err := V6_2_31(h.db, nil, nil, lo); err != nil { // restores the v6.2.31 look-back body
		t.Fatal(err)
	}
	h.db.MustExec(`DROP FUNCTION smart_sending_brand(INT)`)
	camp := h.campaign("b", false, 0, h.listA)
	journey := h.campaign("j", true, 0, h.listA)
	h.db.MustExec(`INSERT INTO campaign_smart_skips (campaign_id, subscriber_id, journey_campaign_id, journey_sent_at) VALUES ($1, 7, $2, '2026-11-01T10:00:00Z')`, camp, journey)

	for i := 0; i < 2; i++ {
		if err := V6_2_32(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_32 run %d: %v", i+1, err)
		}
	}
	if got := h.lookaheadShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}

	// The pre-migration row reads with its journey pair and an empty look-ahead pair.
	var row struct {
		Journey *int    `db:"journey_campaign_id"`
		DueList *int    `db:"due_list_id"`
		DueAt   *string `db:"due_at"`
	}
	if err := h.db.Get(&row, `SELECT journey_campaign_id, due_list_id, due_at::text FROM campaign_smart_skips WHERE campaign_id = $1`, camp); err != nil {
		t.Fatal(err)
	}
	if row.Journey == nil || *row.Journey != journey || row.DueList != nil || row.DueAt != nil {
		t.Fatalf("pre-migration skip row %+v", row)
	}

	// A switch row an operator already set survives a re-run (ON CONFLICT DO NOTHING).
	h.db.MustExec(`UPDATE settings SET value = 'true' WHERE key = 'app.smart_sending_lookahead'`)
	if err := V6_2_32(h.db, nil, nil, lo); err != nil {
		t.Fatal(err)
	}
	var v string
	h.db.Get(&v, `SELECT value::text FROM settings WHERE key = 'app.smart_sending_lookahead'`)
	if v != "true" {
		t.Fatalf("a re-run reset the switch to %s", v)
	}
}
