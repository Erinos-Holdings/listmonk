package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_4 is a FORK migration (erinos footer start-guard), not an upstream release.
// It adds only the app.required_footer_markers setting, a JSON array of strings that
// must appear in a campaign's rendered body before it can be started or scheduled.
// Seeded EMPTY, which skips the marker check entirely and leaves the guard shipping
// inert (only the unsubscribe-link rule runs) until the language-aware footer is applied
// to every live template. The version key sits between the fork's v6.2.3 and any future
// upstream v6.3.0; re-key in the same rebase if upstream ships a v6.2.4.
// Idempotent by construction.
func V6_2_4(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('app.required_footer_markers', '[]') ON CONFLICT DO NOTHING;`)
	return err
}
