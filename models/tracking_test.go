package models

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"testing"
)

// Fork (visual tracking) -- the href rewrite's skip rules (TRACKING-SPEC §3b/§4.2).
func TestRewriteVisualTrackLinksSkipRules(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"plain https href is wrapped",
			`<a href="https://example.com/page">x</a>`,
			`<a href="{{ TrackLink "https://example.com/page" . }}">x</a>`,
		},
		{
			"plain http href is wrapped",
			`<a href="http://example.com/">x</a>`,
			`<a href="{{ TrackLink "http://example.com/" . }}">x</a>`,
		},
		{
			"&amp; entity survives into the TrackLink call verbatim",
			`<a href="https://example.com/?a=1&amp;b=2">x</a>`,
			`<a href="{{ TrackLink "https://example.com/?a=1&amp;b=2" . }}">x</a>`,
		},
		{
			"@TrackLink shorthand is left to the existing pipeline",
			`<a href="https://example.com/p@TrackLink">x</a>`,
			`<a href="https://example.com/p@TrackLink">x</a>`,
		},
		{
			"template expression inside the URL is skipped",
			`<a href="https://example.com/{{ .Subscriber.UUID }}">x</a>`,
			`<a href="https://example.com/{{ .Subscriber.UUID }}">x</a>`,
		},
		{
			"template-expr href never matches (not href-quote-http)",
			`<a href="{{ UnsubscribeURL }}">x</a>`,
			`<a href="{{ UnsubscribeURL }}">x</a>`,
		},
		{
			"backslash in the URL is left plain (illegal in a template string literal)",
			`<a href="https://example.com/\evil">x</a>`,
			`<a href="https://example.com/\evil">x</a>`,
		},
		{
			// Review finding 1: [^"]* matches newlines and a raw LF inside a template
			// string literal is a compile error — the URL must stay plain.
			"raw LF in the URL is left plain",
			"<a href=\"https://example.com/a\nb\">x</a>",
			"<a href=\"https://example.com/a\nb\">x</a>",
		},
		{
			"raw CR in the URL is left plain (control chars skipped for margin)",
			"<a href=\"https://example.com/a\rb\">x</a>",
			"<a href=\"https://example.com/a\rb\">x</a>",
		},
		{
			// Review finding 2: a mid-URL @TrackLink would let the shorthand regexp
			// match inside the emitted quoted string and nest actions — skip on
			// containment, not suffix, confining the damage to upstream parity.
			"mid-URL @TrackLink is left to the existing pipeline",
			`<a href="https://example.com/@TrackLinkFoo">x</a>`,
			`<a href="https://example.com/@TrackLinkFoo">x</a>`,
		},
		{
			"mailto is not matched",
			`<a href="mailto:a@b.c">x</a>`,
			`<a href="mailto:a@b.c">x</a>`,
		},
		{
			"fragment-only href is not matched",
			`<a href="#top">x</a>`,
			`<a href="#top">x</a>`,
		},
		{
			"single-quoted href misses (not corrupted)",
			`<a href='https://example.com/'>x</a>`,
			`<a href='https://example.com/'>x</a>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rewriteVisualTrackLinks(c.in); got != c.want {
				t.Fatalf("rewrite mismatch:\n got  %s\n want %s", got, c.want)
			}
		})
	}
}

// Fork (visual tracking) -- TRACKING-SPEC §4.5: a Safe-encoded Outlook payload
// (real markup from dev campaign 22: the VML roundrect with href=\"…\" and
// hex-encoded brackets/spaces) passes through the rewrite byte-preserved —
// nothing matches inside a Safe payload, and nothing may be corrupted there.
const safeVMLFixture = `{{ Safe "\x3c!--[if\x20mso]\x3e\x3cv:roundrect\x20xmlns:v=\"urn:schemas-microsoft-com:vml\"\x20xmlns:w=\"urn:schemas-microsoft-com:office:word\"\x20href=\"https://listmonk.app\"\x20style=\"height:24pt;v-text-anchor:middle;width:202.5pt;\"\x20arcsize=\"50%\"\x20strokecolor=\"#FBF00B\"\x20fillcolor=\"#FBF00B\"\x3e\x3cw:anchorlock/\x3e\x3ccenter\x20style=\"color:#000000;font-family:Arial,\x20sans-serif;font-size:12pt;font-weight:bold;\"\x3eButton\x3c/center\x3e\x3c/v:roundrect\x3e\x3c![endif]--\x3e" }}`

func TestRewriteSafePayloadBytePreserved(t *testing.T) {
	if got := rewriteVisualTrackLinks(safeVMLFixture); got != safeVMLFixture {
		t.Fatalf("Safe payload not byte-preserved:\n got  %s\n want %s", got, safeVMLFixture)
	}

	// The payload beside a plain anchor: the anchor is wrapped, the payload untouched.
	body := safeVMLFixture + `<a href="https://plain.example/p">p</a>`
	got := rewriteVisualTrackLinks(body)
	if !strings.HasPrefix(got, safeVMLFixture) {
		t.Fatalf("Safe payload corrupted when a sibling anchor is rewritten:\n%s", got)
	}
	if !strings.Contains(got, `{{ TrackLink "https://plain.example/p" . }}`) {
		t.Fatalf("sibling plain anchor not wrapped:\n%s", got)
	}
}

// Fork (visual tracking) -- the live-tag guard (TRACKING-SPEC §3a, finding 4): only a
// real {{ TrackView }} tag suppresses the base pixel, never the bare string.
func TestLiveTrackViewGuard(t *testing.T) {
	for _, s := range []string{`{{ TrackView }}`, `{{TrackView}}`, `{{  TrackView  }}`} {
		if !regLiveTrackView.MatchString(s) {
			t.Fatalf("live tag %q must match the guard", s)
		}
	}
	for _, s := range []string{
		"our TrackView feature explained",
		`<a href="https://example.com/TrackView">docs</a>`,
		`{{ Safe "TrackView" }}`, // the word inside a Safe payload, not a tag
	} {
		if regLiveTrackView.MatchString(s) {
			t.Fatalf("non-tag string %q must not match the guard", s)
		}
	}
}

// Fork (click tracking) -- CLICK-TRACKING-SPEC T1 (I1, I2, I10, I11): the dynamic-href and
// VML-marker passes of TransformTrackLinks, and I3 (static hrefs unchanged).
func TestTransformTrackLinksDynamic(t *testing.T) {
	const marker = `<span data-lm-vml-href="%s"></span>`
	cases := []struct {
		name        string
		contentType string
		in          string
		want        string
	}{
		{
			"dynamic href → TrackLink with the UNEXPANDED text",
			CampaignContentTypeVisual,
			`<a href="{{ .Subscriber.Attribs.site }}">x</a>`,
			`<a href="{{ TrackLink "{{ .Subscriber.Attribs.site }}" . }}">x</a>`,
		},
		{
			"or idiom: &quot; is decoded and escaped as \\\" (I1)",
			CampaignContentTypeVisual,
			`<a href="{{ or .Subscriber.Attribs.site &quot;https://curatedfor.you&quot; }}">x</a>`,
			`<a href="{{ TrackLink "{{ or .Subscriber.Attribs.site \"https://curatedfor.you\" }}" . }}">x</a>`,
		},
		{
			"&amp; and &#39; decode in a dynamic href",
			CampaignContentTypeVisual,
			`<a href="https://x.test/?a=1&amp;b={{ .Subscriber.UUID }}&amp;c=&#39;q&#39;">x</a>`,
			`<a href="{{ TrackLink "https://x.test/?a=1&b={{ .Subscriber.UUID }}&c='q'" . }}">x</a>`,
		},
		{
			"identical expression twice → identical calls (one links row)",
			CampaignContentTypeVisual,
			`<a href="{{ .Subscriber.Attribs.site }}">a</a><a href="{{ .Subscriber.Attribs.site }}">b</a>`,
			`<a href="{{ TrackLink "{{ .Subscriber.Attribs.site }}" . }}">a</a><a href="{{ TrackLink "{{ .Subscriber.Attribs.site }}" . }}">b</a>`,
		},
		{
			"static href still wrapped exactly as before (I3)",
			CampaignContentTypeVisual,
			`<a href="https://example.com/page?a=1&amp;b=2">x</a>`,
			`<a href="{{ TrackLink "https://example.com/page?a=1&amp;b=2" . }}">x</a>`,
		},
		{
			"backslash in a dynamic href → left plain (I2)",
			CampaignContentTypeVisual,
			`<a href="{{ .Subscriber.Attribs.site }}\x">x</a>`,
			`<a href="{{ .Subscriber.Attribs.site }}\x">x</a>`,
		},
		{
			"raw LF in a dynamic href → left plain (I2)",
			CampaignContentTypeVisual,
			"<a href=\"{{ .Subscriber.Attribs.site }}\nx\">x</a>",
			"<a href=\"{{ .Subscriber.Attribs.site }}\nx\">x</a>",
		},
		{
			"@TrackLink inside a dynamic href → left plain (I2)",
			CampaignContentTypeVisual,
			`<a href="https://x.test/{{ .Subscriber.UUID }}@TrackLink">x</a>`,
			`<a href="https://x.test/{{ .Subscriber.UUID }}@TrackLink">x</a>`,
		},
		{
			"listmonk URL function in an href is never wrapped (the resolver cannot evaluate it)",
			CampaignContentTypeVisual,
			`<a href="{{ UnsubscribeURL }}">u</a><a href="{{ ManageURL }}">m</a><a href="{{ OptinURL }}">o</a><a href="{{ MessageURL }}">v</a>`,
			`<a href="{{ UnsubscribeURL }}">u</a><a href="{{ ManageURL }}">m</a><a href="{{ OptinURL }}">o</a><a href="{{ MessageURL }}">v</a>`,
		},
		{
			"hand-typed TrackLink href with raw quotes is not re-wrapped",
			CampaignContentTypeVisual,
			`<a href="{{ TrackLink "https://x.test" }}">x</a>`,
			`<a href="{{ TrackLink "https://x.test" }}">x</a>`,
		},
		{
			"unbalanced braces (attribute split at a raw quote) → left plain",
			CampaignContentTypeVisual,
			`<a href="{{ or .Subscriber.Attribs.site "https://x.test" }}">x</a>`,
			`<a href="{{ or .Subscriber.Attribs.site "https://x.test" }}">x</a>`,
		},
		{
			"VML marker, static value → tracked (I10)",
			CampaignContentTypeVisual,
			`{{ Safe "…href=\"" }}` + fmt.Sprintf(marker, "https://x.test/go?a=1&amp;b=2") + `{{ Safe "\"…" }}`,
			`{{ Safe "…href=\"" }}{{ TrackLink "https://x.test/go?a=1&b=2" . }}{{ Safe "\"…" }}`,
		},
		{
			"VML marker, dynamic value → unexpanded (I10)",
			CampaignContentTypeVisual,
			`{{ Safe "…href=\"" }}` + fmt.Sprintf(marker, "{{ or .Subscriber.Attribs.site &quot;https://curatedfor.you&quot; }}") + `{{ Safe "\"…" }}`,
			`{{ Safe "…href=\"" }}{{ TrackLink "{{ or .Subscriber.Attribs.site \"https://curatedfor.you\" }}" . }}{{ Safe "\"…" }}`,
		},
		{
			"VML marker in a visual→HTML converted document (content_type=html) is still wrapped (I10)",
			CampaignContentTypeHTML,
			`{{ Safe "…href=\"" }}` + fmt.Sprintf(marker, "{{ .Subscriber.Attribs.site }}") + `{{ Safe "\"…" }}`,
			`{{ Safe "…href=\"" }}{{ TrackLink "{{ .Subscriber.Attribs.site }}" . }}{{ Safe "\"…" }}`,
		},
		{
			"VML marker padded by the format-switch beautifier (whitespace around and inside) is consumed",
			CampaignContentTypeHTML,
			"{{ Safe \"…href=\\\"\" }}<span data-lm-vml-href=\"https://x.test/go\">\n    </span>\n    {{ Safe \"\\\"…\" }}",
			`{{ Safe "…href=\"" }}{{ TrackLink "https://x.test/go" . }}{{ Safe "\"…" }}`,
		},
		{
			"richtext and markdown bodies get the marker pass too",
			CampaignContentTypeRichtext,
			fmt.Sprintf(marker, "https://x.test/r"),
			`{{ TrackLink "https://x.test/r" . }}`,
		},
		{
			"skipped marker (backslash) → decoded VALUE, never a literal span (I2)",
			CampaignContentTypeVisual,
			fmt.Sprintf(marker, "https://x.test/a\\b&amp;c"),
			`https://x.test/a\b&c`,
		},
		{
			"skipped marker (static non-http value) → decoded VALUE",
			CampaignContentTypeVisual,
			fmt.Sprintf(marker, "#"),
			`#`,
		},
		{
			"skipped marker (URL function) → decoded VALUE",
			CampaignContentTypeVisual,
			fmt.Sprintf(marker, "{{ UnsubscribeURL }}"),
			`{{ UnsubscribeURL }}`,
		},
		{
			"hostile marker value: quotes cannot terminate the TrackLink literal (I11)",
			CampaignContentTypeVisual,
			fmt.Sprintf(marker, "https://x.test/a&quot;b&lt;c&gt;d&amp;e"),
			`{{ TrackLink "https://x.test/a\"b<c>d&e" . }}`,
		},
		{
			"plain content is untouched",
			CampaignContentTypePlain,
			fmt.Sprintf(marker, "https://x.test/r") + ` <a href="https://x.test">x</a>`,
			fmt.Sprintf(marker, "https://x.test/r") + ` <a href="https://x.test">x</a>`,
		},
		{
			"non-visual static href is NOT wrapped (I3: only the marker pass runs)",
			CampaignContentTypeHTML,
			`<a href="https://example.com/page">x</a>`,
			`<a href="https://example.com/page">x</a>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TransformTrackLinks(c.in, c.contentType); got != c.want {
				t.Fatalf("transform mismatch:\n got  %s\n want %s", got, c.want)
			}
		})
	}
}

// I1 -- the compiled dynamic TrackLink call is a valid template: the whole visual body
// compiles, and rendering with a FuncMap whose TrackLink echoes its argument shows the
// argument is the unexpanded, entity-decoded expression (quote escapes undone).
func TestDynamicTrackLinkCompiles(t *testing.T) {
	var got []string
	funcs := template.FuncMap{
		"TrackLink": func(url string, _ any) string { got = append(got, url); return "/link/x" },
		"TrackView": func(_ any) template.HTML { return "" },
		"Safe":      func(s string) template.HTML { return template.HTML(s) },
	}
	c := &Campaign{
		Subject:     "s",
		ContentType: CampaignContentTypeVisual,
		Body: `<a href="{{ or .Subscriber.Attribs.site &quot;https://curatedfor.you&quot; }}">x</a>` +
			`{{ Safe "href=\"" }}<span data-lm-vml-href="{{ or .Subscriber.Attribs.site &quot;https://curatedfor.you&quot; }}"></span>{{ Safe "\"" }}`,
	}
	if err := c.CompileTemplate(funcs); err != nil {
		t.Fatalf("CompileTemplate: %v", err)
	}
	var b bytes.Buffer
	if err := c.Tpl.ExecuteTemplate(&b, BaseTpl, map[string]any{}); err != nil {
		t.Fatalf("render: %v", err)
	}
	want := `{{ or .Subscriber.Attribs.site "https://curatedfor.you" }}`
	if len(got) != 2 || got[0] != want || got[1] != want {
		t.Fatalf("TrackLink received %#v, want two of %q", got, want)
	}
	if !strings.Contains(b.String(), `href="/link/x"`) || !strings.Contains(b.String(), `href="/link/x"`) {
		t.Fatalf("rendered body lacks the tracked hrefs:\n%s", b.String())
	}
}
