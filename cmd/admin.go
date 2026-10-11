package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// dashboardScope (fork, global brand, integrations GLOBAL-BRAND-SPEC D5) is the Dashboard's read
// decision: scoped=false is the materialized views (blanket list access, no list_id); scoped=true
// is the live scoped queries over listIDs, with allCampaigns their campaigns get_all argument.
type dashboardScope struct {
	scoped       bool
	listIDs      []int
	allCampaigns bool
}

// dashboardListScope (fork, global brand, GLOBAL-BRAND-SPEC D5, I1, I7) is the ONE rule the three
// Dashboard reads share. No list_id is today's behaviour exactly -- blanket list access reads the
// materialized views; a list-scoped user reads the scoped queries over their permitted lists with
// allCampaigns = campaigns:get_all (BRAND-ANALYTICS-SPEC D7). A list_id (the global brand
// selector's effective list set, repeatable) reads the scoped queries over permitted ∩ list_id
// (FilterListsByPerm; the ids unchanged under blanket access), and allCampaigns is FALSE whatever
// the user's grants: a brand's campaigns are those with ANY campaign_lists row on the scope --
// get_all widens which campaigns a user may read, not which are the brand's. The selector itself
// never reaches the server (S1); this is permission ∩ request, as every other list filter.
func dashboardListScope(user auth.User, qp url.Values) (dashboardScope, error) {
	hasAll, permitted := user.GetPermittedLists(auth.PermTypeGet | auth.PermTypeManage)

	raw, ok := qp["list_id"]
	if !ok || len(raw) == 0 {
		if hasAll {
			return dashboardScope{}, nil
		}
		return dashboardScope{scoped: true, listIDs: permitted, allCampaigns: user.HasPerm(auth.PermCampaignsGetAll)}, nil
	}

	ids, err := parseStringIDs(raw)
	if err != nil {
		return dashboardScope{}, err
	}
	if hasAll {
		return dashboardScope{scoped: true, listIDs: ids, allCampaigns: false}, nil
	}
	out := user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, ids)
	if out == nil {
		out = []int{}
	}
	return dashboardScope{scoped: true, listIDs: out, allCampaigns: false}, nil
}

// dashboardScopeFor is dashboardListScope on a request, with a malformed list_id as a 400.
func (a *App) dashboardScopeFor(c echo.Context) (dashboardScope, error) {
	sc, err := dashboardListScope(auth.GetUser(c), c.QueryParams())
	if err != nil {
		return sc, echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	return sc, nil
}

// GetDashboardCharts returns chart data points to render ont he dashboard.
func (a *App) GetDashboardCharts(c echo.Context) error {
	// Fork (brand analytics, BRAND-ANALYTICS-SPEC D6/D7; global brand, GLOBAL-BRAND-SPEC D5) -- a
	// list-scoped user, or any list_id, reads live, scoped charts; blanket list access with no
	// list_id keeps the materialized view unchanged.
	sc, err := a.dashboardScopeFor(c)
	if err != nil {
		return err
	}
	if sc.scoped {
		out, err := a.core.GetDashboardChartsScoped(sc.listIDs, sc.allCampaigns)
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
	// Fork (brand analytics, BRAND-ANALYTICS-SPEC D6/D7; global brand, GLOBAL-BRAND-SPEC D5) -- as
	// GetDashboardCharts: list-scoped users and any list_id get live counts over the scope
	// (scoped: true), never the global numbers.
	sc, err := a.dashboardScopeFor(c)
	if err != nil {
		return err
	}
	if sc.scoped {
		out, err := a.core.GetDashboardCountsScoped(sc.listIDs, sc.allCampaigns)
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
// stats, integrations CLIENT-STATS-SPEC D6). The scoped variant is selected exactly as in
// GetDashboardCharts (dashboardListScope, GLOBAL-BRAND-SPEC D5): a list-scoped user, or any
// list_id, gets the rollup over the scope; blanket list access with no list_id is every campaign.
// The panel's own ?brand= picker is gone (GLOBAL-BRAND-SPEC D5, superseding CLIENT-STATS-SPEC D7):
// the global brand selector's list_id is the one filter.
func (a *App) GetDashboardClients(c echo.Context) error {
	sc, err := a.dashboardScopeFor(c)
	if err != nil {
		return err
	}
	if sc.scoped {
		rows, err := a.core.GetDashboardClientsScoped(sc.listIDs, sc.allCampaigns)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, okResp{models.DashboardClients{Scoped: true, Clients: rows}})
	}

	rows, err := a.core.GetDashboardClients()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{models.DashboardClients{Clients: rows}})
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
