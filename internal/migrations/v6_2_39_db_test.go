package migrations

// Fork (campaign pause time) -- integrations listmonk/CAMPAIGN-STATUS-FILTER-SPEC I4: v6.2.39 adds
// campaigns.paused_at and backfills ONLY paused campaigns whose paused_at is NULL, to updated_at;
// a running and a draft campaign stay NULL, an existing paused_at is not overwritten, and a second
// run changes nothing. schema.sql declares the same DDL verbatim. Same opt-in harness as
// evergreen_db_test.go (LISTMONK_TEST_PG). The writer (update-campaign-status) is I1/I2 in
// cmd/campaigns_paused_at_db_test.go.

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCampaignPausedAtMigration_V6_2_39(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(campaignPausedAtDDL)) {
		t.Fatal("campaignPausedAtDDL is not in schema.sql verbatim")
	}

	var fresh string
	h.db.Get(&fresh, `SELECT data_type || ' ' || is_nullable FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'campaigns' AND column_name = 'paused_at'`)
	if fresh != "timestamp with time zone YES" {
		t.Fatalf("fresh schema campaigns.paused_at = %q", fresh)
	}

	// A v6.2.38 database: no column.
	h.db.MustExec(`ALTER TABLE campaigns DROP COLUMN paused_at`)

	upd := map[string]time.Time{
		"paused":   time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		"running":  time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC),
		"draft":    time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
		"paused-2": time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
	}
	ids := map[string]int{}
	for name, at := range upd {
		id := h.campaign(name, false, 0, h.listA)
		status := strings.TrimSuffix(name, "-2")
		h.db.MustExec(`UPDATE campaigns SET status = $2, updated_at = $3 WHERE id = $1`, id, status, at)
		ids[name] = id
	}

	if err := V6_2_39(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_39 run 1: %v", err)
	}

	var got string
	h.db.Get(&got, `SELECT data_type || ' ' || is_nullable FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'campaigns' AND column_name = 'paused_at'`)
	if got != fresh {
		t.Fatalf("migration campaigns.paused_at = %q, schema.sql %q", got, fresh)
	}

	pausedAt := func(name string) sql.NullTime {
		var ts sql.NullTime
		if err := h.db.Get(&ts, `SELECT paused_at FROM campaigns WHERE id = $1`, ids[name]); err != nil {
			t.Fatal(err)
		}
		return ts
	}

	if ts := pausedAt("paused"); !ts.Valid || !ts.Time.Equal(upd["paused"]) {
		t.Fatalf("paused row: paused_at %v, want updated_at %v", ts, upd["paused"])
	}
	for _, n := range []string{"running", "draft"} {
		if ts := pausedAt(n); ts.Valid {
			t.Fatalf("%s row backfilled: %v", n, ts.Time)
		}
	}

	// A paused row that already carries a paused_at is never overwritten -- set one, then re-run.
	existing := time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC)
	h.db.MustExec(`UPDATE campaigns SET paused_at = $2 WHERE id = $1`, ids["paused-2"], existing)

	snapshot := func() string {
		var s string
		if err := h.db.Get(&s, `SELECT STRING_AGG(id::TEXT || '=' || COALESCE(paused_at::TEXT, '-') || '/' || updated_at::TEXT, ' ' ORDER BY id) FROM campaigns`); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := snapshot()
	if err := V6_2_39(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_39 run 2: %v", err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("second run changed rows\n before %s\n after  %s", before, after)
	}
	if ts := pausedAt("paused-2"); !ts.Valid || !ts.Time.Equal(existing) {
		t.Fatalf("existing paused_at overwritten: %v, want %v", ts, existing)
	}
}
