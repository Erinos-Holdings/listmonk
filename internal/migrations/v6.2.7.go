package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_7 is a FORK migration (erinos send retry, integrations SEND-RETRY-SPEC D5), not an
// upstream release. It adds campaign_send_failures — one row per recipient a campaign
// could not reach (a message that exhausted the SMTP pool's attempts, or whose template
// failed to render). Until this table existed the only trace of such a loss was a log line
// in a container, and a 10-recipient shortfall on 2026-09-10 was found by arithmetic.
//
// The version key sits between the fork's v6.2.6 and any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.7. Idempotent by construction.
func V6_2_7(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS campaign_send_failures (
		id            BIGSERIAL PRIMARY KEY,
		campaign_id   INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE ON UPDATE CASCADE,
		subscriber_id INTEGER NOT NULL,
		email         TEXT NOT NULL,
		stage         TEXT NOT NULL,
		error         TEXT NOT NULL,
		created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_campaign_send_failures_camp ON campaign_send_failures(campaign_id, created_at DESC);`)
	return err
}
