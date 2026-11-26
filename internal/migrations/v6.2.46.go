package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_46 is a FORK migration (the 60-day engaged count's speed, integrations
// listmonk/SILENT-PREVIEW-SPEED-SPEC D1), not an upstream release. Additive. In one transaction it
// creates idx_campaign_smart_skips_sub on campaign_smart_skips (subscriber_id, campaign_id): the
// table's primary key leads with campaign_id, so brand_silent_skips' per-member probe (by
// subscriber) walked the whole key on every member. No function, query or result changes. A plain
// (non-concurrent) build: migrations run in a transaction, and the table is small.
//
// ROLLBACK: pin the previous image (erinos.525). The index is inert to it and may stay; dropping it
// is optional:
//
//	DROP INDEX IF EXISTS idx_campaign_smart_skips_sub;
//
// The version key sits after the fork's v6.2.45 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.46. Idempotent by construction.
func V6_2_46(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(smartSkipsSubscriberIndexDDL); err != nil {
		return err
	}
	return tx.Commit()
}

// smartSkipsSubscriberIndexDDL is mirrored in schema.sql (keep the two identical; v6_2_46_db_test.go
// asserts it verbatim).
const smartSkipsSubscriberIndexDDL = `
CREATE INDEX IF NOT EXISTS idx_campaign_smart_skips_sub ON campaign_smart_skips (subscriber_id, campaign_id);
`
