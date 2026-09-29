package migrations

// Fork (inspect scope) -- integrations INSPECT-SCOPE-SPEC I8: v6.2.15 creates the append-only
// campaign_structure_records table and its fingerprint index, grants NO role anything (the
// permission lives in permissions.json), leaves the legacy campaign_structure_verifications table
// and its rows untouched, is idempotent, and schema.sql mirrors the DDL. Same opt-in harness as
// evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStructureRecordsMigration(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	h.db.MustExec(`INSERT INTO roles (type, name, permissions) VALUES ('user', 'Super Admin', '{campaigns:manage}')`)
	h.db.MustExec(`INSERT INTO roles (type, name, permissions) VALUES ('user', 'Marketing', '{campaigns:manage,campaigns:review}')`)
	h.db.MustExec(`INSERT INTO roles (type, name, permissions) VALUES ('user', 'Re-save API', '{campaigns:review}')`)
	rolesBefore := func() string {
		var r []string
		h.db.Select(&r, `SELECT id || ':' || array_to_string(permissions, ',') FROM roles ORDER BY id`)
		return strings.Join(r, "|")
	}
	before := rolesBefore()

	// The legacy table and its one row predate the migration.
	h.db.MustExec(`INSERT INTO campaign_structure_verifications (fingerprint, test_id, components, verified_by) VALUES ($1, 'legacy-test', '{"blocks":["Text"]}', 'listmonk_resave')`, strings.Repeat("e", 64))
	legacy := func() string {
		var s string
		h.db.Get(&s, `SELECT fingerprint || '|' || test_id || '|' || components::TEXT || '|' || verified_by || '|' || verified_at::TEXT FROM campaign_structure_verifications`)
		return s
	}
	legacyBefore := legacy()

	// Idempotent and additive: on a v6.2.14 database (the table dropped), then twice more.
	h.db.MustExec(`DROP TABLE campaign_structure_records`)
	for i := 0; i < 3; i++ {
		if err := V6_2_15(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_15 run %d: %v", i+1, err)
		}
	}

	if got := rolesBefore(); got != before {
		t.Fatalf("the migration changed role permissions:\n before %s\n after  %s", before, got)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM roles WHERE permissions @> '{campaigns:review_structure}'`)
	if n != 0 {
		t.Fatalf("%d role(s) granted campaigns:review_structure by the migration", n)
	}
	if got := legacy(); got != legacyBefore {
		t.Fatalf("legacy row changed: %s -> %s", legacyBefore, got)
	}
	var legacyCols []string
	h.db.Select(&legacyCols, `SELECT column_name FROM information_schema.columns WHERE table_name = 'campaign_structure_verifications' ORDER BY ordinal_position`)
	if fmt.Sprint(legacyCols) != "[fingerprint verified_at test_id components verified_by]" {
		t.Fatalf("legacy table columns changed: %v", legacyCols)
	}

	var cols []string
	h.db.Select(&cols, `SELECT column_name || ' ' || data_type || ' ' || is_nullable FROM information_schema.columns WHERE table_name = 'campaign_structure_records' ORDER BY ordinal_position`)
	if got, want := fmt.Sprint(cols), fmt.Sprint([]string{
		"id bigint NO", "fingerprint text NO", "campaign_id integer YES", "verified_at timestamp with time zone NO", "verified_by text NO",
		"test_id text NO", "components jsonb NO", "canary jsonb NO", "clients ARRAY NO", "roster ARRAY NO", "stage text NO", "modes ARRAY NO",
	}); got != want {
		t.Fatalf("campaign_structure_records columns = %s\nwant %s", got, want)
	}
	var idx int
	h.db.Get(&idx, `SELECT COUNT(*) FROM pg_indexes WHERE indexname = 'idx_campaign_structure_records_fingerprint' AND indexdef LIKE '%(fingerprint)%'`)
	if idx != 1 {
		t.Fatal("idx_campaign_structure_records_fingerprint missing")
	}

	// Append-only by construction: two rows for one fingerprint coexist (no unique key on it).
	fp := strings.Repeat("a", 64)
	for _, stage := range []string{"1", "2"} {
		h.db.MustExec(`INSERT INTO campaign_structure_records (fingerprint, test_id, stage) VALUES ($1, 't', $2)`, fp, stage)
	}
	h.db.Get(&n, `SELECT COUNT(*) FROM campaign_structure_records WHERE fingerprint = $1`, fp)
	if n != 2 {
		t.Fatalf("%d rows for one fingerprint, want 2", n)
	}
	if _, err := h.db.Exec(`INSERT INTO campaign_structure_records (fingerprint, test_id, stage) VALUES ($1, 't', 'three')`, fp); err == nil {
		t.Fatal("a bogus stage was accepted")
	}

	// The permission is registered in permissions.json (the role editor offers it).
	raw, err := os.ReadFile(filepath.Join("..", "..", "permissions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var groups []struct {
		Group       string   `json:"group"`
		Permissions []string `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &groups); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range groups {
		for _, p := range g.Permissions {
			found = found || (g.Group == "campaigns" && p == "campaigns:review_structure")
		}
	}
	if !found {
		t.Fatal("permissions.json does not register campaigns:review_structure under campaigns")
	}

	// schema.sql mirrors the migration DDL exactly.
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(structureRecordsDDL)) {
		t.Fatal("schema.sql does not carry the v6.2.15 structure records DDL verbatim")
	}
}
