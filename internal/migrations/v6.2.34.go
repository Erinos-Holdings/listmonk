package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_34 is a FORK migration (one engagement definition, integrations
// listmonk/DOC-ALIGNMENT-SPEC D2), not an upstream release. It adds ONE function:
//   - brand_engagement(list, since, skip_url, view_client) -- per subscriber, the newest click
//     (excluding any link whose URL contains skip_url -- the Manage Preferences host) and the newest
//     view with client = view_client (the Gmail proxy) after `since`, on ANY campaign of ANY list
//     carrying the same brand tag as `list` (newsletter, step, first-order, SKU, segment and the four
//     queue lists; evergreens included). A list with no brand tag yields its own campaigns only. Both
//     arms filter subscriber_id IS NOT NULL (untracked sends store NULL, and one NULL under the
//     sunset predicate's NOT IN would hold nobody).
//
// Additive and uncalled: nothing in the binary calls it; the integrations ListmonkSunset Lambda
// selects it through the subscriber query API only once its SUNSET_ENGAGEMENT_BRAND switch is on.
// The tables it reads (lists, campaign_lists, link_clicks, links, campaign_views) are already on
// the query allowlist (internal/core/subscribers.go allowedSubQueryTables). LANGUAGE sql STABLE,
// written like v6.2.10's subscription_engagement, so the validator and the planner treat it the
// same way. subscription_engagement is NOT changed.
//
// ROLLBACK: pin the previous image and re-apply the host; the older binary never calls the
// function, so it is inert to it.
//
// The version key sits after the fork's v6.2.33 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.34. Idempotent by construction (CREATE OR REPLACE).
func V6_2_34(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(brandEngagementDDL); err != nil {
		return err
	}

	return tx.Commit()
}

// brandEngagementDDL is mirrored in schema.sql (after campaign_brand, comments aside). Keep the
// two identical.
const brandEngagementDDL = `
CREATE OR REPLACE FUNCTION brand_engagement(p_list_id INT, p_since TIMESTAMPTZ, p_skip_url TEXT, p_view_client TEXT)
RETURNS TABLE (subscriber_id INT, last_click_at TIMESTAMPTZ, last_view_at TIMESTAMPTZ) AS $$
    WITH brand_lists AS (
        SELECT l.id FROM lists l
        WHERE l.id = p_list_id
            OR (list_brand_tag(l.tags) IS NOT NULL
                AND list_brand_tag(l.tags) = (SELECT list_brand_tag(b.tags) FROM lists b WHERE b.id = p_list_id))
    ),
    brand_camps AS (
        SELECT DISTINCT cl.campaign_id AS id FROM campaign_lists cl WHERE cl.list_id IN (SELECT id FROM brand_lists)
    ),
    clicks AS (
        SELECT k.subscriber_id, MAX(k.created_at) AS at
        FROM link_clicks k JOIN links lk ON (lk.id = k.link_id)
        WHERE k.subscriber_id IS NOT NULL AND k.created_at > p_since
            AND k.campaign_id IN (SELECT id FROM brand_camps)
            AND (COALESCE(p_skip_url, '') = '' OR STRPOS(lk.url, p_skip_url) = 0)
        GROUP BY k.subscriber_id
    ),
    views AS (
        SELECT v.subscriber_id, MAX(v.created_at) AS at
        FROM campaign_views v
        WHERE v.subscriber_id IS NOT NULL AND v.created_at > p_since AND v.client = p_view_client
            AND v.campaign_id IN (SELECT id FROM brand_camps)
        GROUP BY v.subscriber_id
    )
    SELECT COALESCE(k.subscriber_id, v.subscriber_id), k.at, v.at
    FROM clicks k FULL OUTER JOIN views v ON (v.subscriber_id = k.subscriber_id);
$$ LANGUAGE sql STABLE;
`
