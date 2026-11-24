package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_44 is a FORK migration (SMS Admin, integrations listmonk/SMS-LINKS-ADMIN-SPEC D8/D11), not
// an upstream release. Additive. In one transaction it creates the table sms_promo_codes (a
// promotion value is an SMS code iff it has a row; brand is the marking's, NULL = All brands) with
// its unique LOWER(code) index, and grants sms:manage to Super Admin (role 1) -- the v6.2.17
// pattern: an upgraded database gets a new permission on role 1 only by migration
// (TestSuperAdminHoldsEveryPermission).
//
// ROLLBACK: pin the previous image (erinos.498). The table and the grant are inert to it -- but a
// role carrying sms:manage cannot be re-saved on the older binary (it refuses an unknown
// permission, the v6.2.14 note); strip it first only if a role edit is needed before re-release:
//
//	UPDATE roles SET permissions = array_remove(permissions, 'sms:manage');
//
// The version key sits after the fork's v6.2.43 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.44. Idempotent by construction.
func V6_2_44(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(smsPromoCodesDDL); err != nil {
		return err
	}
	if _, err := tx.Exec(smsManageGrantDDL); err != nil {
		return err
	}
	return tx.Commit()
}

// smsPromoCodesDDL is mirrored in schema.sql (keep the two identical; v6_2_44_db_test.go asserts it
// verbatim).
const smsPromoCodesDDL = `
CREATE TABLE IF NOT EXISTS sms_promo_codes (
    id          SERIAL PRIMARY KEY,
    code        TEXT NOT NULL,
    brand       TEXT NULL REFERENCES brands(slug) ON UPDATE CASCADE ON DELETE SET NULL,
    created_by  TEXT NOT NULL,
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sms_promo_codes_code ON sms_promo_codes (LOWER(code));
`

// smsManageGrantDDL is the v6.2.14 grant pattern. A fresh install needs nothing: schema.sql carries
// no role rows, and cmd/install.go grants role 1 every permission of permissions.json.
const smsManageGrantDDL = `
UPDATE roles SET permissions = permissions || '{sms:manage}' WHERE id = 1 AND NOT permissions @> '{sms:manage}';
`
