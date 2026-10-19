package migrations

// Fork (shop-link frozen) -- integrations SHOP-LINK-FROZEN-SPEC D1 / I4: v6.2.25 creates
// campaign_shop_link_recipients with the D1 columns and primary key (the only index), no foreign
// key to subscribers, is idempotent, and schema.sql contains the migration's DDL preceded by its
// DROP TABLE IF EXISTS line (a Contains pin, not a tail pin -- F2). Same opt-in harness as
// evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShopLinkRecipientsMigration_V6_2_25(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	// A v6.2.24 database: no recipient table.
	h.db.MustExec(`DROP TABLE IF EXISTS campaign_shop_link_recipients`)

	// Idempotent: twice.
	for i := 0; i < 2; i++ {
		if err := V6_2_25(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_25 run %d: %v", i+1, err)
		}
	}

	want := map[string]string{
		"campaign_id":   "integer NO",
		"subscriber_id": "integer NO",
		"brand":         "text NO",
		"kind":          "text NO",
	}
	var ncol int
	h.db.Get(&ncol, `SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'campaign_shop_link_recipients'`)
	if ncol != len(want) {
		t.Fatalf("campaign_shop_link_recipients has %d columns, want %d", ncol, len(want))
	}
	for col, w := range want {
		var got string
		h.db.Get(&got, `SELECT data_type || ' ' || is_nullable FROM information_schema.columns WHERE table_name = 'campaign_shop_link_recipients' AND column_name = $1`, col)
		if got != w {
			t.Fatalf("campaign_shop_link_recipients.%s = %q, want %q", col, got, w)
		}
	}

	// The primary key is (campaign_id, subscriber_id, brand), and it is the only index.
	var pk string
	h.db.Get(&pk, `SELECT pg_get_indexdef(i.indexrelid) FROM pg_index i WHERE i.indrelid = 'campaign_shop_link_recipients'::regclass AND i.indisprimary`)
	if !strings.Contains(pk, "(campaign_id, subscriber_id, brand)") {
		t.Fatalf("primary key = %q, want (campaign_id, subscriber_id, brand)", pk)
	}
	var nidx int
	h.db.Get(&nidx, `SELECT COUNT(*) FROM pg_index WHERE indrelid = 'campaign_shop_link_recipients'::regclass`)
	if nidx != 1 {
		t.Fatalf("%d indexes on campaign_shop_link_recipients, want the primary key only", nidx)
	}

	// The only foreign key is campaigns (F1 -- none to subscribers).
	var fks []string
	if err := h.db.Select(&fks, `SELECT confrelid::regclass::TEXT FROM pg_constraint WHERE conrelid = 'campaign_shop_link_recipients'::regclass AND contype = 'f'`); err != nil {
		t.Fatal(err)
	}
	if len(fks) != 1 || fks[0] != "campaigns" {
		t.Fatalf("foreign keys reference %v, want [campaigns] only", fks)
	}

	// ON DELETE CASCADE from campaigns; a subscriber id with no subscriber row is accepted (no FK),
	// and a re-run keeps rows.
	var camp int
	if err := h.db.Get(&camp, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id) VALUES (gen_random_uuid(), 'C', 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id`); err != nil {
		t.Fatal(err)
	}
	h.db.MustExec(`INSERT INTO campaign_shop_link_recipients (campaign_id, subscriber_id, brand, kind) VALUES ($1, 2147483000, 'ruze', 'self')`, camp)
	if err := V6_2_25(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_25 re-run: %v", err)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM campaign_shop_link_recipients WHERE campaign_id = $1`, camp)
	if n != 1 {
		t.Fatalf("re-run left %d rows, want 1", n)
	}
	h.db.MustExec(`DELETE FROM campaigns WHERE id = $1`, camp)
	h.db.Get(&n, `SELECT COUNT(*) FROM campaign_shop_link_recipients WHERE campaign_id = $1`, camp)
	if n != 0 {
		t.Fatalf("deleting the campaign left %d recipient rows (want ON DELETE CASCADE)", n)
	}

	// schema.sql contains the DROP line + the migration's DDL, byte-equal.
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	mirror := "DROP TABLE IF EXISTS campaign_shop_link_recipients CASCADE;\n" + strings.TrimSpace(shopLinkRecipientsDDL)
	if !strings.Contains(string(schema), mirror) {
		t.Fatal("schema.sql does not contain the DROP line followed by the v6.2.25 DDL, byte-equal")
	}
}
