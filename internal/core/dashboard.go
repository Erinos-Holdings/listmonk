package core

import (
	"net/http"

	"github.com/jmoiron/sqlx/types"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// GetDashboardCharts returns chart data points to render on the dashboard.
func (c *Core) GetDashboardCharts() (types.JSONText, error) {
	_ = c.refreshCache(matDashboardCharts, false)

	var out types.JSONText
	if err := c.q.GetDashboardCharts.Get(&out); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetDashboardCounts returns stats counts to show on the dashboard.
func (c *Core) GetDashboardCounts() (types.JSONText, error) {
	_ = c.refreshCache(matDashboardCounts, false)

	var out types.JSONText
	if err := c.q.GetDashboardCounts.Get(&out); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard stats", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetDashboardCountsScoped (fork, brand analytics, BRAND-ANALYTICS-SPEC D7) returns the Dashboard
// counts of a list-scoped user: listIDs is P; allCampaigns (campaigns:get_all) counts every
// campaign instead of those targeting P. A live read -- it never touches refreshCache, which would
// refresh the global view for nothing.
func (c *Core) GetDashboardCountsScoped(listIDs []int, allCampaigns bool) (types.JSONText, error) {
	if listIDs == nil {
		listIDs = []int{}
	}

	var out types.JSONText
	if err := c.q.GetDashboardCountsScoped.Get(&out, pq.Array(listIDs), allCampaigns); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard stats", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetDashboardChartsScoped (fork, brand analytics, BRAND-ANALYTICS-SPEC D7) is the chart
// counterpart of GetDashboardCountsScoped. Live, no refreshCache.
func (c *Core) GetDashboardChartsScoped(listIDs []int, allCampaigns bool) (types.JSONText, error) {
	if listIDs == nil {
		listIDs = []int{}
	}

	var out types.JSONText
	if err := c.q.GetDashboardChartsScoped.Get(&out, pq.Array(listIDs), allCampaigns); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetDashboardBrands (fork, client stats, CLIENT-STATS-SPEC D7) returns every list brand tag with
// the ids of its lists: the main Dashboard's brand picker and its server-side brand resolution.
func (c *Core) GetDashboardBrands() ([]models.DashboardBrand, error) {
	out := []models.DashboardBrand{}
	if err := c.q.GetDashboardBrands.Select(&out); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard clients", "error", pqErrMsg(err)))
	}
	return out, nil
}

// GetDashboardClients (fork, client stats, CLIENT-STATS-SPEC D6/D7) returns views and clicks per
// email-client token over the Dashboard window: listIDs nil = every campaign (all brands), else
// the campaigns on those lists (one brand's). Live, no refreshCache.
func (c *Core) GetDashboardClients(listIDs []int) ([]models.CampaignAnalyticsClient, error) {
	var lists any
	if listIDs != nil {
		lists = pq.Array(listIDs)
	}

	out := []models.CampaignAnalyticsClient{}
	if err := c.q.GetDashboardClients.Select(&out, lists); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard clients", "error", pqErrMsg(err)))
	}
	return out, nil
}

// GetDashboardClientsScoped (fork, client stats, CLIENT-STATS-SPEC D7) is GetDashboardClients for a
// list-scoped user, with the GetDashboardCountsScoped arguments. Live, no refreshCache.
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
