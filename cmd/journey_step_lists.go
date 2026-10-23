package main

// Fork (journey form UX, integrations JOURNEY-UX-SPEC A1.1) -- a "still deciding" step list (a
// list whose join_trigger kind is no_purchase) is filled by the NoPurchaseSweep alone: a hand-added
// member is joined outside the step's timing, and the running delay-0 evergreen on the list mails
// them at once. A browser SESSION may therefore never ADD a membership row to one; an API token
// (the sweep's and the sunset's writers) passes. "Add" is a row that does not exist yet: a
// round-trip carrying existing membership, a removal, an unsubscribe and a hold all stay allowed,
// so cleanup remains possible. Every membership-adding handler calls refuseStepListAdd before its
// write, so a refused request writes nothing. The UI filters the pickers (campaignListUx.mjs);
// this is the control.

import (
	"net/http"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
)

// refuseStepListAdd refuses a session write that would add a membership row to a step list among
// listIDs. existing is the subscribers the write targets; empty means every step list in listIDs
// counts as an add (a new subscriber, an import in any mode, a by-query add, whose rows are not
// known up front).
func (a *App) refuseStepListAdd(user auth.User, existing, listIDs []int) error {
	if user.Type == auth.UserTypeAPI {
		return nil
	}
	_, name, found, err := a.core.StepListAdd(existing, listIDs)
	if err != nil {
		return err
	}
	if found {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("lists.noPurchaseManualAdd", "name", name))
	}
	return nil
}
