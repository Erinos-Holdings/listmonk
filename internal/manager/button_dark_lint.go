package manager

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Fork (BUTTON-DARK-MODE-SPEC D2/D5/D6) -- a Button whose label is dark on a light fill is an
// EMPTY pill in Windows Outlook dark mode: Word lightens a label it judges dark and never
// touches the VML fillcolor, so the lightened label lands on the still-light pill (campaign 49,
// Inspect DqU865vB5L9GkVQgGXubfXOq4RjWiZY0CP30IvyMewHle, 2026-09-14; runbook hazard 54). No
// client hook survives Word, so the fix is the author's (a light label and border on a
// non-light fill); this lint only names the buttons that provably fail.
//
// It reads the VML the mail actually carries -- the roundrect's fillcolor and the colour of
// the single <center> label buildVmlLabel (postProcess.ts) emits inside it -- so it judges what
// Word sees, stored bodies included. Light-on-dark is degraded in Word, not invisible, and is
// deliberately not warned (spec §8 Q1). Anything it cannot read is a MISS, never a warning.
//
// TWO-LANGUAGE CONSTANTS: the thresholds and the colour parser are ports of
// frontend/email-builder/src/darkSim.js (DARK_INK, LIGHT_GROUND, parseCssColor, luma), so the
// warning agrees with what the preview's Gmail-style scheme shows for the same CSS colour.
// The media classifier's darkInk/nearWhite (internal/media/optimizer) classify upload pixels
// and are deliberately NOT these. TestButtonDarkThresholdsPinned and TestButtonDarkParseColor
// pin both sides -- change them together.
const (
	buttonDarkInk     = 0.3 // darkSim.js DARK_INK: label luma <= this is "dark"
	buttonLightGround = 0.6 // darkSim.js LIGHT_GROUND: fill luma >= this is "light"
)

// buttonLabelMaxRunes bounds the label text quoted in a warning.
const buttonLabelMaxRunes = 40

var (
	reVMLRoundrectOpen  = regexp.MustCompile(`(?is)<v:roundrect\b[^>]*>`)
	reVMLRoundrectClose = regexp.MustCompile(`(?i)</v:roundrect\s*>`)
	reVMLCenterOpen     = regexp.MustCompile(`(?is)<center\b[^>]*>`)
	reVMLCenterClose    = regexp.MustCompile(`(?i)</center\s*>`)
	reFillColorAttr     = regexp.MustCompile(`(?is)(?:^|[\s"'])fillcolor\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	reCSSColorDecl      = regexp.MustCompile(`(?i)(?:^|;)\s*color\s*:\s*([^;]+)`)
	reAnyTag            = regexp.MustCompile(`(?s)<[^>]*>`)

	reCSSHexShort = regexp.MustCompile(`(?i)^#([0-9a-f])([0-9a-f])([0-9a-f])$`)
	reCSSHexLong  = regexp.MustCompile(`(?i)^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$`)
	reCSSRGBFn    = regexp.MustCompile(`(?i)^rgba?\(\s*(\d{1,3})\s*[,\s]\s*(\d{1,3})\s*[,\s]\s*(\d{1,3})\s*(?:[,/][^)]*)?\)$`)
)

// parseCSSColor is darkSim.js::parseCssColor: 3/6-digit hex and rgb()/rgba() only. Keywords,
// #rrggbbaa, out-of-range channels and anything else return ok=false. Two known divergences,
// both unreachable in compiled mail and both a miss here (never a false warning): JS trim()
// strips a trailing U+FEFF where TrimSpace does not, and JS \s is Unicode where RE2's is
// ASCII, so "#fff\ufeff" and "rgb(0,\u00a00,0)" parse in the simulator and not here.
func parseCSSColor(value string) (r, g, b int, ok bool) {
	v := strings.TrimSpace(value)

	if m := reCSSHexShort.FindStringSubmatch(v); m != nil {
		r, _ := strconv.ParseInt(m[1]+m[1], 16, 0)
		g, _ := strconv.ParseInt(m[2]+m[2], 16, 0)
		b, _ := strconv.ParseInt(m[3]+m[3], 16, 0)
		return int(r), int(g), int(b), true
	}

	if m := reCSSHexLong.FindStringSubmatch(v); m != nil {
		r, _ := strconv.ParseInt(m[1], 16, 0)
		g, _ := strconv.ParseInt(m[2], 16, 0)
		b, _ := strconv.ParseInt(m[3], 16, 0)
		return int(r), int(g), int(b), true
	}

	if m := reCSSRGBFn.FindStringSubmatch(v); m != nil {
		r, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[3])
		if r <= 255 && g <= 255 && b <= 255 {
			return r, g, b, true
		}
	}

	return 0, 0, 0, false
}

