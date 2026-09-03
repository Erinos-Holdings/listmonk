package manager

import (
	"fmt"
	"mime/quotedprintable"
	"regexp"
	"strings"
)

// GmailClipWarnBytes is the quoted-printable-encoded size above which a rendered
// message gets a clip warning. Gmail clips HTML parts over ~102 KB — measured on
// the encoded part, which is how smtppool writes it — and a clipped copy hides the
// trailing tracking pixel, so opens go unrecorded too. 90 KB leaves headroom for
// per-subscriber URL length variance (encoding inflation is modeled, not part of
// the headroom).
const GmailClipWarnBytes = 90 * 1024

// WarnNoPreheader is surfaced when a campaign is started with no preheader text.
// Start sends the last saved campaign, hence the Save reminder.
const WarnNoPreheader = "No preheader text is set — inboxes will preview whatever body text comes first. " +
	"Setting the Preheader field is a cheap open-rate win (Save before Start; Start sends the last saved campaign)."

// reImgTagWarn matches whole <img> tags for the embed lint. Separate from
// reInlineImage, which only matches data-embed-marked tags.
var reImgTagWarn = regexp.MustCompile(`(?is)<img\b[^>]*>`)

// reAltAttr extracts an alt attribute so lint messages can name the offending image.
// The leading delimiter class (not \b) keeps hyphenated attributes like data-alt from
// matching — \b sees a boundary between '-' and 'a'.
var reAltAttr = regexp.MustCompile(`(?is)(?:^|[\s"'])alt\s*=\s*(?:"([^"]*)"|'([^']*)')`)

