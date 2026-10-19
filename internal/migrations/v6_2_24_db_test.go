package migrations

// Fork (shop-link rate) -- integrations SHOP-LINK-RATE-SPEC D1 / I4: v6.2.24 creates
// campaign_shop_link_audience with the D1 columns and primary key, is idempotent, and schema.sql's
// tail block is the migration's DDL preceded by its DROP TABLE IF EXISTS line. Same opt-in harness
// as evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShopLinkAudienceMigration_V6_2_24(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	// A v6.2.23 database: no snapshot table.
	h.db.MustExec(`DROP TABLE IF EXISTS campaign_shop_link_audience`)

	// Idempotent: twice.
	for i := 0; i < 2; i++ {
		if err := V6_2_24(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_24 run %d: %v", i+1, err)
		}
	}

	want := map[string]string{
		"campaign_id": "integer NO",
		"brand":       "text NO",
		"kind":        "text NO",
		"audience":    "integer NO",
		"taken_at":    "timestamp with time zone NO",
	}
	var ncol int
	h.db.Get(&ncol, `SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'campaign_shop_link_audience'`)
	if ncol != len(want) {
		t.Fatalf("campaign_shop_link_audience has %d columns, want %d", ncol, len(want))
	}
	for col, w := range want {
		var got string
		h.db.Get(&got, `SELECT data_type || ' ' || is_nullable FROM information_schema.columns WHERE table_name = 'campaign_shop_link_audience' AND column_name = $1`, col)
		if got != w {
			t.Fatalf("campaign_shop_link_audience.%s = %q, want %q", col, got, w)
		}
	}

	// The primary key is (campaign_id, brand, kind), and it is the only index.
	var pk string
	h.db.Get(&pk, `SELECT pg_get_indexdef(i.indexrelid) FROM pg_index i WHERE i.indrelid = 'campaign_shop_link_audience'::regclass AND i.indisprimary`)
	if !strings.Contains(pk, "(campaign_id, brand, kind)") {
		t.Fatalf("primary key = %q, want (campaign_id, brand, kind)", pk)
	}
	var nidx int
	h.db.Get(&nidx, `SELECT COUNT(*) FROM pg_index WHERE indrelid = 'campaign_shop_link_audience'::regclass`)
	if nidx != 1 {
		t.Fatalf("%d indexes on campaign_shop_link_audience, want the primary key only", nidx)
	}

	// ON DELETE CASCADE from campaigns, and taken_at defaults to now.
	var camp int
	if err := h.db.Get(&camp, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id) VALUES (gen_random_uuid(), 'C', 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id`); err != nil {
		t.Fatal(err)
	}
	h.db.MustExec(`INSERT INTO campaign_shop_link_audience (campaign_id, brand, kind, audience) VALUES ($1, 'ruze', 'self', 3)`, camp)
	var stamped bool
	h.db.Get(&stamped, `SELECT taken_at IS NOT NULL FROM campaign_shop_link_audience WHERE campaign_id = $1`, camp)
	if !stamped {
		t.Fatal("taken_at has no default")
	}
	// A re-run keeps rows.
	if err := V6_2_24(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_24 re-run: %v", err)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM campaign_shop_link_audience WHERE campaign_id = $1`, camp)
	if n != 1 {
		t.Fatalf("re-run left %d rows, want 1", n)
	}
	h.db.MustExec(`DELETE FROM campaigns WHERE id = $1`, camp)
	h.db.Get(&n, `SELECT COUNT(*) FROM campaign_shop_link_audience WHERE campaign_id = $1`, camp)
	if n != 0 {
		t.Fatalf("deleting the campaign left %d snapshot rows (want ON DELETE CASCADE)", n)
	}

	// schema.sql's tail block = the DROP line + the migration's DDL, byte-equal (F8).
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	mirror := "DROP TABLE IF EXISTS campaign_shop_link_audience CASCADE;\n" + strings.TrimSpace(shopLinkAudienceDDL)
	if !strings.HasSuffix(strings.TrimSpace(string(schema)), mirror) {
		t.Fatal("schema.sql's last block is not the DROP line followed by the v6.2.24 DDL, byte-equal")
	}
}
