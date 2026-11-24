package migrations

// Fork (SMS Admin) -- integrations listmonk/SMS-LINKS-ADMIN-SPEC I7: v6.2.44 brings a v6.2.43
// database to exactly schema.sql's shape (sms_promo_codes, its unique LOWER(code) index, the brands
// foreign key), schema.sql declares the DDL verbatim, role 1 is granted sms:manage (and no other
// role), and a second run changes nothing and keeps the rows. The role-1 superset invariant is
// TestSuperAdminHoldsEveryPermission. Same opt-in harness as evergreen_db_test.go
// (LISTMONK_TEST_PG).

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (h *evergreenHarness) smsShape() string {
	var cols []string
	h.db.Select(&cols, `SELECT column_name || ' ' || data_type || ' ' || is_nullable || ' ' || COALESCE(column_default, '-')
		FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'sms_promo_codes'
		ORDER BY ordinal_position`)
	var idx []string
	h.db.Select(&idx, `SELECT indexdef FROM pg_indexes WHERE tablename = 'sms_promo_codes' ORDER BY indexname`)
	var fks []string
	h.db.Select(&fks, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conrelid = 'sms_promo_codes'::regclass ORDER BY contype, conname`)
	return strings.Join(cols, ", ") + " | " + strings.Join(idx, "; ") + " | " + strings.Join(fks, "; ")
}

func TestSMSPromoCodesMigration_V6_2_44(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(smsPromoCodesDDL)) {
		t.Fatal("smsPromoCodesDDL is not in schema.sql verbatim")
	}

	fresh := h.smsShape()
	for _, want := range []string{
		"id integer NO nextval(", "code text NO -", "brand text YES -", "created_by text NO -",
		"created_at timestamp with time zone NO now()",
		"CREATE UNIQUE INDEX idx_sms_promo_codes_code ON public.sms_promo_codes USING btree (lower(code))",
		"sms_promo_codes_pkey",
		"FOREIGN KEY (brand) REFERENCES brands(slug) ON UPDATE CASCADE ON DELETE SET NULL",
	} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh schema lacks %q: %s", want, fresh)
		}
	}

	var admin, marketing int
	h.db.Get(&admin, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Super Admin', '{campaigns:get}') RETURNING id`)
	if admin != 1 {
		t.Skipf("harness created the admin role as id %d, not 1 (fresh database expected)", admin)
	}
	h.db.Get(&marketing, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Marketing', '{campaigns:get,campaigns:manage}') RETURNING id`)

	// A v6.2.43 database: no table.
	h.db.MustExec(`DROP TABLE sms_promo_codes`)

	if err := V6_2_44(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_44 run 1: %v", err)
	}
	if got := h.smsShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}
	if p := permSet(t, h, admin); !p["sms:manage"] || !p["campaigns:get"] {
		t.Fatalf("role 1 after v6.2.44: %v", p)
	}
	if permSet(t, h, marketing)["sms:manage"] {
		t.Fatal("a non-admin role was granted sms:manage")
	}

	h.db.MustExec(`INSERT INTO sms_promo_codes (code, created_by) VALUES ('newCustomerSMS', 'listmonk:t')`)
	if err := V6_2_44(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_44 run 2: %v", err)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM sms_promo_codes`)
	if n != 1 {
		t.Fatalf("second run left %d rows, want 1", n)
	}
	if got := h.smsShape(); got != fresh {
		t.Fatal("second run shape differs")
	}
	var grants int
	h.db.Get(&grants, `SELECT COUNT(*) FROM roles, UNNEST(permissions) p WHERE id = 1 AND p = 'sms:manage'`)
	if grants != 1 {
		t.Fatalf("role 1 carries sms:manage %d times after two runs, want 1", grants)
	}

	// The unique index is case-insensitive.
	if _, err := h.db.Exec(`INSERT INTO sms_promo_codes (code, created_by) VALUES ('NEWCUSTOMERSMS', 'listmonk:t')`); err == nil {
		t.Fatal("a case-insensitive duplicate was accepted")
	}
}
