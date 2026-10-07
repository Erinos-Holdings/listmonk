package migrations

// Fork (client stats) -- integrations CLIENT-STATS-SPEC I2: v6.2.19 adds a nullable client TEXT
// (no default) to campaign_views and link_clicks, is idempotent, leaves existing rows intact with a
// NULL client, keeps the older image's inserts working, fails on a held lock instead of waiting
// (lock_timeout, one transaction), and schema.sql declares the same columns. The v6_2_16 pattern;
// same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestClientColumnsMigration(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	// A v6.2.18 database: the columns dropped, with pre-existing tracking rows.
	h.db.MustExec(`ALTER TABLE campaign_views DROP COLUMN client`)
	h.db.MustExec(`ALTER TABLE link_clicks DROP COLUMN client`)
	var camp, sub, link int
	h.db.Get(&camp, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id) VALUES (gen_random_uuid(), 'c', 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id`)
	h.db.Get(&sub, `INSERT INTO subscribers (uuid, email, name) VALUES (gen_random_uuid(), 'm@x', 'M') RETURNING id`)
	h.db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://x.test') RETURNING id`)
	h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id, country, created_at) VALUES ($1, $2, 'US', '2026-01-01'), ($1, NULL, NULL, '2026-01-02')`, camp, sub)
	h.db.MustExec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, country, created_at) VALUES ($1, $2, $3, 'GB', '2026-01-03')`, camp, sub, link)
	snapshot := func() string {
		var r []string
		h.db.Select(&r, `SELECT 'v' || id || ':' || campaign_id || ':' || COALESCE(subscriber_id::TEXT, '-') || ':' || COALESCE(country, '-') || ':' || created_at::TEXT FROM campaign_views
			UNION ALL SELECT 'c' || id || ':' || campaign_id || ':' || COALESCE(subscriber_id::TEXT, '-') || ':' || link_id || ':' || COALESCE(country, '-') || ':' || created_at::TEXT FROM link_clicks ORDER BY 1`)
		return strings.Join(r, "|")
	}
	before := snapshot()

	// Idempotent: runs twice.
	for i := 0; i < 2; i++ {
		if err := V6_2_19(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_19 run %d: %v", i+1, err)
		}
	}

	for _, table := range []string{"campaign_views", "link_clicks"} {
		var col string
		h.db.Get(&col, `SELECT data_type || ' ' || is_nullable || ' ' || COALESCE(column_default, '-') FROM information_schema.columns WHERE table_name = $1 AND column_name = 'client'`, table)
		if col != "text YES -" {
			t.Fatalf("%s.client = %q, want \"text YES -\"", table, col)
		}
		var nonNull int
		h.db.Get(&nonNull, `SELECT COUNT(*) FROM `+table+` WHERE client IS NOT NULL`)
		if nonNull != 0 {
			t.Fatalf("%s: %d pre-existing row(s) got a client", table, nonNull)
		}
	}
	if got := snapshot(); got != before {
		t.Fatalf("existing rows changed:\n before %s\n after  %s", before, got)
	}

	// The old image's inserts (explicit column lists without client) still work.
	h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id, country) VALUES ($1, $2, 'US')`, camp, sub)
	h.db.MustExec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, country) VALUES ($1, $2, $3, 'US')`, camp, sub, link)

	// lock_timeout and one transaction: with both columns dropped and a reader holding a lock on
	// link_clicks (the SECOND ALTER), the migration fails within the timeout and the first ALTER
	// (campaign_views) rolls back with it.
	h.db.MustExec(`ALTER TABLE campaign_views DROP COLUMN client`)
	h.db.MustExec(`ALTER TABLE link_clicks DROP COLUMN client`)
	holder := h.db.MustBegin()
	holder.MustExec(`LOCK TABLE link_clicks IN ACCESS SHARE MODE`)
	start := time.Now()
	err := V6_2_19(h.db, nil, nil, lo)
	elapsed := time.Since(start)
	holder.Rollback()
	if err == nil || !strings.Contains(err.Error(), "lock timeout") {
		t.Fatalf("migration under a held lock: err = %v, want a lock timeout", err)
	}
	if elapsed > 15*time.Second {
		t.Fatalf("migration waited %s under a held lock", elapsed)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM information_schema.columns WHERE table_name IN ('campaign_views', 'link_clicks') AND column_name = 'client'`)
	if n != 0 {
		t.Fatalf("a failed migration left a partial change: %d client column(s), want 0 (the campaign_views ALTER did not roll back)", n)
	}
	if err := V6_2_19(h.db, nil, nil, lo); err != nil {
		t.Fatalf("re-run after the lock cleared: %v", err)
	}

	// schema.sql declares the same column in both CREATE TABLEs.
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"campaign_views", "link_clicks"} {
		m := regexp.MustCompile(`(?s)CREATE TABLE ` + table + ` \((.*?)\n\);`).FindStringSubmatch(string(schema))
		if m == nil || !regexp.MustCompile(`\n\s*client\s+TEXT NULL,?\n`).MatchString(m[1]+"\n") {
			t.Fatalf("schema.sql CREATE TABLE %s does not declare client TEXT NULL", table)
		}
	}
	if !strings.Contains(clientColumnsDDL, "campaign_views ADD COLUMN IF NOT EXISTS client TEXT NULL") ||
		!strings.Contains(clientColumnsDDL, "link_clicks ADD COLUMN IF NOT EXISTS client TEXT NULL") {
		t.Fatal("migration DDL and the schema.sql column drifted")
	}
}
