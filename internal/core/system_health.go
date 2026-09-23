package core

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Fork (system health, integrations SES-HEALTH-SPEC D6). The system-wide documents are computed
// outside the fork (the integrations BrandHealth Lambda) and stored verbatim; these functions
// move them in and out of system_health and compute nothing.

// UpsertSystemHealth writes a validated batch of documents (a JSON array) in one statement, so
// the batch is all-or-nothing. It returns the number of rows written.
func (c *Core) UpsertSystemHealth(rows json.RawMessage) (int64, error) {
	res, err := c.q.UpsertSystemHealth.Exec(string(rows))
	if err != nil {
		c.log.Printf("error upserting system health: %v", err)
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "system health", "error", pqErrMsg(err)))
	}

	n, _ := res.RowsAffected()
	return n, nil
}

// GetSystemHealthHistory returns one kind's documents over the last `days` days, newest first.
// An unknown kind is an empty list, not an error.
func (c *Core) GetSystemHealthHistory(kind string, days int) ([]json.RawMessage, error) {
	var rows []brandHealthDoc
	if err := c.q.GetSystemHealthHistory.Select(&rows, kind, days); err != nil {
		c.log.Printf("error fetching system health: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "system health", "error", pqErrMsg(err)))
	}

	return docsOf(rows), nil
}
