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
),
-- Fork (brand matrix, integrations listmonk/BRAND-MATRIX-SPEC D8) -- H. Bounces, the hard-type
-- bounce rows on C (all-time, as every Overview number). An ADDED field -- eb keeps every bounce
-- row (hazard 96, one Bounces definition) and is no longer displayed on the Overview.
ebh AS (
    SELECT COUNT(*) AS num FROM bounces
    WHERE campaign_id IN (SELECT id FROM camps) AND type = 'hard'
),
-- Fork (campaign conversions, integrations CAMPAIGN-CONVERSIONS-SPEC D11) -- tracked and matched on
-- the Messages-sent panel, the stored campaign_conversions rows of C, lifetime (as Views and Clicks
-- are), orders and sales. meta = the latest conversions doc's asOf, floors and trackedStale verbatim
-- (NULL with no doc). Sums only, no rule.
convsum AS (
    SELECT measure, COUNT(*) AS orders, COALESCE(SUM(sales_usd), 0) AS sales FROM campaign_conversions
    WHERE campaign_id IN (SELECT id FROM camps) GROUP BY measure
),
convdoc AS (
    SELECT doc FROM system_health WHERE kind = 'conversions' ORDER BY day DESC LIMIT 1
)
SELECT JSON_BUILD_OBJECT(
    'conversions', JSON_BUILD_OBJECT(
        'tracked', JSON_BUILD_OBJECT(
            'orders', COALESCE((SELECT orders FROM convsum WHERE measure = 'tracked'), 0),
            'sales_usd', COALESCE((SELECT sales FROM convsum WHERE measure = 'tracked'), 0)),
        'matched', JSON_BUILD_OBJECT(
            'orders', COALESCE((SELECT orders FROM convsum WHERE measure = 'matched'), 0),
            'sales_usd', COALESCE((SELECT sales FROM convsum WHERE measure = 'matched'), 0)),
        'meta', (SELECT JSON_BUILD_OBJECT('as_of', doc->'asOf', 'tracked_floor', doc->'trackedFloor',
            'matched_floor', doc->'matchedFloor', 'tracked_stale', doc->'trackedStale') FROM convdoc)
    ),
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
        'complaints', (SELECT num FROM ebc),
        'hard_bounces', (SELECT num FROM ebh)
    ),
    'scoped', NOT $3::BOOLEAN
) AS data;

