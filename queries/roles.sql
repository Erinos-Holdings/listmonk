-- name: get-user-roles
WITH mainroles AS (
    SELECT ur.* FROM roles ur WHERE type = 'user' AND ur.parent_id IS NULL AND
    CASE WHEN $1::INT != 0 THEN ur.id = $1 ELSE TRUE END
),
listPerms AS (
    SELECT ur.parent_id, JSONB_AGG(JSONB_BUILD_OBJECT('id', ur.list_id, 'name', lists.name, 'permissions', ur.permissions)) AS listPerms
    FROM roles ur
    LEFT JOIN lists ON(lists.id = ur.list_id)
    WHERE ur.parent_id IS NOT NULL GROUP BY ur.parent_id
)
SELECT p.*, COALESCE(l.listPerms, '[]'::JSONB) AS "list_permissions" FROM mainroles p
    LEFT JOIN listPerms l ON p.id = l.parent_id ORDER BY p.created_at;

-- name: get-list-roles
-- Fork (brand roles, integrations listmonk/BRAND-ROLES-SPEC D2/D5) -- each list role's brand grants
-- (role_brands) and their resolution through role_list_grants, the projection get-user and
-- get-users use verbatim. The per-list child rows are never read.
SELECT p.*,
    (SELECT COALESCE(JSONB_AGG(JSONB_BUILD_OBJECT('id', g.list_id, 'name', g.list_name, 'brand', g.brand,
        'permissions', g.permissions) ORDER BY g.list_id), '[]'::JSONB)
        FROM role_list_grants(p.id) g) AS "list_permissions",
    (SELECT COALESCE(JSONB_AGG(JSONB_BUILD_OBJECT('brand', b.brand, 'level', b.level)
        ORDER BY b.level DESC, b.brand NULLS FIRST), '[]'::JSONB)
        FROM role_brands b WHERE b.role_id = p.id) AS "brand_grants"
FROM roles p WHERE p.type = 'list' AND p.parent_id IS NULL ORDER BY p.created_at;

-- name: update-list-role
-- Fork (brand roles, BRAND-ROLES-SPEC D5) -- update-role restricted to a list role (update-role has
-- no type predicate); no row for any other id, which the handler answers with a 400.
UPDATE roles SET name=$2, permissions='{}', updated_at=NOW() WHERE id=$1 AND type='list' AND parent_id IS NULL RETURNING *;

-- name: delete-role-brands
-- Fork (brand roles, BRAND-ROLES-SPEC D5) -- the first half of the atomic grant replacement (the
-- second is insert-role-brands, in the same transaction).
DELETE FROM role_brands WHERE role_id = $1;

-- name: insert-role-brands
-- $2 the brands ('' = All brands, stored NULL), $3 the levels, position for position.
INSERT INTO role_brands (role_id, brand, level)
    SELECT $1, NULLIF(x.brand, ''), x.level FROM UNNEST($2::TEXT[], $3::TEXT[]) AS x(brand, level);

-- name: get-brand-slugs-in
-- Fork (brand roles, BRAND-ROLES-SPEC D5) -- the given slugs that are brands rows (the 400 for an
-- unknown brand before the write).
SELECT slug FROM brands WHERE slug = ANY($1::TEXT[]);


-- name: create-role
INSERT INTO roles (name, type, permissions, created_at, updated_at) VALUES($1, $2, $3, NOW(), NOW()) RETURNING *;

-- name: update-role
UPDATE roles SET name=$2, permissions=$3 WHERE id=$1 and parent_id IS NULL RETURNING *;

-- name: delete-role
DELETE FROM roles WHERE id=$1;
