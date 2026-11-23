package migrations

// Fork (brand roles) -- integrations listmonk/BRAND-ROLES-SPEC I7: v6.2.42 brings a v6.2.41 database
// to schema.sql's shape (role_brands, its unique index, role_list_grants -- schema.sql declares the
// DDL verbatim), and converts every list role's per-list child rows into brand grants: brand = the
// child's list's brand tag, manage when the child carries list:manage else analytics, several
// children on one brand collapse to the highest level, untagged and unknown-brand children (and a
// child with no permissions) dropped; the children are left in place; the conversion runs ONCE --
// a grant deleted after the migration stays deleted when the migration runs again. Same opt-in
// harness as evergreen_db_test.go (LISTMONK_TEST_PG). The readers are cmd/brand_roles_db_test.go.

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func (h *evergreenHarness) roleBrandsShape() string {
	var fn []string
	if err := h.db.Select(&fn, `SELECT pg_get_functiondef(p.oid) FROM pg_proc p WHERE p.proname = 'role_list_grants'`); err != nil {
		h.t.Fatal(err)
	}
	var cols []string
	h.db.Select(&cols, `SELECT column_name || ' ' || data_type || ' ' || is_nullable || ' ' || COALESCE(column_default, '-')
		FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'role_brands'
		ORDER BY ordinal_position`)
	var idx []string
	h.db.Select(&idx, `SELECT indexdef FROM pg_indexes WHERE tablename = 'role_brands' ORDER BY indexname`)
	var cons []string
	h.db.Select(&cons, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = 'role_brands'::regclass ORDER BY 1`)
	return strings.Join(cols, ", ") + " | " + strings.Join(idx, "; ") + " | " + strings.Join(cons, "; ") + " | " + strings.Join(fn, "; ")
}

func (h *evergreenHarness) rbList(name string, tags ...string) int {
	h.t.Helper()
	var id int
	if tags == nil {
		tags = []string{}
	}
	if err := h.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), $1, 'private', 'single', $2) RETURNING id`,
		name, pq.StringArray(tags)); err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *evergreenHarness) rbListRole(name string) int {
	h.t.Helper()
	var id int
	if err := h.db.Get(&id, `INSERT INTO roles (type, name, permissions) VALUES ('list', $1, '{}') RETURNING id`, name); err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *evergreenHarness) rbChild(role, list int, perms string) {
	h.t.Helper()
	h.db.MustExec(`INSERT INTO roles (type, parent_id, list_id, permissions) VALUES ('list', $1, $2, $3::TEXT[])`, role, list, perms)
}

// grants is a role's role_brands rows as "brand:level" ("*" = All brands), sorted.
func (h *evergreenHarness) grants(role int) string {
	h.t.Helper()
	var rows []string
	if err := h.db.Select(&rows, `SELECT COALESCE(brand, '*') || ':' || level FROM role_brands WHERE role_id = $1 ORDER BY 1`, role); err != nil {
		h.t.Fatal(err)
	}
	return strings.Join(rows, ",")
}

