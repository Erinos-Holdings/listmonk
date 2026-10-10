package models

// Fork (persona From) -- integrations PERSONA-FROM-SPEC I1, I2 and the pure half of I3
// (IsPersonaFrom). Every ACCEPTED persona also asserts the wire round-trip: the From header is
// from_email re-serialised by net/mail (smtppool.formatAddress), so the name net/mail reads back
// must be the stored one. Fixture addresses are example.test only.

import (
	"fmt"
	"net/mail"
	"strings"
	"testing"
)

const personaTestFrom = "Thirsty Girl <hello@tg.example.test>"

// wireRoundTrip asserts the persona From survives net/mail unchanged.
func wireRoundTrip(t *testing.T, persona, brandFrom string) {
	t.Helper()
	a, err := mail.ParseAddress(PersonaFrom(persona, brandFrom))
	if err != nil {
		t.Fatalf("%q: net/mail cannot parse %q: %v", persona, PersonaFrom(persona, brandFrom), err)
	}
	if a.Name != persona || a.Address != BareAddress(brandFrom) {
		t.Fatalf("%q: wire name %q address %q, want %q %q", persona, a.Name, a.Address, persona, BareAddress(brandFrom))
	}
	// And the serialised header parses back to the same name (what a recipient's client reads).
	b, err := mail.ParseAddress(a.String())
	if err != nil || b.Name != persona {
		t.Fatalf("%q: serialised %q reads back as %q (%v)", persona, a.String(), b.Name, err)
	}
}

func TestPersonaProblem(t *testing.T) {
	refused := []struct {
		name, persona, key string
	}{
		{"empty", "", "brands.personaEmpty"},
		{"blank", "   ", "brands.personaEmpty"},
		{"too long", strings.Repeat("N", 49) + " Thirsty Girl", "brands.personaTooLong"}, // 62
		{"non-ASCII", "Natashá at Thirsty Girl", "brands.personaInvalid"},
		{"CR", "Natasha\rat Thirsty Girl", "brands.personaInvalid"},
		{"LF", "Natasha\nat Thirsty Girl", "brands.personaInvalid"},
		{"CRLF header injection", "Natasha at Thirsty Girl\r\nBcc: x@y.example.test", "brands.personaInvalid"},
		{"TAB", "Natasha\tat Thirsty Girl", "brands.personaInvalid"},
		{"DEL", "Natasha\x7fat Thirsty Girl", "brands.personaInvalid"},
		{"at sign", "Natasha @ Thirsty Girl", "brands.personaInvalid"},
		{"less than", "Natasha < Thirsty Girl", "brands.personaInvalid"},
		{"greater than", "Natasha > Thirsty Girl", "brands.personaInvalid"},
		{"double quote", `Natasha "N" at Thirsty Girl`, "brands.personaInvalid"},
		{"comma", "Natasha, at Thirsty Girl", "brands.personaInvalid"},
		{"semicolon", "Natasha; at Thirsty Girl", "brands.personaInvalid"},
		{"colon", "Natasha: Thirsty Girl", "brands.personaInvalid"},
		{"open paren", "Natasha (TG at Thirsty Girl", "brands.personaInvalid"},
		{"close paren", "Natasha TG) at Thirsty Girl", "brands.personaInvalid"},
		{"parentheses (a comment, silently dropped)", "Natasha (TG) at Thirsty Girl", "brands.personaInvalid"},
		{"open bracket", "Natasha [TG at Thirsty Girl", "brands.personaInvalid"},
		{"close bracket", "Natasha TG] at Thirsty Girl", "brands.personaInvalid"},
		{"backslash", `Natasha \ at Thirsty Girl`, "brands.personaInvalid"},
		{"double space", "Natasha  at Thirsty Girl", "brands.personaInvalid"},
		{"leading space", " Natasha at Thirsty Girl", "brands.personaInvalid"},
		{"trailing space", "Natasha at Thirsty Girl ", "brands.personaInvalid"},
		{"no brand name", "Natasha", "brands.personaNoBrandName"},
		{"case-only variant of the brand name", "natasha at thirsty girl", "brands.personaNoBrandName"},
		{"brand name upper-cased", "Natasha at THIRSTY GIRL", "brands.personaNoBrandName"},
		{"the display name alone", "Thirsty Girl", "brands.personaIsBrand"},
	}
	for _, c := range refused {
		t.Run("refused/"+c.name, func(t *testing.T) {
			key, args := PersonaProblem(c.persona, personaTestFrom)
			if key != c.key {
				t.Fatalf("%q: key %q want %q", c.persona, key, c.key)
			}
			if len(args)%2 != 0 {
				t.Fatalf("%q: odd args %v", c.persona, args)
			}
		})
	}

	// The key names the brand name the persona must contain, and the cap.
	if _, args := PersonaProblem("Natasha", personaTestFrom); fmt.Sprint(args) != "[persona Natasha brand Thirsty Girl]" {
		t.Fatalf("personaNoBrandName args %v", args)
	}
	if _, args := PersonaProblem(strings.Repeat("N", 49)+" Thirsty Girl", personaTestFrom); len(args) != 4 || args[3] != "60" {
		t.Fatalf("personaTooLong args %v", args)
	}

	// A persona of exactly the cap passes; one over does not.
	at60 := strings.Repeat("N", 47) + " Thirsty Girl"
	if len(at60) != PersonaMaxLen {
		t.Fatalf("fixture: %d", len(at60))
	}

	accepted := []struct{ persona, from string }{
		{"Natasha at Thirsty Girl", personaTestFrom},
		{"O'Neil at Thirsty Girl", personaTestFrom},
		{"Dr. Thirsty Girl Team", personaTestFrom},
		{"Jean-Luc at Thirsty Girl", personaTestFrom},
		{"Nat & Jo at Thirsty Girl 2", personaTestFrom},
		{at60, personaTestFrom},
		// One valid persona per live brand display name: the 14 display_names of the live
		// GET /api/brands (read 2026-10-10), spelled and cased exactly as the brand Froms are.
		{"Robbie at Curated", "Curated <hello@curated.example.test>"},
		{"Natasha at Thirsty Girl", "Thirsty Girl <hello@thirstygirl.example.test>"},
		{"Ana at Liyora", "Liyora <hello@liyora.example.test>"},
		{"Joy at JoyBelle", "JoyBelle <hello@joybelle.example.test>"},
		{"Kim at BNKD Wellness", "BNKD Wellness <hello@bnkd.example.test>"},
		{"Elizabeth at Ruze Pouches", "Ruze Pouches <hello@ruze.example.test>"},
		{"Tori at Shala", "Shala <hello@shala.example.test>"},
		{"Mia at BEMIJADE", "BEMIJADE <hello@bemijade.example.test>"},
		{"Team BRIM beauty", "BRIM beauty <hello@brim.example.test>"},
		{"Sam at High Contrast Beauty", "High Contrast Beauty <hello@highcontrast.example.test>"},
		{"Lee at KNOWN FOR FRAGRANCE", "KNOWN FOR FRAGRANCE <hello@knownfor.example.test>"},
		{"Ava at SUGARSTRAND Hair", "SUGARSTRAND Hair <hello@sugarstrand.example.test>"},
		{"Zoe at Vocab Skin", "Vocab Skin <hello@vocabskin.example.test>"},
		{"Jen at Younique", "Younique <hello@younique.example.test>"},
	}
	for _, c := range accepted {
		t.Run("accepted/"+c.persona, func(t *testing.T) {
			if key, args := PersonaProblem(c.persona, c.from); key != "" {
				t.Fatalf("%q on %q refused: %s %v", c.persona, c.from, key, args)
			}
			wireRoundTrip(t, c.persona, c.from)
			if !IsPersonaFrom(PersonaFrom(c.persona, c.from), c.from, []string{c.persona}) {
				t.Fatalf("%q: its own canonical From is not accepted", c.persona)
			}
		})
	}

	// A brand whose From is a bare address has no display name to contain: no persona is valid.
	if key, _ := PersonaProblem("Natasha at Acme", "hello@acme.example.test"); key == "" {
		t.Fatal("a bare-address brand accepted a persona")
	}
}

