-- name: get-dashboard-charts
SELECT data FROM mat_dashboard_charts;

-- name: get-dashboard-counts-scoped
-- raw: true
-- Fork (brand analytics, BRAND-ANALYTICS-SPEC D7; dashboard counts, integrations
-- DASHBOARD-COUNTS-SPEC D1/D4/D5/D6). The ONE Dashboard counts query, live (no materialized view,
-- no cache) for every reader -- mat_dashboard_counts is no longer read. $1 = the scope's list ids,
-- $2 = campaigns get_all (C = every campaign, else the campaigns with ANY campaign_lists row on $1
-- -- the BRAND-ANALYTICS D2 rule), $3 = unscoped (blanket list access, no list_id), which reads
-- every list and every campaign (the Go side passes $2 = true with it) and is the only read that
-- computes orphans (account-wide, else 0). Subscribers are the SUBSCRIPTIONS of the PUBLIC lists in
-- scope (a brand's public list is its master opt-in list, every private list a derivative), summed
-- per segment and per language with the Lists grid's own definitions (subscription_segment,
-- subscriber_lang, send_lang) from the base tables. grid has query-lists' subscriber_grid shape;
-- total and blocklisted are its all row. engagement sums get-campaign-stats over C, the counted
-- expression interpolated at boot with the same statsSel (cmd/init.go), so the two never disagree.
WITH camps AS (
    SELECT c.id, c.status, c.sent FROM campaigns c
    WHERE $2::BOOLEAN OR EXISTS (
        SELECT 1 FROM campaign_lists cl WHERE cl.campaign_id = c.id AND cl.list_id = ANY($1::INT[])
    )
),
lst AS (
    SELECT id, type, optin, status FROM lists WHERE $3::BOOLEAN OR id = ANY($1::INT[])
),
seg AS (
    SELECT subscription_segment(sl.status, sl.meta, s.status, l.optin) AS segment,
        subscriber_lang(s.attribs) AS lang, COUNT(*)::BIGINT AS n
    FROM lst l
    JOIN subscriber_lists sl ON (sl.list_id = l.id)
    JOIN subscribers s ON (s.id = sl.subscriber_id)
    -- Active only (Stage 4 F1) -- the brand scope's effective list set is loaded active-only, so
    -- All brands must equal the sum of the brands; an archived list keeps its subscriptions.
    WHERE l.type = 'public' AND l.status = 'active'
    GROUP BY 1, 2
),
cells AS (
    SELECT send_lang(lang) AS k, segment, SUM(n)::BIGINT AS n FROM seg GROUP BY 1, 2
    UNION ALL
    SELECT 'none' AS k, segment, SUM(n)::BIGINT AS n FROM seg WHERE lang = 'none' GROUP BY 2
    UNION ALL
    SELECT 'all' AS k, segment, SUM(n)::BIGINT AS n FROM seg GROUP BY 2
),
gridrows AS (
    SELECT k, JSONB_BUILD_OBJECT(
        'active', COALESCE(SUM(n) FILTER (WHERE segment = 'active'), 0),
        'held', COALESCE(SUM(n) FILTER (WHERE segment = 'held'), 0),
        'unsubscribed', COALESCE(SUM(n) FILTER (WHERE segment = 'unsubscribed'), 0),
        'pending', COALESCE(SUM(n) FILTER (WHERE segment = 'pending'), 0),
        'blocked', COALESCE(SUM(n) FILTER (WHERE segment = 'blocked'), 0),
        'total', COALESCE(SUM(n), 0)) AS cell
    FROM cells GROUP BY k
),
grid AS (
    SELECT COALESCE((SELECT JSONB_OBJECT_AGG(k, cell) FROM gridrows), '{}'::JSONB) AS g
),
fullgrid AS (
    SELECT g || JSONB_BUILD_OBJECT('all', COALESCE(g->'all',
        '{"active": 0, "held": 0, "unsubscribed": 0, "pending": 0, "blocked": 0, "total": 0}'::JSONB)) AS g
    FROM grid
),
ev AS (
    SELECT campaign_id, COUNT(%[1]s) AS num FROM campaign_views
    WHERE campaign_id IN (SELECT id FROM camps) GROUP BY campaign_id
),
ec AS (
    SELECT campaign_id, COUNT(%[1]s) AS num FROM link_clicks
    WHERE campaign_id IN (SELECT id FROM camps) GROUP BY campaign_id
),
eb AS (
    SELECT campaign_id, COUNT(campaign_id) AS num FROM bounces
    WHERE campaign_id IN (SELECT id FROM camps) GROUP BY campaign_id
),
-- Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC R2) -- Complaints is a named SUBSET
-- of Bounces (raw rows of type complaint on C); eb above keeps get-campaign-stats' definition,
-- every bounce row, complaints included (hazard 96, one Bounces definition).
ebc AS (
    SELECT COUNT(*) AS num FROM bounces
    WHERE campaign_id IN (SELECT id FROM camps) AND type = 'complaint'
)
SELECT JSON_BUILD_OBJECT(
    'subscribers', JSON_BUILD_OBJECT(
        'total', (SELECT (g->'all'->>'total')::BIGINT FROM fullgrid),
        'blocklisted', (SELECT (g->'all'->>'blocked')::BIGINT FROM fullgrid),
        'orphans', (CASE WHEN $3::BOOLEAN THEN (
            SELECT COUNT(*) FROM subscribers s
            WHERE NOT EXISTS (SELECT 1 FROM subscriber_lists sl WHERE sl.subscriber_id = s.id)
        ) ELSE 0 END),
        'grid', (SELECT g FROM fullgrid)
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
    'engagement', JSON_BUILD_OBJECT(
        'views', (SELECT COALESCE(SUM(num), 0) FROM ev),
        'clicks', (SELECT COALESCE(SUM(num), 0) FROM ec),
        'bounces', (SELECT COALESCE(SUM(num), 0) FROM eb),
        'complaints', (SELECT num FROM ebc)
    ),
    'scoped', NOT $3::BOOLEAN
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

-- name: get-dashboard-clients
-- Fork (client stats, CLIENT-STATS-SPEC D6). Views and clicks per email-client token for the
-- main Dashboard's client panel over every campaign, live (no materialized view). Any scope (a
-- list-scoped user, the global brand's list_id) reads get-dashboard-clients-scoped instead
-- (GLOBAL-BRAND-SPEC D5, which retired the panel's own brand parameter). Per table, the window is
-- get-dashboard-charts-scoped's rule exactly (31 days ending on the day of the latest event of
-- those campaigns, by MAX(created_at) over the campaign_id index). Raw rows, as the Dashboard charts
-- count. Unknown client (NULL) is reported as the empty string.
WITH camps AS (
    SELECT c.id FROM campaigns c
),
vd AS (
    SELECT MAX(created_at)::DATE AS to_date FROM campaign_views WHERE campaign_id IN (SELECT id FROM camps)
),
v AS (
    SELECT COALESCE(client, '') AS client, COUNT(*) AS n FROM campaign_views
        WHERE campaign_id IN (SELECT id FROM camps)
        AND created_at >= (SELECT to_date FROM vd) - INTERVAL '30 DAY'
        AND created_at < (SELECT to_date FROM vd) + INTERVAL '1 day'
        GROUP BY 1
),
cd AS (
    SELECT MAX(created_at)::DATE AS to_date FROM link_clicks WHERE campaign_id IN (SELECT id FROM camps)
),
c AS (
    SELECT COALESCE(client, '') AS client, COUNT(*) AS n FROM link_clicks
        WHERE campaign_id IN (SELECT id FROM camps)
        AND created_at >= (SELECT to_date FROM cd) - INTERVAL '30 DAY'
        AND created_at < (SELECT to_date FROM cd) + INTERVAL '1 day'
        GROUP BY 1
)
SELECT COALESCE(v.client, c.client) AS client, COALESCE(v.n, 0) AS views, COALESCE(c.n, 0) AS clicks
    FROM v FULL OUTER JOIN c ON v.client = c.client;

-- name: get-dashboard-clients-scoped
-- Fork (client stats, CLIENT-STATS-SPEC D6/D7; GLOBAL-BRAND-SPEC D5). get-dashboard-clients over a
-- scope, with $1/$2 exactly as get-dashboard-counts-scoped ($1 = the scope's list ids, $2 =
-- campaigns get_all, else the campaigns with ANY campaign_lists row on $1; always false under a
-- global brand list_id). Same window and counting.
WITH camps AS (
    SELECT c.id FROM campaigns c
    WHERE $2::BOOLEAN OR EXISTS (
        SELECT 1 FROM campaign_lists cl WHERE cl.campaign_id = c.id AND cl.list_id = ANY($1::INT[])
    )
),
vd AS (
    SELECT MAX(created_at)::DATE AS to_date FROM campaign_views WHERE campaign_id IN (SELECT id FROM camps)
),
v AS (
    SELECT COALESCE(client, '') AS client, COUNT(*) AS n FROM campaign_views
        WHERE campaign_id IN (SELECT id FROM camps)
        AND created_at >= (SELECT to_date FROM vd) - INTERVAL '30 DAY'
        AND created_at < (SELECT to_date FROM vd) + INTERVAL '1 day'
        GROUP BY 1
),
cd AS (
    SELECT MAX(created_at)::DATE AS to_date FROM link_clicks WHERE campaign_id IN (SELECT id FROM camps)
),
c AS (
    SELECT COALESCE(client, '') AS client, COUNT(*) AS n FROM link_clicks
        WHERE campaign_id IN (SELECT id FROM camps)
        AND created_at >= (SELECT to_date FROM cd) - INTERVAL '30 DAY'
        AND created_at < (SELECT to_date FROM cd) + INTERVAL '1 day'
        GROUP BY 1
)
SELECT COALESCE(v.client, c.client) AS client, COALESCE(v.n, 0) AS views, COALESCE(c.n, 0) AS clicks
    FROM v FULL OUTER JOIN c ON v.client = c.client;

-- name: get-dashboard-countries
-- Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC R4). The Dashboard's Location panel,
-- get-dashboard-clients keyed by country instead of client -- every campaign, the same window
-- (31 days ending on the day of the latest event of those campaigns, per table) and the same RAW
-- row count (CLIENT-STATS-SPEC D6's charts rule; the Analytics Location table dedupes per
-- (subscriber, campaign) under individual tracking -- a recorded divergence). Unknown country
-- (NULL) is reported as the empty string. Any scope reads get-dashboard-countries-scoped.
WITH camps AS (
    SELECT c.id FROM campaigns c
),
vd AS (
    SELECT MAX(created_at)::DATE AS to_date FROM campaign_views WHERE campaign_id IN (SELECT id FROM camps)
),
v AS (
    SELECT COALESCE(country, '') AS country, COUNT(*) AS n FROM campaign_views
        WHERE campaign_id IN (SELECT id FROM camps)
        AND created_at >= (SELECT to_date FROM vd) - INTERVAL '30 DAY'
        AND created_at < (SELECT to_date FROM vd) + INTERVAL '1 day'
        GROUP BY 1
),
cd AS (
    SELECT MAX(created_at)::DATE AS to_date FROM link_clicks WHERE campaign_id IN (SELECT id FROM camps)
),
c AS (
    SELECT COALESCE(country, '') AS country, COUNT(*) AS n FROM link_clicks
        WHERE campaign_id IN (SELECT id FROM camps)
        AND created_at >= (SELECT to_date FROM cd) - INTERVAL '30 DAY'
        AND created_at < (SELECT to_date FROM cd) + INTERVAL '1 day'
        GROUP BY 1
)
SELECT COALESCE(v.country, c.country) AS country, COALESCE(v.n, 0) AS views, COALESCE(c.n, 0) AS clicks
    FROM v FULL OUTER JOIN c ON v.country = c.country;

-- name: get-dashboard-countries-scoped
-- Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC R4). get-dashboard-countries over a
-- scope, with $1/$2 exactly as get-dashboard-clients-scoped. Same window and counting.
WITH camps AS (
    SELECT c.id FROM campaigns c
    WHERE $2::BOOLEAN OR EXISTS (
        SELECT 1 FROM campaign_lists cl WHERE cl.campaign_id = c.id AND cl.list_id = ANY($1::INT[])
    )
),
vd AS (
    SELECT MAX(created_at)::DATE AS to_date FROM campaign_views WHERE campaign_id IN (SELECT id FROM camps)
),
v AS (
    SELECT COALESCE(country, '') AS country, COUNT(*) AS n FROM campaign_views
        WHERE campaign_id IN (SELECT id FROM camps)
        AND created_at >= (SELECT to_date FROM vd) - INTERVAL '30 DAY'
        AND created_at < (SELECT to_date FROM vd) + INTERVAL '1 day'
        GROUP BY 1
),
cd AS (
    SELECT MAX(created_at)::DATE AS to_date FROM link_clicks WHERE campaign_id IN (SELECT id FROM camps)
),
c AS (
    SELECT COALESCE(country, '') AS country, COUNT(*) AS n FROM link_clicks
        WHERE campaign_id IN (SELECT id FROM camps)
        AND created_at >= (SELECT to_date FROM cd) - INTERVAL '30 DAY'
        AND created_at < (SELECT to_date FROM cd) + INTERVAL '1 day'
        GROUP BY 1
)
SELECT COALESCE(v.country, c.country) AS country, COALESCE(v.n, 0) AS views, COALESCE(c.n, 0) AS clicks
    FROM v FULL OUTER JOIN c ON v.country = c.country;

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
