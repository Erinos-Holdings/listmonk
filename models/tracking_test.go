package models

import (
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