func TestBrandRolesMigration_V6_2_42(t *testing.T) {
	h := newEvergreenHarness(t)

	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(roleBrandsDDL)) {
		t.Fatal("roleBrandsDDL is not in schema.sql verbatim")
	}

	fresh := h.roleBrandsShape()
	for _, want := range []string{
		"role_id integer NO -", "brand text YES -", "level text NO -",
		"idx_role_brands", "(role_id, COALESCE(brand, '*'::text))",
		"FOREIGN KEY (role_id) REFERENCES roles(id) ON UPDATE CASCADE ON DELETE CASCADE",
		"FOREIGN KEY (brand) REFERENCES brands(slug) ON UPDATE CASCADE ON DELETE CASCADE",
		"CHECK ((level = ANY (ARRAY['manage'::text, 'analytics'::text])))",
		"CHECK (((brand IS NOT NULL) OR (level = 'analytics'::text)))",
		"role_list_grants(p_role_id integer)",
	} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh schema lacks %q: %s", want, fresh)
		}
	}

	// A v6.2.41 database: no table, no function -- and per-list child rows to convert.
	h.db.MustExec(`DROP FUNCTION role_list_grants(INT)`)
	h.db.MustExec(`DROP TABLE role_brands`)

	for _, b := range []string{"shala", "younique", "ruze"} {
		h.db.MustExec(`INSERT INTO brands (slug, from_email) VALUES ($1, 'hello@example.test')`, b)
	}
	shala1 := h.rbList("Shala one", "brand:shala")
	shala2 := h.rbList("Shala two", "brand:shala")
	younique := h.rbList("Younique", "brand:younique")
	ruze := h.rbList("Ruze", "brand:ruze")
	untagged := h.rbList("Untagged", "seed")
	ghost := h.rbList("Ghost brand", "brand:ghost") // a brand: tag with no brands row

	// Role 8's shape: two manage children on two brands.
	r8 := h.rbListRole("shala_younique")
	h.rbChild(r8, shala1, "{list:get,list:manage}")
	h.rbChild(r8, younique, "{list:get,list:manage}")
	// Role 16's shape: a get-only child.
	r16 := h.rbListRole("ruze_pouches")
	h.rbChild(r16, ruze, "{list:get}")
	// Several children on one brand collapse to the highest level; manage-only counts as manage.
	mixed := h.rbListRole("mixed")
	h.rbChild(mixed, shala1, "{list:get}")
	h.rbChild(mixed, shala2, "{list:manage}")
	// Dropped: untagged, unknown brand, no permissions -- the role ends with no grants.
	dropped := h.rbListRole("dropped")
	h.rbChild(dropped, untagged, "{list:get,list:manage}")
	h.rbChild(dropped, ghost, "{list:get,list:manage}")
	h.rbChild(dropped, ruze, "{}")

	var children int
	h.db.Get(&children, `SELECT COUNT(*) FROM roles WHERE parent_id IS NOT NULL`)

	var logged strings.Builder
	lo := log.New(&logged, "", 0)
	if err := V6_2_42(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_42 run 1: %v", err)
	}
	if got := h.roleBrandsShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}

	for _, c := range []struct {
		role int
		want string
	}{
		{r8, "shala:manage,younique:manage"},
		{r16, "ruze:analytics"},
		{mixed, "shala:manage"},
		{dropped, ""},
	} {
		if got := h.grants(c.role); got != c.want {
			t.Errorf("role %d grants = %q, want %q", c.role, got, c.want)
		}
	}
	for _, want := range []string{
		"list " + itoaM(untagged) + " dropped (untagged list)",
		"list " + itoaM(ghost) + " dropped (brand ghost has no brands row)",
		"list " + itoaM(ruze) + " dropped (no permissions)",
	} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("the migration log lacks %q:\n%s", want, logged.String())
		}
	}

	// The children are left in place.
	var after int
	h.db.Get(&after, `SELECT COUNT(*) FROM roles WHERE parent_id IS NOT NULL`)
	if after != children {
		t.Fatalf("child rows %d -> %d, want them left in place", children, after)
	}

	// The conversion runs once: a grant deleted after the release stays deleted, a grant added
	// stays, when the migration runs again.
	h.db.MustExec(`DELETE FROM role_brands WHERE role_id = $1 AND brand = 'younique'`, r8)
	h.db.MustExec(`INSERT INTO role_brands (role_id, brand, level) VALUES ($1, NULL, 'analytics')`, r16)
	if err := V6_2_42(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_42 run 2: %v", err)
	}
	if got := h.grants(r8); got != "shala:manage" {
		t.Fatalf("after a re-run role 8 grants = %q, want the deleted younique grant to stay deleted", got)
	}
	if got := h.grants(r16); got != "*:analytics,ruze:analytics" {
		t.Fatalf("after a re-run role 16 grants = %q", got)
	}
	if got := h.roleBrandsShape(); got != fresh {
		t.Fatal("the second run changed the shape")
	}
}

func itoaM(n int) string {
	return strconv.Itoa(n)
}
