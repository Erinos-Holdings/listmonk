package main

// Fork (sunset v2) -- integrations SUNSET-V2-SPEC D8: the re-engagement queue countdown. The
// ListmonkSunset Lambda computes {oldestDays, archiveInDays, asOf} per queue list each run and
// writes it here; the Lists page renders it (frontend/src/listSunset.mjs). The fork stores and
// renders, never computes.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
)

// maxListSunsetBody bounds the request body: the document is three small fields.
const maxListSunsetBody = 4096

// validateListSunset is the shape check on the countdown document: exactly the keys oldestDays
// (an integer >= 0, required), archiveInDays (an integer, or null/absent -- queue 1 has none)
// and asOf (an RFC 3339 timestamp, required). Anything else is refused, so what is stored
// verbatim is always this shape.
func validateListSunset(body []byte) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil || doc == nil {
		return fmt.Errorf("sunset must be a JSON object")
	}
	if dec.More() {
		return fmt.Errorf("sunset must be one JSON object")
	}
	for k := range doc {
		switch k {
		case "oldestDays", "archiveInDays", "asOf":
		default:
			return fmt.Errorf("sunset has an unknown key: %s", k)
		}
	}
	intOf := func(v any) (int64, bool) {
		n, ok := v.(json.Number)
		if !ok {
			return 0, false
		}
		i, err := n.Int64()
		return i, err == nil
	}
	if n, ok := intOf(doc["oldestDays"]); !ok || n < 0 {
		return fmt.Errorf("sunset.oldestDays must be an integer >= 0")
	}
	if v, ok := doc["archiveInDays"]; ok && v != nil {
		if _, ok := intOf(v); !ok {
			return fmt.Errorf("sunset.archiveInDays must be an integer or null")
		}
	}
	s, ok := doc["asOf"].(string)
	if !ok {
		return fmt.Errorf("sunset.asOf must be an RFC 3339 timestamp")
	}
	if _, err := time.Parse(time.RFC3339, s); err != nil {
		return fmt.Errorf("sunset.asOf must be an RFC 3339 timestamp")
	}
	return nil
}

// PutListSunset handles PUT /api/lists/:id/sunset (list manage permission on the list).
func (a *App) PutListSunset(c echo.Context) error {
	user := auth.GetUser(c)
	id := getID(c)
	if err := user.HasListPerm(auth.PermTypeManage, id); err != nil {
		return err
	}

	body, err := io.ReadAll(io.LimitReader(c.Request().Body, maxListSunsetBody+1))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(body) > maxListSunsetBody {
		return echo.NewHTTPError(http.StatusBadRequest, "sunset body too large")
	}
	if err := validateListSunset(body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := a.core.UpdateListSunset(id, bytes.TrimSpace(body)); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{true})
}
