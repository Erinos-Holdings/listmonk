package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_30 is a FORK migration (usage decisions, 2026-10-31 polish), not an upstream release. It
// widens usage_decisions' decision CHECK from accept/reject to accept/reject/current/recommended:
// an item whose text carries a "Recommended:" line is decided Current | Recommended | Reject, every
// other item Accept | Reject (cmd/usage_decisions.go enforces which per item). Existing rows are
// untouched and stay valid; the append-only trigger is not involved (ALTER TABLE is DDL, not a row
// mutation).
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary only ever inserts
// accept/reject and reads any decision string; a stored current/recommended row would read in the
// older UI as "Accepted by <name>" (its line is reject-or-accepted) -- wrong but plausible, so roll
// back before Marketing records one. The wider CHECK is harmless to the older binary; no DDL
// reversal is needed.
//
// The version key sits after the fork's v6.2.29 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.30. Idempotent: DROP CONSTRAINT IF EXISTS, then ADD.
func V6_2_30(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(usageDecisionsCheckDDL); err != nil {
		return err
	}

	return tx.Commit()
}

// usageDecisionsCheckDDL produces exactly the CHECK schema.sql declares inline on the decision
// column (Postgres names an inline column check <table>_<column>_check). Keep the value lists
// identical.
const usageDecisionsCheckDDL = `
ALTER TABLE usage_decisions DROP CONSTRAINT IF EXISTS usage_decisions_decision_check;
ALTER TABLE usage_decisions ADD CONSTRAINT usage_decisions_decision_check
    CHECK (decision IN ('accept', 'reject', 'current', 'recommended'));
`
