package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_47 is a FORK migration (approved brand logos, integrations listmonk/LOGO-APPROVAL-SPEC D3/D4),
// not an upstream release. Additive. In one transaction (SET LOCAL lock_timeout = '5s', the v6.2.21
// shape) it adds two columns to brands -- logos_approved and logos_rejected, TEXT[] NOT NULL DEFAULT
// '{}' (every existing row gets the empty lists; the review's D3.5 reads them, and their one writer
// is the update-brand-logos query behind POST /api/brands/:slug/logos) -- and grants the new
// brands:logos permission by role id to 1 (Super Admin), 2 (Marketing), 3 (Admin), 11 (Brand
// Manager) and 21 (Marketing Approver). A missing id is a no-op; no API role receives it.
//
// ROLLBACK: pin the previous image. The older binary names its brands columns and ignores these
// two; no DDL reversal is needed. A role carrying brands:logos cannot be re-saved on the older
// binary (it refuses an unknown permission, the v6.2.14 note); strip it first only if a role edit is
// needed before re-release:
//
//	UPDATE roles SET permissions = array_remove(permissions, 'brands:logos');
//
// The version key sits after the fork's v6.2.46 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.47. Idempotent by construction.
func V6_2_47(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(brandLogosColumnsDDL); err != nil {
		return err
	}
	if _, err := tx.Exec(brandLogosGrantDDL); err != nil {
		return err
	}
	return tx.Commit()
}

// brandLogosColumnsDDL is mirrored in schema.sql (the statement after the personas column; keep the
// two identical -- v6_2_47_db_test.go asserts it verbatim).
const brandLogosColumnsDDL = `
ALTER TABLE brands ADD COLUMN IF NOT EXISTS logos_approved TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE brands ADD COLUMN IF NOT EXISTS logos_rejected TEXT[] NOT NULL DEFAULT '{}';
`

// brandLogosGrantDDL is the v6.2.14 grant pattern over the D4 role ids. A fresh install needs
// nothing: schema.sql carries no role rows, and cmd/install.go grants role 1 every permission of
// permissions.json.
const brandLogosGrantDDL = `
UPDATE roles SET permissions = permissions || '{brands:logos}'
    WHERE id IN (1, 2, 3, 11, 21) AND NOT permissions @> '{brands:logos}';
`