-- name: get-dashboard-charts-scoped
-- raw: true
-- Fork (brand analytics, BRAND-ANALYTICS-SPEC D7; dashboard sends, integrations
-- DASHBOARD-SENDS-SPEC D2-D5). The ONE Dashboard charts query, live for every reader --
-- mat_dashboard_charts is no longer read. $1/$2/$3 exactly as get-dashboard-counts-scoped ($1 = the
-- scope's list ids, $2 = campaigns get_all, $3 = unscoped). Revision 2 (R1) -- both series cover
-- the 60 Mountain (America/Denver) calendar days ending YESTERDAY there, every day bucketed in that
-- zone (DST-aware, never a fixed offset, never the session time zone). The zone is the ONE literal
-- in the zone CTE below and MUST equal SES_DAILY_ZONE in the integrations lib/ses-health.ts, which
-- buckets the Sends series (R5; no test spans the repos).
-- clicks = link_clicks of the campaigns C per day, zero-filled. The mode is interpolated at boot
-- (cmd/init.go, beside statsSel) as TRUE when individual tracking is ON -- each (campaign,
-- subscriber) pair counts once, on the Mountain day of its FIRST click on that campaign (a pair
-- clicked before the window is not counted, NULL subscribers never) -- and FALSE when OFF (raw
-- rows). The constant prunes the other branch at plan time.
-- sends = the per-day Send of the LATEST system_health ses document that carries a daily series
-- (written by the integrations BrandHealth Lambda; a failed SES read writes none), summed over its brands rows -- every row when unscoped (unattributed
-- included), else the rows whose brand is the brand tag of a list in $1 (unattributed never).
-- Only the document's days inside the window, no zero fill (a stale document ends a day early).
WITH camps AS (
    SELECT c.id FROM campaigns c
    WHERE $2::BOOLEAN OR EXISTS (
        SELECT 1 FROM campaign_lists cl WHERE cl.campaign_id = c.id AND cl.list_id = ANY($1::INT[])
    )
),
zone AS (
    SELECT 'America/Denver'::TEXT AS tz
),
win AS (
    SELECT tz, (NOW() AT TIME ZONE tz)::DATE - 60 AS d0, (NOW() AT TIME ZONE tz)::DATE - 1 AS d1 FROM zone
),
bounds AS (
    SELECT d0, d1, (d0::TIMESTAMP AT TIME ZONE tz) AS t0, ((d1 + 1)::TIMESTAMP AT TIME ZONE tz) AS t1 FROM win
),
days AS (
    SELECT GENERATE_SERIES(d0, d1, INTERVAL '1 day')::DATE AS d FROM win
),
raw_clicks AS (
    SELECT (lc.created_at AT TIME ZONE (SELECT tz FROM zone))::DATE AS d FROM link_clicks lc
    WHERE NOT %[1]s
        AND lc.campaign_id IN (SELECT id FROM camps)
        AND lc.created_at >= (SELECT t0 FROM bounds) AND lc.created_at < (SELECT t1 FROM bounds)
),
pair_clicks AS (
    SELECT (MIN(lc.created_at) AT TIME ZONE (SELECT tz FROM zone))::DATE AS d FROM link_clicks lc
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
    SELECT doc FROM system_health
    WHERE kind = 'ses' AND JSONB_PATH_EXISTS(doc, '$.brands[*].daily[*] ? (@.day.type() == "string")')
    ORDER BY day DESC LIMIT 1
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

-- name: get-dashboard-brand-matrix
-- raw: true
-- Fork (brand matrix, integrations listmonk/BRAND-MATRIX-SPEC D3/D5/D7). The Dashboard's Brand
-- Matrix pane, one row per brand (the lists' brand tag, list_brand_tag), live. $1 = the scope's
-- list ids, $2 = unscoped (blanket list access, no list_id -- every list), as
-- get-dashboard-counts-scoped's $1/$3. Only ACTIVE lists form a brand (the brand scope's effective
-- list set is loaded active-only, so a single brand's row equals its All-brands row); an untagged
-- list forms no row. Per brand
--   subscribers = the subscriptions of the brand's PUBLIC lists per subscription_segment (the one
--     segment definition), current state. total keeps the one definition (pending included).
--   C = the brand's campaigns, any campaign_lists row on the brand's lists. A click or bounce row
--     whose campaign_id is NULL (deleted campaign) is attributable to no brand and drops out.
--   views, clicks, hard_bounces, blocks, complaints = over C and the 60 America/Denver calendar
--     days ending yesterday there (get-dashboard-charts-scoped's window, DST-aware). The zone is the
--     ONE literal in the zone CTE below and MUST equal get-dashboard-charts-scoped's (a fork test
--     asserts it) and SES_DAILY_ZONE in the integrations lib/ses-health.ts (no test spans the repos).
--     views and clicks follow the charts statement's mode constant (interpolated at boot,
--     cmd/init.go) -- TRUE = individual tracking, each (campaign, subscriber) pair once, when its
--     FIRST view/click on that campaign is in the window, NULL subscribers never; FALSE = raw
--     campaign_views / link_clicks rows. views byte-follow the clicks CTEs on campaign_views
--     (hazard 96's one definition, windowed as clicks are); Apple MPP auto-opens inflate them
--     (the pane's footnote says so), so their stored threshold reads as a relative cross-brand
--     signal, never an absolute one.
--   blocks = the hard bounces of the BLOCK class. The predicate is a byte-mirror of the OUTPUT of
--     classPredicate("hard","block") in the integrations lib/bounce-rules.ts, whose BOUNCE_CLASSES
--     is the canonical class map (append-only, classes never renamed -- no test spans the repos;
--     that file names this site). Its output, verbatim
--     (COALESCE(b.meta->'bounce'->'bouncedRecipients'->0->>'status', '') LIKE '5.7.%') AND NOT (COALESCE(b.meta->'bounce'->>'bounceSubType', '') = 'Suppressed' OR COALESCE(b.meta->'bounce'->>'bounceSubType', '') = 'OnAccountSuppressionList')
--     In the statement below the LIKE pattern is written '5.7.%%' because this text goes through
--     fmt.Sprintf at boot -- the prepared SQL carries '5.7.%' exactly (a fork test asserts it).
--   sent = the brand row's daily sends summed over the same window, from the LATEST system_health
--     ses document that carries a daily series (the charts statement's sesdoc), NULL when that
--     document has no in-window day for the brand. unattributed is never a row.
--   health = the brand's own LATEST brand_health row (no default-row fallback) -- status and
--     inputs.spamRate.detail.value as spam_rate when it is a number; an empty object with no row.
--     GP-SPAM-VISIBILITY-SPEC D2 -- spam_status = inputs.spamRate.status verbatim (absent when the
--     doc lacks it), spam_flagged = TRUE when the stored compliance rows carry a NEEDS_WORK
--     USER_REPORTED_SPAM_RATE requirement, absent otherwise. Both are stored fields projected,
--     never a rule computed here. The fork holds no threshold or floor VALUE; a mechanical
--     comparison of a displayed rate against values stored on the row is rendering
--     (DASHBOARDS-POLISH-SPEC D1, which narrows the stores-and-renders invariant).
--   thresholds = DASHBOARDS-POLISH-SPEC D1/I2 -- the same latest brand_health doc's `thresholds`
--     object (the Matrix engagement warn values and volume floors), verbatim (doc -> 'thresholds');
--     the key is ABSENT when the doc lacks it (or there is no doc), so the row colors nothing. The
--     values live in the integrations lib/brand-health.ts MATRIX_THRESHOLDS (no test spans the
--     repos); the Dashboard's brandMatrix.mjs compares the displayed rate against them.
--   usage = BRAND-USAGE-SPEC D2/I1 -- the same latest brand_health doc's `usage` object, verbatim
--     (doc -> 'usage'); the key is ABSENT when the doc lacks it (or there is no doc). Every usage
--     rule and threshold lives in the integrations lib/brand-usage.ts, which names this site; the
--     fork computes none (no test spans the repos). The row object is built as JSONB so the key can
--     be omitted -- its consumers read it by key, never by order.
WITH lst AS (
    SELECT id, type, optin, list_brand_tag(tags) AS brand FROM lists
    WHERE ($2::BOOLEAN OR id = ANY($1::INT[])) AND status = 'active'
        AND list_brand_tag(tags) IS NOT NULL AND list_brand_tag(tags) <> 'unattributed'
),
brands AS (
    SELECT DISTINCT brand FROM lst
),
subs AS (
    SELECT l.brand,
        COUNT(*) FILTER (WHERE g.seg = 'active') AS active,
        COUNT(*) FILTER (WHERE g.seg = 'held') AS held,
        COUNT(*) FILTER (WHERE g.seg = 'unsubscribed') AS unsubscribed,
        COUNT(*) FILTER (WHERE g.seg = 'blocked') AS blocked,
        COUNT(*) AS total
    FROM lst l
    JOIN subscriber_lists sl ON (sl.list_id = l.id)
    JOIN subscribers s ON (s.id = sl.subscriber_id)
    CROSS JOIN LATERAL (SELECT subscription_segment(sl.status, sl.meta, s.status, l.optin) AS seg) g
    WHERE l.type = 'public'
    GROUP BY l.brand
),
bcamps AS (
    SELECT DISTINCT l.brand, cl.campaign_id AS id FROM lst l JOIN campaign_lists cl ON (cl.list_id = l.id)
),
zone AS (
    SELECT 'America/Denver'::TEXT AS tz
),
win AS (
    SELECT tz, (NOW() AT TIME ZONE tz)::DATE - 60 AS d0, (NOW() AT TIME ZONE tz)::DATE - 1 AS d1 FROM zone
),
bounds AS (
    SELECT d0, d1, (d0::TIMESTAMP AT TIME ZONE tz) AS t0, ((d1 + 1)::TIMESTAMP AT TIME ZONE tz) AS t1 FROM win
),
raw_views AS (
    SELECT bc.brand, COUNT(*) AS n FROM campaign_views cv
    JOIN bcamps bc ON (bc.id = cv.campaign_id)
    WHERE NOT %[1]s
        AND cv.created_at >= (SELECT t0 FROM bounds) AND cv.created_at < (SELECT t1 FROM bounds)
    GROUP BY bc.brand
),
first_view_pairs AS (
    SELECT cv.campaign_id, cv.subscriber_id FROM campaign_views cv
    WHERE %[1]s
        AND cv.subscriber_id IS NOT NULL
        AND cv.campaign_id IN (SELECT id FROM bcamps)
        AND cv.created_at >= (SELECT t0 FROM bounds) AND cv.created_at < (SELECT t1 FROM bounds)
    GROUP BY cv.campaign_id, cv.subscriber_id
    HAVING NOT EXISTS (
        SELECT 1 FROM campaign_views p
        WHERE p.subscriber_id = cv.subscriber_id AND p.campaign_id = cv.campaign_id
            AND p.created_at < (SELECT t0 FROM bounds)
    )
),
pair_views AS (
    SELECT bc.brand, COUNT(*) AS n FROM first_view_pairs fv JOIN bcamps bc ON (bc.id = fv.campaign_id) GROUP BY bc.brand
),
raw_clicks AS (
    SELECT bc.brand, COUNT(*) AS n FROM link_clicks lc
    JOIN bcamps bc ON (bc.id = lc.campaign_id)
    WHERE NOT %[1]s
        AND lc.created_at >= (SELECT t0 FROM bounds) AND lc.created_at < (SELECT t1 FROM bounds)
    GROUP BY bc.brand
),
first_pairs AS (
    SELECT lc.campaign_id, lc.subscriber_id FROM link_clicks lc
    WHERE %[1]s
        AND lc.subscriber_id IS NOT NULL
        AND lc.campaign_id IN (SELECT id FROM bcamps)
        AND lc.created_at >= (SELECT t0 FROM bounds) AND lc.created_at < (SELECT t1 FROM bounds)
    GROUP BY lc.campaign_id, lc.subscriber_id
    HAVING NOT EXISTS (
        SELECT 1 FROM link_clicks p
        WHERE p.subscriber_id = lc.subscriber_id AND p.campaign_id = lc.campaign_id
            AND p.created_at < (SELECT t0 FROM bounds)
    )
),
pair_clicks AS (
    SELECT bc.brand, COUNT(*) AS n FROM first_pairs fp JOIN bcamps bc ON (bc.id = fp.campaign_id) GROUP BY bc.brand
),
bnc AS (
    SELECT bc.brand,
        COUNT(*) FILTER (WHERE b.type = 'hard') AS hard_bounces,
        COUNT(*) FILTER (WHERE b.type = 'hard'
            AND (COALESCE(b.meta->'bounce'->'bouncedRecipients'->0->>'status', '') LIKE '5.7.%%') AND NOT (COALESCE(b.meta->'bounce'->>'bounceSubType', '') = 'Suppressed' OR COALESCE(b.meta->'bounce'->>'bounceSubType', '') = 'OnAccountSuppressionList')
        ) AS blocks,
        COUNT(*) FILTER (WHERE b.type = 'complaint') AS complaints
    FROM bounces b
    JOIN bcamps bc ON (bc.id = b.campaign_id)
    WHERE b.created_at >= (SELECT t0 FROM bounds) AND b.created_at < (SELECT t1 FROM bounds)
    GROUP BY bc.brand
),
sesdoc AS (
    SELECT doc FROM system_health
    WHERE kind = 'ses' AND JSONB_PATH_EXISTS(doc, '$.brands[*].daily[*] ? (@.day.type() == "string")')
    ORDER BY day DESC LIMIT 1
),
ses_sent AS (
    SELECT r->>'brand' AS brand, SUM((x->>'sends')::NUMERIC)::BIGINT AS n FROM sesdoc,
        JSONB_ARRAY_ELEMENTS(CASE WHEN JSONB_TYPEOF(doc->'brands') = 'array' THEN doc->'brands' ELSE '[]'::JSONB END) r,
        JSONB_ARRAY_ELEMENTS(CASE WHEN JSONB_TYPEOF(r->'daily') = 'array' THEN r->'daily' ELSE '[]'::JSONB END) x,
        win
    WHERE r->>'brand' <> 'unattributed'
        AND x->>'day' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' AND JSONB_TYPEOF(x->'sends') = 'number'
        AND (x->>'day')::DATE BETWEEN win.d0 AND win.d1
    GROUP BY 1
),
bh AS (
    SELECT DISTINCT ON (brand) brand, status, doc FROM brand_health ORDER BY brand, day DESC
),
-- Fork (campaign conversions, integrations CAMPAIGN-CONVERSIONS-SPEC D11/D12, I12) -- tracked and
-- matched = the stored campaign_conversions rows grouped by their STORED brand (the brand the writer
-- computed the sales for -- Stage 4 F7, so a list re-tagged between runs can never put one row in two
-- brand rows) whose purchased_at falls in the SAME window (bounds, from the one zone CTE above --
-- never a second literal), as orders and sales. Counts always (the pct/num toggle never applies).
-- Each row carries one brand, so a brand row is the sum of its campaigns' rows. conversions =
-- the latest conversions doc's asOf, floors and trackedStale verbatim (NULL with no doc).
conv AS (
    SELECT cc.brand, cc.measure, COUNT(*) AS orders, COALESCE(SUM(cc.sales_usd), 0) AS sales
    FROM campaign_conversions cc
    WHERE cc.purchased_at >= (SELECT t0 FROM bounds) AND cc.purchased_at < (SELECT t1 FROM bounds)
    GROUP BY 1, 2
),
convdoc AS (
    SELECT doc FROM system_health WHERE kind = 'conversions' ORDER BY day DESC LIMIT 1
)
SELECT JSON_BUILD_OBJECT(
    'conversions', (SELECT JSON_BUILD_OBJECT('as_of', doc->'asOf', 'tracked_floor', doc->'trackedFloor',
        'matched_floor', doc->'matchedFloor', 'tracked_stale', doc->'trackedStale') FROM convdoc),
    'rows', COALESCE((
        SELECT JSON_AGG(JSONB_BUILD_OBJECT(
            'brand', b.brand,
            'subscribers', JSON_BUILD_OBJECT(
                'total', COALESCE(s.total, 0),
                'active', COALESCE(s.active, 0),
                'held', COALESCE(s.held, 0),
                'unsubscribed', COALESCE(s.unsubscribed, 0),
                'blocked', COALESCE(s.blocked, 0)
            ),
            'sent', ss.n,
            'views', COALESCE(rv.n, 0) + COALESCE(pv.n, 0),
            'clicks', COALESCE(rc.n, 0) + COALESCE(pc.n, 0),
            'hard_bounces', COALESCE(bn.hard_bounces, 0),
            'blocks', COALESCE(bn.blocks, 0),
            'complaints', COALESCE(bn.complaints, 0),
            'tracked', JSONB_BUILD_OBJECT('orders', COALESCE(ct.orders, 0), 'sales_usd', COALESCE(ct.sales, 0)),
            'matched', JSONB_BUILD_OBJECT('orders', COALESCE(cm.orders, 0), 'sales_usd', COALESCE(cm.sales, 0)),
            'health', JSONB_STRIP_NULLS(JSONB_BUILD_OBJECT(
                'status', bh.status,
                'spam_rate', CASE WHEN JSONB_TYPEOF(bh.doc->'inputs'->'spamRate'->'detail'->'value') = 'number'
                    THEN bh.doc->'inputs'->'spamRate'->'detail'->'value' END,
                'spam_status', bh.doc->'inputs'->'spamRate'->>'status',
                'spam_flagged', CASE WHEN JSONB_PATH_EXISTS(bh.doc, '$.inputs.config.detail.rows[*] ? (@.requirement == "USER_REPORTED_SPAM_RATE" && @.status == "NEEDS_WORK")')
                    THEN TRUE END
            ))
        ) || CASE WHEN bh.doc ? 'usage' THEN JSONB_BUILD_OBJECT('usage', bh.doc->'usage') ELSE '{}'::JSONB END
          || CASE WHEN bh.doc ? 'thresholds' THEN JSONB_BUILD_OBJECT('thresholds', bh.doc->'thresholds') ELSE '{}'::JSONB END
        ORDER BY b.brand)
        FROM brands b
        LEFT JOIN subs s ON (s.brand = b.brand)
        LEFT JOIN raw_views rv ON (rv.brand = b.brand)
        LEFT JOIN pair_views pv ON (pv.brand = b.brand)
        LEFT JOIN raw_clicks rc ON (rc.brand = b.brand)
        LEFT JOIN pair_clicks pc ON (pc.brand = b.brand)
        LEFT JOIN bnc bn ON (bn.brand = b.brand)
        LEFT JOIN ses_sent ss ON (ss.brand = b.brand)
        LEFT JOIN bh ON (bh.brand = b.brand)
        LEFT JOIN conv ct ON (ct.brand = b.brand AND ct.measure = 'tracked')
        LEFT JOIN conv cm ON (cm.brand = b.brand AND cm.measure = 'matched')
    ), '[]'),
    'scoped', NOT $2::BOOLEAN
) AS data;

-- name: get-dashboard-skipped-langs
-- raw: true
-- Fork (integrations listmonk/DASHBOARDS-POLISH3-SPEC D5/I5). The Brand Usage pane's Languages /
-- Skipped counts, live (no Lambda) -- per brand and per language L in fr, es, de, it, how many
-- ACTIVE subscribers in language L sit on a brand list whose window campaigns all skipped L.
-- $1/$2 exactly as get-dashboard-brand-matrix ($1 = the scope's list ids, $2 = unscoped); the
-- matrix endpoint merges the rows onto its own (internal/core/dashboard.go MergeSkippedLangs).
--   lists = the brand's ACTIVE lists (any type; an archived list contributes nothing), branded as
--     the matrix's rows are (list_brand_tag, unattributed never).
--   activity on a list = a REGULAR campaign on it that is either a FINISHED non-evergreen with
--     started_at in the window and sent above zero, or a currently RUNNING evergreen. Window = the
--     60 America/Denver calendar days ending yesterday there -- the charts and matrix window; the
--     zone is the ONE literal in the zone CTE below and MUST equal theirs (a fork test asserts it).
--   a campaign's language (attribs.lang, a closed set) decides its role for L -- exactly L, or NO
--     language (it targets everyone -- models.Campaign.Lang), COVERS L; a SPECIFIC language other
--     than L is an ANCHOR. A list is L-skipping when it has an anchor and no cover for L.
--   count = DISTINCT subscribers whose subscriber_lang (the one language partition) is L and whose
--     subscription_segment (the one segment definition) is active, on ANY of the brand's
--     L-skipping lists -- one subscriber counts once however many lists or campaigns missed them.
--     other and none languages are never counted.
--   lists = the L-skipping lists holding at least one counted subscriber, ascending (the pane's
--     link set) -- so a language with count 0 has no lists. Only (brand, lang) pairs with a
--     counted subscriber are returned; the endpoint fills the rest with count 0 and no lists.
-- No percent sign anywhere in this text (the house rule for raw statements).
WITH lst AS (
    SELECT id, optin, list_brand_tag(tags) AS brand FROM lists
    WHERE ($2::BOOLEAN OR id = ANY($1::INT[])) AND status = 'active'
        AND list_brand_tag(tags) IS NOT NULL AND list_brand_tag(tags) <> 'unattributed'
),
zone AS (
    SELECT 'America/Denver'::TEXT AS tz
),
win AS (
    SELECT tz, (NOW() AT TIME ZONE tz)::DATE - 60 AS d0, (NOW() AT TIME ZONE tz)::DATE - 1 AS d1 FROM zone
),
bounds AS (
    SELECT d0, d1, (d0::TIMESTAMP AT TIME ZONE tz) AS t0, ((d1 + 1)::TIMESTAMP AT TIME ZONE tz) AS t1 FROM win
),
acts AS (
    SELECT DISTINCT cl.list_id, NULLIF(c.attribs->>'lang', '') AS lang
    FROM campaigns c
    JOIN campaign_lists cl ON (cl.campaign_id = c.id)
    WHERE cl.list_id IN (SELECT id FROM lst)
        AND c.type = 'regular'
        AND (
            (c.evergreen = FALSE AND c.status = 'finished' AND c.sent > 0
                AND c.started_at >= (SELECT t0 FROM bounds) AND c.started_at < (SELECT t1 FROM bounds))
            OR (c.evergreen = TRUE AND c.status = 'running')
        )
),
langs AS (
    SELECT UNNEST(ARRAY['fr', 'es', 'de', 'it']::TEXT[]) AS lang
),
skipping AS (
    SELECT l.brand, l.id AS list_id, l.optin, g.lang FROM lst l CROSS JOIN langs g
    WHERE EXISTS (SELECT 1 FROM acts a WHERE a.list_id = l.id AND a.lang IS NOT NULL AND a.lang <> g.lang)
        AND NOT EXISTS (SELECT 1 FROM acts a WHERE a.list_id = l.id AND (a.lang IS NULL OR a.lang = g.lang))
),
hits AS (
    SELECT sk.brand, sk.lang, sk.list_id, sl.subscriber_id
    FROM skipping sk
    JOIN subscriber_lists sl ON (sl.list_id = sk.list_id)
    JOIN subscribers s ON (s.id = sl.subscriber_id)
    WHERE subscription_segment(sl.status, sl.meta, s.status, sk.optin) = 'active'
        AND subscriber_lang(s.attribs) = sk.lang
),
agg AS (
    SELECT brand, lang, COUNT(DISTINCT subscriber_id) AS n, ARRAY_AGG(DISTINCT list_id ORDER BY list_id) AS lists
    FROM hits GROUP BY brand, lang
)
SELECT COALESCE(JSON_AGG(JSON_BUILD_OBJECT('brand', brand, 'lang', lang, 'count', n, 'lists', lists) ORDER BY brand, lang), '[]') AS data
FROM agg;

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
