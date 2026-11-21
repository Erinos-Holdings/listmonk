package models

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Fork (promo series, integrations listmonk/PROMO-SERIES-SPEC D1). The three promo-series fields of
// a broadcast, stored in its attribs and served as top-level effective fields (core.FillAudience).
// This file is the ONE home of their rule; the page's promoSeries.mjs mirrors it for display only,
// and is the only normaliser (the server validates, it never rewrites attribs).
//
// Whether a campaign's promo fields skip anyone is NOT decided here: that is the SQL function
// promo_applicable_tags (schema.sql), which Go re-implements none of.
const (
	PromoCodeAttrib = "promo_code"
	PromoStepAttrib = "promo_step"
	PromoTagsAttrib = "promo_tags"

	// PromoTagsMax caps the Purchase tags list; PromoTagMaxLen caps one tag (runes);
	// PromoPrefixMin is the shortest prefix a `*` tag may carry.
	PromoTagsMax   = 20
	PromoTagMaxLen = 64
	PromoPrefixMin = 3
)

// PromoCodeProblem is the naming standard's name rule (NameProblem's three checks, no CATALOG
// exemption) applied to a Code that is already trimmed: "" when valid, else the i18n key.
func PromoCodeProblem(code string) string {
	if strings.TrimSpace(code) == "" {
		return "naming.nameEmpty"
	}
	for _, r := range code {
		if unicode.IsControl(r) {
			return "naming.nameControl"
		}
	}
	if utf8.RuneCountInString(code) > NameMaxLen {
		return "naming.nameTooLong"
	}
	return ""
}

// PromoTagProblem reports whether one Purchase tag (as given, already trimmed) breaks D1's rule:
// 1-64 characters, no control character, no comma, a `*` only as the LAST character and then a
// prefix of at least 3 characters. "" when valid.
func PromoTagProblem(tag string) string {
	n := utf8.RuneCountInString(tag)
	if n < 1 || n > PromoTagMaxLen {
		return "length"
	}
	for _, r := range tag {
		if unicode.IsControl(r) {
			return "control"
		}
	}
	if strings.Contains(tag, ",") {
		return "comma"
	}
	if i := strings.Index(tag, "*"); i >= 0 {
		if i != len(tag)-1 {
			return "star"
		}
		if utf8.RuneCountInString(tag[:i]) < PromoPrefixMin {
			return "prefix"
		}
	}
	return ""
}

// promoStepOK is the JSON number 1, 2 or 3 (json decoding gives float64).
func promoStepOK(v any) bool {
	f, ok := v.(float64)
	return ok && (f == 1 || f == 2 || f == 3)
}

// ValidatePromo checks the three attribs keys (D1, Stage 2 S7). It refuses any other shape, a Step
// or Purchase tags without a Code, and every non-normal form: an untrimmed Code or tag, a
// case-insensitive duplicate tag, an empty array, a key present with a null value. It returns ""
// when valid, else the campaigns.* i18n key of the refusal.
func ValidatePromo(attribs JSON) string {
	code, hasCode := attribs[PromoCodeAttrib]
	if hasCode {
		s, ok := code.(string)
		if !ok || s != strings.TrimSpace(s) || PromoCodeProblem(s) != "" {
			return "campaigns.invalidPromoCode"
		}
	}
	step, hasStep := attribs[PromoStepAttrib]
	if hasStep && !promoStepOK(step) {
		return "campaigns.invalidPromoStep"
	}
	tags, hasTags := attribs[PromoTagsAttrib]
	if hasTags {
		arr, ok := tags.([]any)
		if !ok || len(arr) == 0 || len(arr) > PromoTagsMax {
			return "campaigns.invalidPromoTags"
		}
		seen := map[string]bool{}
		for _, t := range arr {
			s, ok := t.(string)
			if !ok || s != strings.TrimSpace(s) || PromoTagProblem(s) != "" {
				return "campaigns.invalidPromoTags"
			}
			k := strings.ToLower(s)
			if seen[k] {
				return "campaigns.invalidPromoTags"
			}
			seen[k] = true
		}
	}
	if (hasStep || hasTags) && !hasCode {
		return "campaigns.promoNeedsCode"
	}
	return ""
}

// ParsePromoTagsParam parses the preview route's comma-separated `tags` query value (D5): each
// piece trimmed (parsing, not normalising -- the page sends what the user typed), empty pieces
// dropped, each piece held to PromoTagProblem, case-insensitive duplicates dropped (first kept),
// at most PromoTagsMax. ok is false on any malformed tag.
func ParsePromoTagsParam(v string) ([]string, bool) {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if PromoTagProblem(p) != "" {
			return nil, false
		}
		k := strings.ToLower(p)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	if len(out) > PromoTagsMax {
		return nil, false
	}
	return out, true
}

// PromoCode is the served Code: the stored string, "" when absent or not a string.
func (c *Campaign) PromoCode() string {
	s, _ := c.Attribs[PromoCodeAttrib].(string)
	return s
}

// PromoStep is the served Step: 1, 2 or 3, 0 when absent or any other value.
func (c *Campaign) PromoStep() int {
	if v := c.Attribs[PromoStepAttrib]; promoStepOK(v) {
		return int(v.(float64))
	}
	return 0
}

// PromoTags is the served Purchase tags: the stored strings, [] when absent.
func (c *Campaign) PromoTags() []string {
	out := []string{}
	arr, _ := c.Attribs[PromoTagsAttrib].([]any)
	for _, t := range arr {
		if s, ok := t.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
