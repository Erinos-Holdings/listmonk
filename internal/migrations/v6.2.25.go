package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_25 is a FORK migration (shop-link frozen, integrations SHOP-LINK-FROZEN-SPEC D1), not an
// upstream release. It creates campaign_shop_link_recipients: one row per (campaign, subscriber,
// brand) holding the shop-link kind that recipient held for that brand when the broadcast was
// claimed (written by the manager's store in the same statement as the campaign_shop_link_audience
// summary; the Shop Link Attribution chart buckets clicks through it). No data row is written here.
//
// No foreign key to subscribers (Stage 2 F1): a cascade from subscribers would scan this table on
// every subscriber delete, and a deleted subscriber's clicks already read unknown (link_clicks'
// subscriber_id is set NULL), so the row is never joined again. The primary key is the only index.
//
// One transaction with SET LOCAL lock_timeout = '5s' (the v6.2.24 shape). The migration carries
// the CREATE only; schema.sql's mirror block is the same CREATE preceded by a DROP TABLE IF EXISTS
// -- never a DROP in a migration.
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary never reads the table;
// no DDL reversal is needed.
//
// The version key sits after the fork's v6.2.24 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.25. Idempotent by construction.
func V6_2_25(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(shopLinkRecipientsDDL); err != nil {
		return err
	}
	return tx.Commit()
}

// shopLinkRecipientsDDL is mirrored in schema.sql (after its DROP TABLE IF EXISTS line). Keep the
// two identical.
const shopLinkRecipientsDDL = `
CREATE TABLE IF NOT EXISTS campaign_shop_link_recipients (
    campaign_id      INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    subscriber_id    INTEGER NOT NULL,
    brand            TEXT NOT NULL,
    kind             TEXT NOT NULL,
    PRIMARY KEY (campaign_id, subscriber_id, brand)
);
`
