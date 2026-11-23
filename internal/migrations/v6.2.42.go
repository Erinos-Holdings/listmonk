package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_42 is a FORK migration (brand roles, integrations listmonk/BRAND-ROLES-SPEC D1/D2/D6), not an
// upstream release. In one transaction it:
//
//  1. creates the table role_brands (a list role's brand grants: brand NULL = All brands, analytics
//     only; level manage or analytics) with its unique (role_id, COALESCE(brand, '*')) index, and the
//     function role_list_grants (the ONE resolution of a role's grants to lists, read by get-user,
//     get-users and get-list-roles);
//  2. ONLY when step 1 actually created role_brands (to_regclass checked before the DDL, inside the
//     same transaction), converts every list role's per-list child rows (roles with parent_id set,
//     type list) into brand grants: brand = list_brand_tag of the child's list, level manage when the
//     child's permissions include list:manage, else analytics; several children on one brand
//     collapse to the highest level. A child on an untagged list, on a list whose brand: tag has no
//     brands row, or with no permissions is dropped and logged. A re-run finds the table and converts
//     nothing, so a grant deleted after the release stays deleted (D6, I7).
//
// The child rows are LEFT IN PLACE and never read again by this or any later code (their deletion is
// a later release, gate G8).
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary ignores role_brands and
// role_list_grants and reads the child rows again: every brand-role edit made after the release is
// lost and any grant removed since is restored -- review the list roles after a rollback. A
// roll-forward does not re-convert (the version is recorded): re-run gate G1 and re-apply on the form
// any role change made on the old image.
//
// The version key sits after the fork's v6.2.41 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.42.
func V6_2_42(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}

	var had bool
	if err := tx.Get(&had, `SELECT to_regclass('role_brands') IS NOT NULL`); err != nil {
		return err
	}
	if _, err := tx.Exec(roleBrandsDDL); err != nil {
		return err
	}
	if !had {
		var drops []struct {
			RoleID int    `db:"role_id"`
			ListID int    `db:"list_id"`
			Reason string `db:"reason"`
		}
		if err := tx.Select(&drops, roleBrandsConversionDrops); err != nil {
			return err
		}
		for _, d := range drops {
			if lo != nil {
				lo.Printf("v6.2.42: list role %d: per-list grant on list %d dropped (%s)", d.RoleID, d.ListID, d.Reason)
			}
		}
		if _, err := tx.Exec(roleBrandsConversion); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// roleBrandsDDL is mirrored in schema.sql (the last block, comments aside). Keep the two identical
// (v6_2_42_db_test.go asserts it verbatim).
//
// role_list_grants: per list carrying a brand: tag, the grants of the role that reach it (a grant on
// the list's brand, or an All brands grant) -- list:get + list:manage when any of them is manage,
// else list:analytics (manage > analytics per list, D2). A list with no brand tag is never reached.
// Lists of every status. list_brand_tag remains the one statement of a list's brand.
const roleBrandsDDL = `
CREATE TABLE IF NOT EXISTS role_brands (
    role_id  INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE ON UPDATE CASCADE,
    brand    TEXT NULL REFERENCES brands(slug) ON DELETE CASCADE ON UPDATE CASCADE,
    level    TEXT NOT NULL CHECK (level IN ('manage', 'analytics')),
    CHECK (brand IS NOT NULL OR level = 'analytics')
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_role_brands ON role_brands (role_id, COALESCE(brand, '*'));
CREATE OR REPLACE FUNCTION role_list_grants(p_role_id INT) RETURNS TABLE (list_id INT, list_name TEXT, brand TEXT, permissions TEXT[]) AS $$
    SELECT l.id, l.name::TEXT, list_brand_tag(l.tags),
        CASE WHEN BOOL_OR(g.level = 'manage') THEN ARRAY['list:get', 'list:manage']::TEXT[]
            ELSE ARRAY['list:analytics']::TEXT[] END
    FROM role_brands g
    JOIN lists l ON list_brand_tag(l.tags) IS NOT NULL AND (g.brand IS NULL OR g.brand = list_brand_tag(l.tags))
    WHERE g.role_id = p_role_id
    GROUP BY l.id, l.name, list_brand_tag(l.tags)
    ORDER BY l.id;
$$ LANGUAGE sql STABLE;
`

// roleBrandsConversionDrops names every child row step 2 does not convert, with the reason.
const roleBrandsConversionDrops = `
SELECT c.parent_id AS role_id, c.list_id,
    CASE WHEN l.id IS NULL THEN 'list missing'
        WHEN list_brand_tag(l.tags) IS NULL THEN 'untagged list'
        WHEN b.slug IS NULL THEN 'brand ' || list_brand_tag(l.tags) || ' has no brands row'
        ELSE 'no permissions' END AS reason
FROM roles c
JOIN roles p ON p.id = c.parent_id AND p.type = 'list' AND p.parent_id IS NULL
LEFT JOIN lists l ON l.id = c.list_id
LEFT JOIN brands b ON b.slug = list_brand_tag(l.tags)
WHERE c.parent_id IS NOT NULL AND c.type = 'list'
    AND (b.slug IS NULL OR CARDINALITY(c.permissions) = 0)
ORDER BY c.parent_id, c.list_id;
`

// roleBrandsConversion is step 2: one grant per (role, brand), manage when any child on the brand
// carries list:manage.
const roleBrandsConversion = `
INSERT INTO role_brands (role_id, brand, level)
SELECT c.parent_id, b.slug,
    CASE WHEN BOOL_OR('list:manage' = ANY(c.permissions)) THEN 'manage' ELSE 'analytics' END
FROM roles c
JOIN roles p ON p.id = c.parent_id AND p.type = 'list' AND p.parent_id IS NULL
JOIN lists l ON l.id = c.list_id
JOIN brands b ON b.slug = list_brand_tag(l.tags)
WHERE c.parent_id IS NOT NULL AND c.type = 'list' AND CARDINALITY(c.permissions) > 0
GROUP BY c.parent_id, b.slug;
`
