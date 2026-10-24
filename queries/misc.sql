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
-- raw: true
-- Fork (brand analytics, BRAND-ANALYTICS-SPEC D7; dashboard sends, integrations
-- DASHBOARD-SENDS-SPEC D2-D5). The ONE Dashboard charts query, live for every reader --
-- mat_dashboard_charts is no longer read. $1/$2/$3 exactly as get-dashboard-counts-scoped ($1 = the
-- scope's list ids, $2 = campaigns get_all, $3 = unscoped). Both series cover the 30 UTC days
-- ending yesterday, every day bucketed in UTC (never the session time zone).
-- clicks = link_clicks of the campaigns C per day, zero-filled. The mode is interpolated at boot
-- (cmd/init.go, beside statsSel) as TRUE when individual tracking is ON -- each (campaign,
-- subscriber) pair counts once, on the UTC day of its FIRST click on that campaign (a pair clicked
-- before the window is not counted, NULL subscribers never) -- and FALSE when OFF (raw rows). The
-- constant prunes the other branch at plan time.
-- sends = the per-day Send of the LATEST system_health ses document (written by the integrations
-- BrandHealth Lambda), summed over its brands rows -- every row when unscoped (unattributed
-- included), else the rows whose brand is the brand tag of a list in $1 (unattributed never).
-- Only the document's days inside the window, no zero fill (a stale document ends a day early).
WITH camps AS (
    SELECT c.id FROM campaigns c
    WHERE $2::BOOLEAN OR EXISTS (
        SELECT 1 FROM campaign_lists cl WHERE cl.campaign_id = c.id AND cl.list_id = ANY($1::INT[])
    )
),
win AS (
    SELECT (NOW() AT TIME ZONE 'UTC')::DATE - 30 AS d0, (NOW() AT TIME ZONE 'UTC')::DATE - 1 AS d1
),
bounds AS (
    SELECT d0, d1, (d0::TIMESTAMP AT TIME ZONE 'UTC') AS t0, ((d1 + 1)::TIMESTAMP AT TIME ZONE 'UTC') AS t1 FROM win
),
days AS (
    SELECT GENERATE_SERIES(d0, d1, INTERVAL '1 day')::DATE AS d FROM win
),
raw_clicks AS (
    SELECT (lc.created_at AT TIME ZONE 'UTC')::DATE AS d FROM link_clicks lc
    WHERE NOT %[1]s
        AND lc.campaign_id IN (SELECT id FROM camps)
        AND lc.created_at >= (SELECT t0 FROM bounds) AND lc.created_at < (SELECT t1 FROM bounds)
),
pair_clicks AS (
    SELECT (MIN(lc.created_at) AT TIME ZONE 'UTC')::DATE AS d FROM link_clicks lc
    WHERE %[1]s
        AND lc.subscriber_id IS NOT NULL
        AND lc.campaign_id IN (SELECT id FROM camps)
        AND lc.created_at >= (SELECT t0 FROM bounds) AND lc.created_at < (SELECT t1 FROM bounds)
    GROUP BY lc.campaign_id, lc.subscriber_id
    HAVING NOT EXISTS (
        SELECT 1 FROM link_clicks p
        WHERE p.subscriber_id = lc.subscriber_id AND p.campaign_id = lc.campaign_id
            AND p.created_at < (SELECT t0 FROM bounds)
    )
),
click_days AS (
    SELECT d, COUNT(*) AS n FROM (
        SELECT d FROM raw_clicks UNION ALL SELECT d FROM pair_clicks
    ) x GROUP BY d
),
clicks AS (
    SELECT JSON_AGG(JSON_BUILD_OBJECT('date', days.d, 'count', COALESCE(cd.n, 0)) ORDER BY days.d)
    FROM days LEFT JOIN click_days cd ON cd.d = days.d
),
sesdoc AS (
    SELECT doc FROM system_health WHERE kind = 'ses' ORDER BY day DESC LIMIT 1
),
scope_brands AS (
    SELECT DISTINCT SUBSTR(t, 7) AS brand FROM lists l, UNNEST(l.tags) AS t
    WHERE l.id = ANY($1::INT[]) AND LEFT(t, 6) = 'brand:'
),
ses_rows AS (
    SELECT r FROM sesdoc,
        JSONB_ARRAY_ELEMENTS(CASE WHEN JSONB_TYPEOF(doc->'brands') = 'array' THEN doc->'brands' ELSE '[]'::JSONB END) r
    WHERE $3::BOOLEAN
        OR (r->>'brand' <> 'unattributed' AND r->>'brand' IN (SELECT brand FROM scope_brands))
),
send_days AS (
    SELECT (x->>'day')::DATE AS d, SUM((x->>'sends')::NUMERIC)::BIGINT AS n FROM ses_rows,
        JSONB_ARRAY_ELEMENTS(CASE WHEN JSONB_TYPEOF(r->'daily') = 'array' THEN r->'daily' ELSE '[]'::JSONB END) x
    WHERE x->>'day' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' AND JSONB_TYPEOF(x->'sends') = 'number'
    GROUP BY 1
),
sends AS (
    SELECT JSON_AGG(JSON_BUILD_OBJECT('date', sd.d, 'count', sd.n) ORDER BY sd.d)
    FROM send_days sd, win WHERE sd.d BETWEEN win.d0 AND win.d1
)
SELECT JSON_BUILD_OBJECT(
    'sends', COALESCE((SELECT * FROM sends), '[]'),
    'clicks', COALESCE((SELECT * FROM clicks), '[]'),
    'scoped', NOT $3::BOOLEAN
) AS data;

-- name: get-dashboard-clients
-- Fork (client stats, CLIENT-STATS-SPEC D6). Views and clicks per email-client token for the
-- main Dashboard's client panel over every campaign, live (no materialized view). Any scope (a
-- list-scoped user, the global brand's list_id) reads get-dashboard-clients-scoped instead
-- (GLOBAL-BRAND-SPEC D5, which retired the panel's own brand parameter). Per table, the window is
-- the charts' former rule, kept here (31 days ending on the day of the latest event of those
-- campaigns, by MAX(created_at) over the campaign_id index; the charts themselves moved to a UTC
-- window ending yesterday in DASHBOARD-SENDS-SPEC). Raw rows. Unknown client (NULL) is reported as
-- the empty string.
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
