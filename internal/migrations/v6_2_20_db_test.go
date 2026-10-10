package migrations

// Fork (brand picker) -- integrations BRAND-PICKER-SPEC I8: v6.2.20 creates the brands table and
// backfills one row per brand from the lists' brand:/from:/site: tags, is a no-op on re-run, leaves
// the lists unchanged, and FAILS -- naming the list, with no brands row written -- on a from:/site:
// disagreement within a brand, a half-tagged list, a case-only slug collision, and a brand failing
// models.BrandProblem(..., nil, nil). schema.sql declares the same table. Same opt-in harness as
// evergreen_db_test.go (LISTMONK_TEST_PG); fixture names are example.test only.

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func TestBrandsBackfillMigration(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	list := func(name string, tags ...string) {
		t.Helper()
		if tags == nil {
			tags = []string{}
		}
		h.db.MustExec(`INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), $1, 'private', 'single', $2)`, name, pq.StringArray(tags))
	}
	rows := func() string {
		var out []string
		h.db.Select(&out, `SELECT slug || '=' || from_email || '=' || COALESCE(site, '-') FROM brands ORDER BY slug`)
		return strings.Join(out, "|")
	}
	listState := func() string {
		var out []string
		h.db.Select(&out, `SELECT name || ':' || ARRAY_TO_STRING(tags, ',') || ':' || updated_at::TEXT FROM lists ORDER BY id`)
		return strings.Join(out, "|")
	}
	reset := func() {
		h.db.MustExec(`DROP TABLE IF EXISTS brands`)
		h.db.MustExec(`DELETE FROM lists`)
	}

	// A v6.2.19 database: no brands table. The harness's own list A is untagged.
	reset()
	list("Curated one", "brand:curated", "from:Curated <hello@curatedfor.example.test>")
	list("Curated two", " brand: curated ", "from: Curated <hello@curatedfor.example.test> ", "holiday")
	list("Shop", "brand:shop", "from:hello@shop.example.test", "site:https://store.shop.example.test")
	list("Shop two", "brand:shop", "from:hello@shop.example.test")
	list("Untagged", "seed")
	list("Repermission", "repermission:4")
	lists := listState()

	for i := 0; i < 2; i++ {
		if err := V6_2_20(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_20 run %d: %v", i+1, err)
		}
	}
	if got := rows(); got != "curated=Curated <hello@curatedfor.example.test>=-|shop=hello@shop.example.test=https://store.shop.example.test" {
		t.Fatalf("backfilled rows = %q", got)
	}
	if got := listState(); got != lists {
		t.Fatalf("the migration changed lists:\n before %s\n after  %s", lists, got)
	}

	// No-op on re-run with rows present, even when a row has since diverged (PUT /api/brands is
	// the writer after the upgrade, never the migration).
	h.db.MustExec(`UPDATE brands SET from_email = 'Curated Edited <hello@curatedfor.example.test>' WHERE slug = 'curated'`)
	var stamp string
	h.db.Get(&stamp, `SELECT string_agg(updated_at::TEXT, ',' ORDER BY slug) FROM brands`)
	if err := V6_2_20(h.db, nil, nil, lo); err != nil {
		t.Fatalf("re-run: %v", err)
	}
	var stamp2 string
	h.db.Get(&stamp2, `SELECT string_agg(updated_at::TEXT, ',' ORDER BY slug) FROM brands`)
	if got := rows(); !strings.HasPrefix(got, "curated=Curated Edited") || stamp != stamp2 {
		t.Fatalf("re-run was not a no-op: %q (%s vs %s)", got, stamp, stamp2)
	}

	// The failure shapes: each is an error naming the offending list, and no brands row.
	for _, c := range []struct {
		name  string
		setup func()
		want  []string
	}{
		{"from disagreement", func() {
			list("Acme A", "brand:acme", "from:Acme <hello@acme.example.test>")
			list("Acme B", "brand:acme", "from:Acme Shop <hello@acme.example.test>")
		}, []string{`"Acme A"`, `"Acme B"`, `brand "acme"`}},
		{"two sites", func() {
			list("Acme A", "brand:acme", "from:hello@acme.example.test", "site:https://a.example.test")
			list("Acme B", "brand:acme", "from:hello@acme.example.test", "site:https://b.example.test")
		}, []string{`"Acme B"`, "site:"}},
		{"half-tagged brand", func() {
			list("Fine", "brand:fine", "from:hello@fine.example.test")
			list("Half", "brand:acme")
		}, []string{`"Half"`, "half-tagged"}},
		{"half-tagged from", func() {
			list("Half from", "from:hello@acme.example.test")
		}, []string{`"Half from"`, "half-tagged"}},
		{"case collision", func() {
			list("Lower", "brand:acme", "from:hello@acme.example.test")
			list("Upper", "brand:Acme", "from:hello@acme.example.test")
		}, []string{`"Lower"`, `"Upper"`, "only by case"}},
		{"BrandProblem", func() {
			list("Accented", "brand:acme", "from:Acmé <hello@acme.example.test>")
		}, []string{`"Accented"`, "lists.brandFromTagNotASCII"}},
		{"reserved slug", func() {
			list("Health", "brand:health", "from:hello@acme.example.test")
		}, []string{`"Health"`, "lists.brandSlugReserved"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			reset()
			c.setup()
			err := V6_2_20(h.db, nil, nil, lo)
			if err == nil {
				t.Fatal("want an error")
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Fatalf("error %q does not name %s", err, w)
				}
			}
			// Rolled back whole: not even the table remains.
			var n int
			h.db.Get(&n, `SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'brands'`)
			if n != 0 {
				var rowsN int
				h.db.Get(&rowsN, `SELECT COUNT(*) FROM brands`)
				t.Fatalf("a failed migration left the brands table (%d rows)", rowsN)
			}
		})
	}

	// On a schema.sql database (table present, empty) a failure writes no row either.
	reset()
	h.db.MustExec(brandsDDL)
	list("Half", "brand:acme")
	if err := V6_2_20(h.db, nil, nil, lo); err == nil {
		t.Fatal("half-tagged on a fresh schema: want an error")
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM brands`)
	if n != 0 {
		t.Fatalf("%d brands rows after a failed backfill", n)
	}

	// schema.sql declares the same table and index as the migration.
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	if !strings.Contains(norm(string(schema)), norm(brandsDDL)) {
		t.Fatal("schema.sql and the v6.2.20 DDL drifted")
	}
}
