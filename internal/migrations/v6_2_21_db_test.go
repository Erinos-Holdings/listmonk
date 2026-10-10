package migrations

// Fork (persona From) -- integrations PERSONA-FROM-SPEC I9: v6.2.21 adds personas TEXT[] NOT NULL
// DEFAULT '{}' to brands, gives every existing row the empty set, leaves the rows otherwise
// intact, is idempotent (a second run is no error and keeps a set written since), and schema.sql
// declares the same statement. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG);
// fixture names are example.test only.

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersonasColumnMigration(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	// A v6.2.20 database: the brands table without the column, with pre-existing rows.
	h.db.MustExec(`ALTER TABLE brands DROP COLUMN IF EXISTS personas`)
	h.db.MustExec(`INSERT INTO brands (slug, from_email, site) VALUES
		('acme', 'Acme <hello@acme.example.test>', NULL),
		('shop', 'hello@shop.example.test', 'https://store.shop.example.test')`)
	snapshot := func() string {
		var r []string
		h.db.Select(&r, `SELECT slug || '=' || from_email || '=' || COALESCE(site, '-') || '=' || created_at::TEXT || '=' || updated_at::TEXT FROM brands ORDER BY slug`)
		return strings.Join(r, "|")
	}
	before := snapshot()

	if err := V6_2_21(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_21: %v", err)
	}

	var col string
	h.db.Get(&col, `SELECT data_type || ' ' || udt_name || ' ' || is_nullable || ' ' || COALESCE(column_default, '-') FROM information_schema.columns WHERE table_name = 'brands' AND column_name = 'personas'`)
	if col != "ARRAY _text NO '{}'::text[]" {
		t.Fatalf("brands.personas = %q", col)
	}
	var nonEmpty, rows int
	h.db.Get(&rows, `SELECT COUNT(*) FROM brands`)
	h.db.Get(&nonEmpty, `SELECT COUNT(*) FROM brands WHERE personas IS DISTINCT FROM '{}'::TEXT[]`)
	if rows != 2 || nonEmpty != 0 {
		t.Fatalf("%d rows, %d with a non-empty persona set (want 2, 0)", rows, nonEmpty)
	}
	if got := snapshot(); got != before {
		t.Fatalf("existing rows changed:\n before %s\n after  %s", before, got)
	}

	// Idempotent: a second run is no error and keeps a set written since the first.
	h.db.MustExec(`UPDATE brands SET personas = '{"Nat at Acme"}' WHERE slug = 'acme'`)
	if err := V6_2_21(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_21 re-run: %v", err)
	}
	var kept string
	h.db.Get(&kept, `SELECT ARRAY_TO_STRING(personas, ',') FROM brands WHERE slug = 'acme'`)
	if kept != "Nat at Acme" {
		t.Fatalf("re-run changed a stored set: %q", kept)
	}

	// schema.sql declares the same statement as the migration.
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	if !strings.Contains(norm(string(schema)), norm(personasColumnDDL)) {
		t.Fatal("schema.sql and the v6.2.21 DDL drifted")
	}
}
