package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_1 is a FORK migration (erinos template freeze), not an upstream release.
// It adds campaigns.frozen_template_body: the resolved template body snapshotted
// onto the campaign row on its first transition to 'running', after which the
// campaign renders from the snapshot forever — editing the shared template no
// longer changes what an approved/sent campaign renders. NULL means "never ran"
// (drafts, scheduled-but-not-started) and those keep rendering the live template.
//
// The version key sits between upstream's v6.2.0 and any future v6.3.0, so on a
// rebase this runs (or has already run) before upstream's next migration. If
// upstream ever ships its own v6.2.1, re-key this one in the same rebase.
// Idempotent by construction.
func V6_2_1(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	if _, err := db.Exec(`
		ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS frozen_template_body TEXT NULL;
	`); err != nil {
		return err
	}
	return nil
}
