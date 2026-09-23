package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_12 is a FORK migration (brand health, integrations BRAND-HEALTH-SPEC D2/D3/D10), not an
// upstream release. It adds:
//
//   - brand_health (brand, day): one computed document per brand per day, stored verbatim as
//     JSONB. The fork stores and renders it and computes nothing (D1): every rule, threshold and
//     Google/DNSBL read lives in the integrations BrandHealth Lambda, which writes the rows
//     through PUT /api/brands/health. is_default is a column (not only a document key) so the
//     lists join can find the default-sender row without a brand slug in fork SQL (D3).
//   - list_brand_tag(tags): the value of a list's `brand:` tag, or NULL when untagged. The ONE
//     statement of that rule in SQL, shared by query-lists (the Lists chip, D11) and the
//     brands queries (lists[] per brand). Tags are stored trimmed (models.TrimListTag).
//   - brands:get + brands:manage for Super Admin (role 1), and brands:get for every user role
//     that already holds lists:get_all (D10 -- a read grant only; no role gains brands:manage).
//     The v5.0.0 idempotent grant pattern.
//
// ROLLBACK: the table, the function and the new routes are inert to an older binary, but a role
// carrying brands:* cannot be re-saved on one (role save validates against that binary's
// permissions.json) -- strip them with one UPDATE if a role edit is needed before re-release.
//
// The version key sits after the fork's v6.2.11 and before any future upstream v6.3.0; re-key
// in the same rebase if upstream ships a v6.2.12. Idempotent by construction.
func V6_2_12(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(brandHealthDDL + brandHealthPermsDDL)
	return err
}

// brandHealthDDL is mirrored in schema.sql. Keep the two identical.
const brandHealthDDL = `
CREATE TABLE IF NOT EXISTS brand_health (
    brand            TEXT NOT NULL,
    day              DATE NOT NULL,
    status           TEXT NOT NULL,
    is_default       BOOLEAN NOT NULL DEFAULT FALSE,
    doc              JSONB NOT NULL,
    computed_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (brand, day)
);
CREATE INDEX IF NOT EXISTS idx_brand_health_brand_day ON brand_health (brand, day DESC);

CREATE OR REPLACE FUNCTION list_brand_tag(tags VARCHAR[]) RETURNS TEXT AS $$
    SELECT NULLIF(SUBSTRING(t FROM 7), '') FROM UNNEST(tags) AS t WHERE t LIKE 'brand:%' LIMIT 1;
$$ LANGUAGE sql IMMUTABLE;
`

// brandHealthPermsDDL is the data half. A fresh install needs none of it: cmd/install.go gives
// Super Admin every permission in permissions.json, and no other role exists yet.
const brandHealthPermsDDL = `
UPDATE roles SET permissions = permissions || '{brands:get}' WHERE id = 1 AND NOT permissions @> '{brands:get}';
UPDATE roles SET permissions = permissions || '{brands:manage}' WHERE id = 1 AND NOT permissions @> '{brands:manage}';
UPDATE roles SET permissions = permissions || '{brands:get}'
    WHERE type = 'user' AND permissions @> '{lists:get_all}' AND NOT permissions @> '{brands:get}';
`
