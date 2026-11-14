-- lists
-- name: get-lists
-- Fork (BRAND-PICKER-SPEC D2) -- brand is the list's brand slug, or empty when untagged.
-- Fork (naming standard, NAMING-STANDARD-SPEC D10) -- former_name, NULL when never renamed.
SELECT *, COALESCE(list_brand_tag(tags), '') AS brand,
    (SELECT f.name FROM former_names f WHERE f.kind = 'list' AND f.id = lists.id) AS former_name
    FROM lists WHERE (CASE WHEN $1 = '' THEN 1=1 ELSE type=$1::list_type END)
    AND (CASE WHEN $2 = '' THEN 1=1 ELSE status=$2::list_status END)
    AND CASE
        -- Optional list IDs based on user permission.
        WHEN $4 = TRUE THEN TRUE ELSE id = ANY($5::INT[])
    END
    ORDER BY CASE WHEN $3 = 'id' THEN id END, CASE WHEN $3 = 'name' THEN name END;

-- name: query-lists
WITH ls AS (
    SELECT COUNT(*) OVER () AS total, lists.*, COALESCE(list_brand_tag(lists.tags), '') AS brand,
        -- Fork (naming standard, NAMING-STANDARD-SPEC D10) -- former_name, NULL when never renamed.
        (SELECT f.name FROM former_names f WHERE f.kind = 'list' AND f.id = lists.id) AS former_name
        FROM lists WHERE
    CASE
        WHEN $1 > 0 THEN id = $1
        WHEN $2 != '' THEN uuid = $2::UUID
        WHEN $3 != '' THEN (TO_TSVECTOR(name) @@ TO_TSQUERY ($3) OR name ILIKE $3)
        ELSE TRUE
    END
    AND ($4 = '' OR type = $4::list_type)
    AND ($5 = '' OR optin = $5::list_optin)
    AND ($6 = '' OR status = $6::list_status)
    AND (CARDINALITY($7::VARCHAR(100)[]) = 0 OR $7 <@ tags)
    AND CASE
        -- Optional list IDs based on user permission.
        WHEN $8 = TRUE THEN TRUE ELSE id = ANY($9::INT[])
    END
    -- Fork (global brand, GLOBAL-BRAND-SPEC D6) -- nobrand=true keeps only the lists with no brand
    -- tag (list_brand_tag, the one statement of the single-brand rule), the No brand context.
    AND (NOT $12::BOOLEAN OR list_brand_tag(lists.tags) IS NULL)
    -- Fork (campaign lists, CAMPAIGN-LISTS-SPEC D6/D7) -- hide_system=true drops the HIDDEN system
    -- lists. This predicate is the SQL statement of cmd isHiddenSystemList (a standard trigger
    -- kind, or a reengage tag that is not a 180d or 270d win-back queue), pinned by
    -- TestQueryListsHideSystemAgreesWithGo. Change both together. GetList always binds false.
    AND (NOT $13::BOOLEAN OR NOT (
        COALESCE(lists.join_trigger->>'kind', '') IN ('first_order', 'no_purchase', 'post_purchase')
        OR EXISTS (SELECT 1 FROM UNNEST(lists.tags) t WHERE BTRIM(t, E' \t\n\r\f\x0B') LIKE 'reengage:%' AND BTRIM(t, E' \t\n\r\f\x0B') !~ '^reengage:\d+:[12]$')
    ))
),
-- Fork (list grid, LIST-GRID-SPEC D4) -- the view is grouped (list_id, status, segment, lang)
-- since v6.2.11, so it is summed to (list_id, status) BEFORE JSONB_OBJECT_AGG. That aggregate
-- keeps the last duplicate key rather than adding them up, and a status now spans many rows.
statuses AS (
    SELECT
        list_id,
        COALESCE(JSONB_OBJECT_AGG(status, subscriber_count) FILTER (WHERE status IS NOT NULL), '{}') AS subscriber_statuses,
        SUM(subscriber_count) AS subscriber_count
    FROM (
        SELECT list_id, status, SUM(subscriber_count)::BIGINT AS subscriber_count
        FROM mat_list_subscriber_stats GROUP BY list_id, status
    ) st
    GROUP BY list_id
),
-- Fork (list grid, LIST-GRID-SPEC D5) -- subscriber_grid. One cell row per (list, key, segment).
-- Keys are the SEND-language rows (send_lang folds the none bucket into en, the only place that
-- fold is stated), the informational none row (a subset of en), and the all row. A key exists
-- only where it counts someone, so total > 0 by construction. The all row is defaulted in the
-- final SELECT for an empty list.
cells AS (
    SELECT list_id, send_lang(lang) AS k, segment, SUM(subscriber_count)::BIGINT AS n
        FROM mat_list_subscriber_stats WHERE segment IS NOT NULL AND subscriber_count > 0 GROUP BY 1, 2, 3
    UNION ALL
    SELECT list_id, 'none' AS k, segment, SUM(subscriber_count)::BIGINT AS n
        FROM mat_list_subscriber_stats WHERE segment IS NOT NULL AND subscriber_count > 0 AND lang = 'none' GROUP BY 1, 3
    UNION ALL
    SELECT list_id, 'all' AS k, segment, SUM(subscriber_count)::BIGINT AS n
        FROM mat_list_subscriber_stats WHERE segment IS NOT NULL AND subscriber_count > 0 GROUP BY 1, 3
),
gridrows AS (
    SELECT list_id, k, JSONB_BUILD_OBJECT(
        'active', COALESCE(SUM(n) FILTER (WHERE segment = 'active'), 0),
        'held', COALESCE(SUM(n) FILTER (WHERE segment = 'held'), 0),
        'unsubscribed', COALESCE(SUM(n) FILTER (WHERE segment = 'unsubscribed'), 0),
        'pending', COALESCE(SUM(n) FILTER (WHERE segment = 'pending'), 0),
        'blocked', COALESCE(SUM(n) FILTER (WHERE segment = 'blocked'), 0),
        'total', COALESCE(SUM(n), 0)) AS cell
    FROM cells GROUP BY list_id, k
),
grid AS (
    SELECT list_id, JSONB_OBJECT_AGG(k, cell) AS subscriber_grid FROM gridrows GROUP BY list_id
),
-- Fork (brand health, BRAND-HEALTH-SPEC D11) -- the latest brand_health row per brand, and the
-- default-sender row (the newest row flagged is_default) that every untagged list carries.
bh_latest AS (
    SELECT DISTINCT ON (brand) brand, day, status, is_default, doc FROM brand_health ORDER BY brand, day DESC
),
bh_default AS (
    SELECT brand, day, status, is_default, doc FROM brand_health WHERE is_default ORDER BY day DESC, brand LIMIT 1
)
-- Fork -- sort the whole result set, THEN paginate. Upstream paginates inside the ls
-- CTE (no ORDER BY there) and sorts only the page, so a sort by created_at re-ordered
-- the 20 rows already picked rather than the full set (seen 2026-09-04). The
-- subscriber_count sort key comes from the join, so the ORDER BY has to live here --
-- and so must OFFSET/LIMIT. COUNT(*) OVER () in ls still reports the full total.
-- ls.id is the tiebreaker -- every sortable column ties (status, subscriber_count, a
-- bulk-created created_at), and without a total order a list can appear on two pages
-- and another on none.
-- Fork (list grid, D9) -- the five segment sort keys read the all row of the grid.
SELECT ls.*, COALESCE(ss.subscriber_statuses, '{}') AS subscriber_statuses, COALESCE(ss.subscriber_count, 0) AS subscriber_count,
    COALESCE(g.subscriber_grid, '{}'::JSONB) || JSONB_BUILD_OBJECT('all', COALESCE(g.subscriber_grid->'all',
        '{"active": 0, "held": 0, "unsubscribed": 0, "pending": 0, "blocked": 0, "total": 0}'::JSONB)) AS subscriber_grid,
    COALESCE((g.subscriber_grid->'all'->>'active')::BIGINT, 0) AS active_count,
    COALESCE((g.subscriber_grid->'all'->>'held')::BIGINT, 0) AS held_count,
    COALESCE((g.subscriber_grid->'all'->>'unsubscribed')::BIGINT, 0) AS unsubscribed_count,
    COALESCE((g.subscriber_grid->'all'->>'pending')::BIGINT, 0) AS pending_count,
    COALESCE((g.subscriber_grid->'all'->>'blocked')::BIGINT, 0) AS blocked_count,
    -- Fork (brand health, D11) -- health and health_tag are JSON literals (null when absent), so
    -- the Lists chip can tell "no row for this tag" (health null, health_tag set) from untagged.
    -- logo_url (BRANDS-UX-SPEC D8) is the row's facts.logoUrl, JSON null when absent.
    COALESCE(CASE WHEN h.brand IS NULL THEN NULL ELSE JSONB_BUILD_OBJECT('brand', h.brand, 'status', h.status,
        'note', h.doc->'note', 'as_of', h.day, 'default', h.is_default,
        'logo_url', h.doc->'facts'->'logoUrl') END, 'null'::JSONB) AS health,
    COALESCE(TO_JSONB(bt.tag), 'null'::JSONB) AS health_tag
    FROM ls LEFT JOIN statuses ss ON (ls.id = ss.list_id) LEFT JOIN grid g ON (ls.id = g.list_id)
    LEFT JOIN LATERAL (SELECT list_brand_tag(ls.tags) AS tag) bt ON TRUE
    LEFT JOIN LATERAL (
        SELECT * FROM bh_latest WHERE bt.tag IS NOT NULL AND bh_latest.brand = bt.tag
        UNION ALL
        SELECT * FROM bh_default WHERE bt.tag IS NULL
    ) h ON TRUE
    ORDER BY %order%, ls.id
    OFFSET $10 LIMIT (CASE WHEN $11 < 1 THEN NULL ELSE $11 END);

