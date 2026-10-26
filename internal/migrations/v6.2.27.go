package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_27 is a FORK migration (on-demand brand-health refresh, integrations
// BRANDHEALTH-REFRESH-SPEC D1), not an upstream release. It adds the settings
// app.health_refresh_url + app.health_refresh_hmac_secret, seeded to the empty string and only
// when absent -- the SES pane's "Recompute now" button and POST /api/system/health/refresh are
// inert until both are pasted into Settings (the go-live is that settings save, an app reload).
// The secret is a SECRET setting: masked on GET and resolved on PUT (cmd/settings.go).
//
// ROLLBACK: pin the previous image. The two rows are inert to an older binary (it neither
// marshals the keys nor reads them).
//
// The version key sits after the fork's v6.2.26 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.27. Idempotent by construction.
func V6_2_27(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(healthRefreshDataDDL)
	return err
}

// healthRefreshDataDDL is mirrored in schema.sql's settings seed. A fresh install gets the keys
// from there.
const healthRefreshDataDDL = `
INSERT INTO settings (key, value) VALUES
    ('app.health_refresh_url', '""'),
    ('app.health_refresh_hmac_secret', '""')
ON CONFLICT DO NOTHING;
`
