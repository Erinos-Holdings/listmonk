package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_41 is a FORK migration (promo series, integrations listmonk/PROMO-SERIES-SPEC D2-D4), not an
// upstream release. Additive. In one transaction it creates:
//
//  1. the table promo_purchases (the purchase feed: one row per order, kind and value, the buyer as
//     a hash of the canonical email) and its (ordered_at) index, and the expression index
//     idx_subs_promo_email_hash on subscribers (promo_email_hash(email)) -- built inside the
//     transaction, so it holds a write lock on subscribers while it builds (seconds);
//  2. six functions -- promo_email_hash (the canonical-email hash), promo_applicable_tags (the ONE
//     applicability rule), promo_campaign_tags (that rule over a campaign's stored values),
//     promo_matching_orders (rule 1 and the 60-day window literal), promo_feed_stale (rule 4 and the
//     24-hour literal) and promo_purchasers (the ONE definition of a purchaser), read by
//     next-campaigns (the claim), get-campaign-lang-audience, get-campaign-silent-audience and
//     get-campaign-promo-audience.
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary never reads the table or
// calls the functions; a 'purchased' skip row it finds is honoured as a skip by every reader (the
// reason is free text since v6.2.40). No DDL reversal is needed.
//
// The version key sits after the fork's v6.2.40 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.41. Idempotent by construction.
func V6_2_41(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(promoSeriesDDL); err != nil {
		return err
	}
	// The expression index has NO planner statistics until subscribers is analyzed, and without them
	// the planner prices an index probe as hundreds of rows and reads every subscriber instead
	// (production, 2026-11-21: 4.5 s per lookup with a valid index; 0.13 s after ANALYZE --
	// integrations PROMO-SERIES-SPEC G6). So the migration analyzes the table it just indexed.
	// Not part of the DDL mirrored in schema.sql (a fresh install has no rows to analyze; autovacuum
	// keeps the statistics from then on).
	if _, err := tx.Exec(`ANALYZE subscribers`); err != nil {
		return err
	}

	return tx.Commit()
}

