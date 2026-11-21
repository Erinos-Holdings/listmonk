-- name: next-campaign-subscribers
-- Returns a batch of subscribers in a given campaign starting from the last checkpoint
-- (last_subscriber_id). Every fetch updates the checkpoint and the sent count, which means
-- every fetch returns a new batch of subscribers until all rows are exhausted.
--
-- In previous versions, get-running-campaign + this was a single query spread across multiple
-- CTEs, but despite numerous permutations and combinations, Postgres query planner simply would not use
-- the right indexes on subscriber_lists when the JOIN or ids were referenced dynamically from campLists
-- (be it a CTE or various kinds of joins). However, statically providing the list IDs to JOIN on ($5::INT[])
-- the query planner works as expected. The difference is staggering. ~15 seconds on a subscribers table with 15m
-- rows and a subscriber_lists table with 70 million rows when fetching subscribers for a campaign with a single list,
-- vs. a few million seconds using this current approach.
WITH campLists AS (
    SELECT lists.id AS list_id, optin FROM lists
    LEFT JOIN campaign_lists ON campaign_lists.list_id = lists.id
    WHERE campaign_lists.campaign_id = $1
),
subs AS (
    SELECT s.*
    FROM (
        SELECT DISTINCT s.id
        FROM subscriber_lists sl
        JOIN campLists ON sl.list_id = campLists.list_id
        JOIN subscribers s ON s.id = sl.subscriber_id
        WHERE
            sl.list_id = ANY($5::INT[])
            -- last_subscriber_id
            AND s.id > $3
             -- max_subscriber_id
            AND s.id <= $4
             -- Subscriber should not be blacklisted.
            AND s.status != 'blocklisted'
            -- Fork (multi-language campaigns) -- same predicate as the next-campaigns count.
            AND ($7::TEXT IS NULL OR COALESCE(NULLIF(LOWER(LEFT(s.attribs->>'lang', 2)), ''), 'en') = $7::TEXT)
            -- Fork (smart sending, integrations SMART-SENDING-SPEC D2) -- never a subscriber the claim
            -- recorded as a Smart Sending skip. The recorded set only, nothing re-evaluated per batch, so
            -- the claim's to_send and the batches agree by construction (hazard 56, sent == to_send).
            AND NOT EXISTS (SELECT 1 FROM campaign_smart_skips k WHERE k.campaign_id = $1 AND k.subscriber_id = s.id)
            -- Fork (hold scope, integrations HOLD-SCOPE-SPEC D1) -- the claim's brand-hold rule, the
            -- same function, per list row.
            AND NOT EXISTS (SELECT 1 FROM brand_hold_rows($1) bh WHERE bh.list_id = sl.list_id AND bh.subscriber_id = s.id)
            AND (
                -- If it's an optin campaign and the list is double-optin, only pick unconfirmed subscribers.
                ($2 = 'optin' AND sl.status = 'unconfirmed' AND campLists.optin = 'double')
                OR (
                    -- It is a regular campaign.
                    $2 != 'optin' AND (
                        -- It is a double optin list. Only pick confirmed subscribers.
                        (campLists.optin = 'double' AND sl.status = 'confirmed') OR

                        -- It is a single optin list. Pick all non-unsubscribed subscribers.
                        (campLists.optin != 'double' AND sl.status != 'unsubscribed')
                    )
                )
            )
        ORDER BY s.id LIMIT $6
    ) subIDs JOIN subscribers s ON (s.id = subIDs.id) ORDER BY s.id
),
u AS (
    UPDATE campaigns
    SET last_subscriber_id = (SELECT MAX(id) FROM subs), updated_at = NOW()
    WHERE (SELECT COUNT(id) FROM subs) > 0 AND id=$1
)
SELECT * FROM subs;
