package models

import (
	"bytes"
	"html/template"
	"testing"
)

// Fork (subscriber names) -- I2: the builder's &quot; is decoded inside {{ ... }} actions, and
// nowhere else (SUBSCRIBER-NAME-SPEC D2).
func TestActionQuoteEntities(t *testing.T) {
	rewrite := func(s string) string {
		for _, r := range regTplFuncs {
			s = r.apply(s)
		}
		return s
	}
	cases := []struct {
		in, want string
	}{
		// All three spellings decode inside an action (the hex form case-insensitively).
		{`Hi {{ or .Subscriber.FirstName &quot;there&quot; }},`, `Hi {{ or .Subscriber.FirstName "there" }},`},
		{`Hi {{ or .Subscriber.FirstName &#34;there&#34; }},`, `Hi {{ or .Subscriber.FirstName "there" }},`},
		{`Hi {{ or .Subscriber.FirstName &#x22;there&#X22; }},`, `Hi {{ or .Subscriber.FirstName "there" }},`},
		// Outside an action nothing is decoded, including between two actions.
		{`&quot;a&quot; {{ .Subscriber.Name }} &quot;b&quot; {{ .Subscriber.Email }} &#34;c&#x22;`, `&quot;a&quot; {{ .Subscriber.Name }} &quot;b&quot; {{ .Subscriber.Email }} &#34;c&#x22;`},
		{`{{ .Subscriber.Name }}&quot;{{ or .Subscriber.FirstName &quot;x&quot; }}`, `{{ .Subscriber.Name }}&quot;{{ or .Subscriber.FirstName "x" }}`},
		// Other entities inside an action are untouched.
		{`{{ or .Subscriber.FirstName &quot;a &amp; b &lt;c&gt;&quot; }}`, `{{ or .Subscriber.FirstName "a &amp; b &lt;c&gt;" }}`},
		// Other spellings are not decoded.
		{`{{ or .Subscriber.FirstName &#034;x&#034; }}`, `{{ or .Subscriber.FirstName &#034;x&#034; }}`},
		// Single-line: an action split across lines is left alone.
		{"{{ or .Subscriber.FirstName\n&quot;x&quot; }}", "{{ or .Subscriber.FirstName\n&quot;x&quot; }}"},
		// A body with no actions is unchanged.
		{`<p>Say &quot;hi&quot; &amp; wave</p>`, `<p>Say &quot;hi&quot; &amp; wave</p>`},
		{`{{ unclosed &quot;x&quot;`, `{{ unclosed &quot;x&quot;`},
		// Decoded before the upstream normalizers, so a builder-escaped TrackLink gets its dot.
		{`{{ TrackLink &quot;https://example.com/&quot; }}`, `{{ TrackLink "https://example.com/" . }}`},
	}
	for _, c := range cases {
		if got := rewrite(c.in); got != c.want {
			t.Errorf("rewrite(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	funcs := template.FuncMap{
		"TrackView": func(_ any) template.HTML { return "" },
		"TrackLink": func(url string, _ any) string { return url },
	}
	for _, ct := range []string{CampaignContentTypeHTML, CampaignContentTypeRichtext, CampaignContentTypeVisual} {
		c := &Campaign{
			Subject:     `Hi {{ or .Subscriber.FirstName "there" }}`, // typed: plain quotes
			ContentType: ct,
			// As the visual builder's Text block (marked) stores it.
			Body: `<p>Hi {{ or .Subscriber.FirstName &quot;there&quot; }},</p><p>&quot;quoted&quot;</p>`,
		}
		if err := c.CompileTemplate(funcs); err != nil {
			t.Fatalf("%s: CompileTemplate: %v", ct, err)
		}
		if c.SubjectTpl == nil {
			t.Fatalf("%s: SubjectTpl is nil", ct)
		}
		for _, tc := range []struct {
			name, subj, greet string
		}{
			{"", "Hi there", "<p>Hi there,</p>"},
			{"Jane Doe", "Hi Jane", "<p>Hi Jane,</p>"},
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
			if want := tc.greet + "<p>&quot;quoted&quot;</p>"; !bytes.Contains(b.Bytes(), []byte(want)) {
				t.Errorf("%s: body for %q = %q, want it to contain %q", ct, tc.name, b.String(), want)
			}
		}
	}
}
