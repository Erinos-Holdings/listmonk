package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_5 is a FORK migration (erinos import presets), not an upstream release.
// It adds only the app.import_presets setting, a JSON array of import presets (see
// internal/subimporter/preset.go). Seeded EMPTY, which hides the feature entirely; the
// presets themselves are data applied by the host's bootstrap, never by a migration, so
// nothing deployment-specific lives in this tree. The version key sits between the fork's
// v6.2.4 and any future upstream v6.3.0; re-key in the same rebase if upstream ships a
// v6.2.5. Idempotent by construction.
func V6_2_5(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('app.import_presets', '[]') ON CONFLICT DO NOTHING;`)
	return err
}
