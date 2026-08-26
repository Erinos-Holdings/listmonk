-- Fork (erinos evergreen campaigns). Comment lines in this file must never contain a
-- colon unless they ARE a goyesql tag (see the queries parse test).

-- name: next-evergreen-subscribers
-- The ONE eligibility query. For running evergreen campaign $1, returns up to $2
-- subscribers who joined its target list after the campaign's started_at watermark
-- (subscriber_lists.confirmed_at, stamped on the transition into 'confirmed'), whose
-- send delay has elapsed, and who have not been sent this campaign (or any campaign
-- in its variant group) since that join. The batch is recorded in campaign_sends in
-- the same statement, so a crash between fetch and send can never double-send.
-- Two expressions are contractually isolated for later milestones -- the ANCHOR
-- (today the list join; step chaining will offer the parent campaign's sent_at) and
-- the EXCLUSION SET (today self + variant group). Change only those.
-- A NULL started_at or NULL confirmed_at compares NULL and is never eligible.
WITH camp AS (
    SELECT id, started_at, send_delay_secs, variant_group_id
    FROM campaigns
    WHERE id = $1 AND evergreen AND status = 'running'
),
excl AS (
    -- exclusion set
    SELECT c.id FROM campaigns c, camp
    WHERE c.id = camp.id
       OR (camp.variant_group_id IS NOT NULL AND c.variant_group_id = camp.variant_group_id)
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
      AND sl.confirmed_at > COALESCE(
            (SELECT MAX(cs.sent_at) FROM campaign_sends cs
             WHERE cs.subscriber_id = sl.subscriber_id AND cs.campaign_id IN (SELECT id FROM excl)),
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
SELECT c.id, c.name FROM campaigns c
JOIN campaign_lists cl ON cl.campaign_id = c.id
WHERE c.evergreen AND c.status = 'running' AND c.id != $1
  AND c.send_delay_secs = $2
  AND cl.list_id = ANY($3::INT[])
  AND NOT (c.variant_group_id IS NOT NULL AND $4::UUID IS NOT NULL AND c.variant_group_id = $4::UUID)
LIMIT 1;
