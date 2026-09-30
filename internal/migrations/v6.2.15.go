package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_15 is a FORK migration (inspect scope, integrations INSPECT-SCOPE-SPEC §2.2/§2.4), not an
// upstream release. It adds:
//
//   - campaign_structure_records: APPEND-ONLY clean rendering-matrix records (S13) -- a surrogate
//     id, many rows per structure fingerprint, so a stage-2 record never overwrites a stage-1 one.
//     Each row carries the S3-normalized components, the server render canary at record time, the
//     Inspect clients the test COMPLETED, the live roster at record time, the stage ('1', '2',
//     'all') and the modes. The legacy campaign_structure_verifications table (v6.2.14) is left
//     untouched: the older image still upserts into it, and the review Lambda's legacy D4 path
//     (STRUCTURE_GATE off) still reads it.
//   - the permission campaigns:review_structure lives in permissions.json. This migration grants
//     it to NO role -- which was a BUG, corrected by v6.2.17: the assumption "Super Admin holds
//     every permission implicitly (auth.User.HasPerm)" is false on an upgraded database, because
//     core.setupUserFields zeroes User.UserRoleID after copying it into UserRole.ID, so the
//     short-circuit never fires and Super Admin holds exactly the permissions stored on role 1.
//     Only cmd/install.go (a fresh install) grants role 1 everything; an upgrade must grant each
//     new permission by migration -- the v6.2.14 pattern, which v6.2.17 applies. The "Re-save API"
//     role (the record script's credential) is granted it BY HAND at release.
//
// ROLLBACK: the table and the new routes are inert to an older binary, but a role carrying
// campaigns:review_structure cannot be re-saved on one (role save validates against that binary's
// permissions.json) -- strip it first with
//
//	UPDATE roles SET permissions = array_remove(permissions, 'campaigns:review_structure');
//
// The version key sits after the fork's v6.2.14 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.15. Idempotent by construction.
func V6_2_15(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(structureRecordsDDL)
	return err
}

// structureRecordsDDL is mirrored in schema.sql. Keep the two identical.
const structureRecordsDDL = `
CREATE TABLE IF NOT EXISTS campaign_structure_records (
    id               BIGSERIAL PRIMARY KEY,
    fingerprint      TEXT NOT NULL,
    campaign_id      INTEGER NULL,
    verified_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    verified_by      TEXT NOT NULL DEFAULT '',
    test_id          TEXT NOT NULL,
    components       JSONB NOT NULL DEFAULT '{}',
    canary           JSONB NOT NULL DEFAULT '{}',
    clients          TEXT[] NOT NULL DEFAULT '{}',
    roster           TEXT[] NOT NULL DEFAULT '{}',
    stage            TEXT NOT NULL CHECK (stage IN ('1', '2', 'all')),
    modes            TEXT[] NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_campaign_structure_records_fingerprint ON campaign_structure_records (fingerprint);
`
