package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_3 is a FORK migration (erinos multi-language campaigns), not an upstream release.
// Language-scoped campaigns need no schema change (campaigns.attribs.lang and
// subscribers.attribs.lang are JSONB keys); this adds only the app.lang_enable setting
// (default false) that hides the campaign form's Language select until the subscriber
// population carries languages. The version key sits between the fork's v6.2.2 and any
// future upstream v6.3.0; re-key in the same rebase if upstream ships a v6.2.3.
// Idempotent by construction.
func V6_2_3(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('app.lang_enable', 'false') ON CONFLICT DO NOTHING;`)
	return err
}
