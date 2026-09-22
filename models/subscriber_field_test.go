package models

import (
	"bytes"
	"html/template"
	"testing"
)

// Fork (subscriber names) -- I2: the $[FIELD|fallback]$ shorthand (SUBSCRIBER-NAME-SPEC D2).
func TestSubscriberFieldShorthand(t *testing.T) {
	rewrite := func(s string) string {
		for _, r := range regTplFuncs {
			s = r.apply(s)
		}
		return s
	}
	cases := []struct {
		in, want string
	}{
		{`Hi $[UD:FIRST_NAME|there]$,`, `Hi {{ or .Subscriber.FirstName "there" }},`},
		{`Hi $[ud:first_name|there]$,`, `Hi {{ or .Subscriber.FirstName "there" }},`},
		{`$[first_name||there]$`, `{{ or .Subscriber.FirstName "there" }}`},
		{`$[LAST_NAME]$`, `{{ or .Subscriber.LastName "" }}`},
		{`$[NAME|friend]$`, `{{ or .Subscriber.Name "friend" }}`},
		{`$[FULL_NAME|friend]$`, `{{ or .Subscriber.Name "friend" }}`},
		{`$[EMAIL]$`, `{{ or .Subscriber.Email "" }}`},
		{`$[loyalty_rewards_creator_site_url|https://x]$`, `{{ or (index .Subscriber.Attribs "loyalty_rewards_creator_site_url") "https://x" }}`},
		{`$[Loyalty_Rewards_Creator_Site_URL]$`, `{{ or (index .Subscriber.Attribs "loyalty_rewards_creator_site_url") "" }}`},
		{`$[FIRST_NAME|there friend!]$ and $[EMAIL]$`, `{{ or .Subscriber.FirstName "there friend!" }} and {{ or .Subscriber.Email "" }}`},
		// Unmatched text is left verbatim.
		{`$[oops`, `$[oops`},
		{`$[bad"quote|x]$`, `$[bad"quote|x]$`},
		{`$[FIRST_NAME|a"b]$`, `$[FIRST_NAME|a"b]$`},
		{`$[FIRST_NAME|a\b]$`, `$[FIRST_NAME|a\b]$`},
		{"$[FIRST_NAME|there\nfriend]$", "$[FIRST_NAME|there\nfriend]$"},
		{"$[FIRST_NAME|there\rfriend]$", "$[FIRST_NAME|there\rfriend]$"},
		{`$[9LIVES]$`, `$[9LIVES]$`},
		{`[FIRST_NAME]`, `[FIRST_NAME]`},
		{`price $5 [each]$`, `price $5 [each]$`},
	}
	for _, c := range cases {
		if got := rewrite(c.in); got != c.want {
			t.Errorf("rewrite(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// hasTplExpr must see the shorthand, or a brace-free subject is sent verbatim.
	if !hasTplExpr(`Hi $[UD:FIRST_NAME|there]$`) {
		t.Error("hasTplExpr misses the shorthand")
	}
	if hasTplExpr(`Hi $[oops`) {
		t.Error("hasTplExpr matched non-shorthand text")
	}

	funcs := template.FuncMap{
		"TrackView": func(_ any) template.HTML { return "" },
		"TrackLink": func(url string, _ any) string { return url },
	}
	for _, ct := range []string{CampaignContentTypeHTML, CampaignContentTypeRichtext, CampaignContentTypeVisual} {
		c := &Campaign{
			Subject:     `Hi $[UD:FIRST_NAME|there]$`, // no braces anywhere
			ContentType: ct,
			Body:        `<p>Hi $[UD:FIRST_NAME|there]$,</p><p>$[missing_attr]$|$[LAST_NAME|x]$</p>`,
		}
		if err := c.CompileTemplate(funcs); err != nil {
			t.Fatalf("%s: CompileTemplate: %v", ct, err)
		}
		if c.SubjectTpl == nil {
			t.Fatalf("%s: SubjectTpl is nil -- a shorthand-only subject would go out verbatim", ct)
		}
		for _, tc := range []struct {
			name, subj, greet, tail string
		}{
			{"", "Hi there", "<p>Hi there,</p>", "<p>|x</p>"},
			{"Jane Doe", "Hi Jane", "<p>Hi Jane,</p>", "<p>|Doe</p>"},
		} {
			data := map[string]any{"Subscriber": Subscriber{Name: tc.name, Attribs: JSON{}}}
			var b bytes.Buffer
			if err := c.SubjectTpl.ExecuteTemplate(&b, ContentTpl, data); err != nil {
				t.Fatalf("%s: subject: %v", ct, err)
			}
			if b.String() != tc.subj {
				t.Errorf("%s: subject for %q = %q, want %q", ct, tc.name, b.String(), tc.subj)
			}
			b.Reset()
			if err := c.Tpl.ExecuteTemplate(&b, BaseTpl, data); err != nil {
				t.Fatalf("%s: body: %v", ct, err)
			}
			if want := tc.greet + tc.tail; !bytes.Contains(b.Bytes(), []byte(want)) {
				t.Errorf("%s: body for %q = %q, want it to contain %q", ct, tc.name, b.String(), want)
			}
		}
	}

	// A shorthand inside a visual href is left alone by the TrackLink wrap (it would nest
	// actions inside the string literal) and rewritten by the regTplFuncs pass instead.
	c := &Campaign{
		Subject:     "s",
		ContentType: CampaignContentTypeVisual,
		Body:        `<a href="https://example.com/?e=$[EMAIL]$">x</a>`,
	}
	if err := c.CompileTemplate(funcs); err != nil {
		t.Fatalf("visual href: CompileTemplate: %v", err)
	}
}
