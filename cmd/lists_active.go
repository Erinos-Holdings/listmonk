package main

// Fork (journey lists) -- integrations listmonk/JOURNEY-LISTS-SPEC D2: PUT /api/lists/:id/active, the
// list's Active switch -- the go-live of its joins. The integrations PurchaseTriggerSync reads
// trigger_active and marks the registry row of an inactive list inactive (the purchase leg and the
// NoPurchaseSweep skip it); trigger_active_at is the no-backfill start date while on. Marketing's:
// the list's manage permission, never lists:journeys or lists:transactional. Rules, in order:
//
//  1. the list is active (not archived) and carries a join_trigger -- else 400;
//  2. the same value as stored -- 200, nothing written (stamps unchanged);
//  3. turning ON needs a running automation on the list (core.ListHasRunningEvergreen, the one
//     definition) -- else 400 lists.activeNeedsRunning; off is always allowed;
//  4. the flag and both stamps in one statement (update-list-active); updated_at untouched.
//
// UpdateList and CreateList never write the three columns -- this is the switch's one route.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

const maxListActiveBody = 256

// parseListActiveBody reads exactly {"active": true|false}.
func parseListActiveBody(body []byte) (bool, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	var doc map[string]json.RawMessage
	if err := dec.Decode(&doc); err != nil || doc == nil || dec.More() || len(doc) != 1 {
		return false, false
	}
	raw, ok := doc["active"]
	if !ok {
		return false, false
	}
	switch string(bytes.TrimSpace(raw)) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

// PutListActive handles PUT /api/lists/:id/active.
func (a *App) PutListActive(c echo.Context) error {
	user := auth.GetUser(c)
	id := getID(c)
	if err := user.HasListPerm(auth.PermTypeManage, id); err != nil {
		return err
	}

	body, err := io.ReadAll(io.LimitReader(c.Request().Body, maxListActiveBody+1))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	active, ok := parseListActiveBody(body)
	if len(body) > maxListActiveBody || !ok {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.activeInvalidBody"))
	}

	cur, err := a.core.GetList(id, "")
	if err != nil {
		return err
	}
	if cur.Status != models.ListStatusActive || len(cur.JoinTrigger) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.activeNeedsTrigger"))
	}

	if cur.TriggerActive != active {
		if active {
			running, err := a.core.ListHasRunningEvergreen(id)
			if err != nil {
				return err
			}
			if !running {
				return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.activeNeedsRunning"))
			}
		}
		changed, err := a.core.UpdateListActive(id, active, user.Username)
		if err != nil {
			return err
		}
		if changed {
			if cur, err = a.core.GetList(id, ""); err != nil {
				return err
			}
		}
	}

	if err := a.decorateList(&cur, true); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{cur})
}
