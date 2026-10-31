package migrations

// Fork (usage decisions) -- integrations USAGE-DECISIONS-SPEC D4 / I8: v6.2.29 creates
// usage_decisions exactly as schema.sql does (columns, constraints, the append-only trigger), is
// idempotent on re-run (the statement trigger is CREATE OR REPLACE), and the table refuses UPDATE,
// DELETE and TRUNCATE while INSERT and SELECT work; an unknown decision is refused; it grants
// brands:decide to role 1 once. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG). The
// queries half of I8 (no UPDATE/DELETE in queries/*.sql) is cmd/usage_decisions_test.go.

import (
	"log"
	"os"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func (h *evergreenHarness) decisionsShape() string {
	var cols []string
	if err := h.db.Select(&cols, `SELECT column_name || ' ' || data_type || ' ' || is_nullable || ' ' || COALESCE(character_maximum_length::TEXT, '') || ' ' || COALESCE(column_default, '')
		FROM information_schema.columns WHERE table_name = 'usage_decisions' ORDER BY ordinal_position`); err != nil {
		h.t.Fatal(err)
	}
	var checks []string
	if err := h.db.Select(&checks, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = 'usage_decisions'::regclass ORDER BY contype, conname`); err != nil {
		h.t.Fatal(err)
	}
	var trig []string
	if err := h.db.Select(&trig, `SELECT pg_get_triggerdef(oid) FROM pg_trigger WHERE tgrelid = 'usage_decisions'::regclass AND NOT tgisinternal ORDER BY tgname`); err != nil {
		h.t.Fatal(err)
	}
	var fn []string
	if err := h.db.Select(&fn, `SELECT prosrc FROM pg_proc WHERE proname = 'usage_decisions_append_only'`); err != nil {
		h.t.Fatal(err)
	}
	return strings.Join(cols, "; ") + " | " + strings.Join(checks, "; ") + " | " + strings.Join(trig, "; ") + " | " + strings.Join(fn, "; ")
}

func TestUsageDecisionsMigration_V6_2_29(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	fresh := h.decisionsShape()
	for _, want := range []string{
		// pg_get_triggerdef renders events in Postgres's canonical order (DELETE before UPDATE).
		"BEFORE DELETE OR UPDATE OR TRUNCATE ON public.usage_decisions FOR EACH STATEMENT",
		// schema.sql carries v6.2.30's wider CHECK; v6.2.29 + v6.2.30 reach it (below).
		"CHECK ((decision = ANY (ARRAY['accept'::text, 'reject'::text, 'current'::text, 'recommended'::text])))",
		"text_hash character NO 64",
		"note text YES",
	} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh schema lacks %q: %s", want, fresh)
		}
	}
	if strings.Contains(fresh, "FOREIGN KEY") {
		t.Fatalf("usage_decisions has a foreign key (D4: none): %s", fresh)
	}

	// A v6.2.28 database: no table, no function. The migration creates the same shape (with
	// v6.2.30's wider CHECK applied after it, as an upgrade runs them), and is idempotent (run twice).
	h.db.MustExec(`DROP TABLE usage_decisions`)
	h.db.MustExec(`DROP FUNCTION usage_decisions_append_only()`)
	for i := 0; i < 2; i++ {
		if err := V6_2_29(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_29 run %d: %v", i+1, err)
		}
	}
	if err := V6_2_30(h.db, nil, nil, lo); err != nil {
		t.Fatal(err)
	}
	if got := h.decisionsShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}

	ins := func(decision string) error {
		_, err := h.db.Exec(`INSERT INTO usage_decisions (item_key, text_hash, decision, note, user_id, user_name)
			VALUES ('np-entry', $1, $2, NULL, 4, 'Kaylinn')`, strings.Repeat("a", 64), decision)
		return err
	}
	if err := ins("accept"); err != nil {
		t.Fatal(err)
	}
	if err := ins("reject"); err != nil {
		t.Fatal(err)
	}
	if err := ins("defer"); err == nil {
		t.Fatal("an unknown decision was accepted")
	}

	// Append-only: every mutation is refused, the rows survive.
	for _, q := range []string{
		`UPDATE usage_decisions SET note = 'x'`,
		`UPDATE usage_decisions SET note = 'x' WHERE false`,
		`DELETE FROM usage_decisions`,
		`DELETE FROM usage_decisions WHERE false`,
		`TRUNCATE usage_decisions`,
	} {
		_, err := h.db.Exec(q)
		if err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("%s: err %v, want the append-only refusal", q, err)
		}
	}
	// A re-run over stored rows keeps them.
	if err := V6_2_29(h.db, nil, nil, lo); err != nil {
		t.Fatal(err)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM usage_decisions`)
	if n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}

	// The role-1 grant, once.
	var admin int
	h.db.Get(&admin, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Super Admin', $1) RETURNING id`, pq.StringArray{"brands:get"})
	if admin == 1 {
		for i := 0; i < 2; i++ {
			if err := V6_2_29(h.db, nil, nil, lo); err != nil {
				t.Fatal(err)
			}
		}
		var perms pq.StringArray
		h.db.Get(&perms, `SELECT permissions FROM roles WHERE id = 1`)
		c := 0
		for _, p := range perms {
			if p == "brands:decide" {
				c++
			}
		}
		if c != 1 {
			t.Fatalf("role 1 holds brands:decide %d times: %v", c, perms)
		}
	}
}
