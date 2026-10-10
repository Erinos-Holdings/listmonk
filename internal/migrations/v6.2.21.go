package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_21 is a FORK migration (persona From, integrations PERSONA-FROM-SPEC D1), not an upstream
// release. It adds personas TEXT[] NOT NULL DEFAULT '{}' to brands: a brand's approved persona
// sender display names (models/personas.go). Every existing row gets the empty set, so nothing
// changes what any campaign may carry until a human adds a persona.
//
// One statement in one transaction with SET LOCAL lock_timeout = '5s' (the v6.2.16/v6.2.20
// shape): a blocked upgrade fails loudly instead of waiting.
//
// ROLLBACK: revert the release commit in the integrations repo (the image pin) and re-apply the
// host. The older binary names its brands columns and ignores this one; no DDL reversal is
// needed. A campaign carrying a persona From is refused on edit by the older binary until it is
// re-saved with the brand From.
//
// The version key sits after the fork's v6.2.20 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.21. Idempotent by construction.
func V6_2_21(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(personasColumnDDL); err != nil {
		return err
	}
	return tx.Commit()
}

// personasColumnDDL is mirrored in schema.sql (the statement after the brands table).
const personasColumnDDL = `
ALTER TABLE brands ADD COLUMN IF NOT EXISTS personas TEXT[] NOT NULL DEFAULT '{}';
`
