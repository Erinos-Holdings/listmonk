package migrations

// Fork (approved brand logos) -- integrations LOGO-APPROVAL-SPEC I5: v6.2.47 brings a v6.2.46
// database to schema.sql's shape (brands.logos_approved and brands.logos_rejected, TEXT[] NOT NULL
// DEFAULT '{}', every existing row the empty lists, the rows otherwise intact), schema.sql declares
// the DDL verbatim, brands:logos is granted to roles 1, 2, 3, 11 and 21 exactly once and to no other
// role (a missing id is a no-op), and a second run changes nothing. Same opt-in harness as
// evergreen_db_test.go (LISTMONK_TEST_PG); fixture names are example.test only.

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (h *evergreenHarness) brandLogoColumns() string {
	var cols []string
	h.db.Select(&cols, `SELECT column_name || ' ' || data_type || ' ' || udt_name || ' ' || is_nullable || ' ' || COALESCE(column_default, '-')
		FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'brands'
		AND column_name IN ('logos_approved', 'logos_rejected') ORDER BY column_name`)
	return strings.Join(cols, ", ")
}

func TestBrandLogosMigration_V6_2_47(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(brandLogosColumnsDDL)) {
		t.Fatal("brandLogosColumnsDDL is not in schema.sql verbatim")
	}
	const wantCols = "logos_approved ARRAY _text NO '{}'::text[], logos_rejected ARRAY _text NO '{}'::text[]"
	if fresh := h.brandLogoColumns(); fresh != wantCols {
		t.Fatalf("fresh schema columns %q, want %q", fresh, wantCols)
	}

	// Roles 1..25 except 11 (a missing grant id is a no-op); 21 already holding nothing special.
	for id := 1; id <= 25; id++ {
		if id == 11 {
			continue
		}
		h.db.MustExec(`INSERT INTO roles (id, type, name, permissions) VALUES ($1, 'user', $2, '{campaigns:get}')`, id, "role "+itoaM(id))
	}

	// A v6.2.46 database: no columns, pre-existing brand rows.
	h.db.MustExec(`ALTER TABLE brands DROP COLUMN IF EXISTS logos_approved`)
	h.db.MustExec(`ALTER TABLE brands DROP COLUMN IF EXISTS logos_rejected`)
	h.db.MustExec(`INSERT INTO brands (slug, from_email, site) VALUES
		('acme', 'Acme <hello@acme.example.test>', NULL),
		('shop', 'hello@shop.example.test', 'https://store.shop.example.test')`)
	snapshot := func() string {
		var r []string
		h.db.Select(&r, `SELECT slug || '=' || from_email || '=' || COALESCE(site, '-') || '=' || ARRAY_TO_STRING(personas, ',') || '=' || updated_at::TEXT FROM brands ORDER BY slug`)
		return strings.Join(r, "|")
	}
	before := snapshot()

	if err := V6_2_47(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_47 run 1: %v", err)
	}
	if got := h.brandLogoColumns(); got != wantCols {
		t.Fatalf("migration columns %q, want %q (schema.sql's)", got, wantCols)
	}
	var nonEmpty int
	h.db.Get(&nonEmpty, `SELECT COUNT(*) FROM brands WHERE logos_approved <> '{}' OR logos_rejected <> '{}'`)
	if nonEmpty != 0 {
		t.Fatalf("%d existing rows got non-empty lists", nonEmpty)
	}
	if got := snapshot(); got != before {
		t.Fatalf("existing rows changed:\n before %s\n after  %s", before, got)
	}

	granted := func() map[int]int {
		var rows []struct {
			ID int `db:"id"`
			N  int `db:"n"`
		}
		h.db.Select(&rows, `SELECT id, COUNT(*) AS n FROM roles, UNNEST(permissions) p WHERE p = 'brands:logos' GROUP BY id ORDER BY id`)
		out := map[int]int{}
		for _, r := range rows {
			out[r.ID] = r.N
		}
		return out
	}
	check := func(run int) {
		t.Helper()
		g := granted()
		want := map[int]bool{1: true, 2: true, 3: true, 21: true}
		if len(g) != len(want) {
			t.Fatalf("run %d: brands:logos on roles %v, want exactly 1, 2, 3, 21 (11 missing)", run, g)
		}
		for id, n := range g {
			if !want[id] || n != 1 {
				t.Fatalf("run %d: role %d carries brands:logos %d times (granted %v)", run, id, n, g)
			}
		}
		// The rest of a granted role's permissions are kept.
		if !has(rolePerms(t, h, 2), "campaigns:get") {
			t.Fatalf("run %d: role 2 lost campaigns:get", run)
		}
	}
	check(1)
	if !strings.Contains(brandLogosGrantDDL, "IN (1, 2, 3, 11, 21)") {
		t.Fatalf("the grant list drifted: %s", brandLogosGrantDDL)
	}

	// Idempotent: a second run is no error, keeps lists written since and grants nothing twice.
	h.db.MustExec(`UPDATE brands SET logos_approved = '{https://media.example.test/a.png}' WHERE slug = 'acme'`)
	if err := V6_2_47(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_47 run 2: %v", err)
	}
	var kept string
	h.db.Get(&kept, `SELECT ARRAY_TO_STRING(logos_approved, ',') FROM brands WHERE slug = 'acme'`)
	if kept != "https://media.example.test/a.png" {
		t.Fatalf("re-run changed a stored list: %q", kept)
	}
	check(2)

	// Role 11 created later and the migration re-run (an upgrade where Brand Manager exists): granted once.
	h.db.MustExec(`INSERT INTO roles (id, type, name, permissions) VALUES (11, 'user', 'Brand Manager', '{brands:get}')`)
	if err := V6_2_47(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_47 run 3: %v", err)
	}
	if g := granted(); g[11] != 1 || len(g) != 5 {
		t.Fatalf("after role 11 exists: %v", g)
	}
}
