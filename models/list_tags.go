package models

// Fork (list-scoped From / brand tags). The PURE core of brand validation. Since
// BRAND-PICKER-SPEC a brand's sending identity lives in the brands table and a list's reserved
// tags are its server-written projection (BrandProjection); BrandProblem is the one rule body,
// called by the brands API (cmd/brands.go, with the configured from_addresses lookup and
// the importer's sanitizer), the v6.2.20 backfill and ListTagsProblem (CAMPAIGN-52-HARDENING
// D7). models imports nothing that imports it, which is what lets every caller share this.
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
// Since BRAND-PICKER-SPEC the list API no longer accepts reserved tags (they are the
// server-written projection of the list's brands row), so this holds only the STRUCTURAL
// rules a tag set can break -- `site:` placement, at most one brand:/from:, both or neither
// -- and delegates every rule about the values to BrandProblem: one implementation
// (CAMPAIGN-52-HARDENING D7).
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

	// No mapping tags at all: an unmapped list, valid by design (the render catalog list).
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

	return BrandProblem(brands[0], froms[0], SiteTagOf(tags), allowedFrom, sanitize)
}

// ReservedBrandSlug is the one slug a brands row may not take: /api/brands/health is a static
// route that resolves before /api/brands/:slug, so a brand named health could never be updated.
const ReservedBrandSlug = "health"

// maxTagLen is lists.tags' element width (VARCHAR(100)[]): a projected tag longer than this
// would make every list write of the brand a 500.
const maxTagLen = 100

// BrandProblem validates one brand's sending identity -- a brands row (BRAND-PICKER-SPEC D1)
// or the brand:/from:/site: values of a tagged list -- and returns the i18n key of the first
// problem with its {param} args, or "" when acceptable. site "" means none.
//
//   - allowedFrom, when non-nil, must report whether a BARE address is one of the
//     configured SMTP from_addresses. nil skips the check (upstream's default has no
//     from_addresses; the v6.2.20 migration receives no SMTP config).
//   - sanitize validates a bare (display-name-less) From the way the importer validates an
//     e-mail; nil skips the shape check for bare addresses.
//
// Rules, in order: the slug is SES-safe and not the reserved `health`; the From is non-empty
// and ASCII (nothing RFC 2047-encodes the header), a whole `Display Name <address>` match or
// a sanitizable bare address, fits a tag, and its address is configured; the site is an
// absolute http(s) URL that fits a tag.
func BrandProblem(slug, from, site string, allowedFrom func(bare string) bool, sanitize func(string) (string, error)) (string, []string) {
	if !ReBrandSlug.MatchString(slug) {
		return "lists.brandTagInvalidSlug", []string{"brand", slug}
	}
	if strings.EqualFold(slug, ReservedBrandSlug) {
		return "lists.brandSlugReserved", []string{"brand", slug}
	}

	if strings.TrimSpace(from) == "" {
		return "lists.brandFromTagInvalid", []string{"from", from}
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

	if len(FromTagPrefix+from) > maxTagLen {
		return "lists.brandFromTagTooLong", []string{"from", from}
	}

	// The address must be a configured sending identity, or the From points at a domain
	// nobody has verified in SES and every campaign on the list dies at send time with a
	// 554 the app log never records.
	if allowedFrom != nil && !allowedFrom(BareAddress(from)) {
		return "lists.brandFromTagUnknownAddress", []string{"from", from}
	}

	if site != "" {
		if !linkresolve.IsAbsoluteHTTP(site) {
			return "lists.siteTagInvalid", nil
		}
		if len(SiteTagPrefix+site) > maxTagLen {
			return "lists.siteTagTooLong", []string{"site", site}
		}
	}

	return "", nil
}

// IsReservedListTag reports whether a tag, judged after TrimListTag, carries one of the
// reserved prefixes (brand:, from:, site:). The list API refuses such a tag: the projection of
// the list's brand is the only writer (BRAND-PICKER-SPEC D2).
func IsReservedListTag(t string) bool {
	t = TrimListTag(t)
	return strings.HasPrefix(t, BrandTagPrefix) || strings.HasPrefix(t, FromTagPrefix) || strings.HasPrefix(t, SiteTagPrefix)
}

// BrandProjection is the reserved tags a list of the brand carries (BRAND-PICKER-SPEC D2):
// brand:<slug>, from:<from_email>, and site:<site> when the row has one -- byte-identical to
// the tags the readers (cmd/campaigns_brand.go, list_brand_tag(), the editor, the integrations
// scripts) parse. The one implementation, used by the list API, PUT /api/brands' re-projection
// and the import preset.
func BrandProjection(b Brand) []string {
	out := []string{BrandTagPrefix + b.Slug, FromTagPrefix + b.FromEmail}
	if b.Site.Valid && b.Site.String != "" {
		out = append(out, SiteTagPrefix+b.Site.String)
	}
	return out
}

// LockedListNames are the lists no API call may update or delete, and of which a second copy may
// not be created (BRAND-PICKER-SPEC D4). The render catalog's list is created once by the
// integrations scripts/sync-catalog-campaigns.ts and never edited; that repo's
// lib/campaign-review/catalog-sync.ts CATALOG_LIST_NAME is pinned to this value by its test.
var LockedListNames = []string{"Render catalog (never send)"}

// IsLockedListName reports whether a list of this name is locked.
func IsLockedListName(name string) bool {
	for _, n := range LockedListNames {
		if name == n {
			return true
		}
	}
	return false
}
