package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_24 is a FORK migration (shop-link rate, integrations SHOP-LINK-RATE-SPEC D1), not an
// upstream release. It creates campaign_shop_link_audience: one row per (campaign, brand, kind)
// holding how many of a broadcast's audience held that shop-link kind for that brand when the
// campaign was claimed (written once by the manager's store at claim; the Shop Link Attribution
// chart's rate denominator). No data row is written here.
//
// One transaction with SET LOCAL lock_timeout = '5s' (the v6.2.23 shape). The migration carries
// the CREATE only; schema.sql's mirror block is the same CREATE preceded by a DROP TABLE IF EXISTS
// (the brands / v6.2.20 convention) -- never a DROP in a migration.
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary never reads the table;
// no DDL reversal is needed.
//
// The version key sits after the fork's v6.2.23 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.24. Idempotent by construction.
func V6_2_24(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(shopLinkAudienceDDL); err != nil {
		return err
	}
	return tx.Commit()
}

// shopLinkAudienceDDL is mirrored in schema.sql (the last block, after its DROP TABLE IF EXISTS
// line). Keep the two identical.
const shopLinkAudienceDDL = `
CREATE TABLE IF NOT EXISTS campaign_shop_link_audience (
    campaign_id      INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    brand            TEXT NOT NULL,
    kind             TEXT NOT NULL,
    audience         INTEGER NOT NULL,
    taken_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (campaign_id, brand, kind)
);
`
