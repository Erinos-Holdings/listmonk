package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_11 is a FORK migration (Lists-page grid, integrations LIST-GRID-SPEC D3/D4/D11/D12),
// not an upstream release. It adds:
//
//   - subscription_segment(sl_status, sl_meta, s_status, l_optin): the ONE definition of the
//     five segments that partition every subscriber_lists row (blocked, held, unsubscribed,
//     pending, active -- first match wins). 'active' is exactly what a regular campaign's send
//     query reaches; that query is NOT refactored onto this function, a test pins agreement
//     (cmd/subscribers_segment_db_test.go, TestActiveMatchesCampaignRecipients). A change to
//     send eligibility must change this function in the same release.
//   - subscriber_lang(attribs): the storage language bucket (none, en, es, fr, de, it, other),
//     agreeing with the send queries' language expression (queries/campaigns.sql,
//     next-campaign-subscribers) on every input. The five codes are literals; TestSubscriberLangCoversCampaignLangs pins
//     them against models.CampaignLangs.
//   - send_lang(bucket): none -> en, else identity. The COALESCE-EN fold, stated nowhere else.
//     All three are IMMUTABLE and read no table, so the subscriber-query EXPLAIN validator
//     passes a predicate that calls them.
//   - mat_list_subscriber_stats regrouped (list_id, status, segment, lang). status keeps its
//     v6.2.10 meaning. EVERY reader must pre-aggregate to the grain it wants: JSONB_OBJECT_AGG
//     keeps the last duplicate key, it does not sum. ROLLBACK RULE: a v6.2.10 binary over this
//     view serves a wrong subscriber_statuses with no error, so pinning the image back also
//     means re-running v6.2.10's view DDL by hand (holdsDDL in v6.2.10.go). templates.lang and
//     the three functions are inert to the old binary and stay.
//   - templates.lang TEXT NOT NULL DEFAULT 'en' (D12).
//   - Regular DRAFT campaigns without attribs.lang are stamped "en" (D11). Finished and
//     cancelled ones keep their history; opt-in campaigns are never touched.
//
// The FIRST statement aborts the whole migration when a language-less regular campaign is
// scheduled, running or paused: stamping it would change who a live send reaches, and leaving
// it would strand a campaign the new start/schedule guard refuses. A failed migration is
// lo.Fatalf on boot with the version unrecorded (cmd/upgrade.go), so on the stateless host an
// abort is a restart loop -- release gate G0 (LIST-GRID-SPEC) makes it unreachable in
// practice. Everything runs in ONE Exec (a multi-statement simple query is one implicit
// transaction), so an abort applies nothing.
//
// The version key sits after the fork's v6.2.10 and before any future upstream v6.3.0; re-key
// in the same rebase if upstream ships a v6.2.11. Idempotent by construction.
func V6_2_11(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(listGridAbortDDL + listGridDDL + listGridDataDDL)
	return err
}

// listGridAbortDDL must stay the first statement of the migration.
const listGridAbortDDL = `
DO $$
DECLARE ids TEXT;
BEGIN
    SELECT STRING_AGG(id::TEXT, ', ' ORDER BY id) INTO ids FROM campaigns
        WHERE type = 'regular' AND status IN ('scheduled', 'running', 'paused')
        AND NULLIF(attribs->>'lang', '') IS NULL;
    IF ids IS NOT NULL THEN
        RAISE EXCEPTION 'v6.2.11 aborted -- language-less regular campaign(s) scheduled, running or paused (id %). Give each a language or cancel it, then upgrade again.', ids;
    END IF;
END $$;
`

// listGridDDL is mirrored in schema.sql. Keep the two identical.
const listGridDDL = `
CREATE OR REPLACE FUNCTION subscription_segment(sl_status subscription_status, sl_meta JSONB, s_status subscriber_status, l_optin list_optin) RETURNS TEXT AS $$
    SELECT CASE
        WHEN s_status = 'blocklisted' THEN 'blocked'
        WHEN sl_status = 'unsubscribed' AND COALESCE(sl_meta ? 'hold', FALSE) THEN 'held'
        WHEN sl_status = 'unsubscribed' THEN 'unsubscribed'
        WHEN sl_status = 'unconfirmed' AND l_optin = 'double' THEN 'pending'
        ELSE 'active' END;
$$ LANGUAGE sql IMMUTABLE;

CREATE OR REPLACE FUNCTION subscriber_lang(attribs JSONB) RETURNS TEXT AS $$
    SELECT CASE
        WHEN NULLIF(LOWER(LEFT(attribs->>'lang', 2)), '') IS NULL THEN 'none'
        WHEN LOWER(LEFT(attribs->>'lang', 2)) IN ('en', 'es', 'fr', 'de', 'it') THEN LOWER(LEFT(attribs->>'lang', 2))
        ELSE 'other' END;
$$ LANGUAGE sql IMMUTABLE;

CREATE OR REPLACE FUNCTION send_lang(bucket TEXT) RETURNS TEXT AS $$
    SELECT CASE WHEN bucket = 'none' THEN 'en' ELSE bucket END;
$$ LANGUAGE sql IMMUTABLE;

DROP INDEX IF EXISTS mat_list_subscriber_stats_idx;
DROP MATERIALIZED VIEW IF EXISTS mat_list_subscriber_stats;
CREATE MATERIALIZED VIEW mat_list_subscriber_stats AS
    SELECT NOW() AS updated_at, lists.id AS list_id,
        (CASE WHEN subscriber_lists.status = 'unsubscribed' AND subscriber_lists.meta ? 'hold' AND subscribers.status <> 'blocklisted'
              THEN 'held' ELSE subscriber_lists.status::TEXT END) AS status,
        (CASE WHEN subscriber_lists.status IS NULL THEN NULL
              ELSE subscription_segment(subscriber_lists.status, subscriber_lists.meta, subscribers.status, lists.optin) END) AS segment,
        (CASE WHEN subscriber_lists.status IS NULL THEN NULL ELSE subscriber_lang(subscribers.attribs) END) AS lang,
        COUNT(subscriber_lists.status) AS subscriber_count
    FROM lists
    LEFT JOIN subscriber_lists ON (subscriber_lists.list_id = lists.id)
    LEFT JOIN subscribers ON (subscribers.id = subscriber_lists.subscriber_id)
    GROUP BY lists.id, 3, 4, 5
    UNION ALL
    SELECT NOW() AS updated_at, 0 AS list_id, NULL AS status, NULL AS segment, NULL AS lang, COUNT(id) AS subscriber_count FROM subscribers;
CREATE UNIQUE INDEX mat_list_subscriber_stats_idx ON mat_list_subscriber_stats (list_id, status, segment, lang);

ALTER TABLE templates ADD COLUMN IF NOT EXISTS lang TEXT NOT NULL DEFAULT 'en';
`

// listGridDataDDL is the data half, which schema.sql has no use for (a fresh install has no
// campaigns; cmd/install.go stamps its sample campaign itself).
const listGridDataDDL = `
UPDATE campaigns SET attribs = (CASE WHEN JSONB_TYPEOF(attribs) = 'object' THEN attribs ELSE '{}'::JSONB END) || '{"lang": "en"}'::JSONB
    WHERE type = 'regular' AND status = 'draft' AND NULLIF(attribs->>'lang', '') IS NULL;
`
