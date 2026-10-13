package migrations

// Fork (sunset v2) -- integrations SUNSET-V2-SPEC §4.1: v6.2.22 adds lists.sunset JSONB NULL (NULL
// on every existing list, rows otherwise intact), replaces subscriber_lists_hold_release() with the
// final-hold guard (a v6.2.21 database keeps releasing an ordinary hold, and from v6.2.22 a hold
// carrying final_at is kept unless listmonk.release_final is set), is idempotent, and schema.sql
// declares the same DDL. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSunsetV2Migration(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	// A v6.2.21 database: no lists.sunset, the v6.2.10 trigger function.
	h.db.MustExec(`ALTER TABLE lists DROP COLUMN IF EXISTS sunset`)
	h.db.MustExec(`CREATE OR REPLACE FUNCTION subscriber_lists_hold_release() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status IS DISTINCT FROM NEW.status AND OLD.meta ? 'hold' AND NEW.meta ? 'hold' THEN
        NEW.meta := subscriber_lists_release_hold(NEW.meta, NEW.status::TEXT);
        NEW.updated_at := NOW();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;`)
	if !strings.Contains(holdsDDL, "RETURN NEW;") || strings.Contains(holdsDDL, "final_at") {
		t.Fatal("the v6.2.10 function this test restores no longer matches holdsDDL's shape")
	}
	h.db.MustExec(`INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'private', 'single')`)
	var listID, subA, subB int
	h.db.Get(&listID, `SELECT id FROM lists WHERE name = 'Acme'`)
	h.db.Get(&subA, `INSERT INTO subscribers (uuid, email, name) VALUES (gen_random_uuid(), 'a@example.test', 'a') RETURNING id`)
	h.db.Get(&subB, `INSERT INTO subscribers (uuid, email, name) VALUES (gen_random_uuid(), 'b@example.test', 'b') RETURNING id`)
	final := `{"hold":{"reason":"never-engaged","source":"t","rule":"r","final_at":"2027-10-12T15:00:00Z"}}`
	h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status, meta) VALUES ($1, $3, 'unsubscribed', $4::JSONB), ($2, $3, 'unsubscribed', $4::JSONB)`, subA, subB, listID, final)

	// Before the migration a final hold is released like any other (the gap the guard closes).
	h.db.MustExec(`UPDATE subscriber_lists SET status = 'confirmed' WHERE subscriber_id = $1 AND list_id = $2`, subA, listID)
	var st string
	h.db.Get(&st, `SELECT status FROM subscriber_lists WHERE subscriber_id = $1 AND list_id = $2`, subA, listID)
	if st != "confirmed" {
		t.Fatalf("pre-migration control: %s", st)
	}

	if err := V6_2_22(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_22: %v", err)
	}
	var col string
	h.db.Get(&col, `SELECT data_type || ' ' || is_nullable FROM information_schema.columns WHERE table_name = 'lists' AND column_name = 'sunset'`)
	if col != "jsonb YES" {
		t.Fatalf("lists.sunset = %q", col)
	}
	var nonNull int
	h.db.Get(&nonNull, `SELECT COUNT(*) FROM lists WHERE sunset IS NOT NULL`)
	if nonNull != 0 {
		t.Fatalf("%d lists carry a sunset after the migration", nonNull)
	}

	// After it: an unflagged status change on a final hold is skipped; a flagged one releases.
	h.db.MustExec(`UPDATE subscriber_lists SET status = 'confirmed' WHERE subscriber_id = $1 AND list_id = $2`, subB, listID)
	h.db.Get(&st, `SELECT status FROM subscriber_lists WHERE subscriber_id = $1 AND list_id = $2`, subB, listID)
	if st != "unsubscribed" {
		t.Fatalf("final hold released without the flag: %s", st)
	}
	tx := h.db.MustBegin()
	tx.MustExec(`SET LOCAL listmonk.release_final = 'true'`)
	tx.MustExec(`UPDATE subscriber_lists SET status = 'confirmed' WHERE subscriber_id = $1 AND list_id = $2`, subB, listID)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var meta string
	h.db.Get(&meta, `SELECT status || ' ' || (meta->'hold_released'->>'to') FROM subscriber_lists WHERE subscriber_id = $1 AND list_id = $2`, subB, listID)
	if meta != "confirmed confirmed" {
		t.Fatalf("flagged release: %q", meta)
	}

	// Idempotent: a re-run is no error and keeps a stored countdown.
	h.db.MustExec(`UPDATE lists SET sunset = '{"oldestDays":3,"archiveInDays":null,"asOf":"2026-11-01T15:00:00Z"}' WHERE id = $1`, listID)
	if err := V6_2_22(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_22 re-run: %v", err)
	}
	var kept string
	h.db.Get(&kept, `SELECT sunset->>'oldestDays' FROM lists WHERE id = $1`, listID)
	if kept != "3" {
		t.Fatalf("re-run changed a stored countdown: %q", kept)
	}

	// schema.sql declares the same DDL.
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	if !strings.Contains(norm(string(schema)), norm(sunsetV2DDL)) {
		t.Fatal("schema.sql and the v6.2.22 DDL drifted")
	}
}
