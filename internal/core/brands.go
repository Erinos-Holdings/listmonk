package core

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// Fork (brand health, integrations BRAND-HEALTH-SPEC D2/D6). The documents are computed outside
// the fork (the integrations BrandHealth Lambda) and stored verbatim; these functions move them
// in and out of brand_health and compute nothing.

type brandHealthDoc struct {
	Doc json.RawMessage `db:"doc"`
}

// UpsertBrandHealth writes a validated batch of documents (a JSON array) in one statement, so
// the batch is all-or-nothing. It returns the number of rows written.
func (c *Core) UpsertBrandHealth(rows json.RawMessage) (int64, error) {
	res, err := c.q.UpsertBrandHealth.Exec(string(rows))
	if err != nil {
		c.log.Printf("error upserting brand health: %v", err)
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "brand health", "error", pqErrMsg(err)))
	}

	n, _ := res.RowsAffected()
	return n, nil
}

// GetBrandHealthLatest returns the latest document per brand, each with a lists[] of the lists
// the user may see (tagged with the brand, plus every untagged list on the default-sender row).
func (c *Core) GetBrandHealthLatest(getAll bool, permittedIDs []int) ([]json.RawMessage, error) {
	var rows []brandHealthDoc
	if err := c.q.GetBrandHealthLatest.Select(&rows, getAll, pq.Array(permittedIDs)); err != nil {
		c.log.Printf("error fetching brand health: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "brand health", "error", pqErrMsg(err)))
	}

	return docsOf(rows), nil
}

// GetBrandHealthHistory returns one brand's documents over the last `days` days, newest first.
func (c *Core) GetBrandHealthHistory(brand string, days int) ([]json.RawMessage, error) {
	var rows []brandHealthDoc
	if err := c.q.GetBrandHealthHistory.Select(&rows, brand, days); err != nil {
		c.log.Printf("error fetching brand health history: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "brand health", "error", pqErrMsg(err)))
	}

	return docsOf(rows), nil
}

func docsOf(rows []brandHealthDoc) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Doc)
	}
	return out
}
