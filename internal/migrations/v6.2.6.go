package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_6 is a FORK migration (erinos click tracking, CLICK-TRACKING-SPEC §3.4), not an
// upstream release. No schema change; it adds the four settings behind personalized-button
// click tracking and redirect-time UTM tagging, each with its default (the host seed is a
// one-time UPDATE-only seed and cannot introduce keys — review F3):
//
//   - app.link_fallback_url — where an unresolvable dynamic link redirects when the campaign
//     has no brand site (unmapped / `curated` / mapping error); blank = public error page.
//   - app.utm_enable — UTM tagging on (D9: ships enabled).
//   - app.utm_hosts — extra storefront hosts beyond the sending domains and `site:` tags.
//   - app.utm_params — name → template of the appended parameters (D8 defaults).
//
// The version key sits between the fork's v6.2.5 and any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.6. Idempotent by construction.
func V6_2_6(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(`
	INSERT INTO settings (key, value) VALUES
		('app.link_fallback_url', '"https://curatedfor.you"'),
		('app.utm_enable', 'true'),
		('app.utm_hosts', '["curatedfor.you","curatedby.you"]'),
		('app.utm_params', '{"utm_source":"listmonk","utm_medium":"email","utm_campaign":"{{ .Campaign.Name }}","utm_content":"{{ .Campaign.ID }}"}')
	ON CONFLICT DO NOTHING;`)
	return err
}
