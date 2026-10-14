package migrations

// Fork (list join triggers) -- integrations LIST-JOIN-TRIGGER-SPEC D1 / I1: v6.2.23 adds the five
// lists columns (join_trigger, transactional, trigger_set_by, trigger_set_at, trigger_sync) with
// NULL / false on every existing list and no data row written, grants lists:transactional to role
// 1 only, is idempotent, and schema.sql declares the same DDL. Same opt-in harness as
// evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func TestListJoinTriggerMigration(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	// A v6.2.22 database: none of the five columns, role 1 without the permission, a non-admin role.
	for _, c := range []string{"join_trigger", "transactional", "trigger_set_by", "trigger_set_at", "trigger_sync"} {
		h.db.MustExec(`ALTER TABLE lists DROP COLUMN IF EXISTS ` + c)
	}
	var admin, marketing int
	h.db.Get(&admin, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Super Admin', $1) RETURNING id`,
		pq.StringArray([]string{"lists:get_all", "lists:manage_all"}))
	h.db.Get(&marketing, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Marketing', '{lists:manage_all}') RETURNING id`)
	if admin != 1 {
		t.Skipf("harness created the admin role as id %d, not 1 (fresh database expected)", admin)
	}
	h.db.MustExec(`INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme', 'private', 'single', '{brand:acme}')`)
	var listID int
	h.db.Get(&listID, `SELECT id FROM lists WHERE name = 'Acme'`)

	// Idempotent: twice.
	for i := 0; i < 2; i++ {
		if err := V6_2_23(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_23 run %d: %v", i+1, err)
		}
	}

	want := map[string]string{
		"join_trigger":   "jsonb YES",
		"transactional":  "boolean NO",
		"trigger_set_by": "text YES",
		"trigger_set_at": "timestamp with time zone YES",
		"trigger_sync":   "jsonb YES",
	}
	for col, w := range want {
		var got string
		h.db.Get(&got, `SELECT data_type || ' ' || is_nullable FROM information_schema.columns WHERE table_name = 'lists' AND column_name = $1`, col)
		if got != w {
			t.Fatalf("lists.%s = %q, want %q", col, got, w)
		}
	}

	// Existing rows: NULL / false, nothing written.
	var touched int
	h.db.Get(&touched, `SELECT COUNT(*) FROM lists WHERE join_trigger IS NOT NULL OR transactional OR trigger_set_by IS NOT NULL OR trigger_set_at IS NOT NULL OR trigger_sync IS NOT NULL`)
	if touched != 0 {
		t.Fatalf("%d lists carry trigger data after the migration", touched)
	}

	// The grant: role 1 exactly once after two runs, no other role.
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM (SELECT UNNEST(permissions) p FROM roles WHERE id = 1) x WHERE p = 'lists:transactional'`)
	if n != 1 {
		t.Fatalf("role 1 holds lists:transactional %d times after two runs, want exactly once", n)
	}
	if permSet(t, h, marketing)["lists:transactional"] {
		t.Fatal("a non-admin role was granted lists:transactional")
	}

	// A re-run keeps stored values.
	h.db.MustExec(`UPDATE lists SET join_trigger = '{"kind":"first_order"}', transactional = true, trigger_sync = '{"state":"live"}' WHERE id = $1`, listID)
	if err := V6_2_23(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_23 re-run: %v", err)
	}
	var kept string
	h.db.Get(&kept, `SELECT (join_trigger->>'kind') || ' ' || transactional::TEXT || ' ' || (trigger_sync->>'state') FROM lists WHERE id = $1`, listID)
	if kept != "first_order true live" {
		t.Fatalf("re-run changed stored values: %q", kept)
	}

	// schema.sql declares the same DDL, byte-equal (whitespace-normalised like the v6.2.22 test).
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	if !strings.Contains(norm(string(schema)), norm(listJoinTriggerDDL)) {
		t.Fatal("schema.sql and the v6.2.23 DDL drifted")
	}
	if !strings.Contains(string(schema), strings.TrimSpace(listJoinTriggerDDL)) {
		t.Fatal("schema.sql's v6.2.23 mirror is not byte-equal to the migration DDL")
	}
}
