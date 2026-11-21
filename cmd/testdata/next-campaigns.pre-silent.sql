-- name: next-campaigns
-- Retreives campaigns that are running (or scheduled and the time's up) and need
-- to be processed. It updates the to_send count and max_subscriber_id of the campaign,
-- that is, the total number of subscribers to be processed across all lists of a campaign.
-- Thus, it has a sideaffect.
-- In addition, it finds the max_subscriber_id, the upper limit across all lists of
-- a campaign. This is used to fetch and slice subscribers for the campaign in next-campaign-subscribers.
WITH camps AS (
    -- Get all running campaigns and their template bodies (if the template's deleted, the default template body instead)
    -- Fork (template freeze) — a previously-frozen campaign renders its snapshot; a
    -- first-time claim reads the live template here and the u CTE below snapshots
    -- exactly this value (same statement snapshot), so what is sent = what is frozen.
    SELECT campaigns.*, COALESCE(campaigns.frozen_template_body, templates.body, (SELECT body FROM templates WHERE is_default = true LIMIT 1), '') AS template_body
    FROM campaigns
    LEFT JOIN templates ON (templates.id = campaigns.template_id)
    WHERE (status='running' OR (status='scheduled' AND NOW() >= campaigns.send_at))
    AND NOT(campaigns.id = ANY($1::INT[]))
),
campLists AS (
    -- Get the list_ids and their optin statuses for the campaigns found in the previous step.
    SELECT lists.id AS list_id, campaign_id, optin FROM lists
    INNER JOIN campaign_lists ON (campaign_lists.list_id = lists.id)
    WHERE campaign_lists.campaign_id = ANY(SELECT id FROM camps)
),
campMedia AS (
    -- Get the list_ids and their optin statuses for the campaigns found in the previous step.
    SELECT campaign_id, ARRAY_AGG(campaign_media.media_id)::INT[] AS media_id FROM campaign_media
    WHERE campaign_id = ANY(SELECT id FROM camps) AND media_id IS NOT NULL
    GROUP BY campaign_id
),
-- Fork (hold scope, integrations HOLD-SCOPE-SPEC D1) -- the brand-hold set, computed ONCE per claimed
-- campaign (as jsends is for brand_journey_skips) so aud anti-joins to it; called inside aud with
-- camps.id it would be a correlated SubPlan run once per audience row. MATERIALIZED, because a CTE
-- read once is otherwise inlined and planned as a parameterised inner side probed per audience row.
held AS MATERIALIZED (
    SELECT camps.id AS campaign_id, bh.list_id, bh.subscriber_id
    FROM camps
    CROSS JOIN LATERAL brand_hold_rows(camps.id) bh
    WHERE NOT camps.evergreen
),
-- Fork (smart sending, integrations SMART-SENDING-SPEC D2) -- the audience join, lifted out of counts
-- unchanged: one row per (claimed non-evergreen campaign, subscriber it would send to).
aud AS (
    SELECT DISTINCT camps.id AS campaign_id, sl.subscriber_id
    FROM camps
    JOIN campLists cl ON cl.campaign_id = camps.id
    JOIN subscriber_lists sl ON sl.list_id = cl.list_id
        AND (
            CASE
                WHEN camps.type = 'optin' THEN sl.status = 'unconfirmed' AND cl.optin = 'double'
                WHEN cl.optin = 'double' THEN sl.status = 'confirmed'
                ELSE sl.status != 'unsubscribed'
            END
        )
    JOIN subscribers s ON (s.id = sl.subscriber_id AND s.status != 'blocklisted')
        -- Fork (multi-language campaigns) -- a campaign carrying attribs.lang counts only
        -- subscribers of that language. Absent = everyone. A subscriber with no lang is
        -- English (COALESCE), so the EN campaign reaches the unknowns too. The subscriber
        -- value is normalised (LOWER, primary subtag) so an imported 'FR' or 'fr-CA' counts.
        AND (camps.attribs->>'lang' IS NULL OR COALESCE(NULLIF(LOWER(LEFT(s.attribs->>'lang', 2)), ''), 'en') = camps.attribs->>'lang')
    -- Fork (evergreen) -- an evergreen campaign is discovered by this query (it is
    -- returned below so the manager pipes it) but never CLAIMED. Its to_send,
    -- max_subscriber_id, status and freeze are not this query's business; its
    -- started_at is stamped by update-campaign-status at the API start instead.
    WHERE NOT camps.evergreen
        -- Fork (hold scope, integrations HOLD-SCOPE-SPEC D1) -- a list row of a subscriber held on
        -- any non-transactional list of that list's brand does not qualify (brand_hold_rows, the
        -- one definition; dark unless app.brand_hold_scope is true). Not a Smart Sending skip -- no
        -- campaign_smart_skips row, simply not in the audience. The set is the held CTE above.
        AND NOT EXISTS (SELECT 1 FROM held bh
            WHERE bh.campaign_id = camps.id AND bh.list_id = sl.list_id AND bh.subscriber_id = sl.subscriber_id)
),
-- Fork (smart sending, D1/D2) -- the skip set is decided HERE, once per claim, and recorded. skips =
-- the audience rows brand_journey_skips also returns for that campaign (the one definition of both
-- halves -- integrations SMART-SENDING-LOOKAHEAD-SPEC D4), above the checkpoint -- a re-claim after
-- a pause must never record a skip for a subscriber already sent (id at or below
-- last_subscriber_id), which would drive to_send below sent. A look-back row carries the journey
-- pair, a look-ahead row the due pair (D5); both are written as they come.
jsends AS (
    SELECT camps.id AS campaign_id, j.subscriber_id, j.journey_campaign_id, j.journey_sent_at,
        j.due_list_id, j.due_at
    FROM camps
    CROSS JOIN LATERAL brand_journey_skips(camps.id) j
    WHERE NOT camps.evergreen
),
skips AS (
    SELECT a.campaign_id, a.subscriber_id, js.journey_campaign_id, js.journey_sent_at,
        js.due_list_id, js.due_at
    FROM aud a
    JOIN camps ON (camps.id = a.campaign_id)
    JOIN jsends js ON (js.campaign_id = a.campaign_id AND js.subscriber_id = a.subscriber_id)
    WHERE a.subscriber_id > camps.last_subscriber_id
),
insSkips AS (
    INSERT INTO campaign_smart_skips (campaign_id, subscriber_id, journey_campaign_id, journey_sent_at, due_list_id, due_at)
    SELECT campaign_id, subscriber_id, journey_campaign_id, journey_sent_at, due_list_id, due_at FROM skips
    ON CONFLICT DO NOTHING
),
-- to_send = the audience minus every skip: this claim's (the skips CTE) and an earlier claim's (the
-- table, a re-claim after a pause). A data-modifying CTE's rows are INVISIBLE to its siblings in the
-- same statement (one snapshot), so counts reads the skips CTE, never the INSERT's effect.
counts AS (
    SELECT a.campaign_id,
        COUNT(*) FILTER (WHERE NOT EXISTS (
            SELECT 1 FROM skips k WHERE k.campaign_id = a.campaign_id AND k.subscriber_id = a.subscriber_id
        ) AND NOT EXISTS (
            SELECT 1 FROM campaign_smart_skips t WHERE t.campaign_id = a.campaign_id AND t.subscriber_id = a.subscriber_id
        )) AS to_send,
        COALESCE(MAX(a.subscriber_id), 0) AS max_subscriber_id
    FROM aud a
    GROUP BY a.campaign_id
),
updateCounts AS (
    WITH uc (campaign_id, sent_count) AS (SELECT * FROM unnest($1::INT[], $2::INT[]))
    UPDATE campaigns
    SET sent = sent + uc.sent_count
    FROM uc WHERE campaigns.id = uc.campaign_id
    RETURNING campaigns.id, campaigns.evergreen, uc.sent_count
),
sendHours AS (
    -- Fork (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC D2) -- the tick flush's
    -- delta lands in the current UTC hour, in this same statement. Broadcasts only -- an evergreen's
    -- sends are counted by mark-evergreen-sent, one per delivery, so they never count twice.
    INSERT INTO campaign_send_hours (campaign_id, hour, n)
    SELECT id, DATE_TRUNC('hour', NOW(), 'UTC'), sent_count FROM updateCounts
    WHERE sent_count > 0 AND NOT evergreen
    ON CONFLICT (campaign_id, hour) DO UPDATE SET n = campaign_send_hours.n + EXCLUDED.n
),
u AS (
    -- For each campaign, update the to_send count and set the max_subscriber_id.
    UPDATE campaigns AS ca
    SET to_send = co.to_send,
        status = (CASE WHEN status != 'running' THEN 'running' ELSE status END),
        max_subscriber_id = co.max_subscriber_id,
        started_at=(CASE WHEN ca.started_at IS NULL THEN NOW() ELSE ca.started_at END),
        -- Fork (template freeze) — first claim snapshots the resolved template body
        -- (camps.template_body — the exact value this claim renders). IS NULL guard —
        -- pause/resume must never overwrite the original snapshot.
        frozen_template_body = COALESCE(ca.frozen_template_body, (SELECT camps.template_body FROM camps WHERE camps.id = ca.id))
    FROM (SELECT * FROM counts) co
    WHERE ca.id = co.campaign_id
)
SELECT camps.*, campMedia.media_id FROM camps LEFT JOIN campMedia ON (campMedia.campaign_id = camps.id);
