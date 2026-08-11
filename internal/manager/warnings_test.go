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
		name    string
		body    string
		want    string // substring expected in the single warning; "" = no warning
	}{
		{
			"hosted image is silent",
			`<img src="https://email.curatedfor.you/uploads/logo.png" alt="logo" width="200">`,
			"",
		},
		{
			"data: URI warns and names the image by alt",
			`<img alt="brand logo" src="data:image/png;base64,iVBORw0KGgo=">`,
			`"brand logo" uses a data: URI`,
		},
		{
			"data: URI without alt is named by src",
			`<img src="data:image/gif;base64,R0lGODlhAQABAAAAACH5BAEKAAEALAAAAAABAAEAAAICTAEAOw==">`,
			`(src "data:image/gif;base64,R0lGODlhAQABAAAAAC…") uses a data: URI`,
		},
		{
			"bare cid: reference is a dead reference",
			`<img src="cid:abc123@email" alt="header">`,
			"no attachment behind it",
		},
		{
			"data-embed opt-in warns as inline CID",
			`<img src="https://email.curatedfor.you/uploads/logo.png" data-embed="true" alt="logo">`,
			"embed inline (CID)",
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
		`<img src="data:image/png;base64,AAAA" alt="one">` +
		`<img src="cid:dead@email" alt="two">` +
		"</body></html>"
	w := RenderWarnings([]byte(body))
	if len(w) != 3 {
		t.Fatalf("expected 3 warnings, got %d: %v", len(w), w)
	}
	if !strings.Contains(w[0], "Gmail clips") {
		t.Fatalf("expected the clip warning first, got %v", w[0])
	}
}
