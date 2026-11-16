package models

// Fork (official footer) -- integrations listmonk/BRAND-FOOTER-SPEC I1/I3: ParseOfficialName and
// OfficialNeutralHasText over the ONE shared case table
// frontend/email-builder/test/fixtures/official-names.json, which the builder, the Vue sweep and
// integrations read too.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type officialNamesTable struct {
	Parse []struct {
		Name string `json:"name"`
		Want *struct {
			Kind  string `json:"kind"`
			Brand string `json:"brand"`
			Lang  string `json:"lang"`
		} `json:"want"`
	} `json:"parse"`
	NeutralText []struct {
		Case   string          `json:"case"`
		Text   bool            `json:"text"`
		Source json.RawMessage `json:"source"`
	} `json:"neutralText"`
}

func loadOfficialNames(t *testing.T) officialNamesTable {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "frontend", "email-builder", "test", "fixtures", "official-names.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx officialNamesTable
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	return fx
}

func TestParseOfficialNameFixture(t *testing.T) {
	fx := loadOfficialNames(t)
	if len(fx.Parse) < 20 {
		t.Fatalf("fixture has %d parse cases", len(fx.Parse))
	}
	for _, c := range fx.Parse {
		kind, brand, lang, ok := ParseOfficialName(c.Name)
		if c.Want == nil {
			if ok {
				t.Errorf("%q: parsed (%s %s %s), want null", c.Name, kind, brand, lang)
			}
			continue
		}
		if !ok || kind != c.Want.Kind || brand != c.Want.Brand || lang != c.Want.Lang {
			t.Errorf("%q: got (%s %q %q %v) want (%s %q %q)", c.Name, kind, brand, lang, ok, c.Want.Kind, c.Want.Brand, c.Want.Lang)
		}
	}
}

func TestOfficialNeutralHasTextFixture(t *testing.T) {
	fx := loadOfficialNames(t)
	if len(fx.NeutralText) < 10 {
		t.Fatalf("fixture has %d neutralText cases", len(fx.NeutralText))
	}
	for _, c := range fx.NeutralText {
		if got := OfficialNeutralHasText(string(c.Source)); got != c.Text {
			t.Errorf("%s: got %v want %v", c.Case, got, c.Text)
		}
	}
	if !OfficialNeutralHasText("{not json") {
		t.Error("unparsable body_source must be refused (reported as text)")
	}
	if OfficialNeutralHasText("") {
		t.Error("an empty body_source holds no text")
	}
}

func TestOfficialPrefixAndASCII(t *testing.T) {
	if !IsOfficialTemplate("Official Footer EN") || IsOfficialTemplate("Official_Footer_EN") || IsOfficialTemplate("official Footer EN") {
		t.Fatal("IsOfficialTemplate: the prefix is `Official ` exactly")
	}
	for name, want := range map[string]bool{
		"Official LIYORA Footer":       true,
		"Official LIYORÁ Footer":       false,
		"Official LIYORÁ Footer EN":    false,
		"Official Thirsty Girl Footer": true,
		"Official Thirsty-Girl Footer": false,
		"Official Footer EN":           true,
	} {
		if got := OfficialBrandIsASCII(name); got != want {
			t.Errorf("OfficialBrandIsASCII(%q) = %v want %v", name, got, want)
		}
	}
}
