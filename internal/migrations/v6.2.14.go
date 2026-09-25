package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_14 is a FORK migration (campaign review, integrations CAMPAIGN-INSPECT-SPEC D5/D6), not an
// upstream release. It adds:
//
//   - campaign_reviews: one row per inspection (at most ONE running row per campaign -- a partial
//     unique index, so two simultaneous Inspect clicks cannot both start a job) (the review Lambda's report stored verbatim as
//     JSONB), keyed by campaign + bundle hash; the status gate reads the newest `complete` row for
//     the campaign's CURRENT hash (internal/review.BundleHash).
//   - campaign_review_dispositions: the Fix for me / I'll fix it / Accept risk decisions, APPEND-ONLY
//     -- this table IS the Accept-risk log (parent spec §6); the latest row per item key wins.
//   - campaign_structure_verifications: fingerprint -> clean rendering-matrix record (parent I8).
//   - settings app.review_url + app.review_hmac_secret, seeded to the empty string -- the gate is ON only while
//     app.review_url is non-empty, so the release is dark until the go-live settings save (D19).
//   - campaigns:review for Super Admin (role 1), the v6.2.12 grant pattern. The review Lambda's
//     "Review API" role gets it by hand (a human step), as does "Re-save API" (D16).
//
// ROLLBACK: the tables, the settings keys and the new routes are inert to an older binary, but a
// role carrying campaigns:review cannot be re-saved on one (role save validates against that
// binary's permissions.json) -- strip it with
//
//	UPDATE roles SET permissions = array_remove(permissions, 'campaigns:review');
//
// if a role edit is needed before re-release.
//
// The version key sits after the fork's v6.2.13 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.14. Idempotent by construction.
func V6_2_14(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(campaignReviewDDL + campaignReviewDataDDL)
	return err
}

// campaignReviewDDL is mirrored in schema.sql. Keep the two identical.
const campaignReviewDDL = `
CREATE TABLE IF NOT EXISTS campaign_reviews (
    id               BIGSERIAL PRIMARY KEY,
    campaign_id      INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE ON UPDATE CASCADE,
    bundle_hash      TEXT NOT NULL,
    job_id           UUID NOT NULL UNIQUE,
    status           TEXT NOT NULL CHECK (status IN ('running', 'complete', 'failed', 'stale')),
    progress         JSONB NOT NULL DEFAULT '{}',
    report           JSONB NULL,
    error            TEXT NOT NULL DEFAULT '',
    requested_by     TEXT NOT NULL DEFAULT '',
    requested_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_campaign_reviews_campaign ON campaign_reviews (campaign_id, requested_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_campaign_reviews_one_running ON campaign_reviews (campaign_id) WHERE status = 'running';

CREATE TABLE IF NOT EXISTS campaign_review_dispositions (
    id               BIGSERIAL PRIMARY KEY,
    campaign_id      INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE ON UPDATE CASCADE,
    bundle_hash      TEXT NOT NULL,
    item_key         TEXT NOT NULL,
    rubric_id        TEXT NOT NULL,
    action           TEXT NOT NULL CHECK (action IN ('accept', 'fixme', 'fixed')),
    note             TEXT NOT NULL DEFAULT '',
    user_id          INTEGER NULL,
    username         TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_campaign_review_dispositions ON campaign_review_dispositions (campaign_id, item_key, created_at DESC);

CREATE TABLE IF NOT EXISTS campaign_structure_verifications (
    fingerprint      TEXT PRIMARY KEY,
    verified_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    test_id          TEXT NOT NULL,
    components       JSONB NOT NULL DEFAULT '{}',
    verified_by      TEXT NOT NULL DEFAULT ''
);
`

// campaignReviewDataDDL is the data half. A fresh install gets the settings from schema.sql and
// every permission for Super Admin from cmd/install.go.
const campaignReviewDataDDL = `
INSERT INTO settings (key, value) VALUES
    ('app.review_url', '""'),
    ('app.review_hmac_secret', '""')
ON CONFLICT DO NOTHING;
UPDATE roles SET permissions = permissions || '{campaigns:review}' WHERE id = 1 AND NOT permissions @> '{campaigns:review}';
`
