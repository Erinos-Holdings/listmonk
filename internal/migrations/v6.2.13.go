package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_13 is a FORK migration (system health, integrations SES-HEALTH-SPEC D1/D6), not an
// upstream release. It adds one generic table:
//
//   - system_health (kind, day): one computed, system-wide document per kind per day, stored
//     verbatim as JSONB -- the brand_health pattern without a brand. The fork stores and renders
//     it and knows nothing about what a kind means (no AWS, no thresholds, no brands); the
//     integrations BrandHealth Lambda computes the documents and writes them through
//     PUT /api/system/health. First kind -- the SES account row on the Dashboard.
//
// No role or permission change: the endpoints reuse brands:get / brands:manage (v6.2.12).
//
// ROLLBACK: the table and the new routes are inert to an older binary.
//
// The version key sits after the fork's v6.2.12 and before any future upstream v6.3.0; re-key
// in the same rebase if upstream ships a v6.2.13. Idempotent by construction.
func V6_2_13(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(systemHealthDDL)
	return err
}

// systemHealthDDL is mirrored in schema.sql. Keep the two identical.
const systemHealthDDL = `
CREATE TABLE IF NOT EXISTS system_health (
    kind             TEXT NOT NULL,
    day              DATE NOT NULL,
    status           TEXT NOT NULL,
    doc              JSONB NOT NULL,
    computed_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (kind, day)
);
CREATE INDEX IF NOT EXISTS idx_system_health_kind_day ON system_health (kind, day DESC);
`
