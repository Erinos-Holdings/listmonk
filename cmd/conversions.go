package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// Fork (campaign conversions, integrations CAMPAIGN-CONVERSIONS-SPEC D6/D7). The fork stores the
// Tracked and Matched rows the integrations CampaignConversions Lambda computes, exports the raw
// clicks it matches on, and sums the rows for display. It computes no attribution and holds no rule.
//
//   PUT /api/campaigns/conversions             brands:manage (the listmonk_health writer)
//   GET /api/campaigns/analytics/clicks/export subscribers:get_all -- SUBSCRIBER DATA, never gated on
//       campaigns:get_analytics: the external listmonk_bi token holds analytics reads and is
//       deliberately denied subscriber data. Its own static route, NOT the maintenance export,
//       whose settings:maintain also authorizes the analytics GC DELETE.
//   GET /api/campaigns/analytics/conversions    the typ == "conversions" branch of
//       GetCampaignViewAnalytics -- the same permission and per-id campaign check as every other
//       Campaign Analytics endpoint (cmd/campaigns.go).

const (
	// conversionsMaxRows caps one PUT body (D7).
	conversionsMaxRows = 500000
	// conversionsMaxSales is NUMERIC(12, 2)'s ceiling.
	conversionsMaxSales = 9999999999.99
	// conversionClicksMaxPerPage caps one click-export page (D7).
	conversionClicksMaxPerPage = 10000
)

var conversionMeasures = map[string]bool{"tracked": true, "matched": true}

type conversionRowIn struct {
	OrderID     *string  `json:"order_id"`
	CampaignID  *int     `json:"campaign_id"`
	Brand       *string  `json:"brand"`
	PurchasedAt *string  `json:"purchased_at"`
	SalesUSD    *float64 `json:"sales_usd"`
}

type conversionRowOut struct {
	OrderID     string  `json:"order_id"`
	CampaignID  int     `json:"campaign_id"`
	Brand       string  `json:"brand"`
	PurchasedAt string  `json:"purchased_at"`
	SalesUSD    float64 `json:"sales_usd"`
}

// validateConversionRows checks a PUT body's rows (D7): each complete, sales_usd >= 0, an order id
// once. It returns the normalized rows and their distinct campaign ids, or an error naming the first
// bad row. An empty list is valid (the measure is replaced by nothing).
func validateConversionRows(rows []conversionRowIn) ([]conversionRowOut, []int, error) {
	if len(rows) > conversionsMaxRows {
		return nil, nil, fmt.Errorf("too many rows (%d > %d)", len(rows), conversionsMaxRows)
	}
	out := make([]conversionRowOut, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	camps := map[int]bool{}
	ids := []int{}
	for i, r := range rows {
		if r.OrderID == nil || strings.TrimSpace(*r.OrderID) == "" {
			return nil, nil, fmt.Errorf("row %d: order_id is required", i)
		}
		if r.CampaignID == nil || *r.CampaignID < 1 {
			return nil, nil, fmt.Errorf("row %d: campaign_id must be a positive integer", i)
		}
		if r.Brand == nil || strings.TrimSpace(*r.Brand) == "" {
			return nil, nil, fmt.Errorf("row %d: brand is required", i)
		}
		if r.PurchasedAt == nil {
			return nil, nil, fmt.Errorf("row %d: purchased_at is required", i)
		}
		at, err := time.Parse(time.RFC3339Nano, *r.PurchasedAt)
		if err != nil {
			return nil, nil, fmt.Errorf("row %d: purchased_at must be an RFC 3339 timestamp", i)
		}
		if r.SalesUSD == nil || math.IsNaN(*r.SalesUSD) || math.IsInf(*r.SalesUSD, 0) || *r.SalesUSD < 0 || *r.SalesUSD > conversionsMaxSales {
			return nil, nil, fmt.Errorf("row %d: sales_usd must be a number >= 0", i)
		}
		oid := *r.OrderID
		if seen[oid] {
			return nil, nil, fmt.Errorf("row %d: duplicate order_id %s", i, oid)
		}
		seen[oid] = true
		if !camps[*r.CampaignID] {
			camps[*r.CampaignID] = true
			ids = append(ids, *r.CampaignID)
		}
		out = append(out, conversionRowOut{
			OrderID: oid, CampaignID: *r.CampaignID, Brand: *r.Brand,
			PurchasedAt: at.UTC().Format(time.RFC3339Nano), SalesUSD: *r.SalesUSD,
		})
	}
	return out, ids, nil
}

// PutCampaignConversions replaces every row of one measure (D7). Body:
// {"measure": "tracked"|"matched", "rows": [{order_id, campaign_id, brand, purchased_at, sales_usd}]}.
// Any failure -- an unknown measure, an incomplete row, sales_usd < 0, an order id twice, a campaign
// id that does not exist -- is a 400 and nothing is written.
func (a *App) PutCampaignConversions(c echo.Context) error {
	var req struct {
		Measure string             `json:"measure"`
		Rows    *[]conversionRowIn `json:"rows"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "rows"))
	}
	if !conversionMeasures[req.Measure] {
		return echo.NewHTTPError(http.StatusBadRequest, "measure must be tracked or matched")
	}
	if req.Rows == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "rows is required")
	}
	rows, ids, err := validateConversionRows(*req.Rows)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(ids) > 0 {
		missing, err := a.core.MissingCampaignIDs(ids)
		if err != nil {
			return err
		}
		if len(missing) > 0 {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("campaign_id %d does not exist", missing[0]))
		}
	}

	batch, err := json.Marshal(rows)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "rows"))
	}
	n, err := a.core.ReplaceCampaignConversions(req.Measure, batch)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{map[string]int64{"rows": n}})
}

// ExportConversionClicks is the keyset click export (D7):
// ?after_id=<n>&since=<RFC 3339>&per_page=<1..10000>. Raw link_clicks rows with a subscriber and a
// campaign, {id, campaign_id, email, created_at}, ascending by id. No computation.
func (a *App) ExportConversionClicks(c echo.Context) error {
	var (
		q       = c.QueryParams()
		afterID int64
		perPage = conversionClicksMaxPerPage
	)
	if s := q.Get("after_id"); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n < 0 {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "after_id"))
		}
		afterID = n
	}
	since, err := time.Parse(time.RFC3339Nano, q.Get("since"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "since"))
	}
	if s := q.Get("per_page"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > conversionClicksMaxPerPage {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "per_page"))
		}
		perPage = n
	}

	out, err := a.core.ExportConversionClicks(afterID, since, perPage)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}
