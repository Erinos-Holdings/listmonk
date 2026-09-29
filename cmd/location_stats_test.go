package main

// Fork (location stats) -- integrations LOCATION-STATS-SPEC I1: only a trimmed, upper-cased
// two-letter ASCII code is stored; everything else is "" (stored NULL).

import "testing"

func TestNormalizeCountry(t *testing.T) {
	for in, want := range map[string]string{
		"US":     "US",
		"us":     "US",
		"uS":     "US",
		" gb ":   "GB",
		"\tDE\n": "DE",
		"XX":     "XX", // a well-formed code CloudFront uses for "unknown" -- stored as sent
		"":       "",
		"   ":    "",
		"U":      "",
		"USA":    "",
		"U1":     "",
		"1U":     "",
		"U-":     "",
		"@[":     "",
		"`{":     "",
		"U S":    "",
		"ÜS":     "", // non-ASCII letter
		"ıs":     "", // dotless i upper-cases to ASCII I -- refused, not folded into "IS"
		"ſe":     "", // long s upper-cases to ASCII S
		"ＵＳ":     "", // fullwidth letters
	} {
		if got := normalizeCountry(in); got != want {
			t.Errorf("normalizeCountry(%q) = %q, want %q", in, got, want)
		}
	}
}
