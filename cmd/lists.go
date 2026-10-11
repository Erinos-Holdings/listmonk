package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// GetLists retrieves lists with additional metadata like subscriber counts.
func (a *App) GetLists(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Get the list IDs (or blanket permission) the user has access to.
	hasAllPerm, permittedIDs := user.GetPermittedLists(auth.PermTypeGet)

	// Minimal query simply returns the list of all lists without JOIN subscriber counts. This is fast.
	minimal, _ := strconv.ParseBool(c.FormValue("minimal"))
	if minimal {
		status := c.FormValue("status")
		res, err := a.core.GetLists("", status, hasAllPerm, permittedIDs)
		if err != nil {
			return err
		}
		if len(res) == 0 {
			return c.JSON(http.StatusOK, okResp{[]struct{}{}})
		}

		// Meta.
		total := len(res)
		out := models.PageResults{
			Results: res,
			Total:   total,
			Page:    1,
			PerPage: total,
		}

		return c.JSON(http.StatusOK, okResp{out})
	}

	// Full list query.
	var (
		query   = strings.TrimSpace(c.FormValue("query"))
		tags    = c.QueryParams()["tag"]
		orderBy = c.FormValue("order_by")
		typ     = c.FormValue("type")
		optin   = c.FormValue("optin")
		status  = c.FormValue("status")
		order   = c.FormValue("order")

		pg = a.pg.NewFromURL(c.Request().URL.Query())

		// Fork (global brand, GLOBAL-BRAND-SPEC D6) -- the No brand context: lists with no brand
		// tag. A named brand is the existing tag=brand:<slug>. Both narrow, never widen, the
		// permission scoping above.
		noBrand, _ = strconv.ParseBool(c.QueryParam("nobrand"))
	)
	res, total, err := a.core.QueryLists(query, typ, optin, status, tags, orderBy, order, hasAllPerm, permittedIDs, noBrand, pg.Offset, pg.Limit)
	if err != nil {
		return err
	}

	out := models.PageResults{
		Query:   query,
		Results: res,
		Total:   total,
		Page:    pg.Page,
		PerPage: pg.PerPage,
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// GetList retrieves a single list by id.
// It's permission checked by the listPerm middleware.
func (a *App) GetList(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Check if the user has access to the list.
	id := getID(c)
	if err := user.HasListPerm(auth.PermTypeGet, id); err != nil {
		return err
	}

	// Get the list from the DB.
	out, err := a.core.GetList(id, "")
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// CreateList handles list creation.
func (a *App) CreateList(c echo.Context) error {
	var r listReq
	if err := c.Bind(&r); err != nil {
		return err
	}
	l := r.List

	// Validate.
	if !strHasLen(l.Name, 1, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.invalidName"))
	}
	// Fork (BRAND-PICKER-SPEC D2) -- reserved tags are refused; the brand's projection is the
	// only writer. Absent brand on create = no reserved tags.
	tags, _, err := a.listTagsFor(r)
	if err != nil {
		return err
	}
	l.Tags = tags
	// Fork (BRAND-PICKER-SPEC D4) -- a second list of a locked name is refused (the route's
	// lists:manage_all check has already run).
	if err := a.refuseSecondLocked(l.Name); err != nil {
		return err
	}

	out, err := a.core.CreateList(l)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateList handles list modification.
// It's permission checked by the listPerm middleware.
func (a *App) UpdateList(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Check if the user has access to the list.
	id := getID(c)
	if err := user.HasListPerm(auth.PermTypeManage, id); err != nil {
		return err
	}

	// Fork (BRAND-PICKER-SPEC D4) -- a locked list is never updated through the API.
	if err := a.refuseLocked(id); err != nil {
		return err
	}

	// Incoming params.
	var r listReq
	if err := c.Bind(&r); err != nil {
		return err
	}
	l := r.List

	// Validate.
	if !strHasLen(l.Name, 1, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.invalidName"))
	}
	// Fork (BRAND-PICKER-SPEC D2) -- reserved tags refused; brand absent keeps the current
	// projection (merged in the update-list statement), "" clears it, a slug projects its row.
	tags, keepReserved, err := a.listTagsFor(r)
	if err != nil {
		return err
	}
	l.Tags = tags
	// A rename ONTO a locked name is refused outright (this list is not locked -- refuseLocked
	// passed above -- so a locked name here is always a rename-into): with no copy present it
	// would lock this list, subscribers and brand included, with SQL as the only undo. Creation
	// (CreateList) is the one way to mint the locked list.
	if models.IsLockedListName(l.Name) {
		return a.lockedListErr(l.Name)
	}

	// Update the list in the DB.
	out, err := a.core.UpdateList(id, l, keepReserved)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// DeleteList deletes a single list by ID.
func (a *App) DeleteList(c echo.Context) error {
	id := getID(c)

	// Check if the user has manage permission for the list.
	user := auth.GetUser(c)
	if err := user.HasListPerm(auth.PermTypeManage, id); err != nil {
		return err
	}
	// Fork (BRAND-PICKER-SPEC D4) -- after the permission check.
	if err := a.refuseLocked(id); err != nil {
		return err
	}

	// Delete the list from the DB.
	// Pass getAll=true since we've already verified permissions above.
	if err := a.core.DeleteLists([]int{id}, "", true, nil); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// DeleteLists deletes multiple lists by IDs or by query.
func (a *App) DeleteLists(c echo.Context) error {
	user := auth.GetUser(c)

	var (
		ids   []int
		query string
		all   bool
	)

	// Check for IDs in query params.
	if len(c.Request().URL.Query()["id"]) > 0 {
		var err error
		ids, err = parseStringIDs(c.Request().URL.Query()["id"])
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
		}
	} else {
		// Check for query param.
		query = strings.TrimSpace(c.FormValue("query"))
		all = c.FormValue("all") == "true"
	}

	// Validate that either IDs or query is provided.
	if len(ids) == 0 && (query == "" && !all) {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", "id or query required"))
	}

	// For ID deletion, check if the user has manage permission for the specific lists.
	if len(ids) > 0 {
		if err := user.HasListPerm(auth.PermTypeManage, ids...); err != nil {
			return err
		}
		// Fork (BRAND-PICKER-SPEC D4) -- refused whole when any is locked.
		if err := a.refuseLocked(ids...); err != nil {
			return err
		}

		// Delete the lists from the DB.
		// Pass getAll=true since we've already verified permissions above.
		if err := a.core.DeleteLists(ids, "", true, nil); err != nil {
			return err
		}
	} else {
		// For query deletion, get the list IDs the user has manage permission for.
		hasAllPerm, permittedIDs := user.GetPermittedLists(auth.PermTypeManage)

		// Fork (BRAND-PICKER-SPEC D4) -- select exactly what the delete-lists predicate would
		// remove, refuse the whole delete naming a locked list among them (a partial delete would
		// hide the refusal), then delete those ids.
		matched, names, err := a.core.ListsMatchingDeleteQuery(query, hasAllPerm, permittedIDs)
		if err != nil {
			return err
		}
		for _, n := range names {
			if models.IsLockedListName(n) {
				return a.lockedListErr(n)
			}
		}
		if len(matched) > 0 {
			if err := a.core.DeleteLists(matched, "", true, nil); err != nil {
				return err
			}
		}
	}

	return c.JSON(http.StatusOK, okResp{true})
}
