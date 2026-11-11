package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_36 is a FORK migration (journey lists, integrations listmonk/JOURNEY-LISTS-SPEC D1/D6/D11),
// not an upstream release. In one transaction it:
//
//  1. adds three lists columns: trigger_active BOOLEAN NOT NULL DEFAULT false (the list's Active
//     switch -- a list's joins are live only while it is true), trigger_active_at TIMESTAMPTZ NULL
//     (the instant of the LAST change of the flag) and trigger_active_by TEXT NULL (who made it);
//  2. ONLY when step 1 actually added trigger_active (checked in information_schema.columns
//     before the ALTER, inside the same transaction), activates every ACTIVE (not archived) list
//     that carries a join_trigger, with NULL stamps -- nothing live changes at the release. A re-run finds the
//     column and activates nothing, so a list saved inactive or switched off since stays off;
//  3. grants lists:journeys to Super Admin (1) and the three system API roles BY ID (4
//     ManagePreferences API -- listmonk_sync, manage_preferences; 7 Re-save API --
//     listmonk_resave; 13 Sunset API -- listmonk_sunset), each only where it is missing. A
//     fresh install has no such API roles (cmd/install.go grants role 1 every permission), so
//     it is a no-op there.
//
// ROLLBACK: pin the previous image and re-apply the host. The older binary ignores the columns
// (the app's queries run on an Unsafe() handle) and the permission; no DDL reversal is needed.
//
// The version key sits after the fork's v6.2.35 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.36. Idempotent by construction.
func V6_2_36(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SET LOCAL lock_timeout = '5s'`); err != nil {
		return err
	}

	var had bool
	if err := tx.Get(&had, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'lists' AND column_name = 'trigger_active')`); err != nil {
		return err
	}
	if _, err := tx.Exec(listTriggerActiveDDL); err != nil {
		return err
	}
	if !had {
		if _, err := tx.Exec(listTriggerActivateExisting); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(listJourneysGrantDDL); err != nil {
		return err
	}
	return tx.Commit()
}

// listTriggerActiveDDL is mirrored in schema.sql (the last block). Keep the two identical.
const listTriggerActiveDDL = `
ALTER TABLE lists ADD COLUMN IF NOT EXISTS trigger_active BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE lists ADD COLUMN IF NOT EXISTS trigger_active_at TIMESTAMP WITH TIME ZONE NULL;
ALTER TABLE lists ADD COLUMN IF NOT EXISTS trigger_active_by TEXT NULL;
`

// listTriggerActivateExisting runs only when the ALTER added the column (V6_2_36 step 2).
// Review F4: an ARCHIVED triggered list is not activated (archiving turns Active off).
const listTriggerActivateExisting = `UPDATE lists SET trigger_active = true WHERE join_trigger IS NOT NULL AND status = 'active';`

// listJourneysGrantDDL grants lists:journeys by role id (review F2: a name match fails silently
// on a re-cased name). type = 'user' -- list roles never hold a global permission.
const listJourneysGrantDDL = `
UPDATE roles SET permissions = permissions || '{lists:journeys}'
WHERE id IN (1, 4, 7, 13) AND type = 'user' AND NOT permissions @> '{lists:journeys}';
`
