package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_31 is a FORK migration (smart sending, integrations listmonk/SMART-SENDING-SPEC D1/D2/D7),
// not an upstream release. It creates campaign_smart_skips -- the skip set a broadcast's claim
// (next-campaigns) decides once and records, one row per subscriber Smart Sending withheld with the
// journey send that caused it -- the function brand_journey_sends_24h, the ONE definition of the rule
// (a same-brand evergreen send in the last 24 hours; brand and window only, every caller joins it to
// the campaign's audience), and a partial index on campaign_sends.sent_at for the window scan (the
// Campaigns list's before-Start count reads it on every load).
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary never reads the table or
// the function; the rows and the index are inert. No DDL reversal is needed.
//
// The version key sits after the fork's v6.2.30 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.31. Idempotent by construction (IF NOT EXISTS, OR
// REPLACE).
func V6_2_31(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(smartSendingDDL); err != nil {
		return err
	}

	return tx.Commit()
}

// smartSendingDDL is mirrored in schema.sql (after its DROP TABLE IF EXISTS campaign_smart_skips
// line, comments aside). Keep the two identical.
const smartSendingDDL = `
CREATE TABLE IF NOT EXISTS campaign_smart_skips (
    campaign_id         INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    subscriber_id       INTEGER NOT NULL,
    journey_campaign_id INTEGER NOT NULL,
    journey_sent_at     TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (campaign_id, subscriber_id)
);
CREATE INDEX IF NOT EXISTS idx_campaign_sends_sent_at ON campaign_sends (sent_at) WHERE sent_at IS NOT NULL;
CREATE OR REPLACE FUNCTION brand_journey_sends_24h(camp_id INT)
RETURNS TABLE (subscriber_id INT, journey_campaign_id INT, journey_sent_at TIMESTAMP WITH TIME ZONE) AS $$
    WITH camp AS (
        SELECT (
            SELECT MIN(list_brand_tag(l.tags)) FROM campaign_lists cl
            JOIN lists l ON (l.id = cl.list_id)
            WHERE cl.campaign_id = c.id
        ) AS brand
        FROM campaigns c
        WHERE c.id = camp_id AND NOT c.evergreen AND c.type = 'regular'
            AND (c.attribs->'smart_sending') IS DISTINCT FROM 'false'::JSONB
    ),
    journeys AS (
        SELECT e.id FROM campaigns e, camp
        WHERE e.evergreen AND camp.brand IS NOT NULL
            AND (
                SELECT MIN(list_brand_tag(l.tags)) FROM campaign_lists cl
                JOIN lists l ON (l.id = cl.list_id)
                WHERE cl.campaign_id = e.id
            ) = camp.brand
    )
    SELECT DISTINCT ON (cs.subscriber_id) cs.subscriber_id, cs.campaign_id, cs.sent_at
    FROM campaign_sends cs
    JOIN journeys j ON (j.id = cs.campaign_id)
    WHERE cs.sent_at IS NOT NULL AND cs.sent_at >= NOW() - INTERVAL '24 hours'
    ORDER BY cs.subscriber_id, cs.sent_at DESC;
$$ LANGUAGE sql STABLE;
`
