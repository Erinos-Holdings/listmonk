package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_28 is a FORK migration (campaign conversions, integrations CAMPAIGN-CONVERSIONS-SPEC D6),
// not an upstream release. It creates campaign_conversions: the Tracked and Matched orders the
// integrations CampaignConversions Lambda computes, at most one row per (measure, order) -- the
// primary key is what makes every dashboard scope a plain sum. The fork stores and sums them; it
// computes no attribution and holds no rule. PUT /api/campaigns/conversions replaces one measure's
// rows in one transaction.
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary never reads or writes
// the table; no DDL reversal is needed.
//
// The version key sits after the fork's v6.2.27 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.28. Idempotent by construction (IF NOT EXISTS).
func V6_2_28(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(campaignConversionsDDL); err != nil {
		return err
	}

	return tx.Commit()
}

// campaignConversionsDDL is mirrored in schema.sql (after its DROP TABLE IF EXISTS line). Keep the
// two identical.
const campaignConversionsDDL = `
CREATE TABLE IF NOT EXISTS campaign_conversions (
    measure          TEXT NOT NULL CHECK (measure IN ('tracked', 'matched')),
    order_id         TEXT NOT NULL,
    campaign_id      INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    brand            TEXT NOT NULL,
    purchased_at     TIMESTAMP WITH TIME ZONE NOT NULL,
    sales_usd        NUMERIC(12, 2) NOT NULL CHECK (sales_usd >= 0),
    PRIMARY KEY (measure, order_id)
);
CREATE INDEX IF NOT EXISTS idx_campaign_conversions_campaign ON campaign_conversions (campaign_id, measure);
`
