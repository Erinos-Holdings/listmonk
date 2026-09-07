package models

// Fork (list-scoped From / brand tags). The PURE core of list-tag validation, shared by the
// list form (cmd/lists_brand.go, which adds the configured from_addresses lookup, the
// importer's e-mail sanitizer and the echo error) and the import presets
// (internal/subimporter/preset.go, which validates a preset's list_tags at boot) — one
// implementation, so the two paths cannot diverge (CAMPAIGN-52-HARDENING D7). models
// imports nothing that imports it, which is what lets both callers share this.
//
// A list carries its brand mapping as two reserved tags, both present or both absent:
//
//	brand:liyora
//	from:Liyora <hello@liyorahair.com>
//
// plus an optional third, valid only beside that pair:
//
//	site:https://shop.liyorahair.com
//
// The functions return i18n KEYS (with their {param} args) rather than messages: the list
// form translates them, the preset loader logs them.

import (
	"regexp"
	"strings"

	"github.com/knadh/listmonk/internal/linkresolve"
)

const (
	// BrandTagPrefix marks the SES `brand` message-tag value (and the click-fallback site).
	BrandTagPrefix = "brand:"
	// FromTagPrefix marks the campaign From (`Display Name <address>` or a bare address).
	FromTagPrefix = "from:"
	// SiteTagPrefix marks the optional storefront URL override for link fallback and UTM.
	SiteTagPrefix = "site:"
)

var (
	// ReBrandSlug: SES message-tag values accept only alphanumerics, `-` and `_`.
	ReBrandSlug = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

	// ReFromAddress is listmonk's own `Display Name <address>` pattern (cmd/campaigns.go
	// aliases it). Unanchored by upstream design — callers that need the whole value to
	// match check the match bounds.
	ReFromAddress = regexp.MustCompile(`((.+?)\s)?<(.+?)@(.+?)>`)
)

// TrimListTag trims a tag the way the stored form has it: outer whitespace, and the
// whitespace between a reserved prefix and its value (`brand: liyora` -> `brand:liyora`).
// Whitespace INSIDE a value (a From display name) is kept. This is what
// internal/core's normalizeListTags does for the reserved prefixes, so a caller that
// writes tags without going through core (the preset's INSERT) stores the same shape.
func TrimListTag(t string) string {
	t = strings.TrimSpace(t)
	for _, prefix := range []string{BrandTagPrefix, FromTagPrefix, SiteTagPrefix} {
		if strings.HasPrefix(t, prefix) {
			return prefix + strings.TrimSpace(strings.TrimPrefix(t, prefix))
		}
	}
	return t
}

// BareAddress extracts `local@domain` from a `Display Name <local@domain>` From value and
// returns the input (trimmed) when it is already bare.
func BareAddress(from string) string {
	if m := ReFromAddress.FindStringSubmatch(from); len(m) == 5 {
		return m[3] + "@" + m[4]
	}
	return strings.TrimSpace(from)
}

// SiteTagOf returns a list's `site:` tag value (trimmed), or "" when it carries none.
func SiteTagOf(tags []string) string {
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if strings.HasPrefix(t, SiteTagPrefix) {
			return strings.TrimSpace(strings.TrimPrefix(t, SiteTagPrefix))
		}
	}
	return ""
}

// SiteTagProblem validates a list's `site:` tags on their own, returning the i18n key of
// the first problem or "" when acceptable: a `site:` tag is allowed only alongside the
// brand:/from: pair (both present — the half-tagged rule refuses a lone one separately),
// at most one per list, and its value must be an absolute http(s) URL.
func SiteTagProblem(tags []string) string {
	var (
		sites          []string
		hasBrand, hasF bool
	)
	for _, t := range tags {
		t = strings.TrimSpace(t)
		switch {
		case strings.HasPrefix(t, SiteTagPrefix):
			sites = append(sites, strings.TrimSpace(strings.TrimPrefix(t, SiteTagPrefix)))
		case strings.HasPrefix(t, BrandTagPrefix):
			hasBrand = true
		case strings.HasPrefix(t, FromTagPrefix):
			hasF = true
		}
	}
	if len(sites) == 0 {
		return ""
	}
	if len(sites) > 1 {
		return "lists.siteTagDuplicate"
	}
	if !hasBrand || !hasF {
		return "lists.siteTagNeedsBrand"
	}
	if !linkresolve.IsAbsoluteHTTP(sites[0]) {
		return "lists.siteTagInvalid"
	}
	return ""
}

