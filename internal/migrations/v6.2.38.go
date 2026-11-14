package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_38 is a FORK migration (naming standard, integrations listmonk/NAMING-STANDARD-SPEC D10
// §4.1), not an upstream release. It adds one table, former_names: per (kind, id) -- a campaign,
// list or template -- the record's ORIGINAL name before its first rename (the rename endpoints
// and the name-changing update paths write it; the API rows carry it as former_name, shown on
// hover; core.CurrentReview hashes a pre-release review with it).
//
// Additive only. No foreign key (three kinds; SERIAL ids are never reused, so an orphan row after
// a delete is harmless and never cleaned). It touches nothing else -- no settings row, no review
// row, no other table.
//
// ROLLBACK: pin the previous image and re-apply the host. cmd/upgrade.go::checkUpgrade refuses
// only migrations NEWER than the database's last one, so the older binary boots and never reads
// the table. No DDL reversal is needed.
//
// The version key sits after the fork's v6.2.37 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.38. Idempotent by construction (IF NOT EXISTS).
func V6_2_38(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(formerNamesDDL); err != nil {
		return err
	}

	return tx.Commit()
}

// formerNamesDDL is mirrored in schema.sql (after the v6.2.37 join-link functions). Keep the two
// identical (v6_2_38_db_test.go compares the table shape).
const formerNamesDDL = `
CREATE TABLE IF NOT EXISTS former_names (
    kind        TEXT NOT NULL CHECK (kind IN ('campaign', 'list', 'template')),
    id          INTEGER NOT NULL,
    name        TEXT NOT NULL,
    renamed_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (kind, id)
);
`
