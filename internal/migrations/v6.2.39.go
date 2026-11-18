package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_39 is a FORK migration (campaign pause time, integrations
// listmonk/CAMPAIGN-STATUS-FILTER-SPEC D1), not an upstream release. In one transaction it:
//
//  1. adds campaigns.paused_at TIMESTAMPTZ NULL -- the instant of the campaign's LATEST
//     transition to paused, written only by update-campaign-status (D2);
//  2. backfills every paused campaign whose paused_at is NULL with its updated_at. That is an
//     APPROXIMATION (spec §7, I11): for a campaign written after its pause it is that later
//     write. The IS NULL guard makes a re-run change no row.
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary ignores the column
// (the app's queries run on an Unsafe() handle); no DDL reversal is needed.
//
// The version key sits after the fork's v6.2.38 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.39. Idempotent by construction.
func V6_2_39(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(campaignPausedAtDDL); err != nil {
		return err
	}
	if _, err := tx.Exec(campaignPausedAtBackfill); err != nil {
		return err
	}

	return tx.Commit()
}

// campaignPausedAtDDL is mirrored in schema.sql (the last block). Keep the two identical
// (v6_2_39_db_test.go asserts it verbatim).
const campaignPausedAtDDL = `
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS paused_at TIMESTAMP WITH TIME ZONE NULL;
`

// campaignPausedAtBackfill -- the user's choice over leaving old pauses blank (D1). Approximate.
const campaignPausedAtBackfill = `UPDATE campaigns SET paused_at = updated_at WHERE status = 'paused' AND paused_at IS NULL;`