// ListTagsProblem validates a list's reserved tags (brand:, from:, site:) and returns the
// i18n key of the first problem with its {param} args (key/value pairs, the shape
// i18n.Ts takes), or "" when the tags are acceptable. Free-form tags are ignored.
//
//   - allowedFrom, when non-nil, must report whether a BARE address is one of the
//     configured SMTP from_addresses (the caller normalises the way its allowlist was
//     built). nil skips the check — upstream's default has no from_addresses, and the
//     preset loader may run before the messenger exists.
//   - sanitize validates a bare (display-name-less) `from:` value the way the importer
//     validates an e-mail; nil skips the shape check for bare addresses.
//
// Rules, in order: `site:` problems; no mapping tags at all is valid (an unmapped list);
// at most one brand: and one from:; both or neither; the brand is a SES-safe slug; the
// From is ASCII (nothing RFC 2047-encodes the header); the From is either a whole
// `Display Name <address>` match or a sanitizable bare address; the address is configured.
func ListTagsProblem(tags []string, allowedFrom func(bare string) bool, sanitize func(string) (string, error)) (string, []string) {
	var brands, froms []string
	for _, t := range tags {
		t = strings.TrimSpace(t)
		switch {
		case strings.HasPrefix(t, BrandTagPrefix):
			brands = append(brands, strings.TrimSpace(strings.TrimPrefix(t, BrandTagPrefix)))
		case strings.HasPrefix(t, FromTagPrefix):
			froms = append(froms, strings.TrimSpace(strings.TrimPrefix(t, FromTagPrefix)))
		}
	}

	if key := SiteTagProblem(tags); key != "" {
		return key, nil
	}

	// No mapping tags at all: an unmapped list, valid by design (the internal seed list and
	// the bounce simulator are deliberately unmapped).
	if len(brands) == 0 && len(froms) == 0 {
		return "", nil
	}

	// Duplicates are ambiguous: the campaign-side resolver would silently take one of them.
	if len(brands) > 1 || len(froms) > 1 {
		return "lists.brandTagsDuplicate", nil
	}

	// One tag without the other is how a brand ends up attributed but wrongly addressed, or
	// vice versa.
	if len(brands) == 0 || len(froms) == 0 {
		return "lists.brandTagsHalfTagged", nil
	}

	brand, from := brands[0], froms[0]

	if !ReBrandSlug.MatchString(brand) {
		return "lists.brandTagInvalidSlug", []string{"brand", brand}
	}

	// The From header is emitted verbatim and nothing RFC 2047-encodes it, so a non-ASCII
	// display name ships a malformed header. `Liyora`, not `Liyorá`.
	for _, r := range from {
		if r > 127 {
			return "lists.brandFromTagNotASCII", []string{"from", from}
		}
	}

	// Same two accepted shapes as a campaign's From: `Display Name <address>` or a bare
	// address. ReFromAddress is unanchored, so the match must consume the whole value or
	// `Liyora <a@b> JUNK` would ship the junk verbatim in the real From header.
	if m := ReFromAddress.FindStringIndex(from); m != nil {
		if m[0] != 0 || m[1] != len(from) {
			return "lists.brandFromTagInvalid", []string{"from", from}
		}
	} else if sanitize != nil {
		if _, err := sanitize(from); err != nil {
			return "lists.brandFromTagInvalid", []string{"from", from}
		}
	}

	// The address must be a configured sending identity, or the tag points at a domain
	// nobody has verified in SES and every campaign on the list dies at send time with a
	// 554 the app log never records.
	if allowedFrom != nil && !allowedFrom(BareAddress(from)) {
		return "lists.brandFromTagUnknownAddress", []string{"from", from}
	}

	return "", nil
}
