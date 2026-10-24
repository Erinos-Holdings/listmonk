package core

import (
	"net/http"

	"github.com/jmoiron/sqlx/types"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// GetDashboardCharts returns chart data points to render on the dashboard.
// Fork (dashboard sends, integrations DASHBOARD-SENDS-SPEC D2) -- the unscoped read is the one live
// charts query over every campaign and every brand's sends (unscoped = true). mat_dashboard_charts
// is no longer read, so it is no longer refreshed here (RefreshMatViews still refreshes it).
func (c *Core) GetDashboardCharts() (types.JSONText, error) {
	return c.dashboardCharts([]int{}, true, true)
}

// GetDashboardCounts returns stats counts to show on the dashboard.
// Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC D5) -- the unscoped read is the one
// live counts query over every list and every campaign (unscoped = true). mat_dashboard_counts is
// no longer read, so it is no longer refreshed here (RefreshMatViews still refreshes it).
func (c *Core) GetDashboardCounts() (types.JSONText, error) {
	return c.dashboardCounts([]int{}, true, true)
}

// GetDashboardCountsScoped (fork, brand analytics, BRAND-ANALYTICS-SPEC D7) returns the Dashboard
// counts of a list-scoped user: listIDs is P; allCampaigns (campaigns:get_all) counts every
// campaign instead of those targeting P. A live read -- it never touches refreshCache, which would
// refresh the global view for nothing.
func (c *Core) GetDashboardCountsScoped(listIDs []int, allCampaigns bool) (types.JSONText, error) {
	return c.dashboardCounts(listIDs, allCampaigns, false)
}

// dashboardCounts (fork, DASHBOARD-COUNTS-SPEC D5) runs the one counts statement for both paths.
func (c *Core) dashboardCounts(listIDs []int, allCampaigns, unscoped bool) (types.JSONText, error) {
	if listIDs == nil {
		listIDs = []int{}
	}

	var out types.JSONText
	if err := c.q.GetDashboardCountsScoped.Get(&out, pq.Array(listIDs), allCampaigns, unscoped); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard stats", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetDashboardChartsScoped (fork, brand analytics, BRAND-ANALYTICS-SPEC D7) is the chart
// counterpart of GetDashboardCountsScoped. Live, no refreshCache.
func (c *Core) GetDashboardChartsScoped(listIDs []int, allCampaigns bool) (types.JSONText, error) {
	return c.dashboardCharts(listIDs, allCampaigns, false)
}

// dashboardCharts (fork, DASHBOARD-SENDS-SPEC D2) runs the one charts statement for both paths.
func (c *Core) dashboardCharts(listIDs []int, allCampaigns, unscoped bool) (types.JSONText, error) {
	if listIDs == nil {
		listIDs = []int{}
	}

	var out types.JSONText
	if err := c.q.GetDashboardChartsScoped.Get(&out, pq.Array(listIDs), allCampaigns, unscoped); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetDashboardClients (fork, client stats, CLIENT-STATS-SPEC D6) returns views and clicks per
// email-client token over the Dashboard window, over every campaign: the unscoped panel. Any scope
// (a list-scoped user, the global brand's list_id) reads GetDashboardClientsScoped
// (GLOBAL-BRAND-SPEC D5). Live, no refreshCache.
func (c *Core) GetDashboardClients() ([]models.CampaignAnalyticsClient, error) {
	out := []models.CampaignAnalyticsClient{}
	if err := c.q.GetDashboardClients.Select(&out); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard clients", "error", pqErrMsg(err)))
	}
	return out, nil
}

// GetDashboardClientsScoped (fork, client stats, CLIENT-STATS-SPEC D7; GLOBAL-BRAND-SPEC D5) is
// GetDashboardClients over a scope, with the GetDashboardCountsScoped arguments. Live, no
// refreshCache.
func (c *Core) GetDashboardClientsScoped(listIDs []int, allCampaigns bool) ([]models.CampaignAnalyticsClient, error) {
	if listIDs == nil {
		listIDs = []int{}
	}

	out := []models.CampaignAnalyticsClient{}
	if err := c.q.GetDashboardClientsScoped.Select(&out, pq.Array(listIDs), allCampaigns); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard clients", "error", pqErrMsg(err)))
	}
	return out, nil
}

// GetDashboardCountries (fork, dashboard counts, integrations DASHBOARD-COUNTS-SPEC R4) is
// GetDashboardClients keyed by country: views and clicks per country code ("" = unknown) over the
// Dashboard window and every campaign, raw rows. Live, no refreshCache.
func (c *Core) GetDashboardCountries() ([]models.CampaignAnalyticsCountry, error) {
	out := []models.CampaignAnalyticsCountry{}
	if err := c.q.GetDashboardCountries.Select(&out); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard countries", "error", pqErrMsg(err)))
	}
	return out, nil
}

// GetDashboardCountriesScoped (fork, dashboard counts, DASHBOARD-COUNTS-SPEC R4) is
// GetDashboardCountries over a scope, with the GetDashboardClientsScoped arguments.
func (c *Core) GetDashboardCountriesScoped(listIDs []int, allCampaigns bool) ([]models.CampaignAnalyticsCountry, error) {
	if listIDs == nil {
		listIDs = []int{}
	}

	out := []models.CampaignAnalyticsCountry{}
	if err := c.q.GetDashboardCountriesScoped.Select(&out, pq.Array(listIDs), allCampaigns); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard countries", "error", pqErrMsg(err)))
	}
	return out, nil
}
