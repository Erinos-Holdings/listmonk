package manager

import (
	"bytes"
	"strings"
	"testing"
)

// The threshold is a deliberate, argued-for constant (90 KB encoded: headroom under
// Gmail's ~102 KB clip for per-subscriber URL variance). A drive-by change should
// fail a test, not slip through a refactor.
func TestClipThresholdPinned(t *testing.T) {
	if GmailClipWarnBytes != 90*1024 {
		t.Fatalf("GmailClipWarnBytes = %d, pinned at 90 KB — see POLISH-SPEC/runbook before changing", GmailClipWarnBytes)
	}
}

// Pins that the measured artifact is the QUOTED-PRINTABLE-ENCODED rendered output,
// not raw bytes: a body whose raw size is under the threshold but whose QP encoding
// is far over it (every '=' becomes '=3D') must warn, while an equal-length body of
// plain letters (QP inflation ~1.3% from soft line breaks alone) must not.
func TestClipWarningMeasuresQPEncodedSize(t *testing.T) {
	const rawLen = 80 * 1024 // under the 90 KB threshold raw

	inflating := bytes.Repeat([]byte("="), rawLen) // QP-encodes to ~3x
	if w := RenderWarnings(inflating); len(w) == 0 || !strings.Contains(w[0], "Gmail clips") {
		t.Fatalf("expected clip warning for %d raw bytes of '=' (QP ~%d), got %v", rawLen, QPEncodedSize(inflating), w)
	}

	benign := bytes.Repeat([]byte("a"), rawLen)
	if w := RenderWarnings(benign); len(w) != 0 {
		t.Fatalf("expected no warning for %d raw bytes of 'a' (QP %d), got %v", rawLen, QPEncodedSize(benign), w)
	}
}

func TestClipWarningBoundary(t *testing.T) {
	// 'a' bodies inflate only by soft line breaks ("=\r\n" every 75 chars, ~4%).
	// Build one that QP-encodes just over the threshold and one comfortably under.
	over := bytes.Repeat([]byte("a"), 89*1024)
	if QPEncodedSize(over) <= GmailClipWarnBytes {
		t.Fatalf("test fixture error: expected QP size of 89 KB of 'a' to exceed %d", GmailClipWarnBytes)
	}
	if w := RenderWarnings(over); len(w) == 0 {
		t.Fatal("expected clip warning just over the encoded threshold")
	}

	under := bytes.Repeat([]byte("a"), 80*1024)
	if w := RenderWarnings(under); len(w) != 0 {
		t.Fatalf("expected no clip warning well under the encoded threshold, got %v", w)
	}
}

func TestEmbedLint(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string // substring expected in the single warning; "" = no warning
	}{
		{
			"hosted image is silent",
			`<img src="https://email.curatedfor.you/uploads/logo.png" alt="logo" width="200">`,
			"",
		},
		{
			"data: URI warns and names the image by alt",
			`<img alt="brand logo" src="data:image/png;base64,iVBORw0KGgo=" width="10">`,
			`"brand logo" uses a data: URI`,
		},
		{
			"data: URI without alt is named by src",
			`<img src="data:image/gif;base64,R0lGODlhAQABAAAAACH5BAEKAAEALAAAAAABAAEAAAICTAEAOw==" width="1">`,
			`(src "data:image/gif;base64,R0lGODlhAQABAAAAAC…") uses a data: URI`,
		},
		{
			"bare cid: reference is a dead reference",
			`<img src="cid:abc123@email" alt="header" width="100">`,
			"no attachment behind it",
		},
		{
			"data-embed opt-in warns as inline CID",
			`<img src="https://email.curatedfor.you/uploads/logo.png" data-embed="true" alt="logo" width="100">`,
			"embed inline (CID)",
		},
		{
			// \b would treat data-alt as alt (boundary between '-' and 'a') and mislabel
			// the image; the delimiter-anchored regex must fall back to naming it by src.
			"hyphenated data-alt attribute is NOT mistaken for alt",
			`<img data-alt="hero" src="data:image/png;base64,AAAA" width="10">`,
			`(src "data:image/png;base64,AAAA") uses a data: URI`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := RenderWarnings([]byte("<html><body>" + c.body + "</body></html>"))
			if c.want == "" {
				if len(w) != 0 {
					t.Fatalf("expected no warnings, got %v", w)
				}
				return
			}
			if len(w) != 1 || !strings.Contains(w[0], c.want) {
				t.Fatalf("expected one warning containing %q, got %v", c.want, w)
			}
		})
	}
}

