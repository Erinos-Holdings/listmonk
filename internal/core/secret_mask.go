package core

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// SecretMask is the rune GET /api/settings substitutes for every rune of a stored secret
// (SMTP passwords, bounce mailbox passwords, S3 secret key, OIDC client secret, ...).
//
// Fork (2026-09-03): it is also a value the settings update path REFUSES to store. Upstream
// preserves a stored secret only when the client sends an empty string, and relies on the
// admin UI to strip the mask before posting. Any other client that round-trips the GET blob
// into PUT /api/settings -- a curl, a script re-applying one unrelated setting -- therefore
// replaced the live SES SMTP credential with 44 mask characters and every send failed with
// `535 Authentication Credentials Invalid` until the next config re-apply. The server, not
// the client, now owns that rule; see ResolveSecret.
const SecretMask = "•"

// ErrSecretContainsMask is returned by ResolveSecret for a value that mixes the display mask
// with other characters. Such a value can only be a corrupted round-trip of the display
// form, so it is refused rather than stored or silently replaced.
var ErrSecretContainsMask = errors.New("value contains the secret display mask character; re-enter the full secret or leave the field empty to keep the stored one")

// MaskSecret returns the display form of a stored secret: one SecretMask per rune, so the
// UI can show that a value exists (and roughly how long it is) without disclosing it.
func MaskSecret(secret string) string {
	return strings.Repeat(SecretMask, utf8.RuneCountInString(secret))
}

// ResolveSecret decides what a settings save stores for one secret field, given the
// incoming value and the currently stored one:
//
//   - "" or a value made only of SecretMask runes (any length) -> current, unchanged;
//   - a value that merely contains SecretMask -> ErrSecretContainsMask;
//   - anything else -> incoming, the new secret.
func ResolveSecret(incoming, current string) (string, error) {
	if incoming == "" || isAllMask(incoming) {
		return current, nil
	}
	if strings.Contains(incoming, SecretMask) {
		return "", ErrSecretContainsMask
	}
	return incoming, nil
}

// IsSecretMask reports whether s is a non-empty string made only of SecretMask runes -- the
// display form of some stored secret, never a secret itself.
func IsSecretMask(s string) bool {
	return s != "" && strings.Trim(s, SecretMask) == ""
}

func isAllMask(s string) bool { return IsSecretMask(s) }
