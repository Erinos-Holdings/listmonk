package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_10 is a FORK migration (holds, integrations SUNSET-SPEC D2/D5/D6), not an upstream
// release. A HOLD is a subscriber_lists row that is 'unsubscribed' AND carries meta.hold (a
// JSON object naming the system that wrote it and why) on a subscriber who is not
// blocklisted. It adds:
//
//   - subscriber_lists_release_hold(meta, to): moves meta.hold to meta.hold_released =
//     {at, to, hold}. The ONE definition of a release, used by the trigger below and by the
//     unsubscribe queries (queries/subscribers.sql, "Fork (holds)").
//   - A BEFORE UPDATE OF status trigger that releases a hold when the row's status really
//     changes (a re-confirm by any writer, an opt-in confirm) and bumps updated_at, because
//     add-subscribers-to-lists' ON CONFLICT writes status only and every consent-time bound
//     reads updated_at. A same-status write is NOT a release here: upstream's retain-status
//     paths (the admin edit form, a bulk add with no status, a CSV import without the
//     overwrite flag) all SET status to its current value, so only the unsubscribe queries
//     release a hold on an already-unsubscribed row, explicitly.
//   - mat_list_subscriber_stats recreated with a 'held' pseudo-status (status is TEXT now).
//     query-subscribers-count-all compares it as text; the pre-fork binary's cast does not
//     work against this view, so a pin-back must re-run the old view DDL.
//   - subscription_engagement(list_id): per confirmed row, the finished regular broadcasts
//     whose send predicate includes it NOW, and its last view/click on this list's campaigns.
//
// The version key sits after the fork's v6.2.9 and before any future upstream v6.3.0; re-key
// in the same rebase if upstream ships a v6.2.10. Idempotent by construction.
func V6_2_10(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(holdsDDL)
	return err
}

// holdsDDL is mirrored in schema.sql. Keep the two identical.
const holdsDDL = `
CREATE OR REPLACE FUNCTION subscriber_lists_release_hold(meta JSONB, rel_to TEXT) RETURNS JSONB AS $$
    SELECT CASE WHEN meta ? 'hold'
        THEN (meta - 'hold') || JSONB_BUILD_OBJECT('hold_released', JSONB_BUILD_OBJECT(
            'at', TO_CHAR(NOW() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
            'to', rel_to,
            'hold', meta->'hold'))
        ELSE meta END;
$$ LANGUAGE sql STABLE;

CREATE OR REPLACE FUNCTION subscriber_lists_hold_release() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status IS DISTINCT FROM NEW.status AND OLD.meta ? 'hold' AND NEW.meta ? 'hold' THEN
        NEW.meta := subscriber_lists_release_hold(NEW.meta, NEW.status::TEXT);
        NEW.updated_at := NOW();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS subscriber_lists_hold_release ON subscriber_lists;
CREATE TRIGGER subscriber_lists_hold_release
    BEFORE UPDATE OF status ON subscriber_lists
    FOR EACH ROW EXECUTE FUNCTION subscriber_lists_hold_release();

DROP INDEX IF EXISTS mat_list_subscriber_stats_idx;
DROP MATERIALIZED VIEW IF EXISTS mat_list_subscriber_stats;
CREATE MATERIALIZED VIEW mat_list_subscriber_stats AS
    SELECT NOW() AS updated_at, lists.id AS list_id,
        (CASE WHEN subscriber_lists.status = 'unsubscribed' AND subscriber_lists.meta ? 'hold' AND subscribers.status <> 'blocklisted'
              THEN 'held' ELSE subscriber_lists.status::TEXT END) AS status,
        COUNT(subscriber_lists.status) AS subscriber_count
    FROM lists
    LEFT JOIN subscriber_lists ON (subscriber_lists.list_id = lists.id)
    LEFT JOIN subscribers ON (subscribers.id = subscriber_lists.subscriber_id)
    GROUP BY lists.id, 3
    UNION ALL
    SELECT NOW() AS updated_at, 0 AS list_id, NULL AS status, COUNT(id) AS subscriber_count FROM subscribers;
CREATE UNIQUE INDEX mat_list_subscriber_stats_idx ON mat_list_subscriber_stats (list_id, status);

CREATE OR REPLACE FUNCTION subscription_engagement(p_list_id INT)
RETURNS TABLE (subscriber_id INT, anchor_at TIMESTAMPTZ, eligible_sends BIGINT,
    first_eligible_send_at TIMESTAMPTZ, last_eligible_send_at TIMESTAMPTZ,
    last_view_at TIMESTAMPTZ, last_click_at TIMESTAMPTZ) AS $$
    WITH members AS (
        SELECT sl.subscriber_id, COALESCE(sl.confirmed_at, sl.created_at) AS anchor_at,
            COALESCE(NULLIF(LOWER(LEFT(s.attribs->>'lang', 2)), ''), 'en') AS lang
        FROM subscriber_lists sl JOIN subscribers s ON (s.id = sl.subscriber_id)
        WHERE sl.list_id = p_list_id AND sl.status = 'confirmed' AND s.status <> 'blocklisted'
    ),
    list_camps AS (
        SELECT c.id, c.type, c.status, c.evergreen, c.started_at, c.max_subscriber_id, c.attribs->>'lang' AS lang
        FROM campaigns c JOIN campaign_lists cl ON (cl.campaign_id = c.id)
        WHERE cl.list_id = p_list_id
    ),
    sends AS (
        SELECT m.subscriber_id, COUNT(*) AS n, MIN(c.started_at) AS first_at, MAX(c.started_at) AS last_at
        FROM members m JOIN list_camps c ON (
            c.type = 'regular' AND c.status = 'finished' AND c.evergreen = FALSE
            AND c.started_at > m.anchor_at
            AND m.subscriber_id <= c.max_subscriber_id
            AND (c.lang IS NULL OR m.lang = c.lang))
        WHERE NOT EXISTS (SELECT 1 FROM campaign_send_failures f
            WHERE f.campaign_id = c.id AND f.subscriber_id = m.subscriber_id)
        GROUP BY m.subscriber_id
    ),
    views AS (
        SELECT m.subscriber_id, MAX(v.created_at) AS at
        FROM members m JOIN campaign_views v ON (v.subscriber_id = m.subscriber_id AND v.created_at > m.anchor_at)
        WHERE v.campaign_id IN (SELECT id FROM list_camps)
        GROUP BY m.subscriber_id
    ),
    clicks AS (
        SELECT m.subscriber_id, MAX(k.created_at) AS at
        FROM members m JOIN link_clicks k ON (k.subscriber_id = m.subscriber_id AND k.created_at > m.anchor_at)
        WHERE k.campaign_id IN (SELECT id FROM list_camps)
        GROUP BY m.subscriber_id
    )
    SELECT m.subscriber_id, m.anchor_at, COALESCE(s.n, 0), s.first_at, s.last_at, v.at, k.at
    FROM members m
    LEFT JOIN sends s ON (s.subscriber_id = m.subscriber_id)
    LEFT JOIN views v ON (v.subscriber_id = m.subscriber_id)
    LEFT JOIN clicks k ON (k.subscriber_id = m.subscriber_id);
$$ LANGUAGE sql STABLE;
`
