package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_9 is a FORK migration (media tags, integrations MEDIA-TAGS-SPEC D1), not an upstream
// release. It adds media.tags — bare lowercase slugs (models.NormalizeMediaTags) the media
// library filters on with OR (overlap) semantics — and a GIN index for the array operators.
// Same column type as lists.tags / campaigns.tags. An empty array means "untagged", so no
// NULL handling anywhere.
//
// The version key sits between the fork's v6.2.8 and any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.9. Idempotent by construction.
func V6_2_9(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	if _, err := db.Exec(`ALTER TABLE media ADD COLUMN IF NOT EXISTS tags VARCHAR(100)[] NOT NULL DEFAULT '{}'`); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_media_tags ON media USING GIN(tags)`)
	return err
}
