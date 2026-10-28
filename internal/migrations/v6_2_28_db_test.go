package migrations

// Fork (campaign conversions) -- integrations CAMPAIGN-CONVERSIONS-SPEC D6/I6: v6.2.28 creates
// campaign_conversions exactly as schema.sql does (columns, primary key, index), is idempotent, and
// the table refuses an order twice within a measure (the primary key every dashboard sum relies
// on), an unknown measure and a negative sale, while the same order may sit once in EACH measure;
// deleting a campaign cascades its rows. Same opt-in harness as evergreen_db_test.go
// (LISTMONK_TEST_PG). The PUT handler's replace-one-measure behaviour is
// cmd/campaign_conversions_db_test.go (T6).

import (
	"log"
	"os"
	"strings"
	"testing"
)

func (h *evergreenHarness) conversionsShape() string {
	var cols []string
	if err := h.db.Select(&cols, `SELECT column_name || ' ' || data_type || ' ' || is_nullable || ' ' || COALESCE(numeric_precision::TEXT, '') || ',' || COALESCE(numeric_scale::TEXT, '')
		FROM information_schema.columns WHERE table_name = 'campaign_conversions' ORDER BY ordinal_position`); err != nil {
		h.t.Fatal(err)
	}
	var idx []string
	if err := h.db.Select(&idx, `SELECT indexdef FROM pg_indexes WHERE tablename = 'campaign_conversions' ORDER BY indexname`); err != nil {
		h.t.Fatal(err)
	}
	var checks []string
	if err := h.db.Select(&checks, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = 'campaign_conversions'::regclass ORDER BY contype, conname`); err != nil {
		h.t.Fatal(err)
	}
	return strings.Join(cols, "; ") + " | " + strings.Join(idx, "; ") + " | " + strings.Join(checks, "; ")
}

func TestCampaignConversionsMigration_V6_2_28(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	fresh := h.conversionsShape()
	if !strings.Contains(fresh, "PRIMARY KEY (measure, order_id)") || !strings.Contains(fresh, "idx_campaign_conversions_campaign") {
		t.Fatalf("fresh schema: %s", fresh)
	}

	// A v6.2.27 database: no table. The migration creates the same shape, and is idempotent.
	h.db.MustExec(`DROP TABLE campaign_conversions`)
	for i := 0; i < 2; i++ {
		if err := V6_2_28(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_28 run %d: %v", i+1, err)
		}
	}
	if got := h.conversionsShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}

	camp := h.campaign("c", false, 0, h.listA)
	ins := func(measure, order string, sales string) error {
		_, err := h.db.Exec(`INSERT INTO campaign_conversions (measure, order_id, campaign_id, brand, purchased_at, sales_usd)
			VALUES ($1, $2, $3, 'alpha', NOW(), $4::NUMERIC)`, measure, order, camp, sales)
		return err
	}
	if err := ins("tracked", "o1", "10.5"); err != nil {
		t.Fatal(err)
	}
	if err := ins("matched", "o1", "10.5"); err != nil {
		t.Fatalf("the same order in the other measure: %v", err)
	}
	if err := ins("tracked", "o1", "1"); err == nil {
		t.Fatal("an order twice within a measure was accepted")
	}
	if err := ins("estimated", "o2", "1"); err == nil {
		t.Fatal("an unknown measure was accepted")
	}
	if err := ins("tracked", "o3", "-0.01"); err == nil {
		t.Fatal("a negative sale was accepted")
	}

	// A re-run over stored rows keeps them.
	if err := V6_2_28(h.db, nil, nil, lo); err != nil {
		t.Fatal(err)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM campaign_conversions`)
	if n != 2 {
		t.Fatalf("rows after re-run = %d, want 2", n)
	}

	// Deleting the campaign cascades.
	h.db.MustExec(`DELETE FROM campaigns WHERE id = $1`, camp)
	h.db.Get(&n, `SELECT COUNT(*) FROM campaign_conversions`)
	if n != 0 {
		t.Fatalf("rows after campaign delete = %d, want 0", n)
	}
}
