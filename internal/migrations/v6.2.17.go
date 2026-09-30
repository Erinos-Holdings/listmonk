package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_17 is a FORK migration (campaign review structure gate), not an upstream release. It grants
// campaigns:review_structure to Super Admin (role 1) -- the grant v6.2.15 omitted.
//
// WHY A MIGRATION: auth.User.HasPerm's Super Admin short-circuit (UserRoleID == 1) never fires on
// a loaded user -- core.setupUserFields copies UserRoleID into UserRole.ID and zeroes it -- so
// Super Admin holds exactly the permissions stored on role 1. A fresh install writes every key of
// permissions.json there (cmd/install.go); an UPGRADED database gets a new permission only by
// migration. Upstream follows the same pattern, and so does the fork's v6.2.14 (campaigns:review).
// The frontend's $can DOES short-circuit on role id 1, so a missed grant shows the button and the
// server refuses it with 403 -- found 2026-09-30 when the admin's Acknowledge on campaign 126 was
// refused; prod was granted by hand the same day, so this is a no-op there.
//
// INVARIANT (asserted by TestSuperAdminHoldsEveryPermission): after every migration, role 1's
// permissions are a superset of permissions.json. Every future fork permission ships with its
// role-1 grant in the same migration.
//
// ROLLBACK: pin the previous image; the grant is inert to an older binary that knows the
// permission (v6.2.15+). For anything older, strip it first with
//
//	UPDATE roles SET permissions = array_remove(permissions, 'campaigns:review_structure');
//
// The version key sits after the fork's v6.2.16 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.17. Idempotent by construction.
func V6_2_17(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(reviewStructureGrantDDL)
	return err
}

// reviewStructureGrantDDL is the v6.2.14 grant pattern. A fresh install needs nothing: schema.sql
// carries no role rows, and cmd/install.go grants role 1 every permission.
const reviewStructureGrantDDL = `
UPDATE roles SET permissions = permissions || '{campaigns:review_structure}' WHERE id = 1 AND NOT permissions @> '{campaigns:review_structure}';
`
