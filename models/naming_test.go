package models

// Fork (naming standard) -- integrations listmonk/NAMING-STANDARD-SPEC I4: NameProblem over the
// ONE shared case table (testdata/name-rule.json), which integrations tests/lib/naming.test.ts
// reads too (lib/naming.ts::nameProblem, the TS mirror).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type nameRuleCase struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Brand    string `json:"brand"`
	Display  string `json:"display"`
	ListType string `json:"listType"`
	Want     string `json:"want"`
}

func TestNameProblemFixture(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "name-rule.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Cases []nameRuleCase `json:"cases"`
	}
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	if len(fx.Cases) < 20 {
		t.Fatalf("fixture has %d cases", len(fx.Cases))
	}
	for _, c := range fx.Cases {
		key, args := NameProblem(c.Kind, c.Name, c.Brand, c.Display, c.ListType)
		if key != c.Want {
			t.Errorf("%s %q (brand %q %q, %s): key %q want %q (args %v)", c.Kind, c.Name, c.Brand, c.Display, c.ListType, key, c.Want, args)
		}
		if len(args)%2 != 0 {
			t.Errorf("%q: odd args %v", c.Name, args)
		}
	}
}

func TestListNameRemainder(t *testing.T) {
	for _, c := range []struct{ name, slug, display, typ, want string }{
		{"RUZE Welcome", "ruze", "Ruze Pouches", ListTypePrivate, "Welcome"},
		{"Ruze Pouches Emails", "ruze", "Ruze Pouches", ListTypePublic, "Emails"},
		{"RUZE Welcome", "ruze", "Ruze Pouches", ListTypePublic, "RUZE Welcome"},
		{"RUZEWelcome", "ruze", "Ruze Pouches", ListTypePrivate, "RUZEWelcome"},
		{"RUZE Welcome", "", "", ListTypePrivate, "RUZE Welcome"},
	} {
		if got := ListNameRemainder(c.name, c.slug, c.display, c.typ); got != c.want {
			t.Errorf("ListNameRemainder(%q, %q, %q, %q) = %q want %q", c.name, c.slug, c.display, c.typ, got, c.want)
		}
	}
}
