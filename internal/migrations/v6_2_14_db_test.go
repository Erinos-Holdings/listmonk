package migrations

// Fork (campaign review) -- integrations CAMPAIGN-INSPECT-SPEC D5: the v6.2.14 migration is
// idempotent, creates the three tables with their keys and cascades, seeds the two settings keys
// empty (the gate ships OFF), grants campaigns:review to Super Admin only, and schema.sql mirrors
// the DDL. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCampaignReviewMigration(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	var admin, marketing int
	h.db.Get(&admin, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Super Admin', '{campaigns:manage}') RETURNING id`)
	h.db.Get(&marketing, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Marketing', '{campaigns:manage}') RETURNING id`)
	h.db.MustExec(`DELETE FROM settings WHERE key IN ('app.review_url', 'app.review_hmac_secret')`)

	// Idempotent and additive: twice, then once on a database where the tables were dropped (an
	// upgrade from v6.2.13).
	for i := 0; i < 2; i++ {
		if err := V6_2_14(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_14 run %d: %v", i+1, err)
		}
	}
	h.db.MustExec(`DROP TABLE campaign_reviews; DROP TABLE campaign_review_dispositions; DROP TABLE campaign_structure_verifications`)
	if err := V6_2_14(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_14 on a v6.2.13 database: %v", err)
	}

	// campaigns:review for role 1 only (the role with id 1 is whichever this harness created first).
	var perms1, permsM string
	h.db.Get(&perms1, `SELECT array_to_string(permissions, ',') FROM roles WHERE id = 1`)
	h.db.Get(&permsM, `SELECT array_to_string(permissions, ',') FROM roles WHERE id = $1`, marketing)
	if admin == 1 && !strings.Contains(perms1, "campaigns:review") {
		t.Fatalf("role 1 permissions = %s, want campaigns:review", perms1)
	}
	if strings.Contains(permsM, "campaigns:review") {
		t.Fatalf("a non-admin role was granted campaigns:review: %s", permsM)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM roles WHERE id = 1 AND permissions @> '{campaigns:review}'`)
	if admin == 1 && n != 1 {
		t.Fatal("the grant is not idempotent")
	}

	// The settings ship empty -- the gate is off until the go-live save.
	for _, k := range []string{"app.review_url", "app.review_hmac_secret"} {
		var v string
		if err := h.db.Get(&v, `SELECT value::TEXT FROM settings WHERE key = $1`, k); err != nil || v != `""` {
			t.Fatalf("%s = %q (%v), want \"\"", k, v, err)
		}
	}

	cols := func(table string) string {
		var c []string
		h.db.Select(&c, `SELECT column_name || ' ' || data_type || ' ' || is_nullable FROM information_schema.columns WHERE table_name = $1 ORDER BY ordinal_position`, table)
		return fmt.Sprint(c)
	}
	if got := cols("campaign_reviews"); got != fmt.Sprint([]string{
		"id bigint NO", "campaign_id integer NO", "bundle_hash text NO", "job_id uuid NO", "status text NO", "progress jsonb NO",
		"report jsonb YES", "error text NO", "requested_by text NO", "requested_at timestamp with time zone NO", "updated_at timestamp with time zone NO",
	}) {
		t.Fatalf("campaign_reviews columns = %s", got)
	}
	if got := cols("campaign_review_dispositions"); got != fmt.Sprint([]string{
		"id bigint NO", "campaign_id integer NO", "bundle_hash text NO", "item_key text NO", "rubric_id text NO", "action text NO",
		"note text NO", "user_id integer YES", "username text NO", "created_at timestamp with time zone NO",
	}) {
		t.Fatalf("campaign_review_dispositions columns = %s", got)
	}
	if got := cols("campaign_structure_verifications"); got != fmt.Sprint([]string{
		"fingerprint text NO", "verified_at timestamp with time zone NO", "test_id text NO", "components jsonb NO", "verified_by text NO",
	}) {
		t.Fatalf("campaign_structure_verifications columns = %s", got)
	}
	for _, idx := range []string{"idx_campaign_reviews_campaign", "idx_campaign_review_dispositions"} {
		var c int
		h.db.Get(&c, `SELECT COUNT(*) FROM pg_indexes WHERE indexname = $1`, idx)
		if c != 1 {
			t.Fatalf("%s missing", idx)
		}
	}

	// Status and action are constrained; deleting a campaign cascades its reviews and log.
	var camp int
	h.db.Get(&camp, `INSERT INTO campaigns (uuid, name, subject, from_email, body, content_type, messenger, type, status)
		VALUES (gen_random_uuid(), 'c', 's', 'f@x', 'b', 'plain', 'email', 'regular', 'draft') RETURNING id`)
	if _, err := h.db.Exec(`INSERT INTO campaign_reviews (campaign_id, bundle_hash, job_id, status) VALUES ($1, 'h', gen_random_uuid(), 'bogus')`, camp); err == nil {
		t.Fatal("a bogus review status was accepted")
	}
	if _, err := h.db.Exec(`INSERT INTO campaign_review_dispositions (campaign_id, bundle_hash, item_key, rubric_id, action) VALUES ($1, 'h', 'k', 'D1.1', 'waive')`, camp); err == nil {
		t.Fatal("a bogus disposition action was accepted")
	}
	h.db.MustExec(`INSERT INTO campaign_reviews (campaign_id, bundle_hash, job_id, status) VALUES ($1, 'h', gen_random_uuid(), 'running')`, camp)
	h.db.MustExec(`INSERT INTO campaign_review_dispositions (campaign_id, bundle_hash, item_key, rubric_id, action) VALUES ($1, 'h', 'k', 'D1.1', 'accept')`, camp)
	h.db.MustExec(`DELETE FROM campaigns WHERE id = $1`, camp)
	var left int
	h.db.Get(&left, `SELECT (SELECT COUNT(*) FROM campaign_reviews) + (SELECT COUNT(*) FROM campaign_review_dispositions)`)
	if left != 0 {
		t.Fatalf("%d review rows survived their campaign", left)
	}

	// schema.sql mirrors the migration DDL exactly, and seeds the two settings.
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(campaignReviewDDL)) {
		t.Fatal("schema.sql does not carry the v6.2.14 campaign review DDL verbatim")
	}
	for _, k := range []string{`('app.review_url', '""')`, `('app.review_hmac_secret', '""')`} {
		if !strings.Contains(string(schema), k) {
			t.Fatalf("schema.sql does not seed %s", k)
		}
	}
}
