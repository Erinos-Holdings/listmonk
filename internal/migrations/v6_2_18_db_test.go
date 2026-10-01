package migrations

// Fork (two-factor) -- integrations PASSKEY-2FA-SPEC I16: the v6.2.18 migration is idempotent,
// creates user_passkeys with its keys and cascade, seeds security.require_twofa false only when
// absent (an existing true is left alone), and schema.sql mirrors the DDL and the seed. Same opt-in
// harness as evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV6_2_18(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	// A v6.2.17 database: no table, no key.
	h.db.MustExec(`DROP TABLE IF EXISTS user_passkeys`)
	h.db.MustExec(`DELETE FROM settings WHERE key = 'security.require_twofa'`)

	for i := 0; i < 2; i++ {
		if err := V6_2_18(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_18 run %d: %v", i+1, err)
		}
	}

	var v string
	if err := h.db.Get(&v, `SELECT value::TEXT FROM settings WHERE key = 'security.require_twofa'`); err != nil || v != "false" {
		t.Fatalf("security.require_twofa = %q (%v), want false", v, err)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM settings WHERE key = 'security.require_twofa'`)
	if n != 1 {
		t.Fatalf("security.require_twofa rows = %d after two runs, want 1", n)
	}

	// An existing true is left untouched.
	h.db.MustExec(`UPDATE settings SET value = 'true' WHERE key = 'security.require_twofa'`)
	if err := V6_2_18(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_18 over true: %v", err)
	}
	h.db.Get(&v, `SELECT value::TEXT FROM settings WHERE key = 'security.require_twofa'`)
	if v != "true" {
		t.Fatalf("security.require_twofa = %q after re-run, want the existing true kept", v)
	}

	var c []string
	h.db.Select(&c, `SELECT column_name || ' ' || data_type || ' ' || is_nullable FROM information_schema.columns WHERE table_name = 'user_passkeys' ORDER BY ordinal_position`)
	if got := fmt.Sprint(c); got != fmt.Sprint([]string{
		"id integer NO", "user_id integer NO", "credential_id bytea NO", "credential jsonb NO",
		"name text NO", "created_at timestamp with time zone NO", "last_used_at timestamp with time zone YES",
	}) {
		t.Fatalf("user_passkeys columns = %s", got)
	}
	h.db.Get(&n, `SELECT COUNT(*) FROM pg_indexes WHERE indexname = 'idx_user_passkeys_user'`)
	if n != 1 {
		t.Fatal("idx_user_passkeys_user missing")
	}

	// credential_id is unique; deleting the user cascades its passkeys.
	var role, uid int
	h.db.Get(&role, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'r', '{}') RETURNING id`)
	h.db.Get(&uid, `INSERT INTO users (username, password_login, email, name, type, user_role_id, status) VALUES ('pk', false, 'pk@example.test', 'pk', 'user', $1, 'enabled') RETURNING id`, role)
	h.db.MustExec(`INSERT INTO user_passkeys (user_id, credential_id, credential, name) VALUES ($1, '\x01', '{}', 'a')`, uid)
	if _, err := h.db.Exec(`INSERT INTO user_passkeys (user_id, credential_id, credential, name) VALUES ($1, '\x01', '{}', 'b')`, uid); err == nil {
		t.Fatal("a duplicate credential_id was accepted")
	}
	h.db.MustExec(`DELETE FROM users WHERE id = $1`, uid)
	h.db.Get(&n, `SELECT COUNT(*) FROM user_passkeys`)
	if n != 0 {
		t.Fatalf("%d passkeys survived their user", n)
	}

	// schema.sql mirrors the migration DDL exactly, and seeds the setting false.
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(passkeysDDL)) {
		t.Fatal("schema.sql does not carry the v6.2.18 user_passkeys DDL verbatim")
	}
	if !strings.Contains(string(schema), `('security.require_twofa', 'false')`) {
		t.Fatal("schema.sql does not seed security.require_twofa false")
	}
}
