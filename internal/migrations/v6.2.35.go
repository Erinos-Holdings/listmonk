package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_35 is a FORK migration (hold scope, integrations listmonk/HOLD-SCOPE-SPEC D1-D3), not an
// upstream release. It adds:
//   - the settings row app.brand_hold_scope, inserted false (DELIBERATELY not a field of the Go
//     settings struct -- a Settings-page save never writes it back). The brand-wide hold is in
//     force only while it is the JSONB true.
//   - brand_hold_rows(campaign) -- the ONE definition of the brand-wide hold: the (campaign list,
//     subscriber) pairs whose row does not qualify because the subscriber is held (unsubscribed +
//     meta.hold, any reason) on any non-transactional list of the same brand tag. Regular,
//     non-evergreen campaigns only; transactional, untagged and re-engagement queue
//     (reengage:<id>:<1-4>) campaign lists are exempt. Read by the five audience readers in
//     queries/campaigns.sql. Writes nothing.
//   - brand_eligible_sends(list) -- the sunset rule's broadcasts counted as sent, across the brand
//     (a Smart Sending skip is not a send; a sibling-list broadcast counts when the member held a
//     qualifying row on it at the send). Uncalled by the binary; the integrations ListmonkSunset
//     Lambda selects it through the subscriber query API only under its SUNSET_COUNT_BRAND switch.
//     It reads campaign_smart_skips, added to internal/core/subscribers.go allowedSubQueryTables.
//
// subscription_engagement is NOT changed.
//
// ROLLBACK: pin the previous image and re-apply the host; the older binary never calls either
// function and never reads the settings row, so all three are inert to it.
//
// The version key sits after the fork's v6.2.34 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.35. Idempotent by construction (ON CONFLICT DO NOTHING,
// CREATE OR REPLACE).
func V6_2_35(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(brandHoldScopeDDL); err != nil {
		return err
	}
	if _, err := tx.Exec(brandEligibleSendsDDL); err != nil {
		return err
	}

	return tx.Commit()
}

// brandHoldScopeDDL and brandEligibleSendsDDL are mirrored in schema.sql (after brand_engagement,
// comments aside); each is contained there verbatim (v6_2_35_db_test.go). Keep them identical.
const brandHoldScopeDDL = `
INSERT INTO settings (key, value) VALUES ('app.brand_hold_scope', 'false') ON CONFLICT DO NOTHING;
CREATE OR REPLACE FUNCTION brand_hold_rows(p_campaign_id INT)
RETURNS TABLE (list_id INT, subscriber_id INT) AS $$
    SELECT x.id, h.subscriber_id
    FROM campaigns c
    JOIN campaign_lists cl ON (cl.campaign_id = c.id)
    JOIN lists x ON (x.id = cl.list_id)
    JOIN lists n ON (n.transactional = FALSE AND list_brand_tag(n.tags) = list_brand_tag(x.tags))
    JOIN subscriber_lists h ON (h.list_id = n.id AND h.status = 'unsubscribed' AND h.meta ? 'hold')
    WHERE c.id = p_campaign_id AND c.type = 'regular' AND c.evergreen = FALSE
        AND x.transactional = FALSE AND list_brand_tag(x.tags) IS NOT NULL
        AND NOT EXISTS (SELECT 1 FROM UNNEST(x.tags) AS t WHERE t ~ '^reengage:[0-9]+:[1-4]$')
        AND (SELECT st.value FROM settings st WHERE st.key = 'app.brand_hold_scope') = 'true'::JSONB;
$$ LANGUAGE sql STABLE PARALLEL SAFE;
`

const brandEligibleSendsDDL = `
CREATE OR REPLACE FUNCTION brand_eligible_sends(p_list_id INT)
RETURNS TABLE (subscriber_id INT, eligible_sends BIGINT, first_eligible_send_at TIMESTAMPTZ) AS $$
    WITH members AS (
        SELECT sl.subscriber_id, GREATEST(COALESCE(sl.confirmed_at, sl.created_at), COALESCE((sl.meta->'hold_released'->>'at')::TIMESTAMPTZ, '-infinity'::TIMESTAMPTZ)) AS anchor_at,
            COALESCE(NULLIF(LOWER(LEFT(s.attribs->>'lang', 2)), ''), 'en') AS lang
        FROM subscriber_lists sl JOIN subscribers s ON (s.id = sl.subscriber_id)
        WHERE sl.list_id = p_list_id AND sl.status = 'confirmed' AND s.status <> 'blocklisted'
    ),
    brand_lists AS (
        SELECT x.id, x.optin FROM lists x
        WHERE x.transactional = FALSE
            AND (x.id = p_list_id
                OR (list_brand_tag(x.tags) IS NOT NULL
                    AND list_brand_tag(x.tags) = (SELECT list_brand_tag(b.tags) FROM lists b WHERE b.id = p_list_id)))
    ),
    camps AS (
        SELECT c.id, c.started_at, c.max_subscriber_id, c.attribs->>'lang' AS lang, cl.list_id
        FROM campaigns c JOIN campaign_lists cl ON (cl.campaign_id = c.id)
        WHERE c.type = 'regular' AND c.status = 'finished' AND c.evergreen = FALSE
            AND cl.list_id IN (SELECT id FROM brand_lists)
    ),
    reach AS (
        SELECT c.id AS campaign_id, c.started_at, c.max_subscriber_id, c.lang, m.subscriber_id, m.anchor_at, m.lang AS sub_lang
        FROM camps c JOIN members m ON (TRUE)
        WHERE c.list_id = p_list_id
        UNION ALL
        SELECT c.id, c.started_at, c.max_subscriber_id, c.lang, m.subscriber_id, m.anchor_at, m.lang
        FROM members m
        JOIN subscriber_lists r ON (r.subscriber_id = m.subscriber_id AND r.list_id <> p_list_id)
        JOIN brand_lists x ON (x.id = r.list_id)
        JOIN camps c ON (c.list_id = x.id)
        WHERE r.created_at <= c.started_at
            AND ((x.optin = 'double' AND r.status = 'confirmed') OR (x.optin <> 'double' AND r.status <> 'unsubscribed'))
    ),
    counted AS (
        SELECT e.subscriber_id, e.campaign_id, e.started_at
        FROM reach e
        WHERE e.started_at > e.anchor_at
            AND e.subscriber_id <= e.max_subscriber_id
            AND (e.lang IS NULL OR e.sub_lang = e.lang)
            AND NOT EXISTS (SELECT 1 FROM campaign_send_failures f
                WHERE f.campaign_id = e.campaign_id AND f.subscriber_id = e.subscriber_id)
            AND NOT EXISTS (SELECT 1 FROM campaign_smart_skips k
                WHERE k.campaign_id = e.campaign_id AND k.subscriber_id = e.subscriber_id)
    )
    SELECT m.subscriber_id, COUNT(DISTINCT k.campaign_id), MIN(k.started_at)
    FROM members m
    LEFT JOIN counted k ON (k.subscriber_id = m.subscriber_id)
    GROUP BY m.subscriber_id;
$$ LANGUAGE sql STABLE;
`