// cssLuma is darkSim.js::luma -- Rec. 709 on the sRGB-encoded channels (gamma space), the
// same formula as the media classifier's lumaAt.
func cssLuma(r, g, b int) float64 {
	return (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)) / 255
}

// ButtonDarkModeWarnings names every VML button whose label is dark on a light fill, one
// warning per button in document order. Pure on the body string.
func ButtonDarkModeWarnings(body string) []string {
	var out []string

	opens := reVMLRoundrectOpen.FindAllStringIndex(body, -1)
	for i, loc := range opens {
		tag := body[loc[0]:loc[1]]
		if strings.HasSuffix(strings.TrimSpace(strings.TrimSuffix(tag, ">")), "/") {
			// Self-closing hand-written VML carries no label.
			continue
		}

		// The shape's content: up to its close tag, and never past the next roundrect.
		end := len(body)
		if i+1 < len(opens) {
			end = opens[i+1][0]
		}
		content := body[loc[1]:end]
		if c := reVMLRoundrectClose.FindStringIndex(content); c != nil {
			content = content[:c[0]]
		}

		fill, ok := attrValue(reFillColorAttr, tag)
		if !ok {
			continue
		}
		fr, fg, fb, ok := parseCSSColor(fill)
		if !ok || cssLuma(fr, fg, fb) < buttonLightGround {
			continue
		}

		center := reVMLCenterOpen.FindStringIndex(content)
		if center == nil {
			continue
		}
		style, ok := attrValue(reStyleAttr, content[center[0]:center[1]])
		if !ok {
			continue
		}
		cm := reCSSColorDecl.FindStringSubmatch(style)
		if cm == nil {
			continue
		}
		lr, lg, lb, ok := parseCSSColor(cm[1])
		if !ok || cssLuma(lr, lg, lb) > buttonDarkInk {
			continue
		}

		label := content[center[1]:]
		if c := reVMLCenterClose.FindStringIndex(label); c != nil {
			label = label[:c[0]]
		}

		// Plain %s, not %q: the label is user-authored copy already tag-stripped, unescaped
		// and whitespace-collapsed, and Go-quoting would show a quote in it as \".
		out = append(out, fmt.Sprintf(
			"Button \"%s\": dark label on a light fill — invisible in Windows Outlook dark mode; "+
				"use a light label and border on a non-light fill.", buttonLabelText(label)))
	}

	return out
}

// attrValue extracts an HTML-unescaped attribute value with a two-group (double/single
// quoted) regex like reStyleAttr. Unescaping matters: a font stack arrives as &quot;…&quot;,
// whose ';' would otherwise split a CSS declaration.
func attrValue(re *regexp.Regexp, tag string) (string, bool) {
	m := re.FindStringSubmatch(tag)
	if m == nil {
		return "", false
	}
	v := m[1]
	if v == "" {
		v = m[2]
	}
	return html.UnescapeString(v), true
}

// buttonLabelText is the <center>'s text content: tags stripped, HTML-unescaped,
// whitespace-collapsed, truncated to buttonLabelMaxRunes.
func buttonLabelText(inner string) string {
	text := strings.Join(strings.Fields(html.UnescapeString(reAnyTag.ReplaceAllString(inner, " "))), " ")
	if r := []rune(text); len(r) > buttonLabelMaxRunes {
		text = string(r[:buttonLabelMaxRunes]) + "…"
	}
	return text
}
