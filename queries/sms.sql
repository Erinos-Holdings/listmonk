-- name: get-sms-promo-rows
-- Fork (SMS Admin, integrations listmonk/SMS-LINKS-ADMIN-SPEC D9) -- per marked SMS code, the
-- orders (distinct order ids) and buyers (distinct canonical-email hashes) of the purchase feed
-- carrying it as a promotion value, by the feed's own comparison (kind promotion, value_lc equal to
-- the lower-cased code -- equality, never a pattern; a SKU never matches), and the first and last
-- order times (NULL with no orders). All time over the feed, no window. The fork aggregates, it
-- decides nothing.
SELECT c.id, c.code, c.brand,
    COUNT(DISTINCT p.order_id)::INT AS orders,
    COUNT(DISTINCT p.email_hash)::INT AS buyers,
    MIN(p.ordered_at) AS first_order_at,
    MAX(p.ordered_at) AS last_order_at,
    c.created_by, c.created_at
FROM sms_promo_codes c
LEFT JOIN promo_purchases p ON (p.kind = 'promotion' AND p.value_lc = LOWER(c.code))
GROUP BY c.id
ORDER BY MAX(p.ordered_at) DESC NULLS LAST, c.code;

-- name: get-sms-promo-feed
-- Fork (SMS Admin, integrations listmonk/SMS-LINKS-ADMIN-SPEC D9) -- the feed's extent and freshness
-- for the report's notice and banner. stale is promo_feed_stale(), the one home of the limit.
SELECT MIN(ordered_at) AS oldest_ordered_at, MAX(received_at) AS last_received_at,
    promo_feed_stale() AS stale
FROM promo_purchases;

-- name: get-sms-promo-known
-- Fork (SMS Admin, integrations listmonk/SMS-LINKS-ADMIN-SPEC D9) -- the add form's suggestion list.
-- The feed's promotion values grouped by value_lc, every group already marked (case-insensitively)
-- left out, each in its most frequent spelling with its distinct order count, most orders first,
-- at most 200.
WITH g AS (
    SELECT p.value_lc,
        MODE() WITHIN GROUP (ORDER BY p.value) AS value,
        COUNT(DISTINCT p.order_id)::INT AS orders
    FROM promo_purchases p
    WHERE p.kind = 'promotion'
        AND NOT EXISTS (SELECT 1 FROM sms_promo_codes c WHERE LOWER(c.code) = p.value_lc)
    GROUP BY p.value_lc
)
SELECT value, orders FROM g ORDER BY orders DESC, value LIMIT 200;

-- name: insert-sms-promo-code
-- Fork (SMS Admin, integrations listmonk/SMS-LINKS-ADMIN-SPEC D10) -- mark one promotion value. The
-- code arrives trimmed and validated; an empty brand is NULL (All brands). A case-insensitive
-- duplicate fails the unique LOWER(code) index and an unknown brand the brands foreign key -- the
-- handler maps each to its status.
INSERT INTO sms_promo_codes (code, brand, created_by) VALUES ($1, NULLIF($2, ''), $3)
RETURNING id, code, brand, created_by, created_at;

-- name: delete-sms-promo-code
-- Fork (SMS Admin, integrations listmonk/SMS-LINKS-ADMIN-SPEC D10) -- unmark. The feed is untouched.
DELETE FROM sms_promo_codes WHERE id = $1;