// Size lint (IMAGE-WIDTH-SPEC §6): an <img> counts as sized when it carries a width
// attribute, a px width in its inline style, a height attribute, or a px height in its
// inline style — the forms Word-based Outlook honors. Everything else it draws at the
// stored pixel size. Attribute regexes are delimiter-anchored like reAltAttr so
// data-width/data-height do not match.
var (
	reWidthAttr  = regexp.MustCompile(`(?is)(?:^|[\s"'])width\s*=\s*(?:"[^"]+"|'[^']+'|[^\s"'>]+)`)
	reHeightAttr = regexp.MustCompile(`(?is)(?:^|[\s"'])height\s*=\s*(?:"[^"]+"|'[^']+'|[^\s"'>]+)`)
	reStyleAttr  = regexp.MustCompile(`(?is)(?:^|[\s"'])style\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	reStyleWidth = regexp.MustCompile(`(?i)(?:^|;)\s*width\s*:\s*\d+(?:\.\d+)?px\s*(?:;|$)`)
	reStyleHght  = regexp.MustCompile(`(?i)(?:^|;)\s*height\s*:\s*\d+(?:\.\d+)?px\s*(?:;|$)`)
)

// countingWriter tallies bytes written; used to size QP output without buffering it.
type countingWriter int

func (c *countingWriter) Write(p []byte) (int, error) {
	*c += countingWriter(len(p))
	return len(p), nil
}

// QPEncodedSize returns the size of b after quoted-printable encoding — the form
// the SMTP sender (smtppool writeMessage) actually emits the HTML part in, and the
// form Gmail's clip threshold applies to. QP inflates attribute-dense email markup
// 10–25%, so raw byte counts under-measure.
func QPEncodedSize(b []byte) int {
	var n countingWriter
	w := quotedprintable.NewWriter(&n)
	// Writes to countingWriter cannot fail; quotedprintable surfaces no other errors.
	w.Write(b)
	w.Close()
	return int(n)
}

// RenderWarnings inspects a fully rendered per-subscriber message body (template
// wrap + preheader + tracking pixel, i.e. what CampaignMessage.render() emits for
// a dummy subscriber) and returns non-blocking send-quality warnings: the Gmail
// clip check and the embedded-image lint. The input must be the rendered output,
// never the raw editor body — the base template and Outlook dual-emit markup count
// toward Gmail's limit too.
func RenderWarnings(rendered []byte) []string {
	var warns []string

	if n := QPEncodedSize(rendered); n > GmailClipWarnBytes {
		warns = append(warns, fmt.Sprintf(
			"The rendered message is ~%d KB encoded for sending (warn threshold %d KB) — Gmail clips messages over ~102 KB "+
				"behind a \"View entire message\" link, and clipped copies never load the tracking pixel, so opens go unrecorded. "+
				"Trim the campaign body.", n/1024, GmailClipWarnBytes/1024))
	}

	warns = append(warns, embedWarnings(rendered)...)
	warns = append(warns, sizeWarnings(rendered)...)
	return warns
}

// sizeWarnings names every <img> with no resolvable width OR height (attribute or px
// style), once per src — the Outlook mso/non-mso dual emit shows one image twice.
// Neutral wording: RenderWarnings also runs for richtext/HTML campaigns, and a
// hotlinked asset is not bounded by the media optimizer. The tracking pixel passes
// because TrackView emits width="1" height="1" (D3.7). Html-block images the visual
// builder never rewrites are named here too — that is the backstop's job.
func sizeWarnings(rendered []byte) []string {
	var out []string
	seen := map[string]bool{}
	for _, tag := range reImgTagWarn.FindAllString(string(rendered), -1) {
		if imgHasSize(tag) {
			continue
		}
		src := extractSrc(tag)
		key := src
		if key == "" {
			key = tag
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, fmt.Sprintf(
			"Image %s has no width or height — Outlook draws it at its stored pixel size, which can be wider than "+
				"the 600px canvas. Give it a width (in the visual builder: open the block and set Width, or re-select "+
				"the image and the builder fills it in).", imgName(tag, src)))
	}
	return out
}

// imgHasSize reports whether an <img> tag carries a width or height attribute, or a
// px width/height in its inline style.
func imgHasSize(tag string) bool {
	if reWidthAttr.MatchString(tag) || reHeightAttr.MatchString(tag) {
		return true
	}
	m := reStyleAttr.FindStringSubmatch(tag)
	if m == nil {
		return false
	}
	style := m[1]
	if style == "" {
		style = m[2]
	}
	return reStyleWidth.MatchString(style) || reStyleHght.MatchString(style)
}

// embedWarnings lints the rendered body for images that bypass hosted media URLs:
// data: URIs (Gmail strips them; Word-based Outlook doesn't render them), bare cid:
// references (no attachment behind them — dead by construction, since inline-embed
// resolution skips tags that are already cid:), and data-embed opt-ins (resolved to
// real CID attachments at send time, but each copy carries the full image weight to
// every recipient, outside the hosted-media/CDN pipeline).
func embedWarnings(rendered []byte) []string {
	var out []string
	for _, tag := range reImgTagWarn.FindAllString(string(rendered), -1) {
		src := extractSrc(tag)
		lsrc := strings.ToLower(src)
		name := imgName(tag, src)

		switch {
		case strings.HasPrefix(lsrc, "data:"):
			out = append(out, fmt.Sprintf(
				"Image %s uses a data: URI — Gmail strips these entirely and Word-based Outlook does not render them, "+
					"so it will be invisible to most recipients. Upload it to the media manager and use its URL.", name))
		case strings.HasPrefix(lsrc, "cid:"):
			out = append(out, fmt.Sprintf(
				"Image %s references cid: content with no attachment behind it and will not display. "+
					"Use a media manager URL instead.", name))
		case strings.Contains(strings.ToLower(tag), attribInlineEmbed):
			out = append(out, fmt.Sprintf(
				"Image %s is set to embed inline (CID) — every sent copy carries the full image bytes and skips the "+
					"hosted media CDN. Prefer a media manager URL unless inline embedding is deliberate.", name))
		}
	}
	return out
}

// imgName labels an offending <img> for a warning: alt text when present,
// otherwise a truncated src, otherwise a generic label.
func imgName(tag, src string) string {
	if m := reAltAttr.FindStringSubmatch(tag); m != nil {
		alt := m[1]
		if alt == "" {
			alt = m[2]
		}
		if alt != "" {
			return fmt.Sprintf("%q", alt)
		}
	}
	if src != "" {
		if len(src) > 40 {
			src = src[:40] + "…"
		}
		return fmt.Sprintf("(src %q)", src)
	}
	return "(no alt text)"
}
