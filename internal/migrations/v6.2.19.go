package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_19 is a FORK migration (client stats, integrations CLIENT-STATS-SPEC D4), not an upstream
// release. It adds a nullable client TEXT to campaign_views and link_clicks: the closed-vocabulary
// token the tracking handlers derive from the request's User-Agent (cmd/public.go classifyClient).
// The raw User-Agent is never stored. NULL means unknown -- every row that predates this migration,
// and any request without a User-Agent.
//
// The v6.2.16 shape exactly: a nullable column with no default is a catalog-only change, but ALTER
// TABLE still takes an ACCESS EXCLUSIVE lock and every pixel/click insert would queue behind it
// while it waits on a long reader. One transaction with SET LOCAL lock_timeout = '5s', so a blocked
// upgrade fails loudly instead of stalling tracking.
//
// ROLLBACK: pin the previous image. The columns are nullable and the older inserts name their
// columns, so an older binary runs unchanged against this schema; no DDL reversal is needed.
//
// The version key sits after the fork's v6.2.18 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.19. Idempotent by construction.
func V6_2_19(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(clientColumnsDDL); err != nil {
		return err
	}
	return tx.Commit()
}

// clientColumnsDDL is mirrored in schema.sql (the client column of both CREATE TABLEs).
const clientColumnsDDL = `
ALTER TABLE campaign_views ADD COLUMN IF NOT EXISTS client TEXT NULL;
ALTER TABLE link_clicks ADD COLUMN IF NOT EXISTS client TEXT NULL;
`
