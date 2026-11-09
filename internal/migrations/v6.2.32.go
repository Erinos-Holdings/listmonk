package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_32 is a FORK migration (smart sending look-ahead, integrations
// listmonk/SMART-SENDING-LOOKAHEAD-SPEC D1-D5/D7), not an upstream release. It adds:
//   - journey_due -- the forecast feed the integrations NoPurchaseSweep and ListmonkSunset Lambdas
//     write (PUT /api/journeys/due): a row means "the run at expected_at will join this subscriber
//     to this list". No foreign keys (an unknown list or subscriber id is inert). The fork never
//     computes a row.
//   - campaign_smart_skips' journey pair made nullable, and the look-ahead pair (due_list_id,
//     due_at) added. No CHECK constraint (S1) -- a raise inside next-campaigns would stall every
//     claim.
//   - smart_sending_brand -- the ONE applicability rule for both halves (the predicate moved out
//     of brand_journey_sends_24h, which is re-created to call it, behaviour unchanged).
//   - brand_journey_due_24h -- the ONE look-ahead definition (forecast joins and the fork's own
//     not-yet-sent joins, due inside the window, on a RUNNING same-brand evergreen); dark unless
//     the settings row app.smart_sending_lookahead is the JSONB true.
//   - brand_journey_skips -- the look-back rows plus the look-ahead rows for subscribers with no
//     look-back row; the claim and the before-Start count read only this.
//   - the settings row app.smart_sending_lookahead, inserted false (DELIBERATELY not a field of the
//     Go settings struct -- a Settings-page save never writes it back).
//
// ROLLBACK (D8): pin the previous image and re-apply the host. The older binary calls
// brand_journey_sends_24h (still present, same behaviour) and writes the journey pair into the
// now-nullable columns; the new table, functions, columns and settings row are inert to it.
//
// The version key sits after the fork's v6.2.31 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.32. Idempotent by construction (IF NOT EXISTS, OR
// REPLACE, ON CONFLICT DO NOTHING; DROP NOT NULL on a nullable column is a no-op).
func V6_2_32(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(smartSendingLookaheadDDL); err != nil {
		return err
	}

	return tx.Commit()
}

// smartSendingLookaheadDDL is mirrored in schema.sql (after its DROP TABLE IF EXISTS journey_due
// line, comments aside). Keep the two identical.
const smartSendingLookaheadDDL = `
CREATE TABLE IF NOT EXISTS journey_due (
    source        TEXT NOT NULL,
    list_id       INTEGER NOT NULL,
    subscriber_id INTEGER NOT NULL,
    expected_at   TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (source, list_id, subscriber_id, expected_at)
);
CREATE INDEX IF NOT EXISTS idx_journey_due_expected_at ON journey_due (expected_at);
ALTER TABLE campaign_smart_skips ALTER COLUMN journey_campaign_id DROP NOT NULL;
ALTER TABLE campaign_smart_skips ALTER COLUMN journey_sent_at DROP NOT NULL;
ALTER TABLE campaign_smart_skips ADD COLUMN IF NOT EXISTS due_list_id INTEGER NULL;
ALTER TABLE campaign_smart_skips ADD COLUMN IF NOT EXISTS due_at TIMESTAMP WITH TIME ZONE NULL;
INSERT INTO settings (key, value) VALUES ('app.smart_sending_lookahead', 'false') ON CONFLICT DO NOTHING;
CREATE OR REPLACE FUNCTION smart_sending_brand(camp_id INT) RETURNS TEXT AS $$
    SELECT (
        SELECT MIN(list_brand_tag(l.tags)) FROM campaign_lists cl
        JOIN lists l ON (l.id = cl.list_id)
        WHERE cl.campaign_id = c.id
    )
    FROM campaigns c
    WHERE c.id = camp_id AND NOT c.evergreen AND c.type = 'regular'
        AND (c.attribs->'smart_sending') IS DISTINCT FROM 'false'::JSONB;
$$ LANGUAGE sql STABLE;
CREATE OR REPLACE FUNCTION brand_journey_sends_24h(camp_id INT)
RETURNS TABLE (subscriber_id INT, journey_campaign_id INT, journey_sent_at TIMESTAMP WITH TIME ZONE) AS $$
    WITH camp AS (
        SELECT smart_sending_brand(camp_id) AS brand
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
CREATE OR REPLACE FUNCTION brand_journey_due_24h(camp_id INT)
RETURNS TABLE (subscriber_id INT, due_list_id INT, due_at TIMESTAMP WITH TIME ZONE) AS $$
    WITH win AS (
        SELECT NOW() - INTERVAL '2 hours' AS lo, NOW() + INTERVAL '24 hours' AS hi
    ),
    camp AS (
        SELECT smart_sending_brand(camp_id) AS brand
    ),
    evs AS (
        SELECT e.id, e.started_at, e.send_delay_secs, e.attribs->>'lang' AS lang
        FROM campaigns e, camp
        WHERE e.evergreen AND e.status = 'running' AND camp.brand IS NOT NULL
            AND e.send_delay_secs BETWEEN 0 AND 315360000
            AND (
                SELECT MIN(list_brand_tag(l.tags)) FROM campaign_lists cl
                JOIN lists l ON (l.id = cl.list_id)
                WHERE cl.campaign_id = e.id
            ) = camp.brand
    ),
    evLists AS (
        SELECT evs.id, evs.started_at, evs.send_delay_secs, evs.lang, cl.list_id
        FROM evs JOIN campaign_lists cl ON (cl.campaign_id = evs.id)
        WHERE cl.list_id IS NOT NULL
    ),
    joins AS (
        SELECT d.subscriber_id, el.list_id, el.lang,
            d.expected_at + MAKE_INTERVAL(secs => el.send_delay_secs) AS due
        FROM evLists el
        JOIN journey_due d ON (d.list_id = el.list_id)
        CROSS JOIN win
        WHERE d.expected_at > win.lo - MAKE_INTERVAL(secs => el.send_delay_secs)
            AND d.expected_at <= win.hi - MAKE_INTERVAL(secs => el.send_delay_secs)
        UNION ALL
        SELECT sl.subscriber_id, el.list_id, el.lang,
            sl.confirmed_at + MAKE_INTERVAL(secs => el.send_delay_secs) AS due
        FROM evLists el
        JOIN subscriber_lists sl ON (sl.list_id = el.list_id)
        CROSS JOIN win
        WHERE sl.status = 'confirmed'
            AND sl.confirmed_at > el.started_at
            AND sl.confirmed_at > win.lo - MAKE_INTERVAL(secs => el.send_delay_secs)
            AND sl.confirmed_at <= win.hi - MAKE_INTERVAL(secs => el.send_delay_secs)
            AND NOT EXISTS (
                SELECT 1 FROM campaign_sends cs
                WHERE cs.campaign_id = el.id AND cs.subscriber_id = sl.subscriber_id
                    AND cs.sent_at >= sl.confirmed_at
            )
    )
    SELECT DISTINCT ON (j.subscriber_id) j.subscriber_id, j.list_id, j.due
    FROM joins j
    JOIN subscribers s ON (s.id = j.subscriber_id)
    WHERE (SELECT st.value FROM settings st WHERE st.key = 'app.smart_sending_lookahead') = 'true'::JSONB
        AND (j.lang IS NULL
           OR COALESCE(NULLIF(LOWER(LEFT(s.attribs->>'lang', 2)), ''), 'en') = j.lang)
    ORDER BY j.subscriber_id, j.due;
$$ LANGUAGE sql STABLE;
CREATE OR REPLACE FUNCTION brand_journey_skips(camp_id INT)
RETURNS TABLE (subscriber_id INT, journey_campaign_id INT, journey_sent_at TIMESTAMP WITH TIME ZONE,
    due_list_id INT, due_at TIMESTAMP WITH TIME ZONE) AS $$
    WITH back AS (
        SELECT b.subscriber_id, b.journey_campaign_id, b.journey_sent_at FROM brand_journey_sends_24h(camp_id) b
    )
    SELECT back.subscriber_id, back.journey_campaign_id, back.journey_sent_at,
        NULL::INT, NULL::TIMESTAMP WITH TIME ZONE
    FROM back
    UNION ALL
    SELECT d.subscriber_id, NULL::INT, NULL::TIMESTAMP WITH TIME ZONE, d.due_list_id, d.due_at
    FROM brand_journey_due_24h(camp_id) d
    WHERE NOT EXISTS (SELECT 1 FROM back WHERE back.subscriber_id = d.subscriber_id);
$$ LANGUAGE sql STABLE;
`
