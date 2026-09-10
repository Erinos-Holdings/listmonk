package main

import (
	"testing"

	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/models"
)

// Fork (editor polish, EDITOR-POLISH-SPEC I4c): validateTemplate's brand field accepts ”
// or a models.ReBrandSlug match, lowercase-folded, and rejects anything else.
func TestValidateTemplateBrand(t *testing.T) {
	i, err := i18n.New([]byte(`{"_.code":"en","_.name":"English","globals.messages.invalidFields":"Invalid fields: {name}"}`))
	if err != nil {
		t.Fatal(err)
	}
	a := &App{i18n: i}

	cases := []struct {
		name    string
		brand   string
		wantErr bool
		wantOut string
	}{
		{"empty (none)", "", false, ""},
		{"lowercase slug", "liyora", false, "liyora"},
		{"mixed-case slug is folded", "Liyora", false, "liyora"},
		{"invalid slug (space)", "bad slug", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := &models.Template{
				Name:    "t",
				Type:    models.TemplateTypeTx,
				Body:    "b",
				Subject: "s",
				Brand:   c.brand,
			}
			err := a.validateTemplate(o)
			if c.wantErr {
				if err == nil {
					t.Fatalf("brand %q: expected error, got nil", c.brand)
				}
				return
			}
			if err != nil {
				t.Fatalf("brand %q: unexpected error: %v", c.brand, err)
			}
			if o.Brand != c.wantOut {
				t.Fatalf("brand %q: got %q want %q", c.brand, o.Brand, c.wantOut)
			}
		})
	}
}
