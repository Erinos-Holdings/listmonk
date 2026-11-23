package migrations

// Fork (brand roles) -- integrations listmonk/BRAND-ROLES-SPEC G8: v6.2.43 deletes exactly the list
// roles' per-list child rows; the list roles themselves, their role_brands grants, user roles and a
// child of a user role are untouched; a re-run deletes nothing. Same opt-in harness as
// evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestBrandRolesChildDelete_V6_2_43(t *testing.T) {
	h := newEvergreenHarness(t)

	for _, b := range []string{"shala", "ruze"} {
		h.db.MustExec(`INSERT INTO brands (slug, from_email) VALUES ($1, 'hello@example.test') ON CONFLICT DO NOTHING`, b)
	}
	shala := h.rbList("Shala one", "brand:shala")
	ruze := h.rbList("Ruze", "brand:ruze")

	r8 := h.rbListRole("shala_younique")
	h.rbChild(r8, shala, "{list:get,list:manage}")
	r16 := h.rbListRole("ruze_pouches")
	h.rbChild(r16, ruze, "{list:get}")
	h.db.MustExec(`INSERT INTO role_brands (role_id, brand, level) VALUES ($1, 'shala', 'manage'), ($2, 'ruze', 'analytics')`, r8, r16)

	var userRole int
	if err := h.db.Get(&userRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'u43', '{campaigns:get}') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	h.db.MustExec(`INSERT INTO roles (type, parent_id, list_id, permissions) VALUES ('list', $1, $2, '{list:get}')`, userRole, shala)

	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := h.db.Get(&n, q, args...); err != nil {
			t.Fatal(err)
		}
		return n
	}

	var logged bytes.Buffer
	lo := log.New(&logged, "", 0)
	if err := V6_2_43(h.db, nil, nil, lo); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logged.String(), "deleted 2 list-role child row(s)") {
		t.Fatalf("log = %q", logged.String())
	}
	if n := count(`SELECT COUNT(*) FROM roles WHERE parent_id IN ($1, $2)`, r8, r16); n != 0 {
		t.Fatalf("list-role children left: %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM roles WHERE id IN ($1, $2, $3)`, r8, r16, userRole); n != 3 {
		t.Fatalf("parent roles left: %d, want 3", n)
	}
	if n := count(`SELECT COUNT(*) FROM roles WHERE parent_id = $1`, userRole); n != 1 {
		t.Fatalf("user-role child rows: %d, want 1 (untouched)", n)
	}
	if g := h.grants(r8) + "|" + h.grants(r16); g != "shala:manage|ruze:analytics" {
		t.Fatalf("grants = %q", g)
	}

	logged.Reset()
	if err := V6_2_43(h.db, nil, nil, lo); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logged.String(), "deleted 0 list-role child row(s)") {
		t.Fatalf("re-run log = %q", logged.String())
	}
}
