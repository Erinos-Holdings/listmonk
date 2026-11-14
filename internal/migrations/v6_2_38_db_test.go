package migrations

// Fork (naming standard) -- integrations listmonk/NAMING-STANDARD-SPEC §4.1 (the migration half):
// v6.2.38 brings a v6.2.37 database to exactly schema.sql's former_names table and is idempotent;
// it keeps existing rows on a re-run. Same opt-in harness as evergreen_db_test.go
// (LISTMONK_TEST_PG). The rename behaviour is cmd/rename_db_test.go.

import (
	"log"
	"os"
	"strings"
	"testing"
)

func (h *evergreenHarness) formerNamesShape() string {
	var cols []string
	if err := h.db.Select(&cols, `SELECT column_name || ' ' || data_type || ' ' || is_nullable || ' ' || COALESCE(column_default, '')
		FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'former_names'
		ORDER BY ordinal_position`); err != nil {
		h.t.Fatal(err)
	}
	var cons []string
	if err := h.db.Select(&cons, `SELECT pg_get_constraintdef(c.oid) FROM pg_constraint c
		JOIN pg_class t ON (t.oid = c.conrelid) WHERE t.relname = 'former_names' ORDER BY 1`); err != nil {
		h.t.Fatal(err)
	}
	return strings.Join(cols, "; ") + " | " + strings.Join(cons, "; ")
}

func TestFormerNamesMigration_V6_2_38(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	fresh := h.formerNamesShape()
	for _, want := range []string{"kind text NO", "id integer NO", "name text NO", "renamed_at timestamp with time zone NO now()",
		"PRIMARY KEY (kind, id)", "'campaign'", "'list'", "'template'"} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh schema lacks %q: %s", want, fresh)
		}
	}

	// A v6.2.37 database: no table.
	h.db.MustExec(`DROP TABLE former_names`)
	for i := 0; i < 2; i++ {
		if err := V6_2_38(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_38 run %d: %v", i+1, err)
		}
	}
	if got := h.formerNamesShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}

	// A re-run keeps rows; the CHECK refuses an unknown kind.
	h.db.MustExec(`INSERT INTO former_names (kind, id, name) VALUES ('campaign', 1, ' Old name ')`)
	if err := V6_2_38(h.db, nil, nil, lo); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := h.db.Get(&name, `SELECT name FROM former_names WHERE kind = 'campaign' AND id = 1`); err != nil || name != " Old name " {
		t.Fatalf("row after re-run: %q %v", name, err)
	}
	if _, err := h.db.Exec(`INSERT INTO former_names (kind, id, name) VALUES ('media', 1, 'x')`); err == nil {
		t.Fatal("an unknown kind was accepted")
	}
}
