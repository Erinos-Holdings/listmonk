package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_33 is a FORK migration (brand unsubscribe, integrations
// listmonk/BRAND-UNSUBSCRIBE-SPEC D1/D2), not an upstream release. It adds the two functions
// that are the ONE definition of a brand unsubscribe's target set:
//   - brand_marketing_list_ids(brand) -- every list carrying the brand tag whose transactional
//     flag is false, irrespective of type or optin.
//   - campaign_brand(uuid) -- the single distinct non-NULL brand tag over a campaign's lists,
//     NULL when there is none or more than one.
//
// No table, column or index: the behaviour change is in queries/subscribers.sql
// (unsubscribe-by-campaign's target set and the new unsubscribe-brand) and the new admin route,
// which are in the binary, not in the schema.
//
// ROLLBACK (the release's one step): pin the previous image and re-apply the host. The older
// binary never calls either function, so both are inert to it; the campaign-link path reverts to
// the campaign's own lists with the old row predicate.
//
// The version key sits after the fork's v6.2.32 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.33. Idempotent by construction (CREATE OR REPLACE).
func V6_2_33(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(brandUnsubscribeDDL); err != nil {
		return err
	}

	return tx.Commit()
}

// brandUnsubscribeDDL is mirrored in schema.sql (after brand_journey_skips, comments aside).
// Keep the two identical.
const brandUnsubscribeDDL = `
CREATE OR REPLACE FUNCTION brand_marketing_list_ids(p_brand TEXT) RETURNS INT[] AS $$
    SELECT COALESCE(ARRAY_AGG(l.id), '{}'::INT[]) FROM lists l
        WHERE p_brand IS NOT NULL AND list_brand_tag(l.tags) = p_brand AND l.transactional = FALSE;
$$ LANGUAGE sql STABLE;

CREATE OR REPLACE FUNCTION campaign_brand(camp_uuid UUID) RETURNS TEXT AS $$
    SELECT CASE WHEN COUNT(DISTINCT list_brand_tag(l.tags)) = 1
        THEN MIN(list_brand_tag(l.tags)) ELSE NULL END
    FROM campaigns c
    JOIN campaign_lists cl ON (cl.campaign_id = c.id)
    JOIN lists l ON (l.id = cl.list_id)
    WHERE c.uuid = camp_uuid AND list_brand_tag(l.tags) IS NOT NULL;
$$ LANGUAGE sql STABLE;
`
