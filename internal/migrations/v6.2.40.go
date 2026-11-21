package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_40 is a FORK migration (60-day opens, integrations listmonk/SIXTY-DAY-OPENS-SPEC D1/D3), not
// an upstream release. In one transaction it:
//
//  1. adds campaign_smart_skips.reason TEXT NULL (no CHECK -- a raise inside next-campaigns would
//     stall every claim): the campaign's skip record now carries lookback, lookahead or silent;
//  2. backfills every existing row whose reason is NULL -- lookahead where due_list_id is set,
//     else lookback (the IS NULL guard makes a re-run change no row);
//  3. creates open_observable_provider(email) -- the ONE set of open-observable providers (gmail,
//     yahoo) -- and brand_silent_skips(campaign, preview) -- the ONE definition of a silent member,
//     read by next-campaigns (the claim, only while attribs.silent_skip is the JSONB true),
//     get-campaign-lang-audience (silent_skipped) and get-campaign-silent-audience (the preview).
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary never calls either
// function and its claim names no reason, so the rows it records carry a NULL reason, which every
// reader reads by the due_list_id column test. No DDL reversal is needed.
//
// The version key sits after the fork's v6.2.39 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.40. Idempotent by construction.
func V6_2_40(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(silentSkipsDDL); err != nil {
		return err
	}
	if _, err := tx.Exec(silentSkipsReasonBackfill); err != nil {
		return err
	}

	return tx.Commit()
}

// silentSkipsReasonBackfill -- the existing Smart Sending rows, by the one column test (D3).
const silentSkipsReasonBackfill = `UPDATE campaign_smart_skips SET reason = (CASE WHEN due_list_id IS NOT NULL THEN 'lookahead' ELSE 'lookback' END) WHERE reason IS NULL;`

