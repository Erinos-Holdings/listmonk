-- Fork (erinos evergreen campaigns). Comment lines in this file must never contain a
-- colon unless they ARE a goyesql tag (see the queries parse test).

-- name: next-evergreen-subscribers
-- The ONE eligibility query. For running evergreen campaign $1, returns up to $2
-- subscribers who joined its target list after the campaign's started_at watermark
-- (subscriber_lists.confirmed_at, stamped on the transition into 'confirmed'), whose
-- send delay has elapsed, and who have not been sent this campaign (or any campaign
-- in its variant group) since that join. The batch is CLAIMED in campaign_sends in the
-- same statement (claimed_at), so a crash between fetch and send can never double-send.
-- The worker marks sent_at on the delivery attempt and deletes a claim it drops
-- unattempted; a claim with no attempt after one hour is treated as abandoned (process
-- died mid-batch) and the subscriber is eligible again -- fails toward one late send.
-- Two expressions are contractually isolated for later milestones -- the ANCHOR
-- (today the list join; step chaining will offer the parent campaign's sent_at) and
-- the EXCLUSION SET (self + variant group + every evergreen sharing a list, see below).
-- Change only those.
-- A NULL started_at or NULL confirmed_at compares NULL and is never eligible.
-- Fork (multi-language campaigns) -- LANGUAGE. camp.lang absent = everyone. ES/FR/DE/IT
-- match the subscriber's language exactly. EN is the fallback -- it matches every
-- lang (including none) NOT claimed by a sibling evergreen on the same list whose status
-- is running OR paused (a pause is temporary and the sibling's watermark still covers
-- the joiner on resume; only cancelling hands the language back to EN) AND whose own
-- watermark covers the joiner (started_at <= confirmed_at) -- a sibling started AFTER
-- the join can never welcome that joiner, so it must not claim them either, or nobody
-- does (any non-zero delay, or the pause-v1/start-v2 edit flow, opens that hole). A
-- sibling in this campaign's own variant group never claims. The subscriber's language
-- is read as COALESCE(NULLIF(LOWER(LEFT(lang, 2)), ''), 'en') so a CSV-imported 'FR' or
-- 'fr-CA' still counts as fr. The welcome language is whatever is known when the delay
-- expires -- no delay, unknown = English.
WITH camp AS (
    SELECT id, started_at, send_delay_secs, variant_group_id, attribs->>'lang' AS lang
    FROM campaigns
    WHERE id = $1 AND evergreen AND status = 'running'
),
excl AS (
    -- exclusion set -- self, the variant group, and EVERY evergreen (any status, any
    -- delay) sharing one of this campaign's lists. One welcome per joiner per list is
    -- the invariant across languages -- a lang-less joiner welcomed in English whose
    -- order then tags them fr must not be welcomed again by the FR evergreen. A
    -- cancelled or finished sibling's campaign_sends keep excluding (retire a welcome
    -- by cancelling, never deleting -- deletion cascades campaign_sends).
    SELECT c.id FROM campaigns c, camp
    WHERE c.id = camp.id
       OR (camp.variant_group_id IS NOT NULL AND c.variant_group_id = camp.variant_group_id)
       OR (c.evergreen AND c.id IN (
            SELECT cl2.campaign_id FROM campaign_lists cl2
            WHERE cl2.list_id IN (SELECT list_id FROM campaign_lists WHERE campaign_id = camp.id)))
),
siblings AS (
    -- language evergreens on the same list that are running or paused, with their
    -- watermark -- a joiner is claimed only if the sibling could actually welcome them
    SELECT c.attribs->>'lang' AS lang, c.started_at FROM campaigns c, camp
    WHERE c.evergreen AND c.id <> camp.id AND c.status IN ('running', 'paused')
      AND c.attribs->>'lang' IS NOT NULL AND c.started_at IS NOT NULL
      AND (camp.variant_group_id IS NULL OR c.variant_group_id IS DISTINCT FROM camp.variant_group_id)
      AND c.id IN (SELECT cl2.campaign_id FROM campaign_lists cl2
                   WHERE cl2.list_id IN (SELECT list_id FROM campaign_lists WHERE campaign_id = camp.id))
),
elig AS (
    SELECT DISTINCT ON (sl.subscriber_id)
        sl.subscriber_id,
        -- anchor
        sl.confirmed_at AS anchor
    FROM subscriber_lists sl
    JOIN campaign_lists cl ON cl.list_id = sl.list_id AND cl.campaign_id = $1
    JOIN subscribers s ON s.id = sl.subscriber_id AND s.status != 'blocklisted'
    CROSS JOIN camp
    WHERE sl.status = 'confirmed'
      AND sl.confirmed_at IS NOT NULL
      AND sl.confirmed_at > camp.started_at
      AND sl.confirmed_at + MAKE_INTERVAL(secs => camp.send_delay_secs) <= NOW()
      AND (camp.lang IS NULL
           OR (camp.lang <> 'en' AND COALESCE(NULLIF(LOWER(LEFT(s.attribs->>'lang', 2)), ''), 'en') = camp.lang)
           OR (camp.lang = 'en' AND NOT EXISTS (
                SELECT 1 FROM siblings
                WHERE siblings.lang = COALESCE(NULLIF(LOWER(LEFT(s.attribs->>'lang', 2)), ''), 'en')
                  AND siblings.started_at <= sl.confirmed_at)))
      AND sl.confirmed_at > COALESCE(
            (SELECT MAX(COALESCE(cs.sent_at, cs.claimed_at)) FROM campaign_sends cs
             WHERE cs.subscriber_id = sl.subscriber_id AND cs.campaign_id IN (SELECT id FROM excl)
               AND (cs.sent_at IS NOT NULL OR cs.claimed_at > NOW() - INTERVAL '1 hour')),
            '-infinity')
    ORDER BY sl.subscriber_id, sl.confirmed_at
),
batch AS (
    SELECT * FROM elig ORDER BY anchor LIMIT $2
),
ins AS (
    INSERT INTO campaign_sends (campaign_id, subscriber_id)
    SELECT $1, subscriber_id FROM batch
)
SELECT s.* FROM subscribers s JOIN batch ON batch.subscriber_id = s.id ORDER BY batch.anchor;

