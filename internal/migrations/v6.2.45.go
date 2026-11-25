package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_45 is a FORK migration (the automation main-list rule, integrations
// lambda/integrations/komigo/SKU-JOURNEYS-SPEC D1), not an upstream release. Additive: it creates
// (or replaces) the function brand_opted_in(subscriber, list) -- an automation sends to a subscriber
// on a brand list only while they hold a confirmed row on the brand's main list
// (brand_master_list_id); the main list itself, reengage queues, transactional and untagged lists
// are exempt; a brand with no single main list fails closed. queries/evergreen.sql
// next-evergreen-subscribers is its one reader -- the send-path change ships with the query, not
// with this function.
//
// ROLLBACK: pin the previous image. The function is inert to the older binary (its query does not
// call it); no DROP is needed.
//
// The version key sits after the fork's v6.2.44 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.45. Idempotent by construction (CREATE OR REPLACE).
func V6_2_45(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(brandOptedInDDL)
	return err
}

// brandOptedInDDL is mirrored in schema.sql (keep the two identical; v6_2_45_db_test.go asserts it
// verbatim).
const brandOptedInDDL = `
CREATE OR REPLACE FUNCTION brand_opted_in(p_subscriber_id INT, p_list_id INT) RETURNS BOOLEAN AS $$
    SELECT CASE
        WHEN l.id IS NULL THEN FALSE
        WHEN list_brand_tag(l.tags) IS NULL THEN TRUE
        WHEN l.transactional THEN TRUE
        WHEN EXISTS (SELECT 1 FROM UNNEST(l.tags) AS t WHERE BTRIM(t, E' \t\n\r\f\x0B') LIKE 'reengage:%') THEN TRUE
        WHEN l.id IS NOT DISTINCT FROM brand_master_list_id(list_brand_tag(l.tags)) THEN TRUE
        ELSE EXISTS (SELECT 1 FROM subscriber_lists m
            WHERE m.subscriber_id = p_subscriber_id AND m.status = 'confirmed'
                AND m.list_id = brand_master_list_id(list_brand_tag(l.tags)))
        END
    FROM (SELECT 1) AS one LEFT JOIN lists l ON (l.id = p_list_id);
$$ LANGUAGE sql STABLE;
`