// silentSkipsDDL is mirrored in schema.sql (the last block, comments aside). Keep the two identical
// (v6_2_40_db_test.go asserts it verbatim).
const silentSkipsDDL = `
ALTER TABLE campaign_smart_skips ADD COLUMN IF NOT EXISTS reason TEXT NULL;
CREATE OR REPLACE FUNCTION open_observable_provider(p_email TEXT) RETURNS TEXT AS $$
    SELECT CASE
        WHEN p_email ~* '@(gmail|googlemail)\.com$' THEN 'gmail'
        WHEN p_email ~* '@(yahoo|aol|ymail|rocketmail)\.' THEN 'yahoo'
        ELSE NULL END;
$$ LANGUAGE sql IMMUTABLE PARALLEL SAFE;
CREATE OR REPLACE FUNCTION brand_silent_skips(camp_id INT, p_preview BOOLEAN)
RETURNS TABLE (subscriber_id INT, provider TEXT) AS $$
    WITH cfg AS (
        SELECT NOW() - INTERVAL '60 days' AS since, 3 AS floor
    ),
    camp AS MATERIALIZED (
        SELECT c.id, (
            SELECT MIN(list_brand_tag(l.tags)) FROM campaign_lists cl
            JOIN lists l ON (l.id = cl.list_id)
            WHERE cl.campaign_id = c.id
        ) AS brand
        FROM campaigns c
        WHERE c.id = camp_id AND c.type = 'regular' AND NOT c.evergreen
            AND (p_preview IS TRUE OR (c.attribs->'silent_skip') = 'true'::JSONB)
    ),
    mem AS MATERIALIZED (
        SELECT DISTINCT sl.subscriber_id, open_observable_provider(s.email) AS provider,
            COALESCE(NULLIF(LOWER(LEFT(s.attribs->>'lang', 2)), ''), 'en') AS lang
        FROM campaign_lists cl
        JOIN subscriber_lists sl ON (sl.list_id = cl.list_id AND sl.status <> 'unsubscribed')
        JOIN subscribers s ON (s.id = sl.subscriber_id AND s.status <> 'blocklisted')
        WHERE EXISTS (SELECT 1 FROM camp WHERE camp.brand IS NOT NULL)
            AND cl.campaign_id = camp_id
            AND open_observable_provider(s.email) IS NOT NULL
    ),
    blists AS (
        SELECT l.id, l.optin FROM lists l
        WHERE l.transactional = FALSE AND list_brand_tag(l.tags) = (SELECT camp.brand FROM camp)
    ),
    bcasts AS MATERIALIZED (
        SELECT DISTINCT c.id, c.started_at, c.max_subscriber_id, c.attribs->>'lang' AS lang, bl.id AS list_id, bl.optin
        FROM campaigns c
        JOIN campaign_lists cl ON (cl.campaign_id = c.id)
        JOIN blists bl ON (bl.id = cl.list_id)
        WHERE EXISTS (SELECT 1 FROM mem)
            AND c.type = 'regular' AND c.status = 'finished' AND c.evergreen = FALSE
            AND c.started_at >= (SELECT cfg.since FROM cfg)
            AND EXISTS (SELECT 1 FROM campaign_views v WHERE v.campaign_id = c.id AND v.subscriber_id IS NOT NULL)
    ),
    fair AS MATERIALIZED (
        SELECT m.subscriber_id, m.provider
        FROM mem m
        JOIN subscriber_lists r ON (r.subscriber_id = m.subscriber_id)
        JOIN bcasts b ON (b.list_id = r.list_id)
        WHERE r.created_at <= b.started_at AND r.updated_at <= b.started_at
            AND COALESCE(r.confirmed_at, r.created_at) <= b.started_at
            AND ((b.optin = 'double' AND r.status = 'confirmed') OR (b.optin <> 'double' AND r.status <> 'unsubscribed'))
            AND m.subscriber_id <= b.max_subscriber_id
            AND (b.lang IS NULL OR m.lang = b.lang)
            AND NOT EXISTS (SELECT 1 FROM campaign_send_failures f
                WHERE f.campaign_id = b.id AND f.subscriber_id = m.subscriber_id)
            AND NOT EXISTS (SELECT 1 FROM campaign_smart_skips k
                WHERE k.campaign_id = b.id AND k.subscriber_id = m.subscriber_id)
        GROUP BY m.subscriber_id, m.provider
        HAVING COUNT(DISTINCT b.id) >= (SELECT cfg.floor FROM cfg)
    ),
    bcamps AS MATERIALIZED (
        SELECT DISTINCT cl.campaign_id AS id FROM campaign_lists cl
        JOIN lists l ON (l.id = cl.list_id)
        WHERE EXISTS (SELECT 1 FROM fair) AND list_brand_tag(l.tags) = (SELECT camp.brand FROM camp)
    )
    SELECT f.subscriber_id, f.provider
    FROM fair f
    WHERE NOT EXISTS (SELECT 1 FROM campaign_views v
            WHERE v.subscriber_id = f.subscriber_id AND v.created_at >= (SELECT cfg.since FROM cfg)
                AND v.campaign_id IN (SELECT bcamps.id FROM bcamps))
        AND NOT EXISTS (SELECT 1 FROM link_clicks k
            WHERE k.subscriber_id = f.subscriber_id AND k.created_at >= (SELECT cfg.since FROM cfg)
                AND k.campaign_id IN (SELECT bcamps.id FROM bcamps))
        AND NOT EXISTS (SELECT 1 FROM subscribers s
            WHERE s.id = f.subscriber_id
                AND (s.attribs->'open_seed'->(SELECT camp.brand FROM camp)->>'at') COLLATE "C"
                    >= TO_CHAR((SELECT cfg.since FROM cfg) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') COLLATE "C")
        AND NOT EXISTS (SELECT 1 FROM campaign_views v
            WHERE v.subscriber_id = f.subscriber_id
                AND v.client = ANY(ARRAY['outlook-windows', 'outlook-mac', 'outlook-mobile', 'thunderbird']));
$$ LANGUAGE sql STABLE;
`
