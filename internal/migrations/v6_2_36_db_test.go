package migrations

// Fork (journey lists) -- integrations listmonk/JOURNEY-LISTS-SPEC D11 / I8: v6.2.36 adds the three
// Active columns, activates every list that carried a join_trigger ONLY on the run that added the
// column (NULL stamps), grants lists:journeys to roles 1/4/7/13 by id exactly once, no other role,
// and a list switched off -- or a trigger saved (inactive) after the first run -- stays off after a
// second run. schema.sql declares the same DDL. Same opt-in harness as evergreen_db_test.go
// (LISTMONK_TEST_PG).

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func TestJourneyListsMigration_V6_2_36(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(listTriggerActiveDDL)) {
		t.Fatal("listTriggerActiveDDL is not in schema.sql verbatim")
	}

	// A v6.2.35 database: none of the three columns.
	for _, c := range []string{"trigger_active", "trigger_active_at", "trigger_active_by"} {
		h.db.MustExec(`ALTER TABLE lists DROP COLUMN IF EXISTS ` + c)
	}

	// Roles by explicit id: the four grantees and the four that must not get it, plus a list role.
	for _, r := range []struct {
		id   int
		typ  string
		name string
	}{
		{1, "user", "Super Admin"}, {2, "user", "Marketing"}, {3, "user", "Admin"}, {4, "user", "ManagePreferences API"},
		{7, "user", "Re-save API"}, {11, "user", "Brand Manager"}, {13, "user", "Sunset API"}, {21, "user", "Marketing Approver"},
	} {
		h.db.MustExec(`INSERT INTO roles (id, type, name, permissions) VALUES ($1, $2, $3, $4)`, r.id, r.typ, r.name,
			pq.StringArray([]string{"lists:get_all", "lists:manage_all"}))
	}
	var listRole int
	h.db.Get(&listRole, `INSERT INTO roles (id, type, parent_id, name, permissions) VALUES (40, 'list', NULL, 'List role', '{}') RETURNING id`)

	var triggered, plain int
	h.db.Get(&triggered, `INSERT INTO lists (uuid, name, type, optin, join_trigger) VALUES (gen_random_uuid(), 'Acme first order', 'private', 'single', '{"kind":"first_order"}') RETURNING id`)
	h.db.Get(&plain, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Plain', 'private', 'single') RETURNING id`)

	if err := V6_2_36(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_36 run 1: %v", err)
	}

	want := map[string]string{
		"trigger_active":    "boolean NO",
		"trigger_active_at": "timestamp with time zone YES",
		"trigger_active_by": "text YES",
	}
	for col, w := range want {
		var got string
		h.db.Get(&got, `SELECT data_type || ' ' || is_nullable FROM information_schema.columns WHERE table_name = 'lists' AND column_name = $1`, col)
		if got != w {
			t.Fatalf("lists.%s = %q, want %q", col, got, w)
		}
	}

	type state struct {
		Active bool    `db:"trigger_active"`
		At     *string `db:"trigger_active_at"`
		By     *string `db:"trigger_active_by"`
	}
	read := func(id int) state {
		var s state
		if err := h.db.Get(&s, `SELECT trigger_active, trigger_active_at::TEXT AS trigger_active_at, trigger_active_by FROM lists WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if s := read(triggered); !s.Active || s.At != nil || s.By != nil {
		t.Fatalf("triggered list after run 1: %+v, want active with NULL stamps", s)
	}
	if s := read(plain); s.Active || s.At != nil || s.By != nil {
		t.Fatalf("untriggered list after run 1: %+v, want false with NULL stamps", s)
	}

	// Between the runs: the triggered list is switched off, and a trigger is saved on the plain list
	// (inactive, the column default).
	h.db.MustExec(`UPDATE lists SET trigger_active = false, trigger_active_at = NOW(), trigger_active_by = 'marketer' WHERE id = $1`, triggered)
	h.db.MustExec(`UPDATE lists SET join_trigger = '{"kind":"no_purchase","days":2}' WHERE id = $1`, plain)

	if err := V6_2_36(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_36 run 2: %v", err)
	}
	if s := read(triggered); s.Active {
		t.Fatal("a list switched off was re-activated by the second run")
	}
	if s := read(plain); s.Active {
		t.Fatal("a trigger saved after the first run was activated by the second run")
	}

	// The grant: exactly once on 1/4/7/13, never elsewhere.
	for _, id := range []int{1, 4, 7, 13} {
		var n int
		h.db.Get(&n, `SELECT COUNT(*) FROM (SELECT UNNEST(permissions) p FROM roles WHERE id = $1) x WHERE p = 'lists:journeys'`, id)
		if n != 1 {
			t.Fatalf("role %d holds lists:journeys %d times after two runs, want exactly once", id, n)
		}
	}
	for _, id := range []int{2, 3, 11, 21, listRole} {
		if permSet(t, h, id)["lists:journeys"] {
			t.Fatalf("role %d was granted lists:journeys", id)
		}
	}
}

// A fresh install (schema.sql already carries the columns): the migration activates nothing.
func TestJourneyListsMigrationFreshSchema_V6_2_36(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)
	var triggered int
	h.db.Get(&triggered, `INSERT INTO lists (uuid, name, type, optin, join_trigger) VALUES (gen_random_uuid(), 'Fresh', 'private', 'single', '{"kind":"first_order"}') RETURNING id`)
	if err := V6_2_36(h.db, nil, nil, lo); err != nil {
		t.Fatal(err)
	}
	var active bool
	h.db.Get(&active, `SELECT trigger_active FROM lists WHERE id = $1`, triggered)
	if active {
		t.Fatal("a fresh schema's list was activated: the column existed, so step 2 must not run")
	}
}
