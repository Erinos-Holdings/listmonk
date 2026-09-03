package core

import (
	"errors"
	"strings"
	"testing"
)

// The invariant these pin: a settings save can never store the display mask as a secret,
// whatever client sent it. 2026-09-03: a full-blob PUT /api/settings stored 44 mask runes
// as the SES SMTP password and every campaign send failed 535 (LISTMONK-RUNBOOK hazard 49).
func TestResolveSecret(t *testing.T) {
	const stored = "AbCd/EfGh+IjKl0123456789abcdefghijklmnopqrst" // 44 chars, like an SES SMTP password

	cases := []struct {
		name     string
		incoming string
		want     string
		wantErr  error
	}{
		{"empty keeps stored", "", stored, nil},
		{"exact-length mask keeps stored", MaskSecret(stored), stored, nil},
		{"mask of another length keeps stored", strings.Repeat(SecretMask, 5), stored, nil},
		{"single mask rune keeps stored", SecretMask, stored, nil},
		{"new value replaces stored", "new-secret-value", "new-secret-value", nil},
		{"same value as stored is stored", stored, stored, nil},
		{"multibyte secret is stored verbatim", "pässwörd•free?no:ünïcode", "", ErrSecretContainsMask},
		{"mask prefix is refused", SecretMask + "tail", "", ErrSecretContainsMask},
		{"mask suffix is refused", "head" + SecretMask, "", ErrSecretContainsMask},
		{"mask inside is refused", "he" + SecretMask + "ad", "", ErrSecretContainsMask},
		{"whitespace-only is a real (bad) value, not a mask", "   ", "   ", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveSecret(tc.incoming, stored)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// With nothing stored, the mask (which the UI would never show for an empty secret) still
// resolves to the stored empty value rather than being persisted.
func TestResolveSecretNoStoredValue(t *testing.T) {
	got, err := ResolveSecret(strings.Repeat(SecretMask, 3), "")
	if err != nil || got != "" {
		t.Fatalf("got %q, %v; want \"\", nil", got, err)
	}
}

// MaskSecret must produce exactly what ResolveSecret recognises, per rune not per byte:
// a multibyte secret masks to the same rune count and round-trips to unchanged.
func TestMaskSecretRoundTrip(t *testing.T) {
	for _, s := range []string{"", "a", "pässwörd", "日本語のパスワード", strings.Repeat("x", 44)} {
		m := MaskSecret(s)
		if got := len([]rune(m)); got != len([]rune(s)) {
			t.Fatalf("MaskSecret(%q) has %d runes, want %d", s, got, len([]rune(s)))
		}
		if got, err := ResolveSecret(m, s); err != nil || got != s {
			t.Fatalf("ResolveSecret(mask(%q)) = %q, %v; want unchanged", s, got, err)
		}
	}
}
