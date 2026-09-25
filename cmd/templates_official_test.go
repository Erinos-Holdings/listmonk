package main

import (
	"testing"

	"github.com/knadh/listmonk/models"
	null "gopkg.in/volatiletech/null.v6"
)

// Fork (official footer) -- integrations OFFICIAL-FOOTER-SPEC I11, the pure rules: a rename off
// the Official_ prefix, a create/rename onto an existing Official_ name, and an official
// body_source holding an OfficialFooter block are refused; other updates pass.
func TestOfficialTemplateRefusal(t *testing.T) {
	tpl := func(id int, name, source string) models.Template {
		o := models.Template{Name: name, Type: models.TemplateTypeCampaignVisual}
		o.ID = id
		if source != "" {
			o.BodySource = null.StringFrom(source)
		}
		return o
	}
	const plain = `{"root":{"type":"EmailLayout","data":{"childrenIds":["a"]}},"a":{"type":"Text","data":{"props":{"text":"x"}}}}`
	const nested = `{"root":{"type":"EmailLayout","data":{"childrenIds":["a"]}},"a":{"type": "OfficialFooter","data":{"props":{"kind":"corporate"}}}}`
	all := []models.Template{tpl(30, "Official_Footer_EN", plain), tpl(14, "Official_RUZE_Footer_EN", plain), tpl(7, "LIYORA After Purchase", nested)}
	stored30 := all[0]
	stored7 := all[2]

	cases := []struct {
		name   string
		stored *models.Template
		in     models.Template
		want   string
	}{
		{"official update, same name", &stored30, tpl(30, "Official_Footer_EN", plain), ""},
		{"official rename within the prefix", &stored30, tpl(30, "Official_Footer_ES", plain), ""},
		{"official rename off the prefix", &stored30, tpl(30, "Footer EN", plain), "templates.officialRename"},
		{"official rename onto an existing official name", &stored30, tpl(30, "Official_RUZE_Footer_EN", plain), "templates.officialDuplicate"},
		{"create onto an existing official name", nil, tpl(0, "Official_Footer_EN", plain), "templates.officialDuplicate"},
		{"create a new official name", nil, tpl(0, "Official_Footer_DE", plain), ""},
		{"official body holding an OfficialFooter block", &stored30, tpl(30, "Official_Footer_EN", nested), "templates.officialNested"},
		{"create official holding an OfficialFooter block", nil, tpl(0, "Official_Footer_IT", nested), "templates.officialNested"},
		{"non-official template may hold OfficialFooter blocks", &stored7, tpl(7, "LIYORA After Purchase", nested), ""},
		{"non-official rename onto an existing official name", &stored7, tpl(7, "Official_Footer_EN", plain), "templates.officialDuplicate"},
		{"non-official create, name taken by a non-official row", nil, tpl(0, "LIYORA After Purchase", plain), ""},
		{"no body_source", &stored30, tpl(30, "Official_Footer_EN", ""), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := c.in
			if got := officialTemplateRefusal(c.stored, &in, all); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}

	if !models.IsOfficialTemplate("Official_Footer_EN") || models.IsOfficialTemplate("Copy of Official_Footer_EN") || models.IsOfficialTemplate("official_footer_en") {
		t.Fatal("IsOfficialTemplate: the prefix is exact and case-sensitive")
	}
}
