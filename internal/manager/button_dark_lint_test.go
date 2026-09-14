package manager

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// vmlButton is a Button block as the compiled, rendered mail carries it: the 'border' variant
// of postProcess.ts buildVmlButton/buildVmlLabel inside its mso conditional, followed by the
// non-mso <a> twin (which the lint must ignore -- one warning per button, not per emit).
// The font stack carries &quot; like campaign 66's does.
func vmlButton(fill, label, text string) string {
	return `<!--[if mso]><v:roundrect xmlns:v="urn:schemas-microsoft-com:vml" xmlns:w="urn:schemas-microsoft-com:office:word" ` +
		`href="https://x.test/l" style="height:36pt;v-text-anchor:middle;width:202.5pt;" arcsize="50%" ` +
		`strokecolor="` + label + `" strokeweight="1.5pt" fillcolor="` + fill + `"><w:anchorlock/>` +
		`<center style="color:` + label + `;font-family:Avenir, &quot;Avenir Next LT Pro&quot;, sans-serif;font-size:9pt;font-weight:bold;">` +
		text + `</center></v:roundrect><![endif]-->` +
		`<!--[if !mso]><!--><a href="https://x.test/l" style="background-color:` + fill + `;color:` + label +
		`;border:2px solid ` + label + `;border-radius:64px">` + text + `</a><!--<![endif]-->`
}

const buttonWarnSuffix = `: dark label on a light fill — invisible in Windows Outlook dark mode; use a light label and border on a non-light fill.`

// BUTTON-DARK-MODE-SPEC I3 -- warns on campaign 49's shape, silent on the surviving shapes and
// on anything it cannot read.
func TestButtonDarkLintShapes(t *testing.T) {
	t.Run("campaign 49: #000000 on #F5F5F5 warns", func(t *testing.T) {
		w := ButtonDarkModeWarnings(vmlButton("#F5F5F5", "#000000", "FOLLOW US TO STAY IN THE KNOW"))
		want := `Button "FOLLOW US TO STAY IN THE KNOW"` + buttonWarnSuffix
		if len(w) != 1 || w[0] != want {
			t.Fatalf("expected exactly %q, got %v", want, w)
		}
	})

	silent := []struct{ name, body string }{
		{"campaign 66 as stored: #f9f9f9 on #e54582", vmlButton("#e54582", "#f9f9f9", "FOLLOW TO STAY IN THE KNOW")},
		{"#ffffff on 66's fill", vmlButton("#e54582", "#ffffff", "Shop")},
		{"campaign 52 / template 29: #fbf00b on #000000 (light-on-dark, D5)", vmlButton("#000000", "#fbf00b", "Shop")},
		{"dark label on a dark fill", vmlButton("#000000", "#111111", "Shop")},
		{"mid-tone #777777 label on white", vmlButton("#ffffff", "#777777", "Shop")},
		{"unparseable fillcolor keyword", vmlButton("white", "#000000", "Shop")},
		{"unparseable 8-digit fillcolor", vmlButton("#F5F5F580", "#000000", "Shop")},
		{"unparseable label colour", vmlButton("#F5F5F5", "black", "Shop")},
		{"roundrect with no <center> (hand-written VML)",
			`<!--[if mso]><v:roundrect fillcolor="#ffffff" strokecolor="#000000"><w:anchorlock/><p style="color:#000000">x</p></v:roundrect><![endif]-->`},
		{"roundrect with no fillcolor",
			`<v:roundrect strokecolor="#000000"><w:anchorlock/><center style="color:#000000">x</center></v:roundrect>`},
		{"plain HTML button only", `<a style="background-color:#F5F5F5;color:#000000;border:2px solid #000">Shop</a>`},
	}
	for _, c := range silent {
		t.Run("silent: "+c.name, func(t *testing.T) {
			if w := ButtonDarkModeWarnings(c.body); len(w) != 0 {
				t.Fatalf("expected no warnings, got %v", w)
			}
		})
	}

	t.Run("zero roundrects is nil", func(t *testing.T) {
		if w := ButtonDarkModeWarnings("<html><body><p style=\"color:#000\">hi</p></body></html>"); w != nil {
			t.Fatalf("expected nil, got %#v", w)
		}
	})

	t.Run("a <center>-less roundrect never borrows the next button's label", func(t *testing.T) {
		body := `<v:roundrect fillcolor="#ffffff"><w:anchorlock/></v:roundrect>` + vmlButton("#e54582", "#ffffff", "Good") +
			`<v:roundrect fillcolor="#ffffff"/>` + vmlButton("#000000", "#fbf00b", "Also good")
		if w := ButtonDarkModeWarnings(body); len(w) != 0 {
			t.Fatalf("expected no warnings, got %v", w)
		}
	})

	t.Run("'font' variant label markup is read as text", func(t *testing.T) {
		body := `<v:roundrect fillcolor="#FFFFFF"><w:anchorlock/><center style="color:rgb(0, 0, 0);font-size:9pt;">` +
			`<font color="#000000"><span style="color:#000000">Shop &amp;   Save</span></font></center></v:roundrect>`
		w := ButtonDarkModeWarnings(body)
		if len(w) != 1 || w[0] != `Button "Shop & Save"`+buttonWarnSuffix {
			t.Fatalf("unexpected warnings: %v", w)
		}
	})

	t.Run("wired into RenderWarnings after the existing lints", func(t *testing.T) {
		body := `<html><body><img src="data:image/png;base64,AAAA" alt="one" width="10">` +
			vmlButton("#F5F5F5", "#000000", "Follow") + `</body></html>`
		w := RenderWarnings([]byte(body))
		if len(w) != 2 || !strings.Contains(w[0], "data: URI") || w[1] != `Button "Follow"`+buttonWarnSuffix {
			t.Fatalf("expected the embed warning then the button warning, got %v", w)
		}
	})
}

