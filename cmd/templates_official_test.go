package main

import (
	"testing"

	"github.com/knadh/listmonk/models"
	null "gopkg.in/volatiletech/null.v6"
)

// Fork (official footer) -- integrations OFFICIAL-FOOTER-SPEC I11, the pure rules: a rename off
// the official prefix, a create/rename onto an existing official name, and an official
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
	// integrations BRAND-FOOTER-SPEC I3/I4: a neutral brand footer may hold no text; names must
	// be one of the three shapes with an ASCII brand segment.
	const icons = `{"root":{"type":"EmailLayout","data":{"childrenIds":["c"]}},"c":{"type":"Container","data":{"props":{"childrenIds":["i","h"]}}},"i":{"type":"Image","data":{"props":{"url":"https://x.invalid/l.png","alt":"LIYORÁ"}}},"h":{"type":"Html","data":{"props":{"contents":"<a href=\"https://facebook.com/x\"><img src=\"https://x.invalid/fb.png\" alt=\"Facebook\"></a>"}}}}`
	const heading = `{"root":{"type":"EmailLayout","data":{"childrenIds":["a"]}},"a":{"type":"Heading","data":{"props":{"text":"RUZE"}}}}`
	const button = `{"root":{"type":"EmailLayout","data":{"childrenIds":["a"]}},"a":{"type":"Button","data":{"props":{"text":"Shop","url":"https://x.invalid"}}}}`
	const htmlText = `{"root":{"type":"EmailLayout","data":{"childrenIds":["a"]}},"a":{"type":"Html","data":{"props":{"contents":"<p>Follow <b>us</b></p>"}}}}`
	all := []models.Template{tpl(30, "Official Footer EN", plain), tpl(14, "Official RUZE Footer", icons), tpl(7, "LIYORA After Purchase", nested), tpl(35, "Official Shala Footer EN", plain)}
	stored30 := all[0]
	stored14 := all[1]
	stored7 := all[2]
	stored35 := all[3]

	cases := []struct {
		name   string
		stored *models.Template
		in     models.Template
		want   string
	}{
		{"official update, same name", &stored30, tpl(30, "Official Footer EN", plain), ""},
		{"official rename within the prefix", &stored30, tpl(30, "Official Footer ES", plain), ""},
		{"official rename off the prefix", &stored30, tpl(30, "Footer EN", plain), "templates.officialRename"},
		{"official rename onto the old prefix is off the prefix", &stored30, tpl(30, "Official_Footer_EN", plain), "templates.officialRename"},
		{"official rename onto an existing official name", &stored30, tpl(30, "Official RUZE Footer", icons), "templates.officialDuplicate"},
		{"create onto an existing official name", nil, tpl(0, "Official Footer EN", plain), "templates.officialDuplicate"},
		{"create a new official name", nil, tpl(0, "Official Footer DE", plain), ""},
		{"official body holding an OfficialFooter block", &stored30, tpl(30, "Official Footer EN", nested), "templates.officialNested"},
		{"create official holding an OfficialFooter block", nil, tpl(0, "Official Footer IT", nested), "templates.officialNested"},
		{"non-official template may hold OfficialFooter blocks", &stored7, tpl(7, "LIYORA After Purchase", nested), ""},
		{"non-official rename onto an existing official name", &stored7, tpl(7, "Official Footer EN", plain), "templates.officialDuplicate"},
		{"non-official create, name taken by a non-official row", nil, tpl(0, "LIYORA After Purchase", plain), ""},
		{"non-official create under the old prefix is an ordinary template", nil, tpl(0, "Official_Footer_EN", plain), ""},
		{"no body_source", &stored30, tpl(30, "Official Footer EN", ""), ""},

		// I4: the shape and the ASCII brand segment.
		{"unparsable official name", nil, tpl(0, "Official Footer en", plain), "templates.officialShape"},
		{"unparsable: Official Footer alone", nil, tpl(0, "Official Footer", plain), "templates.officialShape"},
		{"unparsable: a header, not a footer", nil, tpl(0, "Official RUZE Header", plain), "templates.officialShape"},
		{"unparsable: three-letter language", nil, tpl(0, "Official RUZE Footer ENG", plain), "templates.officialShape"},
		{"rename onto an unparsable official name", &stored14, tpl(14, "Official RUZE footer", icons), "templates.officialShape"},
		{"accented brand segment (neutral)", nil, tpl(0, "Official LIYORÁ Footer", icons), "templates.officialBrandAscii"},
		{"accented brand segment (override)", nil, tpl(0, "Official LIYORÁ Footer EN", plain), "templates.officialBrandAscii"},
		{"punctuation in the brand segment", nil, tpl(0, "Official Thirsty-Girl Footer", icons), "templates.officialBrandAscii"},
		{"ASCII brand with a space", nil, tpl(0, "Official Thirsty Girl Footer", icons), ""},
		{"a long official name is fine (no 30 limit here)", nil, tpl(0, "Official KNOWN FOR FRAGRANCE Footer", icons), ""},

		// I3: the neutral footer holds no text; an override may.
		{"neutral: logo + icons-only Html accepted", nil, tpl(0, "Official LIYORA Footer", icons), ""},
		{"neutral: a Text block refused", nil, tpl(0, "Official LIYORA Footer", plain), "templates.officialNeutralText"},
		{"neutral: a Heading block refused", nil, tpl(0, "Official LIYORA Footer", heading), "templates.officialNeutralText"},
		{"neutral: a Button block refused", nil, tpl(0, "Official LIYORA Footer", button), "templates.officialNeutralText"},
		{"neutral: an Html block with text refused", nil, tpl(0, "Official LIYORA Footer", htmlText), "templates.officialNeutralText"},
		{"neutral update adding text refused", &stored14, tpl(14, "Official RUZE Footer", plain), "templates.officialNeutralText"},
		{"override renamed to neutral keeps its text: refused", &stored35, tpl(35, "Official Shala Footer", plain), "templates.officialNeutralText"},
		{"override with a Text block accepted", nil, tpl(0, "Official LIYORA Footer FR", plain), ""},
		{"corporate with a Text block accepted", &stored30, tpl(30, "Official Footer EN", plain), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := c.in
			if got := officialTemplateRefusal(c.stored, &in, all); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}

	// A rename without a body_source is judged on the stored body.
	t.Run("neutral rename without body_source reads the stored body", func(t *testing.T) {
		in := tpl(35, "Official Shala Footer", "")
		if got := officialTemplateRefusal(&stored35, &in, all); got != "templates.officialNeutralText" {
			t.Fatalf("got %q", got)
		}
	})

	// BRAND-FOOTER S4-2: an EMPTY body_source keeps the stored one (update-template), so the guards
	// judge the stored source, exactly as for an absent one.
	t.Run("neutral rename with an empty body_source reads the stored body", func(t *testing.T) {
		in := tpl(35, "Official Shala Footer", "")
		in.BodySource = null.StringFrom("")
		if got := officialTemplateRefusal(&stored35, &in, all); got != "templates.officialNeutralText" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("rename onto an official name without body_source reads the stored body (nested)", func(t *testing.T) {
		for _, src := range []null.String{{}, null.StringFrom("")} {
			in := tpl(7, "Official Footer DE", "")
			in.BodySource = src
			if got := officialTemplateRefusal(&stored7, &in, all); got != "templates.officialNested" {
				t.Fatalf("source valid=%v: got %q", src.Valid, got)
			}
		}
	})

	if !models.IsOfficialTemplate("Official Footer EN") || models.IsOfficialTemplate("Copy of Official Footer EN") || models.IsOfficialTemplate("official Footer EN") || models.IsOfficialTemplate("Official_Footer_EN") {
		t.Fatal("IsOfficialTemplate: the prefix is exact and case-sensitive")
	}
}
