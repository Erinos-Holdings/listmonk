package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_8 is a FORK migration (editor polish, integrations EDITOR-POLISH-SPEC D4), not an
// upstream release. It adds templates.brand — the Templates form's "Brand swatches" dropdown
// selection, which today is session state only and is forgotten when the modal closes. Empty
// string means "no brand" (the dropdown's None sentinel), so no NULL handling anywhere.
//
// Editor-only metadata: it drives the Templates form's swatches and rebrand-sweep provenance
// seed and does not feed campaign brand derivation (a campaign's brand is derived from its
// target lists' brand:/from: tags, unrelated to this column).
//
// The version key sits between the fork's v6.2.7 and any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.8. Idempotent by construction.
func V6_2_8(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(`ALTER TABLE templates ADD COLUMN IF NOT EXISTS brand TEXT NOT NULL DEFAULT ''`)
	return err
}
