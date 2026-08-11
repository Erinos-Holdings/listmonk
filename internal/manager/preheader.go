package manager

import (
	"html/template"
	"regexp"
	"strings"
)

// regBodyTag locates the opening <body> tag so the preheader lands before any visible
// content. Visual campaigns carry their own full HTML document in the campaign body,
// while richtext/html campaigns get theirs from the base template — so the tag has to be
// found in the rendered output, not in any one source.
var regBodyTag = regexp.MustCompile(`(?i)<body[^>]*>`)

// preheaderFiller pads the preview snippet so inboxes don't run the preheader into the
// first visible body text. &zwnj; keeps clients from collapsing the run of spaces.
var preheaderFiller = strings.Repeat("&nbsp;&zwnj;", 80)

// injectPreheader inserts hidden preview text immediately after the opening <body> tag
// (prepended if the rendered message has none). The style stack is the widely used
// belt-and-suspenders hiding set: display:none alone is ignored by some clients, and
// mso-hide:all is what Word/Outlook honors. The text is HTML-escaped here — it is plain
// text (possibly template-rendered) by contract, never markup.
func injectPreheader(body []byte, text string) []byte {
	div := `<div class="lm-preheader" style="display:none;font-size:1px;line-height:1px;max-height:0;max-width:0;opacity:0;overflow:hidden;mso-hide:all;">` +
		template.HTMLEscapeString(text) + preheaderFiller + `</div>`

	loc := regBodyTag.FindIndex(body)
	if loc == nil {
		return append([]byte(div), body...)
	}

	out := make([]byte, 0, len(body)+len(div))
	out = append(out, body[:loc[1]]...)
	out = append(out, div...)
	out = append(out, body[loc[1]:]...)
	return out
}
