package core

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx/types"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// Fork (campaign conversions, integrations CAMPAIGN-CONVERSIONS-SPEC D6/D7). The Tracked and Matched
// rows are computed outside the fork (the integrations CampaignConversions Lambda) and stored
// verbatim; these functions move them in and out of campaign_conversions, export the raw clicks the
// Lambda matches on, and sum. No attribution and no rule lives here.

// ConversionClick is one row of the keyset click export: a link_clicks row with a subscriber and a
// campaign, the subscriber's CURRENT email.
type ConversionClick struct {
	ID         int64     `db:"id" json:"id"`
	CampaignID int       `db:"campaign_id" json:"campaign_id"`
	Email      string    `db:"email" json:"email"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

// ExportConversionClicks returns up to perPage clicks with id > afterID and created_at >= since,
// ascending by id (keyset pagination).
func (c *Core) ExportConversionClicks(afterID int64, since time.Time, perPage int) ([]ConversionClick, error) {
	out := []ConversionClick{}
	if err := c.q.ExportCampaignClicksKeyset.Select(&out, afterID, since, perPage); err != nil {
		c.log.Printf("error exporting conversion clicks: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.analytics}", "error", pqErrMsg(err)))
	}
	return out, nil
}

// MissingCampaignIDs returns the ids in ids that are no campaign.
func (c *Core) MissingCampaignIDs(ids []int) ([]int, error) {
	var found []int
	if err := c.q.GetExistingCampaignIDs.Select(&found, pq.Array(ids)); err != nil {
		c.log.Printf("error checking campaign ids: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaigns}", "error", pqErrMsg(err)))
	}
	have := make(map[int]bool, len(found))
	for _, id := range found {
		have[id] = true
	}
	missing := []int{}
	for _, id := range ids {
		if !have[id] {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

// ReplaceCampaignConversions replaces every row of one measure with rows (a validated JSON array) in
// ONE transaction -- the other measure is untouched, and any failure writes nothing. It returns the
// number of rows inserted.
func (c *Core) ReplaceCampaignConversions(measure string, rows json.RawMessage) (int64, error) {
	tx, err := c.db.Beginx()
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "conversions", "error", pqErrMsg(err)))
	}
	defer tx.Rollback()

	if _, err := tx.Stmtx(c.q.DeleteCampaignConversions).Exec(measure); err != nil {
		c.log.Printf("error deleting conversions: %v", err)
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "conversions", "error", pqErrMsg(err)))
	}
	res, err := tx.Stmtx(c.q.InsertCampaignConversions).Exec(measure, string(rows))
	if err != nil {
		c.log.Printf("error inserting conversions: %v", err)
		return 0, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "conversions", "error", pqErrMsg(err)))
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "conversions", "error", pqErrMsg(err)))
	}
	return n, nil
}

// GetCampaignConversionOrders returns the stored orders behind the requested campaigns' Tracked
// and Matched totals with purchased_at in [from, to], one row per (order, campaign), newest first.
// The dates are validated by the caller.
func (c *Core) GetCampaignConversionOrders(campIDs []int, fromDate, toDate string) (types.JSONText, error) {
	var out types.JSONText
	if err := c.q.GetCampaignConversionOrders.Get(&out, pq.Array(campIDs), fromDate, toDate); err != nil {
		c.log.Printf("error fetching campaign conversion orders: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.analytics}", "error", pqErrMsg(err)))
	}
	return out, nil
}

// GetCampaignAnalyticsConversions returns, per requested campaign, the Tracked and Matched orders and
// sales with purchased_at in [from, to] (NULL for a measure the latest conversions doc does not
// cover), with the doc's asOf, floors and trackedStale. The dates are validated by the caller.
func (c *Core) GetCampaignAnalyticsConversions(campIDs []int, fromDate, toDate string) (types.JSONText, error) {
	var out types.JSONText
	if err := c.q.GetCampaignConversions.Get(&out, pq.Array(campIDs), fromDate, toDate); err != nil {
		c.log.Printf("error fetching campaign conversions: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.analytics}", "error", pqErrMsg(err)))
	}
	return out, nil
}
