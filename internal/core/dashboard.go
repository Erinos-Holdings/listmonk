package core

import (
	"encoding/json"
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

// GetDashboardBrandMatrix (fork, brand matrix, integrations BRAND-MATRIX-SPEC D3/D7) runs the one
// matrix statement: listIDs is the scope (ignored when unscoped, which reads every list). Live.
// DASHBOARDS-POLISH3-SPEC D5/I6 -- the skipped-language statement runs over the SAME scope and its
// counts are merged onto every row as `skipped` (MergeSkippedLangs).
func (c *Core) GetDashboardBrandMatrix(listIDs []int, unscoped bool) (types.JSONText, error) {
	if listIDs == nil {
		listIDs = []int{}
	}

	var out types.JSONText
	if err := c.q.GetDashboardBrandMatrix.Get(&out, pq.Array(listIDs), unscoped); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard brand matrix", "error", pqErrMsg(err)))
	}

	var skipped types.JSONText
	if err := c.q.GetDashboardSkippedLangs.Get(&skipped, pq.Array(listIDs), unscoped); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard skipped languages", "error", pqErrMsg(err)))
	}

	merged, err := MergeSkippedLangs(out, skipped)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard brand matrix", "error", err.Error()))
	}
	return merged, nil
}

// SkippedLangs (fork, integrations DASHBOARDS-POLISH3-SPEC D5) are the languages the Brand Usage
// pane's Skipped leaves show, in order -- English excluded by design (international transparency).
var SkippedLangs = []string{"fr", "es", "de", "it"}

// SkippedLang is one language's skipped entry on a matrix row.
type SkippedLang struct {
	Count int   `json:"count"`
	Lists []int `json:"lists"`
}

// MergeSkippedLangs (DASHBOARDS-POLISH3-SPEC D5/I6) puts `skipped` on EVERY matrix row: each of
// SkippedLangs as {count, lists}, {0, []} when the skipped statement returned nothing for that
// (brand, lang). The row's other keys pass through verbatim (json.RawMessage); the document's other
// top-level keys (scoped) too.
func MergeSkippedLangs(matrix, skipped types.JSONText) (types.JSONText, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(matrix, &doc); err != nil {
		return nil, err
	}
	var rows []map[string]json.RawMessage
	if raw, ok := doc["rows"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, err
		}
	}

	var found []struct {
		Brand string `json:"brand"`
		Lang  string `json:"lang"`
		Count int    `json:"count"`
		Lists []int  `json:"lists"`
	}
	if len(skipped) > 0 {
		if err := json.Unmarshal(skipped, &found); err != nil {
			return nil, err
		}
	}
	byBrand := map[string]map[string]SkippedLang{}
	for _, f := range found {
		if byBrand[f.Brand] == nil {
			byBrand[f.Brand] = map[string]SkippedLang{}
		}
		lists := f.Lists
		if lists == nil {
			lists = []int{}
		}
		byBrand[f.Brand][f.Lang] = SkippedLang{Count: f.Count, Lists: lists}
	}

	for _, row := range rows {
		var brand string
		if raw, ok := row["brand"]; ok {
			_ = json.Unmarshal(raw, &brand)
		}
		out := make(map[string]SkippedLang, len(SkippedLangs))
		for _, l := range SkippedLangs {
			s, ok := byBrand[brand][l]
			if !ok {
				s = SkippedLang{Count: 0, Lists: []int{}}
			}
			out[l] = s
		}
		b, err := json.Marshal(out)
		if err != nil {
			return nil, err
		}
		row["skipped"] = b
	}

	if rows == nil {
		rows = []map[string]json.RawMessage{}
	}
	rb, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	doc["rows"] = rb
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return types.JSONText(b), nil
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
