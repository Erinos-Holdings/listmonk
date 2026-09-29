-- name: get-dashboard-charts
SELECT data FROM mat_dashboard_charts;

-- name: get-dashboard-counts
SELECT data FROM mat_dashboard_counts;

-- name: get-dashboard-counts-scoped
-- Fork (brand analytics, BRAND-ANALYTICS-SPEC D7). The Dashboard counts for a list-scoped user,
-- live (no materialized view, no cache), in the mat_dashboard_counts shape plus scoped=true and
-- never a null. $1 = permitted list ids (P), $2 = campaigns get_all (C = every campaign, else the
-- campaigns with ANY campaign_lists row on P -- the D2 rule). Subscribers are every
-- subscriber_lists row on P in any subscription status (the Lists page's population), counted
-- once across lists; orphans are 0 by definition.
WITH camps AS (
    SELECT c.status, c.sent FROM campaigns c
    WHERE $2::BOOLEAN OR EXISTS (
        SELECT 1 FROM campaign_lists cl WHERE cl.campaign_id = c.id AND cl.list_id = ANY($1::INT[])
    )
),
subs AS (
    SELECT DISTINCT sl.subscriber_id FROM subscriber_lists sl WHERE sl.list_id = ANY($1::INT[])
),
lst AS (
    SELECT type, optin FROM lists WHERE id = ANY($1::INT[])
)
SELECT JSON_BUILD_OBJECT(
    'subscribers', JSON_BUILD_OBJECT(
        'total', (SELECT COUNT(*) FROM subs),
        'blocklisted', (SELECT COUNT(*) FROM subs JOIN subscribers s ON s.id = subs.subscriber_id WHERE s.status = 'blocklisted'),
        'orphans', 0
    ),
    'lists', JSON_BUILD_OBJECT(
        'total', (SELECT COUNT(*) FROM lst),
        'private', (SELECT COUNT(*) FROM lst WHERE type = 'private'),
        'public', (SELECT COUNT(*) FROM lst WHERE type = 'public'),
        'optin_single', (SELECT COUNT(*) FROM lst WHERE optin = 'single'),
        'optin_double', (SELECT COUNT(*) FROM lst WHERE optin = 'double')
    ),
    'campaigns', JSON_BUILD_OBJECT(
        'total', (SELECT COUNT(*) FROM camps),
        'by_status', COALESCE((
            SELECT JSON_OBJECT_AGG(status, num) FROM (SELECT status, COUNT(*) AS num FROM camps GROUP BY status) r
        ), '{}'::JSON)
    ),
    'messages', (SELECT COALESCE(SUM(sent), 0) FROM camps),
    'scoped', TRUE
) AS data;

-- name: get-dashboard-charts-scoped
-- Fork (brand analytics, BRAND-ANALYTICS-SPEC D7). Views and clicks per day of the campaigns C
-- ($1/$2 as get-dashboard-counts-scoped). Per table, the window ends on the day of the latest
-- event of C, by MAX(created_at) over the campaign_id index (never ORDER BY id DESC LIMIT 1,
-- which walks the whole table backward), with mat_dashboard_charts' exact bounds (31 days).
WITH camps AS (
    SELECT c.id FROM campaigns c
    WHERE $2::BOOLEAN OR EXISTS (
        SELECT 1 FROM campaign_lists cl WHERE cl.campaign_id = c.id AND cl.list_id = ANY($1::INT[])
    )
),
clicks AS (
    SELECT JSON_AGG(ROW_TO_JSON(r)) FROM (
        WITH d AS (
            SELECT MAX(created_at)::DATE AS to_date FROM link_clicks WHERE campaign_id IN (SELECT id FROM camps)
        )
        SELECT COUNT(*) AS count, created_at::DATE AS date FROM link_clicks
            WHERE campaign_id IN (SELECT id FROM camps)
            AND created_at >= (SELECT to_date FROM d) - INTERVAL '30 DAY'
            AND created_at < (SELECT to_date FROM d) + INTERVAL '1 day'
            GROUP BY date ORDER BY date
    ) r
),
views AS (
    SELECT JSON_AGG(ROW_TO_JSON(r)) FROM (
        WITH d AS (
            SELECT MAX(created_at)::DATE AS to_date FROM campaign_views WHERE campaign_id IN (SELECT id FROM camps)
        )
        SELECT COUNT(*) AS count, created_at::DATE AS date FROM campaign_views
            WHERE campaign_id IN (SELECT id FROM camps)
            AND created_at >= (SELECT to_date FROM d) - INTERVAL '30 DAY'
            AND created_at < (SELECT to_date FROM d) + INTERVAL '1 day'
            GROUP BY date ORDER BY date
    ) r
)
SELECT JSON_BUILD_OBJECT(
    'link_clicks', COALESCE((SELECT * FROM clicks), '[]'),
    'campaign_views', COALESCE((SELECT * FROM views), '[]'),
    'scoped', TRUE
) AS data;

-- name: get-settings
SELECT JSON_OBJECT_AGG(key, value) AS settings FROM (SELECT * FROM settings ORDER BY key) t;

-- name: update-settings
UPDATE settings AS s SET value = c.value
    -- For each key in the incoming JSON map, update the row with the key and its value.
    FROM(SELECT * FROM JSONB_EACH($1)) AS c(key, value) WHERE s.key = c.key;

-- name: update-settings-by-key
UPDATE settings SET value = $2, updated_at = NOW() WHERE key = $1;

-- name: get-db-info
SELECT JSON_BUILD_OBJECT('version', (SELECT VERSION()),
                        'size_mb', (SELECT ROUND(pg_database_size((SELECT CURRENT_DATABASE()))/(1024^2)))) AS info;
