package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_26 is a FORK migration (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC D1/D3/D4),
// not an upstream release. It creates campaign_send_hours: one row per (campaign, UTC hour) holding
// how many messages the campaign sent in that hour. The live writes ride the manager's existing
// sent-count statements (queries/campaigns.sql next-campaigns' updateCounts CTE and
// update-campaign-counts for broadcasts, queries/evergreen.sql mark-evergreen-sent for evergreens --
// D2); this migration only creates the table and backfills history.
//
// The backfill runs ONLY when this migration creates the table (a re-run finds it and does
// nothing), and every insert is ON CONFLICT DO NOTHING besides:
//   - D3 broadcasts: a finished/cancelled non-evergreen campaign with sent > 0 and a started_at gets
//     one row, its whole sent at its started_at hour (exact for single-day broadcasts);
//   - D3 evergreens: campaign_sends' marked rows grouped by hour (exact history);
//   - D4 the Younique winback seed: campaign 217's five tranche hours (sum 46,272), guarded by
//     id = 217 AND sent = 46272 AND status = 'paused' -- a miss writes nothing and is logged; the
//     catch-up is re-running the five inserts by hand (runbook).
// The count of sent > 0 broadcasts the generic backfill skipped (another status, or no started_at)
// is logged so any gap is visible in the bootstrap output (Stage 2 S5a).
//
// One transaction with SET LOCAL lock_timeout = '5s' (the v6.2.24 shape). schema.sql's mirror block
// is the same CREATE preceded by a DROP TABLE IF EXISTS -- never a DROP in a migration.
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary never reads or writes
// the table; no DDL reversal is needed.
//
// The version key sits after the fork's v6.2.25 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.26. Idempotent by construction.
func V6_2_26(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}

	var exists bool
	if err := tx.Get(&exists, `SELECT to_regclass('campaign_send_hours') IS NOT NULL`); err != nil {
		return err
	}
	if _, err := tx.Exec(campaignSendHoursDDL); err != nil {
		return err
	}
	if exists {
		return tx.Commit()
	}

	// D3 -- broadcasts at their started_at hour.
	res, err := tx.Exec(sendHoursBackfillBroadcasts)
	if err != nil {
		return err
	}
	nb, _ := res.RowsAffected()

	// D3 -- evergreens from their per-delivery marks.
	res, err = tx.Exec(sendHoursBackfillEvergreens)
	if err != nil {
		return err
	}
	ne, _ := res.RowsAffected()

	// D4 -- the winback seed (guarded, five rows or none).
	res, err = tx.Exec(sendHoursWinbackSeed)
	if err != nil {
		return err
	}
	ns, _ := res.RowsAffected()

	var skipped []int
	if err := tx.Select(&skipped, sendHoursSkippedBroadcasts); err != nil {
		return err
	}

	lo.Printf("v6.2.26 campaign_send_hours: backfilled %d broadcast row(s), %d evergreen hour row(s); winback seed (campaign 217) wrote %d row(s)", nb, ne, ns)
	if ns == 0 {
		lo.Printf("v6.2.26 campaign_send_hours: the campaign 217 seed guard did not match (id 217, sent 46272, status paused) -- no seed rows; re-run the five inserts by hand per the runbook if 217's series is needed")
	}
	lo.Printf("v6.2.26 campaign_send_hours: %d broadcast(s) with sent > 0 skipped by the generic backfill (status not finished/cancelled, or no started_at): %v", len(skipped), skipped)

	return tx.Commit()
}

// campaignSendHoursDDL is mirrored in schema.sql (after its DROP TABLE IF EXISTS line). Keep the two
// identical.
const campaignSendHoursDDL = `
CREATE TABLE IF NOT EXISTS campaign_send_hours (
    campaign_id      INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE ON UPDATE CASCADE,
    hour             TIMESTAMP WITH TIME ZONE NOT NULL,
    n                INTEGER NOT NULL,
    PRIMARY KEY (campaign_id, hour)
);
`

const sendHoursBackfillBroadcasts = `
INSERT INTO campaign_send_hours (campaign_id, hour, n)
SELECT id, DATE_TRUNC('hour', started_at, 'UTC'), sent FROM campaigns
WHERE NOT evergreen AND sent > 0 AND started_at IS NOT NULL AND status IN ('finished', 'cancelled')
ON CONFLICT (campaign_id, hour) DO NOTHING`

const sendHoursBackfillEvergreens = `
INSERT INTO campaign_send_hours (campaign_id, hour, n)
SELECT campaign_id, DATE_TRUNC('hour', sent_at, 'UTC'), COUNT(*) FROM campaign_sends
WHERE sent_at IS NOT NULL
GROUP BY 1, 2
ON CONFLICT (campaign_id, hour) DO NOTHING`

// The D4 rows: the user's tranche log reconciled against the Younique account's SES hourly Send
// sums (read 2026-10-26; operational fact). Sum 46,272 = campaign 217's sent.
const sendHoursWinbackSeed = `
INSERT INTO campaign_send_hours (campaign_id, hour, n)
SELECT 217, v.hour, v.n FROM (VALUES
    ('2026-10-22 00:00:00+00'::TIMESTAMPTZ, 8107),
    ('2026-10-22 23:00:00+00'::TIMESTAMPTZ, 10999),
    ('2026-10-23 22:00:00+00'::TIMESTAMPTZ, 8433),
    ('2026-10-23 23:00:00+00'::TIMESTAMPTZ, 5587),
    ('2026-10-25 00:00:00+00'::TIMESTAMPTZ, 13146)
) AS v (hour, n)
WHERE EXISTS (SELECT 1 FROM campaigns WHERE id = 217 AND sent = 46272 AND status = 'paused')
ON CONFLICT (campaign_id, hour) DO NOTHING`

const sendHoursSkippedBroadcasts = `
SELECT id FROM campaigns
WHERE NOT evergreen AND sent > 0 AND NOT (started_at IS NOT NULL AND status IN ('finished', 'cancelled'))
ORDER BY id`
