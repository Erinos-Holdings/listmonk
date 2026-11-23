package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_43 is a FORK migration (brand roles, integrations listmonk/BRAND-ROLES-SPEC D6 / gate G8),
// not an upstream release. It deletes every list role's per-list child row (roles with parent_id
// set, type list, whose parent is a list role). v6.2.42 converted them into role_brands grants and
// left them in place only as the rollback path to a pre-v6.2.42 image; no code reads or writes them.
// A child of a USER role is not touched (none exist; upstream's get-user-roles still joins them).
//
// ROLLBACK: there is no image rollback below v6.2.42 after this runs -- an older binary would read no
// child rows and every brand-role holder would lose list access. The rows deleted in production
// (2026-11-23) are recorded in the spec's G8 line for a hand re-insert.
//
// The version key sits after the fork's v6.2.42 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.43.
func V6_2_43(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	res, err := db.Exec(`DELETE FROM roles c USING roles p
		WHERE c.parent_id = p.id AND c.type = 'list' AND p.type = 'list'`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	lo.Printf("v6.2.43: deleted %d list-role child row(s) (brand roles, BRAND-ROLES-SPEC G8)", n)
	return nil
}
