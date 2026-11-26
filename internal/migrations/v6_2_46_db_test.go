package migrations

// Fork (60-day engaged speed) -- integrations listmonk/SILENT-PREVIEW-SPEED-SPEC I1: schema.sql
// declares v6.2.46's DDL verbatim, and on a database without the index (a v6.2.45 database; the
// schema.sql install already holds it, so it is dropped first) two runs leave exactly one
// idx_campaign_smart_skips_sub on campaign_smart_skips (subscriber_id, campaign_id), the same
// definition the schema.sql install had. Same opt-in harness as evergreen_db_test.go
// (LISTMONK_TEST_PG).

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (h *evergreenHarness) smartSkipsSubIndex() []string {
	var idx []string
	if err := h.db.Select(&idx, `SELECT indexdef FROM pg_indexes
		WHERE tablename = 'campaign_smart_skips' AND indexname = 'idx_campaign_smart_skips_sub'`); err != nil {
		h.t.Fatal(err)
	}
	return idx
}

func TestSmartSkipsSubscriberIndexMigration_V6_2_46(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(smartSkipsSubscriberIndexDDL)) {
		t.Fatal("smartSkipsSubscriberIndexDDL is not in schema.sql verbatim")
	}

	fresh := h.smartSkipsSubIndex()
	if len(fresh) != 1 || !strings.Contains(fresh[0], "USING btree (subscriber_id, campaign_id)") {
		t.Fatalf("schema.sql install index: %v", fresh)
	}

	// A v6.2.45 database: no index.
	h.db.MustExec(`DROP INDEX idx_campaign_smart_skips_sub`)
	if got := h.smartSkipsSubIndex(); len(got) != 0 {
		t.Fatalf("index survived the drop: %v", got)
	}
	for i := 0; i < 2; i++ {
		if err := V6_2_46(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_46 run %d: %v", i+1, err)
		}
	}
	got := h.smartSkipsSubIndex()
	if len(got) != 1 || got[0] != fresh[0] {
		t.Fatalf("migration index differs from schema.sql\n migration %v\n schema    %v", got, fresh)
	}
}
