package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_22 is a FORK migration (sunset v2, integrations SUNSET-V2-SPEC §4.1), not an upstream
// release. It:
//
//   - adds lists.sunset JSONB NULL: the re-engagement queue countdown the ListmonkSunset Lambda
//     computes and PUT /api/lists/:id/sunset stores verbatim ({oldestDays, archiveInDays, asOf}).
//     The fork stores and renders, never computes (D8). NULL on every existing row.
//   - replaces subscriber_lists_hold_release() (v6.2.10) with a version that REFUSES to release a
//     FINAL hold (meta.hold.final_at set, the 365-day archive stamp, D5/D6) unless the session
//     setting listmonk.release_final is 'true' for the transaction. A refused write leaves the row
//     unchanged (RETURN NULL skips it) and raises a NOTICE; the statement's response is unchanged,
//     so the refusal is silent at the API by design (D6). The lists action sets the setting only
//     for a request carrying release_final: true (core.SessionFlags). Every other release rule of
//     v6.2.10 is kept as it was.
//
// One transaction with SET LOCAL lock_timeout = '5s' (the v6.2.21 shape).
//
// ROLLBACK: revert the release commit in the integrations repo (the image pin) and re-apply the
// host. The older binary ignores the column (the app's queries run on an Unsafe() handle, so an
// unmapped column scans away) and no DDL reversal is needed for it. The trigger stays this one
// under the older binary -- harmless, since no hold carries final_at until the Lambda's archive
// phase writes one, and the older Lambda never does.
//
// The version key sits after the fork's v6.2.21 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.22. Idempotent by construction.
func V6_2_22(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(sunsetV2DDL); err != nil {
		return err
	}
	return tx.Commit()
}

// sunsetV2DDL is mirrored in schema.sql (the last block, after the v6.2.10 hold functions it
// replaces at the end of a fresh install). Keep the two identical.
const sunsetV2DDL = `
ALTER TABLE lists ADD COLUMN IF NOT EXISTS sunset JSONB NULL;

CREATE OR REPLACE FUNCTION subscriber_lists_hold_release() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status IS DISTINCT FROM NEW.status AND OLD.meta ? 'hold' AND NEW.meta ? 'hold' THEN
        IF (OLD.meta->'hold') ? 'final_at'
            AND current_setting('listmonk.release_final', true) IS DISTINCT FROM 'true' THEN
            RAISE NOTICE 'final hold kept: subscriber % list % (release needs release_final)', OLD.subscriber_id, OLD.list_id;
            RETURN NULL;
        END IF;
        NEW.meta := subscriber_lists_release_hold(NEW.meta, NEW.status::TEXT);
        NEW.updated_at := NOW();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
`
