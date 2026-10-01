package main

// Fork (password policy) -- integrations PASSWORD-POLICY-SPEC I1: validatePassword implements
// D1/D2 exactly. No database; runs in CI.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestValidatePassword(t *testing.T) {
	// A 16-code-point password of 2-byte characters (32 bytes) with all four classes.
	multiByte := "Ééééééééééééé1!a"
	if n := utf8.RuneCountInString(multiByte); n != 16 || len(multiByte) <= 16 || len(multiByte) > 72 {
		t.Fatalf("fixture: %d code points, %d bytes", n, len(multiByte))
	}

	// Exactly 72 and 73 bytes, ASCII, all four classes.
	base := "Aa1!"
	at72 := base + strings.Repeat("x", 72-len(base))
	at73 := at72 + "x"

	// A multi-byte password under 72 code points but over 72 bytes (D4's accepted rare case).
	overBytes := "Aa1!" + strings.Repeat("é", 35) // 4 + 70 = 74 bytes, 39 code points

	cases := []struct {
		name string
		pw   string
		ok   bool
	}{
		{"16 chars, all four classes", "Abcdefghijklm1!x", true},
		{"15 chars, all four classes", "Abcdefghijkl1!x", false},
		{"no upper case", "abcdefghijklm1!x", false},
		{"no lower case", "ABCDEFGHIJKLM1!X", false},
		{"no digit", "Abcdefghijklmn!x", false},
		{"no special", "Abcdefghijklmn1x", false},
		{"16 code points, multi-byte, under 72 bytes", multiByte, true},
		{"exactly 72 bytes", at72, true},
		{"73 bytes", at73, false},
		{"under 72 code points but over 72 bytes", overBytes, false},
		{"inner space is the special character", "Correct horse 9X", true},
		{"leading space", " Abcdefghijklm1!x", false},
		{"trailing space", "Abcdefghijklm1!x ", false},
		{"leading tab", "\tAbcdefghijklm1!x", false},
		{"NUL", "Abcdefgh\x00ijklm1!x", false},
		{"other control character", "Abcdefgh\x07ijklm1!x", false},
		{"DEL control character", "Abcdefgh\x7fijklm1!x", false},
		{"invalid UTF-8", "Abcdefgh\xffijklm1!x", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		if got := validatePassword(c.pw); got != c.ok {
			t.Errorf("%s (%q, %d bytes): validatePassword = %v, want %v", c.name, c.pw, len(c.pw), got, c.ok)
		}
	}
}