-- name: get-lists-by-optin
-- Can have a list of IDs or a list of UUIDs.
SELECT * FROM lists WHERE (CASE WHEN $1 != '' THEN optin=$1::list_optin ELSE TRUE END) AND
    (CASE WHEN $2::INT[] IS NOT NULL THEN id = ANY($2::INT[])
          WHEN $3::UUID[] IS NOT NULL THEN uuid = ANY($3::UUID[])
    END) ORDER BY name;

-- name: get-list-types
-- Retrieves the private|public type of lists by ID or uuid. Used for filtering.
SELECT id, uuid, type FROM lists WHERE
    (CASE WHEN $1::INT[] IS NOT NULL THEN id = ANY($1::INT[])
          WHEN $2::UUID[] IS NOT NULL THEN uuid = ANY($2::UUID[])
    END);

-- name: create-list
INSERT INTO lists (uuid, name, type, optin, status, tags, description) VALUES($1, $2, $3, $4, $5, $6, $7) RETURNING id;

-- name: update-list
WITH l AS (
    UPDATE lists SET
        name=(CASE WHEN $2 != '' THEN $2 ELSE name END),
        type=(CASE WHEN $3 != '' THEN $3::list_type ELSE type END),
        optin=(CASE WHEN $4 != '' THEN $4::list_optin ELSE optin END),
        status=(CASE WHEN $5 != '' THEN $5::list_status ELSE status END),
        -- Fork (BRAND-PICKER-SPEC D2) -- $6 is the free tags plus, when the request named a
        -- brand, its projection. $8 true (brand absent) keeps the list's CURRENT reserved tags,
        -- merged here in the one statement so a concurrent brand re-projection is never lost.
        -- Fork (FINAL-QUEUE-GUARD-SPEC D2) -- the list's CURRENT reengage: queue tags are kept
        -- whatever $6 carries (not repeated when $6 already has them); a new one in $6 is added.
        tags=$6::VARCHAR(100)[] || (CASE WHEN $8::BOOLEAN THEN ARRAY(
            SELECT t FROM UNNEST(lists.tags) AS t WHERE t LIKE 'brand:%' OR t LIKE 'from:%' OR t LIKE 'site:%'
        ) ELSE '{}' END)::VARCHAR(100)[] || ARRAY(
            SELECT t FROM UNNEST(lists.tags) AS t WHERE t LIKE 'reengage:%' AND t <> ALL(COALESCE($6::VARCHAR(100)[], '{}'))
        )::VARCHAR(100)[],
        description=(CASE WHEN $7 != '' THEN $7 ELSE description END),
        -- Fork (journey lists, integrations JOURNEY-LISTS-SPEC review F4) -- archiving an ACTIVE
        -- list switches its Active switch off, stamped with $9 (the request's user); an
        -- unarchive never turns it back on. SET expressions read the old row.
        trigger_active=(CASE WHEN $5 = 'archived' AND trigger_active THEN false ELSE trigger_active END),
        trigger_active_at=(CASE WHEN $5 = 'archived' AND trigger_active THEN NOW() ELSE trigger_active_at END),
        trigger_active_by=(CASE WHEN $5 = 'archived' AND trigger_active THEN $9 ELSE trigger_active_by END),
        updated_at=NOW()
    WHERE id = $1
    RETURNING id, name
),
c AS (
    UPDATE campaign_lists SET list_name = l.name FROM l WHERE campaign_lists.list_id = l.id RETURNING 1
)
SELECT COUNT(*) FROM l, c;

-- name: update-list-sunset
-- Fork (sunset v2, SUNSET-V2-SPEC D8) -- stores the queue countdown $2 verbatim. updated_at is
-- deliberately left alone: the Lambda writes this daily and it is not an edit of the list.
UPDATE lists SET sunset=$2::JSONB WHERE id = $1;

-- name: update-list-trigger
-- Fork (list join triggers, LIST-JOIN-TRIGGER-SPEC D4) -- one statement, compared against the OLD
-- row. $2 and $3 are the RESULTING join_trigger and transactional (a field absent from the request
-- carries its current value). Nothing changes, and the stamps stay, unless either differs, and
-- trigger_sync is nulled only when join_trigger changed (SET expressions read the old row).
-- RowsAffected tells the caller whether anything changed.
-- Fork (journey lists, integrations JOURNEY-LISTS-SPEC D3) -- $5 is "the request changed the list's
-- brand". A change of join_trigger, transactional or the brand turns an ACTIVE list's Active switch
-- off (stamped with $4); a brand-only change of an active list satisfies the guard through $5, and
-- the trigger stamps then stay (they record trigger/transactional changes only). An inactive or
-- untriggered list's brand-only change still no-ops.
UPDATE lists SET
    trigger_sync = (CASE WHEN join_trigger IS DISTINCT FROM $2::JSONB THEN NULL ELSE trigger_sync END),
    trigger_active = (CASE WHEN trigger_active AND (join_trigger IS DISTINCT FROM $2::JSONB OR transactional IS DISTINCT FROM $3 OR $5::BOOLEAN)
        THEN false ELSE trigger_active END),
    trigger_active_at = (CASE WHEN trigger_active AND (join_trigger IS DISTINCT FROM $2::JSONB OR transactional IS DISTINCT FROM $3 OR $5::BOOLEAN)
        THEN NOW() ELSE trigger_active_at END),
    trigger_active_by = (CASE WHEN trigger_active AND (join_trigger IS DISTINCT FROM $2::JSONB OR transactional IS DISTINCT FROM $3 OR $5::BOOLEAN)
        THEN $4 ELSE trigger_active_by END),
    join_trigger = $2::JSONB,
    transactional = $3,
    trigger_set_by = (CASE WHEN join_trigger IS DISTINCT FROM $2::JSONB OR transactional IS DISTINCT FROM $3 THEN $4 ELSE trigger_set_by END),
    trigger_set_at = (CASE WHEN join_trigger IS DISTINCT FROM $2::JSONB OR transactional IS DISTINCT FROM $3 THEN NOW() ELSE trigger_set_at END)
WHERE id = $1 AND (join_trigger IS DISTINCT FROM $2::JSONB OR transactional IS DISTINCT FROM $3 OR ($5::BOOLEAN AND trigger_active));

-- name: update-list-active
-- Fork (journey lists, integrations JOURNEY-LISTS-SPEC D2.4) -- the Active switch, one statement:
-- the flag and both stamps. updated_at is left alone (the trigger-sync posture: not an edit of the
-- list's content). A no-op (0 rows) when the value is already stored or the list carries no trigger.
UPDATE lists SET trigger_active = $2, trigger_active_at = NOW(), trigger_active_by = $3
WHERE id = $1 AND join_trigger IS NOT NULL AND trigger_active IS DISTINCT FROM $2;

-- name: get-list-running-evergreen
-- Fork (journey lists, integrations JOURNEY-LISTS-SPEC D2.3/D2a) -- the ONE definition of "a running
-- automation on this list": a campaign with evergreen = true and status running whose campaign_lists
-- include the list. Turning a list Active needs it; GET /api/lists/:id reports it.
SELECT EXISTS (
    SELECT 1 FROM campaigns c JOIN campaign_lists cl ON cl.campaign_id = c.id
    WHERE cl.list_id = $1 AND c.evergreen = true AND c.status = 'running'
);

-- name: update-list-trigger-sync
-- Fork (list join triggers, LIST-JOIN-TRIGGER-SPEC D6) -- stores the reconciler's write-back $2
-- verbatim. updated_at is left alone, as for update-list-sunset (not an edit of the list).
UPDATE lists SET trigger_sync=$2::JSONB WHERE id = $1;

-- name: update-lists-date
UPDATE lists SET updated_at=NOW() WHERE id = ANY($1);

-- name: delete-lists
DELETE FROM lists
WHERE CASE
    WHEN CARDINALITY($1::INT[]) > 0 THEN id = ANY($1)
    ELSE ($2 = '' OR to_tsvector(name) @@ to_tsquery($2))
END
AND CASE
    -- Optional list IDs based on user permission.
    WHEN $3 = TRUE THEN TRUE ELSE id = ANY($4::INT[])
END;

-- name: get-locked-lists
-- Fork (BRAND-PICKER-SPEC D4) -- which of the ids $1 carry a locked name $2.
SELECT id, name FROM lists WHERE id = ANY($1::INT[]) AND name = ANY($2::TEXT[]) ORDER BY id;

-- name: count-lists-by-name
SELECT COUNT(*) FROM lists WHERE name = $1;

-- name: get-step-list-add
-- Fork (integrations JOURNEY-UX-SPEC A1.1) -- the first list among $2 whose join_trigger kind is
-- a STEP kind -- no_purchase (a Welcome series step) or post_purchase (a Post-purchase series step,
-- integrations LIFECYCLE-SERIES-SPEC S2; the Go helper isStepTriggerKind names the same two) -- on
-- which a write would ADD a membership row: any such
-- list when $1 (the existing subscriber ids) is empty -- a new subscriber, an import, a by-query
-- add -- else one on which some subscriber of $1 has no row yet. At most one row.
SELECT l.id, l.name FROM lists l
WHERE l.id = ANY($2::INT[]) AND l.join_trigger->>'kind' IN ('no_purchase', 'post_purchase')
  AND (COALESCE(CARDINALITY($1::INT[]), 0) = 0 OR EXISTS (
    SELECT 1 FROM UNNEST($1::INT[]) AS s(id)
    WHERE NOT EXISTS (SELECT 1 FROM subscriber_lists sl WHERE sl.subscriber_id = s.id AND sl.list_id = l.id)
  ))
ORDER BY l.id
LIMIT 1;

-- name: get-subscriber-list-ids
-- Fork (journey lists, integrations JOURNEY-LISTS-SPEC D7 M2/M3) -- every list a subscriber has a
-- membership row on, any status: the before-set the lock compares a replacing update against.
SELECT list_id FROM subscriber_lists WHERE subscriber_id = $1 ORDER BY list_id;

-- name: get-delete-lists-by-query
-- Fork (BRAND-PICKER-SPEC D4) -- the ids and names delete-lists would remove for a by-query
-- delete (its exact predicate with an empty id array), so the handler can refuse a delete that
-- matches a locked list before anything is removed.
SELECT id, name FROM lists
WHERE ($1 = '' OR to_tsvector(name) @@ to_tsquery($1))
AND CASE
    WHEN $2 = TRUE THEN TRUE ELSE id = ANY($3::INT[])
END
ORDER BY id;