// BUTTON-DARK-MODE-SPEC I4 -- one warning per failing button, in document order, with the
// label truncated to 40 runes.
func TestButtonDarkLintPerButtonOrder(t *testing.T) {
	long := "Découvrez la nouvelle collection d’automne dès aujourd’hui"
	body := "<html><body>" +
		vmlButton("#F5F5F5", "#000000", "First") +
		vmlButton("#e54582", "#ffffff", "Survives") +
		vmlButton("#ffffff", "#262626", long) +
		"</body></html>"

	w := ButtonDarkModeWarnings(body)
	if len(w) != 2 {
		t.Fatalf("expected two warnings, got %d: %v", len(w), w)
	}
	if w[0] != `Button "First"`+buttonWarnSuffix {
		t.Fatalf("first warning: %q", w[0])
	}
	want := `Button "` + string([]rune(long)[:40]) + `…"` + buttonWarnSuffix
	if w[1] != want {
		t.Fatalf("second warning:\n got %q\nwant %q", w[1], want)
	}
}

// BUTTON-DARK-MODE-SPEC I7 -- the thresholds are ports of darkSim.js's DARK_INK/LIGHT_GROUND.
// Pinned both as literals (the TestClipThresholdPinned pattern) and against the JS source, so
// a change on either side fails here rather than letting the warning and the preview drift.
func TestButtonDarkThresholdsPinned(t *testing.T) {
	if buttonDarkInk != 0.3 || buttonLightGround != 0.6 {
		t.Fatalf("buttonDarkInk/buttonLightGround = %v/%v, pinned at 0.3/0.6 — BUTTON-DARK-MODE-SPEC D5; change darkSim.js with them",
			buttonDarkInk, buttonLightGround)
	}

	src, err := os.ReadFile(filepath.Join("..", "..", "frontend", "email-builder", "src", "darkSim.js"))
	if err != nil {
		t.Fatalf("reading darkSim.js: %v", err)
	}
	for name, want := range map[string]float64{"DARK_INK": buttonDarkInk, "LIGHT_GROUND": buttonLightGround} {
		m := regexp.MustCompile(`const\s+` + name + `\s*=\s*([0-9.]+)\s*;`).FindSubmatch(src)
		if m == nil {
			t.Fatalf("darkSim.js no longer declares %s", name)
		}
		got, _ := strconv.ParseFloat(string(m[1]), 64)
		if got != want {
			t.Fatalf("darkSim.js %s = %v, Go = %v — the two languages drifted", name, got, want)
		}
	}
}

// I7 -- parseCSSColor accepts exactly darkSim.js::parseCssColor's forms (cases mirror
// frontend/email-builder/test/dark-sim.test.cjs).
func TestButtonDarkParseColor(t *testing.T) {
	type rgb struct{ r, g, b int }
	ok := map[string]rgb{
		"#fff":                 {255, 255, 255},
		"#FFF":                 {255, 255, 255},
		"#000000":              {0, 0, 0},
		"#F5F5F5":              {245, 245, 245},
		"  #e54582 ":           {229, 69, 130},
		"rgb(255, 255, 255)":   {255, 255, 255},
		"rgb(0,0,0)":           {0, 0, 0},
		"rgb(1 2 3)":           {1, 2, 3},
		"rgba(0,0,0,0.9)":      {0, 0, 0},
		"rgba(10, 20, 30, .5)": {10, 20, 30},
		"RGB(0,0,0)":           {0, 0, 0},
	}
	for in, want := range ok {
		r, g, b, parsed := parseCSSColor(in)
		if !parsed || (rgb{r, g, b}) != want {
			t.Errorf("parseCSSColor(%q) = %v,%v,%v ok=%v; want %v", in, r, g, b, parsed, want)
		}
	}

	for _, in := range []string{
		"rgb(300,0,0)", "#00000080", "#0008", "#ff", "black", "transparent", "inherit",
		"linear-gradient(#fff,#000)", "", "#gggggg",
	} {
		if _, _, _, parsed := parseCSSColor(in); parsed {
			t.Errorf("parseCSSColor(%q) parsed; darkSim.js rejects it", in)
		}
	}
}