// promoSeriesDDL is mirrored in schema.sql (the last block, comments aside). Keep the two identical
// (v6_2_41_db_test.go asserts it verbatim).
//
// promo_purchasers: the matching orders first, then the subscribers holding those hashes (by
// idx_subs_promo_email_hash, one probe per matching hash), then their membership of the campaign's
// lists; no rows when the tags are empty or the feed is stale (fail open).
//
// idx_subs_promo_email_hash (integrations PROMO-SERIES-SPEC Stage 4 V1) depends on
// promo_email_hash's definition: CHANGING THAT FUNCTION REQUIRES `REINDEX INDEX
// idx_subs_promo_email_hash` IN THE SAME MIGRATION (a stale index answers the old hashes). So does a
// PostgreSQL major-version upgrade or any change of the database's collation / lc_ctype (Stage 4b
// W2: LOWER follows lc_ctype, so stored entries for non-ASCII addresses can go stale). Its
// build takes a write lock on subscribers for its duration (seconds), so this migration runs only
// inside the release gate's no-broadcast window.
const promoSeriesDDL = `
CREATE TABLE IF NOT EXISTS promo_purchases (
    order_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    value TEXT NOT NULL,
    value_lc TEXT NOT NULL,
    email_hash TEXT NOT NULL,
    ordered_at TIMESTAMP WITH TIME ZONE NOT NULL,
    received_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (order_id, kind, value)
);
CREATE INDEX IF NOT EXISTS idx_promo_purchases_ordered_at ON promo_purchases (ordered_at);
CREATE OR REPLACE FUNCTION promo_email_hash(email TEXT) RETURNS TEXT AS $$
    SELECT ENCODE(SHA256(CONVERT_TO(
        CASE WHEN x.p > 1 AND SUBSTR(x.e, x.p + 1) IN ('gmail.com', 'googlemail.com')
            THEN REPLACE(LEFT(x.e, x.p - 1), '.', '') || '@gmail.com'
            ELSE x.e END, 'UTF8')), 'hex')
    FROM (
        SELECT y.e, (CASE WHEN STRPOS(REVERSE(y.e), '@') > 0
            THEN LENGTH(y.e) - STRPOS(REVERSE(y.e), '@') + 1 ELSE 0 END) AS p
        FROM (SELECT LOWER(BTRIM(COALESCE(email, ''), ' ')) AS e) y
    ) x;
$$ LANGUAGE sql IMMUTABLE PARALLEL SAFE;
CREATE INDEX IF NOT EXISTS idx_subs_promo_email_hash ON subscribers (promo_email_hash(email));
CREATE OR REPLACE FUNCTION promo_applicable_tags(p_type TEXT, p_evergreen BOOLEAN, p_code JSONB, p_step JSONB, p_tags JSONB)
RETURNS TEXT[] AS $$
    SELECT CASE
        WHEN p_type = 'regular' AND p_evergreen IS FALSE
            AND JSONB_TYPEOF(p_code) = 'string' AND BTRIM(p_code #>> '{}', ' ') <> ''
            AND (p_step = '2'::JSONB OR p_step = '3'::JSONB)
        THEN COALESCE((
            SELECT ARRAY_AGG(DISTINCT LOWER(BTRIM(t.v #>> '{}', ' ')))
            FROM JSONB_ARRAY_ELEMENTS(CASE WHEN JSONB_TYPEOF(p_tags) = 'array' THEN p_tags ELSE '[]'::JSONB END) t(v)
            WHERE JSONB_TYPEOF(t.v) = 'string' AND BTRIM(t.v #>> '{}', ' ') <> ''
        ), ARRAY[]::TEXT[])
        ELSE ARRAY[]::TEXT[] END;
$$ LANGUAGE sql IMMUTABLE;
CREATE OR REPLACE FUNCTION promo_campaign_tags(camp_id INT) RETURNS TEXT[] AS $$
    SELECT COALESCE((
        SELECT promo_applicable_tags(c.type::TEXT, c.evergreen,
            c.attribs->'promo_code', c.attribs->'promo_step', c.attribs->'promo_tags')
        FROM campaigns c WHERE c.id = camp_id
    ), ARRAY[]::TEXT[]);
$$ LANGUAGE sql STABLE;
CREATE OR REPLACE FUNCTION promo_matching_orders(p_tags TEXT[]) RETURNS TABLE (order_id TEXT, email_hash TEXT) AS $$
    SELECT DISTINCT p.order_id, p.email_hash
    FROM promo_purchases p
    WHERE COALESCE(CARDINALITY(p_tags), 0) > 0
        AND p.ordered_at >= NOW() - INTERVAL '60 days'
        AND EXISTS (
            SELECT 1 FROM UNNEST(p_tags) t(tag)
            WHERE (p.kind = 'promotion' AND RIGHT(t.tag, 1) <> '*' AND p.value_lc = LOWER(t.tag))
                OR (p.kind = 'sku' AND RIGHT(t.tag, 1) = '*' AND LENGTH(t.tag) > 3
                    AND LEFT(p.value_lc, LENGTH(t.tag) - 1) = LOWER(LEFT(t.tag, LENGTH(t.tag) - 1)))
        );
$$ LANGUAGE sql STABLE;
CREATE OR REPLACE FUNCTION promo_feed_stale() RETURNS BOOLEAN AS $$
    SELECT NOT EXISTS (SELECT 1 FROM promo_purchases p WHERE p.received_at >= NOW() - INTERVAL '24 hours');
$$ LANGUAGE sql STABLE;
CREATE OR REPLACE FUNCTION promo_purchasers(p_camp_id INT, p_tags TEXT[]) RETURNS TABLE (subscriber_id INT) AS $$
    WITH m AS MATERIALIZED (
        SELECT DISTINCT o.email_hash FROM promo_matching_orders(p_tags) o
        WHERE COALESCE(CARDINALITY(p_tags), 0) > 0 AND NOT promo_feed_stale()
    ),
    s AS MATERIALIZED (
        SELECT x.id FROM m
        CROSS JOIN LATERAL (
            SELECT sb.id FROM subscribers sb WHERE promo_email_hash(sb.email) = m.email_hash
        ) x
    )
    SELECT DISTINCT sl.subscriber_id
    FROM s
    JOIN subscriber_lists sl ON (sl.subscriber_id = s.id)
    WHERE EXISTS (SELECT 1 FROM campaign_lists cl WHERE cl.campaign_id = p_camp_id AND cl.list_id = sl.list_id);
$$ LANGUAGE sql STABLE;
`
