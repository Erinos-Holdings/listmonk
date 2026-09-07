package main

import (
	"net/http"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// validateBrandTags refuses a list whose `brand:`/`from:`/`site:` tags are invalid, BEFORE
// they are stored. This is the tag-edit-time half of the list-scoped From design; the
// campaign-save half lives in campaigns_brand.go and re-checks the same properties at use time.
//
// WHY HERE AND NOT ONLY AT CAMPAIGN SAVE: internal/core stores these tags verbatim
// (normalizeListTags), so this validation is what stands between a typo and a stored bad value.
// Without it, a bad tag sits dormant until it breaks a campaign nobody touched, weeks after
// someone edited a list -- the exact deferred-failure this feature exists to remove. Refusing at
// list save puts the error in front of the person making the edit.
//
// The rules themselves are the pure core models.ListTagsProblem (CAMPAIGN-52-HARDENING D7),
// shared with the import presets so a preset-created list is validated by the same code
// that validates the form. This wrapper adds only what the App has: the configured
// from_addresses lookup, the importer's e-mail sanitizer (domain allow/blocklists) and the
// translated echo error.
//
// Direct SQL writes bypass this by construction, as they bypass everything else.
func (a *App) validateBrandTags(l models.List) error {
	key, args := models.ListTagsProblem(l.Tags, configuredFromLookup(), a.importer.SanitizeEmail)
	if key == "" {
		return nil
	}
	return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts(key, args...))
}

// siteTagProblem (fork, CLICK-TRACKING-SPEC §3.4 / I7a) is models.SiteTagProblem; kept as
// the package-local name its table test (lists_brand_test.go) pins.
func siteTagProblem(tags []string) string {
	return models.SiteTagProblem(tags)
}
