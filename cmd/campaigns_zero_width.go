package main

import (
	"strings"

	"github.com/knadh/listmonk/models"
)

// Fork (CAMPAIGN-52-HARDENING I9) -- invisible zero-width characters in the SUBJECT and
// attribs.preheader fields. Campaign 52's subject arrived with U+200B on both sides of an
// emoji: ZMA's subject editor inserts them around a picked emoji, and an inbox line pasted
// from there carries them into listmonk. They count toward subject length and can render as
// a visible box in older clients. Save and Start WARN (the send-quality nudge pattern —
// Start proceeds): refusing would block a deliberate emoji-spacing use, and would not catch
// the real defect anyway (a pasted snippet, which no character check sees).
//
// FIELDS ONLY, NEVER THE BODY: the rendered body carries eighty `&zwnj;` (U+200C) by design
// (internal/manager preheaderFiller), so a body check would warn on every campaign.
const zeroWidthRunes = "\u200b\u200c\u200d\ufeff"

// hasZeroWidth reports whether s carries U+200B, U+200C, U+200D or U+FEFF.
func hasZeroWidth(s string) bool {
	return strings.ContainsAny(s, zeroWidthRunes)
}

// zeroWidthFields names the campaign fields (subject, preheader) that carry a zero-width
// character. Pure; the raw attribs value is read (not the trimmed accessor) so a
// zero-width character at either end is seen too.
func zeroWidthFields(subject string, attribs models.JSON) []string {
	var out []string
	if hasZeroWidth(subject) {
		out = append(out, "subject")
	}
	if p, _ := attribs["preheader"].(string); hasZeroWidth(p) {
		out = append(out, "preheader")
	}
	return out
}

// zeroWidthWarnings translates zeroWidthFields for a campaign into send-quality warnings.
func (a *App) zeroWidthWarnings(c *models.Campaign) []string {
	var out []string
	for _, f := range zeroWidthFields(c.Subject, c.Attribs) {
		out = append(out, a.i18n.Ts("campaigns.warnZeroWidth", "field", f))
	}
	return out
}
