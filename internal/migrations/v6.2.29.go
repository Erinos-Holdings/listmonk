package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_29 is a FORK migration (usage decisions, integrations USAGE-DECISIONS-SPEC D3/D4), not an
// upstream release. It creates usage_decisions -- Marketing's accept/reject record of the lifecycle
// series items on Brands > Usage (the texts are internal/decisions/lifecycle.json) -- with an
// append-only trigger that refuses UPDATE, DELETE and TRUNCATE at the statement level, so an app
// bug cannot rewrite history (the fork's queries hold only an INSERT and a SELECT for it). No
// foreign key to users: a deleted user's history survives, and user_name is a snapshot. It also
// grants the new brands:decide permission to Super Admin (role 1), keeping the v6.2.17 invariant
// (TestSuperAdminHoldsEveryPermission). cmd/usage_decisions.go::canDecide admits Super Admin by
// role either way (D3, amended 2026-10-30).
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary never reads or writes
// the table, and an unknown permission on role 1 is inert; no DDL reversal is needed.
//
// The version key sits after the fork's v6.2.28 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.29. Idempotent by construction (IF NOT EXISTS, OR
// REPLACE, the guarded grant).
func V6_2_29(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}
	if _, err := tx.Exec(usageDecisionsDDL); err != nil {
		return err
	}
	if _, err := tx.Exec(brandsDecideGrantDDL); err != nil {
		return err
	}

	return tx.Commit()
}

// usageDecisionsDDL is mirrored in schema.sql (after its DROP TABLE IF EXISTS line). Keep the two
// identical. The function is CREATE OR REPLACE in both: schema.sql's DROP TABLE ... CASCADE drops
// the trigger but not the function.
const usageDecisionsDDL = `
CREATE TABLE IF NOT EXISTS usage_decisions (
    id               BIGSERIAL PRIMARY KEY,
    item_key         TEXT NOT NULL,
    text_hash        CHAR(64) NOT NULL,
    decision         TEXT NOT NULL CHECK (decision IN ('accept', 'reject')),
    note             TEXT NULL,
    user_id          INTEGER NOT NULL,
    user_name        TEXT NOT NULL,
    created_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE OR REPLACE FUNCTION usage_decisions_append_only() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'usage_decisions is append-only: % refused', TG_OP;
END;
$$ LANGUAGE plpgsql;
CREATE OR REPLACE TRIGGER usage_decisions_append_only
    BEFORE UPDATE OR DELETE OR TRUNCATE ON usage_decisions
    FOR EACH STATEMENT EXECUTE FUNCTION usage_decisions_append_only();
`

// brandsDecideGrantDDL is the v6.2.14 grant pattern. A fresh install needs nothing: schema.sql
// carries no role rows, and cmd/install.go grants role 1 every permission.
const brandsDecideGrantDDL = `
UPDATE roles SET permissions = permissions || '{brands:decide}' WHERE id = 1 AND NOT permissions @> '{brands:decide}';
`
