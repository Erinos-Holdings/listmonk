-- Fork (naming standard, integrations listmonk/NAMING-STANDARD-SPEC D10/D13 §4.2). The former-name
-- rule and the name-only renames. NO COLON IN ANY COMMENT LINE (goyesql tags).

-- name: record-former-name
-- The former-name rule (D10, §4.2 step 4) in one statement, run in the same transaction as the
-- name write and BEFORE it. $1 kind (campaign, list, template), $2 id, $3 the new name. The old
-- name is read from the record's own table, exactly as stored (untrimmed).
--   no row and new differs from old -- insert (kind, id, old);
--   a row whose name equals new     -- delete it (a rename back);
--   otherwise                       -- keep the row (the ORIGINAL pre-standard name is kept).
-- Idempotent when the name is unchanged, so the general update paths may always run it.
WITH cur AS (
    SELECT name FROM campaigns WHERE $1 = 'campaign' AND id = $2
    UNION ALL SELECT name FROM lists WHERE $1 = 'list' AND id = $2
    UNION ALL SELECT name FROM templates WHERE $1 = 'template' AND id = $2
),
del AS (
    DELETE FROM former_names WHERE kind = $1 AND id = $2 AND name = $3 RETURNING 1
)
INSERT INTO former_names (kind, id, name)
    SELECT $1, $2, cur.name FROM cur
    WHERE cur.name <> $3 AND NOT EXISTS (SELECT 1 FROM del)
    ON CONFLICT (kind, id) DO NOTHING;

-- name: get-former-name
-- A record's former name, or no row.
SELECT name FROM former_names WHERE kind = $1 AND id = $2;

-- name: rename-campaign
-- The name ONLY (D13). updated_at is NOT bumped, so a rename never looks like a content edit, and
-- status, lists, body, attribs, from_email and schedule are untouched whatever the status.
UPDATE campaigns SET name = $2 WHERE id = $1 RETURNING id;

-- name: rename-list
-- The name ONLY, plus campaign_lists.list_name for that list (the statement update-list uses).
-- updated_at is NOT bumped.
WITH l AS (
    UPDATE lists SET name = $2 WHERE id = $1 RETURNING id, name
),
c AS (
    UPDATE campaign_lists SET list_name = l.name FROM l WHERE campaign_lists.list_id = l.id RETURNING 1
)
SELECT id FROM l;

-- name: rename-template
-- The name ONLY. updated_at is NOT bumped.
UPDATE templates SET name = $2 WHERE id = $1 RETURNING id;
