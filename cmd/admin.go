package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/captcha"
	"github.com/knadh/listmonk/internal/subimporter"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	null "gopkg.in/volatiletech/null.v6"
)

type serverConfig struct {
	RootURL            string `json:"root_url"`
	FromEmail          string `json:"from_email"`
	PublicSubscription struct {
		Enabled          bool        `json:"enabled"`
		CaptchaEnabled   bool        `json:"captcha_enabled"`
		CaptchaProvider  null.String `json:"captcha_provider"`
		CaptchaKey       null.String `json:"captcha_key"`
		AltchaComplexity int         `json:"altcha_complexity"`
		RedirectURLs     []string    `json:"redirect_urls"`
	} `json:"public_subscription"`
	Privacy struct {
		DisableTracking    bool `json:"disable_tracking"`
		IndividualTracking bool `json:"individual_tracking"`
	} `json:"privacy"`
	MediaProvider string          `json:"media_provider"`
	Messengers    []string        `json:"messengers"`
	Langs         []i18nLang      `json:"langs"`
	Lang          string          `json:"lang"`
	Permissions   json.RawMessage `json:"permissions"`
	Update        *AppUpdate      `json:"update"`
	NeedsRestart  bool            `json:"needs_restart"`
	HasLegacyUser bool            `json:"has_legacy_user"`
	// Fork (evergreen) -- app.evergreen_enable, gates the campaign form checkbox.
	EvergreenEnabled bool `json:"evergreen_enabled"`
	// Fork (multi-language campaigns) -- app.lang_enable, gates the campaign form Language select.
	LangEnabled bool   `json:"lang_enabled"`
	Version     string `json:"version"`
	// Fork (click tracking) -- app.link_fallback_url / app.utm_enable, for the campaign UI's
	// personalized-link hints.
	LinkFallbackURL string `json:"link_fallback_url"`
	UTMEnabled      bool   `json:"utm_enabled"`
	// Fork (import presets) -- the {key, name} of every loaded preset; empty hides the buttons.
	ImportPresets []subimporter.PresetInfo `json:"import_presets"`
	// Fork (campaign review, CAMPAIGN-INSPECT-SPEC D12) -- app.review_url is set: the SPA shows
	// Inspect and the gated Start; false keeps the pre-release Start button.
	ReviewEnabled bool `json:"review_enabled"`
}

// GetServerConfig returns general server config.
func (a *App) GetServerConfig(c echo.Context) error {
	out := serverConfig{
		RootURL:          a.urlCfg.RootURL,
		FromEmail:        a.cfg.FromEmail,
		Lang:             a.cfg.Lang,
		Permissions:      a.cfg.PermissionsRaw,
		HasLegacyUser:    a.cfg.HasLegacyUser,
		EvergreenEnabled: a.cfg.EvergreenEnabled,
		LangEnabled:      a.cfg.LangEnabled,
		LinkFallbackURL:  a.cfg.LinkFallbackURL,
		UTMEnabled:       a.cfg.UTMEnable,
		ImportPresets:    make([]subimporter.PresetInfo, 0, len(a.importPresets)),
		ReviewEnabled:    reviewEnabled(),
		Privacy: struct {
			DisableTracking    bool `json:"disable_tracking"`
			IndividualTracking bool `json:"individual_tracking"`
		}{
			DisableTracking:    a.cfg.Privacy.DisableTracking,
			IndividualTracking: a.cfg.Privacy.IndividualTracking,
		},
	}
	out.PublicSubscription.Enabled = a.cfg.EnablePublicSubPage
	for i := range a.importPresets {
		out.ImportPresets = append(out.ImportPresets, a.importPresets[i].Info())
	}
	for _, d := range a.cfg.Security.TrustedURLs {
		if d == "*" {
			continue
		}
		out.PublicSubscription.RedirectURLs = append(out.PublicSubscription.RedirectURLs, d)
	}

	// CAPTCHA.
	if a.cfg.Security.Captcha.Altcha.Enabled {
		out.PublicSubscription.CaptchaEnabled = true
		out.PublicSubscription.CaptchaProvider = null.StringFrom(captcha.ProviderAltcha)
		out.PublicSubscription.AltchaComplexity = a.cfg.Security.Captcha.Altcha.Complexity
	} else if a.cfg.Security.Captcha.HCaptcha.Enabled {
		out.PublicSubscription.CaptchaEnabled = true
		out.PublicSubscription.CaptchaProvider = null.StringFrom(captcha.ProviderHCaptcha)
		out.PublicSubscription.CaptchaKey = null.StringFrom(a.cfg.Security.Captcha.HCaptcha.Key)
	}

	out.MediaProvider = a.cfg.MediaUpload.Provider

	// Language list.
	langList, err := getI18nLangList(a.fs)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			fmt.Sprintf("Error loading language list: %v", err))
	}
	out.Langs = langList

	out.Messengers = make([]string, 0, len(a.messengers))
	for _, m := range a.messengers {
		out.Messengers = append(out.Messengers, m.Name())
	}

	a.Lock()
	out.NeedsRestart = a.needsRestart
	out.Update = a.update
	a.Unlock()
	out.Version = versionString

	return c.JSON(http.StatusOK, okResp{out})
}

