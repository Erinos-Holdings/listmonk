package migrations

// Fork (one engagement definition) -- integrations listmonk/DOC-ALIGNMENT-SPEC D2 (the migration
// half): v6.2.34 brings a v6.2.33 database to exactly schema.sql's shape (the brand_engagement
// function body) and is idempotent. Same opt-in harness as evergreen_db_test.go
// (LISTMONK_TEST_PG). The behaviour is cmd/subscribers_hold_db_test.go.

import (
	"log"
	"os"
	"strings"
	"testing"
)

func (h *evergreenHarness) brandEngagementShape() string {
	var fn []string
	if err := h.db.Select(&fn, `SELECT pg_get_functiondef(p.oid) FROM pg_proc p WHERE p.proname = 'brand_engagement'`); err != nil {
		h.t.Fatal(err)
	}
	return strings.Join(fn, "; ")
}

func TestBrandEngagementMigration_V6_2_34(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	fresh := h.brandEngagementShape()
	for _, want := range []string{
		"brand_engagement(p_list_id integer, p_since timestamp with time zone, p_skip_url text, p_view_client text)",
		"k.subscriber_id IS NOT NULL", "v.subscriber_id IS NOT NULL", "list_brand_tag(l.tags)", "STABLE",
	} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh schema lacks %q: %s", want, fresh)
		}
	}

	// A v6.2.33 database: the function does not exist.
	h.db.MustExec(`DROP FUNCTION brand_engagement(INT, TIMESTAMPTZ, TEXT, TEXT)`)
	if got := h.brandEngagementShape(); got != "" {
		t.Fatalf("function survived the drop: %s", got)
	}

	for i := 0; i < 2; i++ {
		if err := V6_2_34(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_34 run %d: %v", i+1, err)
		}
	}
	if got := h.brandEngagementShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}
}
