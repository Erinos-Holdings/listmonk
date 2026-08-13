package main

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/knadh/listmonk/internal/brandtheme"
	"github.com/labstack/echo/v4"
)

// Brand theme proxy for the visual editor's brand color swatches. The why (CORS, soft-fail
// upstream, Found=false semantics) lives with the fetch half in internal/brandtheme.

const (
	// Themes change on the order of never; the TTL only bounds how long a CMS edit or a slug
	// fix takes to show up in the editor. Misses (Found=false) are cached on the same TTL so a
	// mistyped slug cannot hammer the catalog API once per editor open.
	brandThemeTTL = 15 * time.Minute

	// How long a stale entry keeps being served after a FAILED refresh before upstream is
	// tried again, so a catalog outage costs at most one slow (8s client timeout) request
	// per slug per interval instead of hanging every editor open.
	brandThemeRetryHold = time.Minute

	// Entries are never individually evicted (expiry refreshes in place), so cap the map to
	// bound memory against scripted junk slugs; legit traffic is the brand roster, nowhere
	// near this. At the cap the whole map is dropped — a few re-fetches, not a correctness
	// event.
	brandThemeMaxEntries = 512
)

var (
	brandThemeMu    sync.Mutex
	brandThemeCache = map[string]brandThemeEntry{}
)

type brandThemeEntry struct {
	resp brandtheme.Resp

	// freshUntil is when this entry stops being served without an upstream refresh: fetch
	// time + TTL normally, or a short retry hold stamped after a failed refresh.
	freshUntil time.Time
}

// GetBrandTheme handles GET /api/brands/:slug/theme.
func (a *App) GetBrandTheme(c echo.Context) error {
	slug := c.Param("slug")

	// reBrandSlug (campaigns_brand.go) is what list tags are validated against, so anything a
	// campaign can legitimately derive passes. It also confines the slug to [A-Za-z0-9_-],
	// which brandtheme.Fetch requires — the slug is interpolated into a URL path.
	if !reBrandSlug.MatchString(slug) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
	}

	// Catalog slugs are canonically lowercase and the catalog API is case-sensitive (verified
	// 2026-08-12: `Liyora` returns an empty page where `liyora` has a theme). A mixed-case
	// `brand:` tag is valid on the send path, so fold here rather than soft-failing to
	// found=false on a tag that is merely mis-cased. Same precedent as the ZMA registry lookup.
	slug = strings.ToLower(slug)

	brandThemeMu.Lock()
	cached, ok := brandThemeCache[slug]
	brandThemeMu.Unlock()

	if ok && time.Now().Before(cached.freshUntil) {
		return c.JSON(http.StatusOK, okResp{cached.resp})
	}

	resp, err := brandtheme.Fetch(slug)
	if err != nil {
		a.log.Printf("error fetching brand theme for %q: %v", slug, err)

		// Serve stale over failing: the swatch row is a convenience, and a catalog outage
		// should not surface as an editor error. Re-stamp with the retry hold so the next
		// upstream attempt for this slug waits out the hold instead of hanging every request.
		if ok {
			brandThemeMu.Lock()
			brandThemeCache[slug] = brandThemeEntry{resp: cached.resp, freshUntil: time.Now().Add(brandThemeRetryHold)}
			brandThemeMu.Unlock()

			return c.JSON(http.StatusOK, okResp{cached.resp})
		}

		return echo.NewHTTPError(http.StatusBadGateway,
			a.i18n.Ts("globals.messages.errorFetching", "name", "brand theme", "error", err.Error()))
	}

	brandThemeMu.Lock()
	if len(brandThemeCache) >= brandThemeMaxEntries {
		brandThemeCache = map[string]brandThemeEntry{}
	}
	brandThemeCache[slug] = brandThemeEntry{resp: resp, freshUntil: time.Now().Add(brandThemeTTL)}
	brandThemeMu.Unlock()

	return c.JSON(http.StatusOK, okResp{resp})
}
