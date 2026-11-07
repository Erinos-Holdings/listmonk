package migrations

// Fork (smart sending) -- integrations listmonk/SMART-SENDING-SPEC D7: v6.2.31 creates
// campaign_smart_skips, the partial campaign_sends.sent_at index and brand_journey_sends_24h exactly
// as schema.sql does (columns, keys, index definitions, function body), and is idempotent. Same
// opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG). The rule's behaviour is
// cmd/smart_sending_db_test.go.

import (
	"log"
	"os"
	"strings"
	"testing"
)

func (h *evergreenHarness) smartSendingShape() string {
	var cols []string
	if err := h.db.Select(&cols, `SELECT column_name || ' ' || data_type || ' ' || is_nullable || ' ' || COALESCE(column_default, '')
		FROM information_schema.columns WHERE table_name = 'campaign_smart_skips' ORDER BY ordinal_position`); err != nil {
		h.t.Fatal(err)
	}
	var idx []string
	if err := h.db.Select(&idx, `SELECT indexdef FROM pg_indexes WHERE tablename IN ('campaign_smart_skips', 'campaign_sends') ORDER BY indexname`); err != nil {
		h.t.Fatal(err)
	}
	var cons []string
	if err := h.db.Select(&cons, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = 'campaign_smart_skips'::regclass ORDER BY contype, conname`); err != nil {
		h.t.Fatal(err)
	}
	var fn []string
	if err := h.db.Select(&fn, `SELECT pg_get_functiondef(p.oid) FROM pg_proc p WHERE p.proname = 'brand_journey_sends_24h'`); err != nil {
		h.t.Fatal(err)
	}
	return strings.Join(cols, "; ") + " | " + strings.Join(idx, "; ") + " | " + strings.Join(cons, "; ") + " | " + strings.Join(fn, "; ")
}

func TestSmartSendingMigration_V6_2_31(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	fresh := h.smartSendingShape()
	for _, want := range []string{"PRIMARY KEY (campaign_id, subscriber_id)", "idx_campaign_sends_sent_at", "WHERE (sent_at IS NOT NULL)", "brand_journey_sends_24h"} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh schema lacks %q: %s", want, fresh)
		}
	}

	// A v6.2.30 database: none of the three objects. The migration creates the same shape, twice.
	h.db.MustExec(`DROP FUNCTION brand_journey_sends_24h(INT)`)
	h.db.MustExec(`DROP INDEX idx_campaign_sends_sent_at`)
	h.db.MustExec(`DROP TABLE campaign_smart_skips`)
	for i := 0; i < 2; i++ {
		if err := V6_2_31(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_31 run %d: %v", i+1, err)
		}
	}
	if got := h.smartSendingShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}

	// Deleting the campaign cascades its skip rows; a deleted journey leaves them (no FK).
	camp := h.campaign("b", false, 0, h.listA)
	journey := h.campaign("j", true, 0, h.listA)
	h.db.MustExec(`INSERT INTO campaign_smart_skips (campaign_id, subscriber_id, journey_campaign_id, journey_sent_at) VALUES ($1, 1, $2, NOW())`, camp, journey)
	h.db.MustExec(`DELETE FROM campaigns WHERE id = $1`, journey)
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM campaign_smart_skips`)
	if n != 1 {
		t.Fatalf("a deleted journey took %d rows with it", 1-n)
	}
	h.db.MustExec(`DELETE FROM campaigns WHERE id = $1`, camp)
	h.db.Get(&n, `SELECT COUNT(*) FROM campaign_smart_skips`)
	if n != 0 {
		t.Fatalf("%d skip rows survive their campaign", n)
	}
}
