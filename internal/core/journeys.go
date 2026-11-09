package core

import (
	"net/http"
	"time"

	"github.com/jmoiron/sqlx/types"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// Fork (smart sending look-ahead, integrations SMART-SENDING-LOOKAHEAD-SPEC D1/D6). The forecast
// feed (journey_due) is computed outside the fork -- the integrations NoPurchaseSweep and
// ListmonkSunset Lambdas run their own planners with the next run's clock and PUT the joins that run
// will make. These functions store and count it; no due-date rule lives here.

// ReplaceJourneyDue replaces the feed rows of (source, expectedAt) with the given (list, subscriber)
// pairs, and deletes every row past the retention, in ONE transaction -- any failure writes nothing,
// and a retried call converges. listIDs and subIDs are parallel and validated by the caller. It
// returns the number of rows inserted.
func (c *Core) ReplaceJourneyDue(source string, expectedAt time.Time, listIDs, subIDs []int64) (int64, error) {
	tx, err := c.db.Beginx()
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "journey_due", "error", pqErrMsg(err)))
	}
	defer tx.Rollback()

	if _, err := tx.Stmtx(c.q.DeleteJourneyDue).Exec(source, expectedAt); err != nil {
		c.log.Printf("error deleting journey_due: %v", err)
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "journey_due", "error", pqErrMsg(err)))
	}
	res, err := tx.Stmtx(c.q.InsertJourneyDue).Exec(source, expectedAt, pq.Array(listIDs), pq.Array(subIDs))
	if err != nil {
		c.log.Printf("error inserting journey_due: %v", err)
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "journey_due", "error", pqErrMsg(err)))
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "journey_due", "error", pqErrMsg(err)))
	}
	return n, nil
}

// GetJourneyDueFeeds returns {lookahead, feeds: [{source, expected_at, rows, created_at}]} -- the
// switch as the look-ahead function reads it, and the retained feeds' counts. No subscriber ids.
func (c *Core) GetJourneyDueFeeds() (types.JSONText, error) {
	var out types.JSONText
	if err := c.q.GetJourneyDueFeeds.Get(&out); err != nil {
		c.log.Printf("error fetching journey_due feeds: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "journey_due", "error", pqErrMsg(err)))
	}
	return out, nil
}
