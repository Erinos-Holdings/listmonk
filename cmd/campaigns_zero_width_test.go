package main

import (
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/models"
)

// Fork (CAMPAIGN-52-HARDENING T7, I9) -- the pure predicate: the four zero-width code
// points in subject or attribs.preheader are reported by field; ordinary text, emoji, and
// the body's &zwnj; filler are not the fields' business.
func TestZeroWidthFields(t *testing.T) {
	cases := []struct {
		name    string
		subject string
		attribs models.JSON
		want    string
	}{
		{"clean", "Riscatta adesso ✨", models.JSON{"preheader": "Il tuo set è pronto."}, ""},
		{"no attribs", "plain", nil, ""},
		{"ZWSP in subject (ZMA emoji picker)", "sconto \u200b ✨ \u200b", models.JSON{"preheader": "ok"}, "subject"},
		{"ZWNJ in preheader", "ok", models.JSON{"preheader": "a\u200cb"}, "preheader"},
		{"ZWJ in subject", "a\u200db", models.JSON{}, "subject"},
		{"BOM at preheader start", "ok", models.JSON{"preheader": "\ufeffok"}, "preheader"},
		{"both", "\u200b", models.JSON{"preheader": "\ufeff"}, "subject,preheader"},
		{"non-string preheader ignored", "ok", models.JSON{"preheader": 42}, ""},
		{"nbsp and thin space are not zero-width", "a b c", models.JSON{"preheader": "x y"}, ""},
		// EMOJI-PICKER-SPEC D7 / I5: a ZWJ joining a rune above U+00FF is an emoji sequence.
		{"ZWJ family emoji", "\U0001F468‍\U0001F469", models.JSON{}, ""},
		{"ZWJ heart on fire", "❤️‍\U0001F525", models.JSON{}, ""},
		{"ZWJ family emoji in preheader", "ok", models.JSON{"preheader": "hi \U0001F468‍\U0001F469‍\U0001F467"}, ""},
		{"ZWJ at position 0", "‍ab", models.JSON{}, "subject"},
		{"ZWJ after Latin-1", "é‍b", models.JSON{}, "subject"},
		{"ZWSP between emoji", "\U0001F468​\U0001F469", models.JSON{}, "subject"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.Join(zeroWidthFields(c.subject, c.attribs), ","); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}

	// The body's preheader filler is U+200C eighty times and must never be the predicate's
	// input; pinned here as a statement of scope.
	if !hasZeroWidth(strings.Repeat("&nbsp;\u200c", 80)) {
		t.Fatal("U+200C must count when it IS in a checked field")
	}

	// The translated warning names the field.
	i, err := i18n.New([]byte(`{"_.code":"en","_.name":"English","campaigns.warnZeroWidth":"The {field} has zero-width characters."}`))
	if err != nil {
		t.Fatal(err)
	}
	a := &App{i18n: i}
	w := a.zeroWidthWarnings(&models.Campaign{Subject: "x\u200by", Attribs: models.JSON{"preheader": "\ufeffz"}})
	if len(w) != 2 || w[0] != "The subject has zero-width characters." || w[1] != "The preheader has zero-width characters." {
		t.Fatalf("warnings %q", w)
	}
}