// GetDashboardCharts returns chart data points to render ont he dashboard.
func (a *App) GetDashboardCharts(c echo.Context) error {
	// Fork (brand analytics, BRAND-ANALYTICS-SPEC D6/D7) -- a list-scoped user reads live, scoped
	// charts; everyone with blanket list access keeps the materialized view unchanged.
	user := auth.GetUser(c)
	if hasAll, listIDs := user.GetPermittedLists(auth.PermTypeGet | auth.PermTypeManage); !hasAll {
		out, err := a.core.GetDashboardChartsScoped(listIDs, user.HasPerm(auth.PermCampaignsGetAll))
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, okResp{out})
	}

	// Get the chart data from the DB.
	out, err := a.core.GetDashboardCharts()
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// GetDashboardCounts returns stats counts to show on the dashboard.
func (a *App) GetDashboardCounts(c echo.Context) error {
	// Fork (brand analytics, BRAND-ANALYTICS-SPEC D6/D7) -- as GetDashboardCharts: list-scoped
	// users get live counts over their lists (scoped: true), never the global numbers.
	user := auth.GetUser(c)
	if hasAll, listIDs := user.GetPermittedLists(auth.PermTypeGet | auth.PermTypeManage); !hasAll {
		out, err := a.core.GetDashboardCountsScoped(listIDs, user.HasPerm(auth.PermCampaignsGetAll))
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, okResp{out})
	}

	// Get the chart data from the DB.
	out, err := a.core.GetDashboardCounts()
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// GetDashboardClients returns the Dashboard's per-email-client views and clicks (fork, client
// stats, integrations CLIENT-STATS-SPEC D6/D7). The scoped variant is selected exactly as in
// GetDashboardCharts: a list-scoped user gets the rollup over their permitted lists, and ?brand=
// is IGNORED for them (D7, review F6) -- no picker, no brands. Everyone else may pass ?brand=<tag>,
// resolved here to that brand's list ids (a tag no list carries resolves to no lists, so no rows);
// no ?brand= is every campaign.
func (a *App) GetDashboardClients(c echo.Context) error {
	user := auth.GetUser(c)
	if hasAll, listIDs := user.GetPermittedLists(auth.PermTypeGet | auth.PermTypeManage); !hasAll {
		rows, err := a.core.GetDashboardClientsScoped(listIDs, user.HasPerm(auth.PermCampaignsGetAll))
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, okResp{models.DashboardClients{Scoped: true, Brands: []string{}, Clients: rows}})
	}

	brands, err := a.core.GetDashboardBrands()
	if err != nil {
		return err
	}
	out := models.DashboardClients{Brands: make([]string, 0, len(brands))}
	for _, b := range brands {
		out.Brands = append(out.Brands, b.Brand)
	}

	var listIDs []int
	if brand := strings.TrimSpace(c.QueryParam("brand")); brand != "" {
		out.Brand = brand
		listIDs = []int{}
		for _, b := range brands {
			if b.Brand == brand {
				for _, id := range b.ListIDs {
					listIDs = append(listIDs, int(id))
				}
			}
		}
	}

	if out.Clients, err = a.core.GetDashboardClients(listIDs); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// ReloadApp sends a reload signal to the app, causing a full restart.
func (a *App) ReloadApp(c echo.Context) error {
	go func() {
		<-time.After(time.Millisecond * 500)

		// Send the reload signal to trigger the wait loop in main.
		a.chReload <- syscall.SIGHUP
	}()

	return c.JSON(http.StatusOK, okResp{true})
}
