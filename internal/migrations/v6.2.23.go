package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_23 is a FORK migration (list join triggers, integrations LIST-JOIN-TRIGGER-SPEC D1), not an
// upstream release. It:
//
//   - adds five lists columns: join_trigger JSONB NULL (a generic list attribute --
//     {"kind":"first_order"} or {"kind":"skus","skus":[...]}; the fork validates SHAPE only and an
//     external reconciler gives it meaning), transactional BOOLEAN NOT NULL DEFAULT false (gated by
//     the lists:transactional permission), trigger_set_by TEXT NULL and trigger_set_at TIMESTAMPTZ
//     NULL (the audit stamps, written only on a real change), and trigger_sync JSONB NULL (the
//     reconciler's write-back, PUT /api/lists/:id/trigger-sync). NULL / false on every existing
//     row; no data row is written.
//   - grants lists:transactional to Super Admin (role 1) -- the v6.2.17 pattern (see that file for
//     why an upgraded database needs the grant by migration).
//
// One transaction with SET LOCAL lock_timeout = '5s' (the v6.2.22 shape).
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary ignores the columns
// (the app's queries run on an Unsafe() handle, so unmapped columns scan away) and the grant is
// an unknown permission to it; no DDL reversal is needed.
//
// The version key sits after the fork's v6.2.22 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.23. Idempotent by construction.
func V6_2_23(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(listJoinTriggerDDL); err != nil {
		return err
	}
	if _, err := tx.Exec(listTransactionalGrantDDL); err != nil {
		return err
	}
	return tx.Commit()
}

// listJoinTriggerDDL is mirrored in schema.sql (the last block). Keep the two identical.
const listJoinTriggerDDL = `
ALTER TABLE lists ADD COLUMN IF NOT EXISTS join_trigger JSONB NULL;
ALTER TABLE lists ADD COLUMN IF NOT EXISTS transactional BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE lists ADD COLUMN IF NOT EXISTS trigger_set_by TEXT NULL;
ALTER TABLE lists ADD COLUMN IF NOT EXISTS trigger_set_at TIMESTAMP WITH TIME ZONE NULL;
ALTER TABLE lists ADD COLUMN IF NOT EXISTS trigger_sync JSONB NULL;
`

// listTransactionalGrantDDL is the v6.2.14/v6.2.17 grant pattern. A fresh install needs nothing:
// schema.sql carries no role rows, and cmd/install.go grants role 1 every permission.
const listTransactionalGrantDDL = `
UPDATE roles SET permissions = permissions || '{lists:transactional}' WHERE id = 1 AND NOT permissions @> '{lists:transactional}';
`
