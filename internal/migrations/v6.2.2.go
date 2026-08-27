package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_2 is a FORK migration (erinos evergreen campaigns), not an upstream release.
// It adds everything an "evergreen" campaign needs — a campaign that, once started,
// never finishes and keeps sending to subscribers who join its target list after it
// started (welcome emails; later drip steps and A/B variants):
//
//   - campaigns.evergreen / send_delay_secs, plus three RESERVED nullable columns
//     (parent_campaign_id, variant_group_id, variant_index) that nothing reads yet.
//   - subscriber_lists.confirmed_at, stamped by trigger ONLY on the row's transition
//     into 'confirmed' — never on an overwrite of an already-confirmed row, and never
//     while the session setting listmonk.backfill is 'true' (bulk imports, replays).
//     A NULL confirmed_at is never eligible for an evergreen send.
//   - campaign_sends, the per-(campaign, subscriber) send history -- claimed_at at
//     fetch, sent_at at the delivery attempt, deleted only when a queued message is
//     dropped unattempted (see schema.sql).
//   - the app.evergreen_enable setting (default false).
//
// Existing confirmed rows get confirmed_at := updated_at — they all predate any
// evergreen's started_at watermark, so the exact value is irrelevant; it only has
// to be non-null so a later un/re-subscribe transition behaves like any other row.
//
// The version key sits between the fork's v6.2.1 and any future upstream v6.3.0.
// If upstream ever ships its own v6.2.2, re-key this one in the same rebase.
// Idempotent by construction.
func V6_2_2(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	if _, err := db.Exec(`
		ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS evergreen BOOLEAN NOT NULL DEFAULT false;
		ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS send_delay_secs BIGINT NOT NULL DEFAULT 0;
		ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS parent_campaign_id INTEGER NULL REFERENCES campaigns(id) ON DELETE SET NULL;
		ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS variant_group_id UUID NULL;
		ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS variant_index INTEGER NULL;

		ALTER TABLE subscriber_lists ADD COLUMN IF NOT EXISTS confirmed_at TIMESTAMP WITH TIME ZONE NULL;
		UPDATE subscriber_lists SET confirmed_at = updated_at WHERE status = 'confirmed' AND confirmed_at IS NULL;

		CREATE OR REPLACE FUNCTION subscriber_lists_stamp_confirmed_at() RETURNS TRIGGER AS $$
		BEGIN
			IF NEW.status = 'confirmed'
				AND (TG_OP = 'INSERT' OR OLD.status IS DISTINCT FROM 'confirmed')
				AND COALESCE(current_setting('listmonk.backfill', true), '') <> 'true' THEN
				NEW.confirmed_at := NOW();
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS subscriber_lists_confirmed_at ON subscriber_lists;
		CREATE TRIGGER subscriber_lists_confirmed_at
			BEFORE INSERT OR UPDATE OF status ON subscriber_lists
			FOR EACH ROW EXECUTE FUNCTION subscriber_lists_stamp_confirmed_at();

		CREATE TABLE IF NOT EXISTS campaign_sends (
			campaign_id   INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE ON UPDATE CASCADE,
			subscriber_id INTEGER NOT NULL,
			claimed_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			sent_at       TIMESTAMP WITH TIME ZONE NULL
		);
		CREATE INDEX IF NOT EXISTS idx_campaign_sends_camp_sub ON campaign_sends(campaign_id, subscriber_id, claimed_at DESC);
		CREATE INDEX IF NOT EXISTS idx_campaign_sends_sub_camp ON campaign_sends(subscriber_id, campaign_id);
		CREATE INDEX IF NOT EXISTS idx_sub_lists_confirmed_at ON subscriber_lists(list_id, confirmed_at) WHERE confirmed_at IS NOT NULL;

		INSERT INTO settings (key, value) VALUES ('app.evergreen_enable', 'false') ON CONFLICT DO NOTHING;
	`); err != nil {
		return err
	}
	return nil
}
