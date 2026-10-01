package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V6_2_18 is a FORK migration (two-factor, integrations PASSKEY-2FA-SPEC D1), not an upstream
// release. It adds:
//
//   - user_passkeys: one row per WebAuthn credential (the second factor beside TOTP), the
//     library's whole webauthn.Credential stored as JSONB so its flags and counter round-trip.
//     "Has a passkey" is derived from this table; the twofa_type enum is NOT changed (an enum
//     value cannot be removed by pinning the previous image).
//   - settings security.require_twofa, seeded false and only when absent -- enforcement ships
//     OFF and is turned on as a separate approved settings save.
//
// ROLLBACK: pin the previous image. The table and the settings key are inert to an older binary
// (it neither reads the table nor marshals the key, so a settings save there leaves the row
// alone). Note the security effect: the older binary knows nothing of passkeys or the switch, so
// a passkey-only user logs in on the password alone until the release is restored.
//
// The version key sits after the fork's v6.2.17 and before any future upstream v6.3.0; re-key in
// the same rebase if upstream ships a v6.2.18. Idempotent by construction.
func V6_2_18(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
	_, err := db.Exec(passkeysDDL + passkeysDataDDL)
	return err
}

// passkeysDDL is mirrored in schema.sql. Keep the two identical.
const passkeysDDL = `
CREATE TABLE IF NOT EXISTS user_passkeys (
    id               SERIAL PRIMARY KEY,
    user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id    BYTEA NOT NULL UNIQUE,
    credential       JSONB NOT NULL,
    name             TEXT NOT NULL,
    created_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_used_at     TIMESTAMP WITH TIME ZONE NULL
);
CREATE INDEX IF NOT EXISTS idx_user_passkeys_user ON user_passkeys (user_id);
`

// passkeysDataDDL is the data half. A fresh install gets the setting from schema.sql.
const passkeysDataDDL = `
INSERT INTO settings (key, value) VALUES ('security.require_twofa', 'false') ON CONFLICT DO NOTHING;
`
