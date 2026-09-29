package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_16 is a FORK migration (location stats, integrations LOCATION-STATS-SPEC §2.1), not an
// upstream release. It adds a nullable country CHAR(2) to campaign_views and link_clicks: the
// ISO 3166-1 alpha-2 code the CDN resolved for the request (CloudFront-Viewer-Country). NULL means
// unknown -- every row that predates this migration, and any request without a valid header. No
// IP address is stored.
//
// A nullable column with no default is a catalog-only change (no table rewrite), but ALTER TABLE
// still takes an ACCESS EXCLUSIVE lock, and every pixel/click insert would queue behind it while
// it waits on a long reader. The statements therefore run in one transaction with
// SET LOCAL lock_timeout = '5s', so a blocked upgrade fails loudly instead of stalling tracking.
//
// ROLLBACK: pin the previous image. The columns are nullable and the older inserts name their
// columns, so an older binary runs unchanged against this schema; no DDL reversal is needed.
//
// The version key sits after the fork's v6.2.15 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.16. Idempotent by construction.
func V6_2_16(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(locationColumnsDDL); err != nil {
		return err
	}
	return tx.Commit()
}

// locationColumnsDDL is mirrored in schema.sql (the country column of both CREATE TABLEs).
const locationColumnsDDL = `
ALTER TABLE campaign_views ADD COLUMN IF NOT EXISTS country CHAR(2) NULL;
ALTER TABLE link_clicks ADD COLUMN IF NOT EXISTS country CHAR(2) NULL;
`
