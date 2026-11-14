package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_37 is a FORK migration (join link, integrations listmonk/JOIN-LINK-SPEC D3/D4/D6), not
// an upstream release. It adds three functions, the read half of the {{ JoinURL }} route:
//   - brand_master_list_id(brand) -- the single public, non-transactional, active list carrying
//     the brand tag, NULL when there is none or more than one;
//   - join_refusal(subscriber id, brand) -- NULL when the subscriber may join, else the reason
//     (fails closed: an unknown subscriber, a hold whose reason is not never-engaged, a release
//     of such a hold to unsubscribed, blocklisted/disabled, a complaint, any unsubscribed row on
//     a marketing list of the brand);
//   - join_plan(subscriber uuid, campaign uuid) -- the one plan both verbs read; it says join only
//     for a recipient (a non-unsubscribed row on one of the campaign's lists) of a campaign whose
//     rendered send carries the {{ JoinURL }} action -- its body or altbody, or
//     COALESCE(frozen_template_body, the live template, the default template), the send's own
//     template choice (D4 row 6a, S4-2/S4b-1..3) -- so a rewritten unsubscribe URL joins nobody.
//
// No table, column or index. The only write is the binary's join-brand query, which calls
// join_plan; nothing else calls the functions.
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary never calls the
// functions, so they are inert to it.
//
// The version key sits after the fork's v6.2.36 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.37. Idempotent by construction (CREATE OR REPLACE).
func V6_2_37(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(joinLinkDDL); err != nil {
		return err
	}

	return tx.Commit()
}

// joinLinkDDL is mirrored in schema.sql (after the v6.2.36 lists columns, comments aside). Keep
// the two identical (v6_2_37_db_test.go compares the function bodies).
const joinLinkDDL = `
CREATE OR REPLACE FUNCTION brand_master_list_id(p_brand TEXT) RETURNS INT AS $$
    SELECT CASE WHEN COUNT(*) = 1 THEN MIN(l.id) ELSE NULL END FROM lists l
        WHERE p_brand IS NOT NULL AND list_brand_tag(l.tags) = p_brand
            AND l.type = 'public' AND l.transactional = FALSE AND l.status = 'active';
$$ LANGUAGE sql STABLE;

CREATE OR REPLACE FUNCTION join_refusal(p_sub_id INT, p_brand TEXT) RETURNS TEXT AS $$
    SELECT CASE
        WHEN s.id IS NULL THEN 'no_subscriber'
        WHEN s.status IN ('blocklisted', 'disabled') THEN s.status::TEXT
        WHEN EXISTS (SELECT 1 FROM bounces b WHERE b.subscriber_id = s.id AND b.type = 'complaint') THEN 'complaint'
        WHEN EXISTS (SELECT 1 FROM subscriber_lists sl WHERE sl.subscriber_id = s.id AND sl.status = 'unsubscribed'
            AND sl.meta ? 'hold' AND sl.meta->'hold'->>'reason' IS DISTINCT FROM 'never-engaged') THEN 'hold'
        WHEN EXISTS (SELECT 1 FROM subscriber_lists sl WHERE sl.subscriber_id = s.id AND sl.status = 'unsubscribed'
            AND sl.meta->'hold_released'->>'to' = 'unsubscribed'
            AND sl.meta->'hold_released'->'hold'->>'reason' IS DISTINCT FROM 'never-engaged') THEN 'hold_released'
        WHEN EXISTS (SELECT 1 FROM subscriber_lists sl WHERE sl.subscriber_id = s.id AND sl.status = 'unsubscribed'
            AND sl.list_id = ANY(brand_marketing_list_ids(p_brand))) THEN 'brand_optout'
        ELSE NULL END
    FROM (SELECT 1) AS one LEFT JOIN subscribers s ON (s.id = p_sub_id);
$$ LANGUAGE sql STABLE;

CREATE OR REPLACE FUNCTION join_plan(p_sub_uuid UUID, p_camp_uuid UUID)
RETURNS TABLE (outcome TEXT, brand TEXT, list_id INT, lang TEXT) AS $$
    WITH c AS (SELECT cp.id, NULLIF(cp.attribs->>'lang', '') AS lang,
            (COALESCE(cp.body, '') ~ '\{\{-?\s*JoinURL\y'
                OR COALESCE(cp.altbody, '') ~ '\{\{-?\s*JoinURL\y'
                OR COALESCE(cp.frozen_template_body, t.body,
                    (SELECT d.body FROM templates d WHERE d.is_default = true LIMIT 1), '') ~ '\{\{-?\s*JoinURL\y') AS tagged
        FROM campaigns cp LEFT JOIN templates t ON (t.id = cp.template_id) WHERE cp.uuid = p_camp_uuid),
    b AS (SELECT campaign_brand(p_camp_uuid) AS brand),
    m AS (SELECT brand_master_list_id((SELECT b.brand FROM b)) AS list_id),
    s AS (SELECT sb.id FROM subscribers sb WHERE sb.uuid = p_sub_uuid),
    r AS (SELECT sl.status FROM subscriber_lists sl
        WHERE sl.subscriber_id = (SELECT s.id FROM s) AND sl.list_id = (SELECT m.list_id FROM m))
    SELECT CASE
            WHEN NOT EXISTS (SELECT 1 FROM c) THEN 'no_campaign'
            WHEN (SELECT b.brand FROM b) IS NULL THEN 'no_brand'
            WHEN (SELECT m.list_id FROM m) IS NULL THEN 'no_master'
            WHEN join_refusal((SELECT s.id FROM s), (SELECT b.brand FROM b)) IS NOT NULL THEN 'refused'
            WHEN (SELECT r.status FROM r) = 'confirmed' THEN 'already'
            WHEN EXISTS (SELECT 1 FROM r) THEN 'cannot'
            WHEN NOT COALESCE((SELECT c.tagged FROM c), FALSE)
                OR NOT EXISTS (SELECT 1 FROM subscriber_lists rl JOIN campaign_lists cl ON (cl.list_id = rl.list_id)
                    WHERE rl.subscriber_id = (SELECT s.id FROM s) AND cl.campaign_id = (SELECT c.id FROM c)
                        AND rl.status <> 'unsubscribed') THEN 'cannot'
            ELSE 'join' END,
        (SELECT b.brand FROM b), (SELECT m.list_id FROM m), (SELECT c.lang FROM c);
$$ LANGUAGE sql STABLE;
`