func TestPersonasProblem(t *testing.T) {
	set := func(n int) []string {
		out := make([]string, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, fmt.Sprintf("Person %d at Thirsty Girl", i))
		}
		return out
	}

	if key, args := PersonasProblem(set(10), personaTestFrom); key != "" {
		t.Fatalf("10 distinct refused: %s %v", key, args)
	}
	if key, _ := PersonasProblem(nil, personaTestFrom); key != "" {
		t.Fatalf("the empty set refused: %s", key)
	}
	if key, args := PersonasProblem(set(11), personaTestFrom); key != "brands.personasTooMany" || fmt.Sprint(args) != "[max 10]" {
		t.Fatalf("11: %s %v", key, args)
	}
	key, args := PersonasProblem([]string{"Natasha at Thirsty Girl", "Jo at Thirsty Girl", "NATASHA AT Thirsty Girl"}, personaTestFrom)
	if key != "brands.personaDuplicate" || fmt.Sprint(args) != "[first Natasha at Thirsty Girl second NATASHA AT Thirsty Girl]" {
		t.Fatalf("case-insensitive duplicate: %s %v", key, args)
	}
	if key, _ := PersonasProblem([]string{"Jo at Thirsty Girl", "Jo at Thirsty Girl"}, personaTestFrom); key != "brands.personaDuplicate" {
		t.Fatalf("exact duplicate: %s", key)
	}
	// An element failing the single rule is reported by that rule's key.
	if key, _ := PersonasProblem([]string{"Jo at Thirsty Girl", "Natasha"}, personaTestFrom); key != "brands.personaNoBrandName" {
		t.Fatalf("bad element: %s", key)
	}
}

func TestIsPersonaFrom(t *testing.T) {
	personas := []string{"Natasha at Thirsty Girl", "Jo at Thirsty Girl"}
	bare := "hello@tg.example.test"

	for _, from := range []string{"Natasha at Thirsty Girl <" + bare + ">", "Jo at Thirsty Girl <" + bare + ">"} {
		if !IsPersonaFrom(from, personaTestFrom, personas) {
			t.Fatalf("%q refused", from)
		}
	}
	for name, from := range map[string]string{
		"the brand From itself":    personaTestFrom,
		"a persona not on the row": "Kim at Thirsty Girl <" + bare + ">",
		"another bare address":     "Natasha at Thirsty Girl <hello@other.example.test>",
		"quoted":                   `"Natasha at Thirsty Girl" <` + bare + ">",
		"double space":             "Natasha at Thirsty Girl  <" + bare + ">",
		"no space":                 "Natasha at Thirsty Girl<" + bare + ">",
		"trailing text":            "Natasha at Thirsty Girl <" + bare + "> x",
		"bare address":             bare,
		"case variant":             "natasha at thirsty girl <" + bare + ">",
		"empty":                    "",
	} {
		if IsPersonaFrom(from, personaTestFrom, personas) {
			t.Fatalf("%s: %q accepted", name, from)
		}
	}
	if IsPersonaFrom("Natasha at Thirsty Girl <"+bare+">", personaTestFrom, nil) {
		t.Fatal("accepted with no personas on the row")
	}
}