-- name: get-evergreen-collision
-- Another RUNNING evergreen on any of the same lists with the same delay that is not
-- in the same (non-null) variant group. Starting such a campaign is refused -- it
-- would send two welcomes per signup. Returns at most one row.
-- Fork (multi-language campaigns) -- two such evergreens do NOT collide when both carry
-- a lang ($5 is this campaign's) and the langs differ. An absent-lang (everyone)
-- evergreen collides with every language evergreen.
SELECT c.id, c.name FROM campaigns c
JOIN campaign_lists cl ON cl.campaign_id = c.id
WHERE c.evergreen AND c.status = 'running' AND c.id != $1
  AND c.send_delay_secs = $2
  AND cl.list_id = ANY($3::INT[])
  AND NOT (c.variant_group_id IS NOT NULL AND $4::UUID IS NOT NULL AND c.variant_group_id = $4::UUID)
  AND NOT (c.attribs->>'lang' IS NOT NULL AND $5::TEXT IS NOT NULL AND c.attribs->>'lang' <> $5::TEXT)
LIMIT 1;

-- name: mark-evergreen-sent
-- The worker attempted delivery (success or failure -- a failed attempt counts as sent,
-- the campaign's error threshold handles outages).
UPDATE campaign_sends SET sent_at = NOW()
WHERE campaign_id = $1 AND subscriber_id = $2 AND sent_at IS NULL;

-- name: release-evergreen-claim
-- The worker dropped the queued message unattempted (pipe stopped by pause/cancel).
DELETE FROM campaign_sends
WHERE campaign_id = $1 AND subscriber_id = $2 AND sent_at IS NULL;