// A body carrying several problems reports each one; the clip warning leads.
func TestWarningsCombine(t *testing.T) {
	body := "<html><body>" +
		strings.Repeat("=", 90*1024) +
		`<img src="data:image/png;base64,AAAA" alt="one" width="10">` +
		`<img src="cid:dead@email" alt="two" width="10">` +
		"</body></html>"
	w := RenderWarnings([]byte(body))
	if len(w) != 3 {
		t.Fatalf("expected 3 warnings, got %d: %v", len(w), w)
	}
	if !strings.Contains(w[0], "Gmail clips") {
		t.Fatalf("expected the clip warning first, got %v", w[0])
	}
}

// IMAGE-WIDTH-SPEC I7 -- the size lint names an <img> with no width or height once
// across the Outlook mso/non-mso dual emit, and stays silent for every sizing form
// Word honors (width/height attribute, px width/height style) and for the tracking
// pixel as TrackView emits it (width="1" height="1", D3.7).
func TestSizeLint(t *testing.T) {
	const pixel = `<img src="https://lm.test/campaign/c/s/px.png" alt="" width="1" height="1" />`
	silent := []struct{ name, body string }{
		{"width attribute", `<img src="https://x.test/a.png" alt="a" width="26">`},
		{"px width style", `<img src="https://x.test/a.png" alt="a" style="display:block;width:300px;max-width:100%">`},
		{"height attribute", `<img src="https://x.test/a.png" alt="a" height="62">`},
		{"px height style", `<img src="https://x.test/a.png" alt="a" style="height:62px;width:auto">`},
		{"TrackView pixel", pixel},
		{"data-width is not width, but height attr sizes it", `<img src="https://x.test/a.png" data-width="1" height="5">`},
	}
	for _, c := range silent {
		t.Run("silent: "+c.name, func(t *testing.T) {
			if w := RenderWarnings([]byte("<html><body>" + c.body + "</body></html>")); len(w) != 0 {
				t.Fatalf("expected no warnings, got %v", w)
			}
		})
	}

	t.Run("unsized image is named once across the dual emit", func(t *testing.T) {
		unsized := `<img src="https://x.test/hero.png" alt="hero" style="display:block;max-width:100%;border:0">`
		body := "<html><body><!--[if mso]>" + unsized + "<![endif]--><!--[if !mso]><!-->" + unsized + "<!--<![endif]-->" + pixel + "</body></html>"
		w := RenderWarnings([]byte(body))
		if len(w) != 1 {
			t.Fatalf("expected exactly one size warning, got %d: %v", len(w), w)
		}
		if !strings.Contains(w[0], `Image "hero" has no width or height`) {
			t.Fatalf("unexpected wording: %s", w[0])
		}
	})

	t.Run("data-width alone does not count as a width", func(t *testing.T) {
		w := RenderWarnings([]byte(`<html><body><img src="https://x.test/a.png" alt="a" data-width="300"></body></html>`))
		if len(w) != 1 || !strings.Contains(w[0], "has no width or height") {
			t.Fatalf("expected one size warning, got %v", w)
		}
	})

	t.Run("percent style width is not a px size", func(t *testing.T) {
		w := RenderWarnings([]byte(`<html><body><img src="https://x.test/a.png" alt="a" style="width:100%"></body></html>`))
		if len(w) != 1 {
			t.Fatalf("expected one size warning, got %v", w)
		}
	})

	t.Run("two distinct unsized images are two warnings", func(t *testing.T) {
		w := RenderWarnings([]byte(`<html><body><img src="https://x.test/a.png" alt="a"><img src="https://x.test/b.png" alt="b"></body></html>`))
		if len(w) != 2 {
			t.Fatalf("expected two size warnings, got %v", w)
		}
	})
}
