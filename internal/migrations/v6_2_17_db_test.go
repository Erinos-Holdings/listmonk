package migrations

// Fork (campaign review structure gate) -- v6.2.17 grants campaigns:review_structure to Super
// Admin (role 1) idempotently and to no other role, and the fork's INVARIANT holds: after the
// fork's granting migrations, role 1 holds every permission in permissions.json. Super Admin has
// no implicit permissions on an upgraded database (auth.User.HasPerm's short-circuit is dead --
// see v6.2.17.go), so a future fork permission added to permissions.json WITHOUT a role-1 grant
// migration fails TestSuperAdminHoldsEveryPermission. Same opt-in harness as
// evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/lib/pq"
)

// upstreamV620Permissions is upstream listmonk v6.2.0's permissions.json (`git show
// v6.2.0:permissions.json`) -- what role 1 holds on a database installed before any fork grant.
// A fixed list on purpose: the test's job is to prove the fork's migrations carry THIS baseline
// to the current permissions.json.
var upstreamV620Permissions = []string{
	"bounces:get", "bounces:manage",
	"campaigns:get", "campaigns:get_all", "campaigns:get_analytics", "campaigns:manage", "campaigns:manage_all", "campaigns:send",
	"lists:get_all", "lists:manage_all",
	"media:get", "media:manage",
	"roles:get", "roles:manage",
	"settings:get", "settings:maintain", "settings:manage",
	"subscribers:get", "subscribers:get_all", "subscribers:import", "subscribers:manage", "subscribers:sql_query",
	"templates:get", "templates:manage",
	"tx:send",
	"users:get", "users:manage",
	"webhooks:post_bounce",
}

// currentPermissions reads every permission key from the repo's permissions.json -- the same file
// cmd/init.go loads at runtime and cmd/install.go grants role 1 on a fresh install.
func currentPermissions(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "permissions.json"))
	if err != nil {
		t.Fatalf("permissions.json: %v", err)
	}
	var groups []struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &groups); err != nil {
		t.Fatalf("permissions.json: %v", err)
	}
	var out []string
	for _, g := range groups {
		out = append(out, g.Permissions...)
	}
	sort.Strings(out)
	return out
}

// permSet folds rolePerms (v6_2_12_db_test.go) into a lookup.
func permSet(t *testing.T, h *evergreenHarness, id int) map[string]bool {
	out := map[string]bool{}
	for _, p := range rolePerms(t, h, id) {
		out[p] = true
	}
	return out
}

func TestReviewStructureGrantMigration(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	// A v6.2.16 database: role 1 as prod had it on 2026-09-30 (everything but the structure
	// permission), plus a non-admin role that must not be granted.
	var admin, marketing int
	h.db.Get(&admin, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Super Admin', $1) RETURNING id`,
		pq.StringArray(append(append([]string{}, upstreamV620Permissions...), "brands:get", "brands:manage", "campaigns:review")))
	h.db.Get(&marketing, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Marketing', '{campaigns:manage,lists:get_all}') RETURNING id`)
	if admin != 1 {
		t.Skipf("harness created the admin role as id %d, not 1 (fresh database expected)", admin)
	}

	// Idempotent: twice.
	for i := 0; i < 2; i++ {
		if err := V6_2_17(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_17 run %d: %v", i+1, err)
		}
	}

	if !permSet(t, h, admin)["campaigns:review_structure"] {
		t.Fatalf("role 1 was not granted campaigns:review_structure")
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM (SELECT UNNEST(permissions) p FROM roles WHERE id = 1) x WHERE p = 'campaigns:review_structure'`)
	if n != 1 {
		t.Fatalf("role 1 holds campaigns:review_structure %d times after two runs, want exactly once", n)
	}
	if permSet(t, h, marketing)["campaigns:review_structure"] {
		t.Fatalf("a non-admin role was granted campaigns:review_structure")
	}
}

// TestSuperAdminHoldsEveryPermission is the invariant: an upstream-v6.2.0 role 1 carried through
// every fork migration that grants role 1 (in cmd/upgrade.go order) holds every key of the
// current permissions.json. Adding a permission to permissions.json without its grant migration
// fails here -- add the grant (the v6.2.14 pattern) and list the migration below.
func TestSuperAdminHoldsEveryPermission(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	var admin int
	h.db.Get(&admin, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Super Admin', $1) RETURNING id`, pq.StringArray(upstreamV620Permissions))
	if admin != 1 {
		t.Skipf("harness created the admin role as id %d, not 1 (fresh database expected)", admin)
	}

	// Every fork migration that grants role 1, in upgrade order.
	granting := []struct {
		name string
		run  func() error
	}{
		{"v6.2.12", func() error { return V6_2_12(h.db, nil, nil, lo) }},
		{"v6.2.14", func() error { return V6_2_14(h.db, nil, nil, lo) }},
		{"v6.2.17", func() error { return V6_2_17(h.db, nil, nil, lo) }},
	}
	for _, g := range granting {
		if err := g.run(); err != nil {
			t.Fatalf("%s: %v", g.name, err)
		}
	}

	have := permSet(t, h, admin)
	var missing []string
	for _, p := range currentPermissions(t) {
		if !have[p] {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("role 1 lacks %s after every fork grant migration -- Super Admin has no implicit permissions on an upgraded database; add a role-1 grant migration (the v6.2.14 pattern) and list it in this test", strings.Join(missing, ", "))
	}
}
