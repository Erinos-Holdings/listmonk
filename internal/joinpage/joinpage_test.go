package joinpage

import (
	"reflect"
	"strings"
	"testing"
)

// TestJoinCopy_AllLanguages -- I12 (copy complete for five languages; lang precedence and
// normalization). Every language carries every string, the placeholders are substituted, no
// copy promises a welcome (a silent join), and D10's English row is pinned verbatim.
func TestJoinCopy_AllLanguages(t *testing.T) {
	if !reflect.DeepEqual(Langs, []string{"en", "es", "fr", "de", "it"}) {
		t.Fatalf("Langs = %v", Langs)
	}
	if len(copies) != len(Langs) {
		t.Fatalf("copies has %d languages, want %d", len(copies), len(Langs))
	}
	for _, l := range Langs {
		raw, ok := copies[l]
		if !ok {
			t.Fatalf("no copy for %s", l)
		}
		v := reflect.ValueOf(raw)
		for i := 0; i < v.NumField(); i++ {
			if strings.TrimSpace(v.Field(i).String()) == "" {
				t.Fatalf("%s: %s is empty", l, v.Type().Field(i).Name)
			}
		}
		for _, s := range []string{raw.Heading, raw.Body, raw.DoneBody, raw.Already} {
			if !strings.Contains(s, "{brand}") {
				t.Fatalf("%s: %q names no brand", l, s)
			}
		}
		for _, s := range []string{raw.Body, raw.DoneBody, raw.Already} {
			if !strings.Contains(s, "{email}") {
				t.Fatalf("%s: %q names no address", l, s)
			}
		}
		c := For(l, "Shala", "a@b.test")
		cv := reflect.ValueOf(c)
		for i := 0; i < cv.NumField(); i++ {
			s := cv.Field(i).String()
			if strings.Contains(s, "{brand}") || strings.Contains(s, "{email}") {
				t.Fatalf("%s: placeholder survived in %q", l, s)
			}
			for _, w := range []string{"welcome", "bienvenue", "bienvenid", "willkommen", "benvenut"} {
				if strings.Contains(strings.ToLower(s), w) {
					t.Fatalf("%s: %q promises a welcome (the join is silent)", l, s)
				}
			}
		}
	}

	en := For("en", "Shala", "a@b.test")
	if en.Heading != "Get Shala emails" || en.Body != "Confirm below to receive Shala emails at a@b.test." ||
		en.Button != "Confirm" || en.DoneHeading != "You're subscribed" || en.DoneBody != "You'll receive Shala emails at a@b.test." ||
		en.Already != "a@b.test already receives Shala emails." || en.Cannot != "We couldn't update your subscription from this link." {
		t.Fatalf("en copy drifted from D10: %+v", en)
	}
	fr := For("fr", "Shala", "a@b.test")
	if fr.Button != "Confirmer" || fr.Heading != "Recevez les e-mails de Shala" {
		t.Fatalf("fr copy drifted from D10: %+v", fr)
	}

	// Lang precedence and normalization (D10).
	for _, c := range []struct{ plan, sub, want string }{
		{"fr", "en", "fr"},      // the campaign's language wins
		{"", "de", "de"},        // else the subscriber's
		{"", "", "en"},          // else English
		{"FR-ca", "", "fr"},     // primary subtag, any case
		{"fr_CA", "", "fr"},     // underscore form
		{"pt", "fr", "en"},      // an unsupported campaign language is English, not the subscriber's
		{"", "pt-BR", "en"},     // an unsupported subscriber language is English
		{"  it  ", "", "it"},    // trimmed
		{"", "es-MX", "es"},     // subscriber regional tag
		{"english", "", "en"},   // not a subtag of the set
		{"deutsch", "fr", "en"}, // ditto
		{"de", "", "de"},
	} {
		if got := PageLang(c.plan, c.sub); got != c.want {
			t.Fatalf("PageLang(%q, %q) = %q, want %q", c.plan, c.sub, got, c.want)
		}
	}
	if For("pt", "B", "e").Button != "Confirm" {
		t.Fatalf("an unknown language must render English")
	}
}

// TestJoinBrandName -- I12 (brand display name): the display-name part of brands.from_email,
// else the slug.
func TestJoinBrandName(t *testing.T) {
	for _, c := range []struct{ from, slug, want string }{
		{"Shala <hello@shala.com>", "shala", "Shala"},
		{`"Thirsty Girl" <hello@thirstygirl.com>`, "thirstygirl", "Thirsty Girl"},
		{"hello@shala.com", "shala", "shala"},
		{"", "shala", "shala"},
		{"not an address", "shala", "shala"},
		{"  <hello@x.test>  ", "x", "x"},
	} {
		if got := BrandName(c.from, c.slug); got != c.want {
			t.Fatalf("BrandName(%q, %q) = %q, want %q", c.from, c.slug, got, c.want)
		}
	}
}

// TestJoinState -- D2/D11: only join renders the Confirm form; already its own page; every other
// outcome, an unknown one included, the generic Cannot page.
func TestJoinState(t *testing.T) {
	for in, want := range map[string]string{
		"join": StateConfirm, "already": StateAlready,
		"cannot": StateCannot, "refused": StateCannot, "no_campaign": StateCannot,
		"no_brand": StateCannot, "no_master": StateCannot, "": StateCannot, "weird": StateCannot,
	} {
		if got := State(in); got != want {
			t.Fatalf("State(%q) = %q, want %q", in, got, want)
		}
	}
}
