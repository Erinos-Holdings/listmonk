package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/labstack/echo/v4"
)

// Fork (system health, integrations SES-HEALTH-SPEC D1/D6). Generic storage for system-wide
// health documents (one per kind per day) that the integrations BrandHealth Lambda computes --
// the first kind is the SES account row on the Dashboard. The fork knows nothing about what a
// kind means; it validates only the keys it indexes (v, kind, day, status) and stores the rest
// verbatim. Permissions reuse brands:get / brands:manage (no new permission).

var reSystemHealthKind = regexp.MustCompile(`^[a-z0-9_-]{1,40}$`)

// validateSystemHealthRows checks a PUT batch. It returns an error naming the first bad row and
// field, and refuses the whole batch on any one bad row (all-or-nothing), an empty batch, an
// oversized batch, or the same (kind, day) twice (which one upsert statement cannot apply).
func validateSystemHealthRows(rows []json.RawMessage) error {
	if len(rows) == 0 {
		return fmt.Errorf("rows is empty")
	}
	if len(rows) > brandHealthMaxRows {
		return fmt.Errorf("too many rows (%d > %d)", len(rows), brandHealthMaxRows)
	}

	seen := map[string]bool{}
	for i, raw := range rows {
		var d map[string]json.RawMessage
		if err := json.Unmarshal(raw, &d); err != nil || d == nil {
			return fmt.Errorf("row %d is not a JSON object", i)
		}

		var v float64
		if err := json.Unmarshal(d["v"], &v); err != nil || v != 1 {
			return fmt.Errorf("row %d: v must be 1", i)
		}

		kind, ok := jsonString(d["kind"])
		if !ok || !reSystemHealthKind.MatchString(kind) {
			return fmt.Errorf("row %d: kind must match [a-z0-9_-]{1,40}", i)
		}

		day, ok := jsonString(d["day"])
		if !ok || !reISODay.MatchString(day) {
			return fmt.Errorf("row %d: day must be an ISO date (YYYY-MM-DD)", i)
		}
		if _, err := time.Parse("2006-01-02", day); err != nil {
			return fmt.Errorf("row %d: day must be an ISO date (YYYY-MM-DD)", i)
		}

		status, ok := jsonString(d["status"])
		if !ok || !brandHealthStatuses[status] {
			return fmt.Errorf("row %d: status must be one of ok, warn, issues, unknown", i)
		}

		key := kind + "\x00" + day
		if seen[key] {
			return fmt.Errorf("row %d: duplicate (kind, day) %s %s", i, kind, day)
		}
		seen[key] = true
	}

	return nil
}

// PutSystemHealth upserts a batch of system health documents. Body: {"rows": [document, ...]}.
func (a *App) PutSystemHealth(c echo.Context) error {
	var req struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "rows"))
	}
	if err := validateSystemHealthRows(req.Rows); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	batch, err := json.Marshal(req.Rows)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "rows"))
	}

	n, err := a.core.UpsertSystemHealth(batch)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{map[string]int64{"rows": n}})
}

// GetSystemHealthHistory returns one kind's documents, newest first. ?days=N (default 30, max
// 400 -- the brand-health history rule). An unknown kind is an empty list.
func (a *App) GetSystemHealthHistory(c echo.Context) error {
	kind := c.Param("kind")
	if !reSystemHealthKind.MatchString(kind) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "kind"))
	}

	days, err := parseBrandHealthDays(c.QueryParam("days"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "days"))
	}

	out, err := a.core.GetSystemHealthHistory(kind, days)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}
