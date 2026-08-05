package main

import (
	"net/http"
	"strings"

	"github.com/knadh/listmonk/internal/messenger/email"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// validateBrandTags refuses a list whose `brand:`/`from:` brand-mapping tags are invalid,
// BEFORE they are stored. This is the tag-edit-time half of the list-scoped From design; the
// campaign-save half lives in campaigns_brand.go and re-checks the same properties at use time.
//
// WHY HERE AND NOT ONLY AT CAMPAIGN SAVE: internal/core stores these tags verbatim
// (normalizeListTags), so this validation is what stands between a typo and a stored bad value.
// Without it, a bad tag sits dormant until it breaks a campaign nobody touched, weeks after
// someone edited a list — the exact deferred-failure this feature exists to remove. Refusing at
// list save puts the error in front of the person making the edit.
//
// Direct SQL writes bypass this by construction, as they bypass everything else.
func (a *App) validateBrandTags(l models.List) error {
	var brands, froms []string

	for _, t := range l.Tags {
		t = strings.TrimSpace(t)
		switch {
		case strings.HasPrefix(t, brandTagPrefix):
			brands = append(brands, strings.TrimSpace(strings.TrimPrefix(t, brandTagPrefix)))
		case strings.HasPrefix(t, fromTagPrefix):
			froms = append(froms, strings.TrimSpace(strings.TrimPrefix(t, fromTagPrefix)))
		}
	}

	// No mapping tags at all: an unmapped list, valid by design (the internal seed list and the
	// bounce simulator are deliberately unmapped).
	if len(brands) == 0 && len(froms) == 0 {
		return nil
	}

	// Duplicates are ambiguous: the campaign-side resolver would silently take one of them.
	if len(brands) > 1 || len(froms) > 1 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.brandTagsDuplicate"))
	}

	// One tag without the other is how a brand ends up attributed but wrongly addressed, or
	// vice versa. Campaign save refuses a half-tagged list too; refusing it here means the
	// misconfiguration can no longer be stored at all.
	if len(brands) == 0 || len(froms) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.brandTagsHalfTagged"))
	}

	brand, from := brands[0], froms[0]

	// SES message-tag values accept only alphanumerics, `-` and `_`. Same rule and regex as
	// campaign save; enforced here so the bad value is refused when it is typed.
	if !reBrandSlug.MatchString(brand) {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("lists.brandTagInvalidSlug", "brand", brand))
	}

	// The From header is emitted verbatim and nothing RFC 2047-encodes it, so a non-ASCII
	// display name ships a malformed header. `Liyora`, not `Liyorá`.
	for _, r := range from {
		if r > 127 {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("lists.brandFromTagNotASCII", "from", from))
		}
	}

	// Same two accepted shapes as a campaign's From: `Display Name <address>` or a bare
	// address. Stricter than validateCampaignFields in one way, deliberately: reFromAddress is
	// unanchored, so a bare MatchString would pass `Liyora <a@b> JUNK` and the junk would ship
	// verbatim in the real From header (the campaign side exact-matches against this same tag).
	// Requiring the match to consume the whole value refuses that at typing time; anything that
	// passes here still passes the campaign-side checks.
	if m := reFromAddress.FindStringIndex(from); m != nil {
		if m[0] != 0 || m[1] != len(from) {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("lists.brandFromTagInvalid", "from", from))
		}
	} else if _, err := a.importer.SanitizeEmail(from); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("lists.brandFromTagInvalid", "from", from))
	}

	// The address must be a configured sending identity, or the tag points at a domain nobody
	// has verified in SES and every campaign on the list dies at send time with a 554 that the
	// app log never records. Skipped when no SMTP block declares from_addresses, which is
	// upstream's default — same opt-in semantics as the campaign-save check.
	if allowed := configuredFromAddresses(); len(allowed) > 0 {
		if _, ok := allowed[email.NormalizeAddr(bareAddress(from))]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("lists.brandFromTagUnknownAddress", "from", from))
		}
	}

	return nil
}
