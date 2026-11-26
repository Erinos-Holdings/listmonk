package models

// Fork (approved brand logos) -- integrations LOGO-APPROVAL-SPEC I4: LogoURLProblem, the one rule on
// an approved or rejected logo entry, refuses an empty, untrimmed, relative, non-http(s), host-less or
// over-2,048-byte URL and accepts an https media URL.

import (
	"strings"
	"testing"
)

func TestLogoURLProblem(t *testing.T) {
	ok := []string{
		"https://email.curatedfor.you/uploads/202611-BFCM-Drop-4-Email-EN_01.png",
		"http://media.example.test/logo.jpg",
		"HTTPS://media.example.test/logo.jpg",
		"https://media.example.test:8443/a/b.png?v=2#x",
		"https://example.test/" + strings.Repeat("a", BrandLogoURLMax-len("https://example.test/")),
	}
	for _, u := range ok {
		if k := LogoURLProblem(u); k != "" {
			t.Errorf("%q refused: %s", u, k)
		}
	}

	bad := map[string]string{
		"":                                "brands.logoURLEmpty",
		" https://example.test/logo.png":  "brands.logoURLInvalid",
		"https://example.test/logo.png\n": "brands.logoURLInvalid",
		"   ":                             "brands.logoURLInvalid",
		"/uploads/logo.png":               "brands.logoURLInvalid",
		"uploads/logo.png":                "brands.logoURLInvalid",
		"//example.test/logo.png":         "brands.logoURLInvalid",
		"ftp://example.test/logo.png":     "brands.logoURLInvalid",
		"javascript:alert(1)":             "brands.logoURLInvalid",
		"data:image/png;base64,AAAA":      "brands.logoURLInvalid",
		"https:///logo.png":               "brands.logoURLInvalid",
		"https:logo.png":                  "brands.logoURLInvalid",
		"https://":                        "brands.logoURLInvalid",
		"https://exa mple.test/logo.png":  "brands.logoURLInvalid",
		"https://example.test/" + strings.Repeat("a", BrandLogoURLMax): "brands.logoURLTooLong",
	}
	for u, want := range bad {
		if got := LogoURLProblem(u); got != want {
			t.Errorf("%q: got %q, want %q", u, got, want)
		}
	}
	if BrandLogoURLMax != 2048 {
		t.Fatalf("BrandLogoURLMax = %d, want 2048", BrandLogoURLMax)
	}
}
