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
		// Fork (journey lists, JOURNEY-LISTS-SPEC D6) -- journey_locked on every list response.
		markJourneyLocked(res)

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

		// Fork (campaign lists, CAMPAIGN-LISTS-SPEC D7) -- hide_system=true drops the hidden system
		// lists (the Lists page with "View system lists" off). Totals and pagination follow it.
		hideSystem, _ = strconv.ParseBool(c.QueryParam("hide_system"))
	)
	res, total, err := a.core.QueryLists(query, typ, optin, status, tags, orderBy, order, hasAllPerm, permittedIDs, noBrand, hideSystem, pg.Offset, pg.Limit)
	if err != nil {
		return err
	}
	markJourneyLocked(res)

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
	if err := a.decorateList(&out, true); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// decorateList sets the computed response fields (fork, journey lists -- JOURNEY-LISTS-SPEC D6,
// D2a): journey_locked and system_hidden always (one setter, CAMPAIGN-LISTS-SPEC D6),
// running_automation when running (the single-list GET only).
func (a *App) decorateList(l *models.List, running bool) error {
	setListFlags(l)
	if running {
		ok, err := a.core.ListHasRunningEvergreen(l.ID)
		if err != nil {
			return err
		}
		l.RunningAutomation = &ok
	}
	return nil
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
	// Fork (naming standard, NAMING-STANDARD-SPEC D6 §4.3) -- the name rule on create, the limit on
	// the part after the brand prefix; stored trimmed.
	l.Name = strings.TrimSpace(l.Name)
	if err := a.nameRuleErr(models.NameKindList, l.Name, listReqBrand(r, ""), listTypeOr(l.Type, "")); err != nil {
		return err
	}
	// Fork (BRAND-PICKER-SPEC D2) -- reserved tags are refused; the brand's projection is the
	// only writer. Absent brand on create = no reserved tags.
	tags, _, err := a.listTagsFor(r)
	if err != nil {
		return err
	}
	l.Tags = tags
	// Fork (journey lists, JOURNEY-LISTS-SPEC D6/D7 L3) -- a list carrying a reengage: tag (a
	// sunset queue, a standard journey list) is created only by a holder of lists:journeys. POST
	// ignores join_trigger and trigger_active, so no other create can mint a locked list.
	if !canManageJourneys(auth.GetUser(c)) {
		for _, t := range l.Tags {
			if strings.HasPrefix(strings.TrimSpace(t), reengageTagPrefix) {
				return a.journeyLockedErr(l.Name)
			}
		}
	}
	// Fork (BRAND-PICKER-SPEC D4) -- a second list of a locked name is refused (the route's
	// lists:manage_all check has already run).
	if err := a.refuseSecondLocked(l.Name); err != nil {
		return err
	}

	out, err := a.core.CreateList(l)
	if err != nil {
		return err
	}
	if err := a.decorateList(&out, false); err != nil {
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

	// Incoming params.
	var r listReq
	if err := c.Bind(&r); err != nil {
		return err
	}
	l := r.List

	// Fork (journey lists, JOURNEY-LISTS-SPEC D6/D7 L1/L2) -- the stored list is always read: the
	// standard-journey-list lock runs FIRST, on the STORED list (so a request that would clear a
	// locked list's trigger is refused too), and a request setting a standard kind on any list
	// needs lists:journeys. A refused request applies nothing.
	cur, err := a.core.GetList(id, "")
	if err != nil {
		return err
	}
	if !canManageJourneys(user) {
		if isJourneyLockedList(cur) {
			return a.journeyLockedErr(cur.Name)
		}
		if r.JoinTrigger != nil && !isJSONNull(r.JoinTrigger) && isStandardTriggerKind(joinTriggerKind(r.JoinTrigger)) {
			return a.journeyLockedErr(cur.Name)
		}
	}

	// Fork (list join triggers, LIST-JOIN-TRIGGER-SPEC D5) -- the transactional permission check
	// runs before anything is applied (name, tags, brand, join_trigger included), when the request
	// carries a field the trigger rules read.
	// JOURNEY-UX-SPEC A1.1b: a type of public is judged against the stored trigger too.
	needCur := listTriggerCarried(r) || r.Brand != nil || r.Type == models.ListTypePublic
	if needCur {
		if err := a.checkListTriggerPerm(user, r, cur); err != nil {
			return err
		}
	}

	// Fork (BRAND-PICKER-SPEC D4) -- a locked list is never updated through the API.
	if err := a.refuseLocked(id); err != nil {
		return err
	}

	// Validate.
	if !strHasLen(l.Name, 1, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.invalidName"))
	}
	// Fork (BRAND-PICKER-SPEC D2) -- reserved tags refused; brand absent keeps the current
	// projection (merged in the update-list statement), "" clears it, a slug projects its row.
	// Fork (FINAL-QUEUE-GUARD-SPEC D2) -- the list's current reengage: queue tags are merged back
	// in the same statement whatever these tags carry: they key the final-queue campaign refusal.
	// A reengage: tag the list does not already carry is refused (create is the only minting path).
	tags, keepReserved, err := a.listTagsFor(r)
	if err != nil {
		return err
	}
	for _, t := range r.Tags {
		if !strings.HasPrefix(strings.TrimSpace(t), reengageTagPrefix) {
			continue
		}
		if nt := newReengageTag(r.Tags, cur.Tags); nt != "" {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("lists.reservedTag", "tag", nt))
		}
		break
	}
	l.Tags = tags
	// A rename ONTO a locked name is refused outright (this list is not locked -- refuseLocked
	// passed above -- so a locked name here is always a rename-into): with no copy present it
	// would lock this list, subscribers and brand included, with SQL as the only undo. Creation
	// (CreateList) is the one way to mint the locked list.
	if models.IsLockedListName(l.Name) {
		return a.lockedListErr(l.Name)
	}
	// Fork (naming standard, NAMING-STANDARD-SPEC D3/D6 §4.3) -- only a CHANGED name is judged (an
	// unchanged longer name stays valid until renamed; "" keeps the stored name); a changed one is
	// stored trimmed, judged against the brand and type the list will have. The former-name rule
	// runs in core.UpdateList's transaction.
	if l.Name != "" && l.Name != cur.Name {
		l.Name = strings.TrimSpace(l.Name)
		if l.Name != cur.Name {
			if err := a.nameRuleErr(models.NameKindList, l.Name, listReqBrand(r, cur.Brand), listTypeOr(l.Type, cur.Type)); err != nil {
				return err
			}
		}
	}

	// Fork (list join triggers, D3/D2) -- the trigger shape and the brand rule are validated
	// BEFORE the main update, so a 400 leaves the whole list unchanged.
	var tp listTriggerPlan
	if needCur {
		if tp, err = a.planListTrigger(r, cur); err != nil {
			return err
		}
	}
	// Fork (journey lists, JOURNEY-LISTS-SPEC D3, review F1) -- a brand change turns an active
	// list's Active switch off through the trigger statement, so it is a trigger write too (the
	// test checkListTriggerPerm makes). planListTrigger ran (r.Brand != nil is in needCur), so tp
	// carries the resulting trigger and transactional.
	brandChanged := r.Brand != nil && strings.TrimSpace(*r.Brand) != cur.Brand
	if brandChanged {
		tp.write = true
	}

	// Update the list in the DB.
	out, err := a.core.UpdateList(id, l, keepReserved, user.Username)
	if err != nil {
		return err
	}

	// Fork (list join triggers, D4) -- the trigger write, one statement after update-list (not
	// atomic with it: an error here leaves the main update applied and is returned; a retry
	// converges). It no-ops, stamps included, when nothing differs from the stored row.
	if tp.write {
		changed, err := a.core.UpdateListTrigger(id, tp.joinTrigger, tp.transactional, user.Username, brandChanged)
		if err != nil {
			return err
		}
		if changed {
			if out, err = a.core.GetList(id, ""); err != nil {
				return err
			}
		}
	}
	if err := a.decorateList(&out, false); err != nil {
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
	// Fork (journey lists, JOURNEY-LISTS-SPEC D7 L4) -- a standard journey list needs lists:journeys.
	if err := a.refuseJourneyLocked(user, id); err != nil {
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
		// Fork (journey lists, JOURNEY-LISTS-SPEC D7 L5) -- refused whole when any is a standard
		// journey list and the caller lacks lists:journeys.
		if err := a.refuseJourneyLocked(user, ids...); err != nil {
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
		// Fork (journey lists, JOURNEY-LISTS-SPEC D7 L5) -- the matched set first, then refused
		// whole when any of it is a standard journey list (the refuseLocked pattern).
		if err := a.refuseJourneyLocked(user, matched...); err != nil {
			return err
		}
		if len(matched) > 0 {
			if err := a.core.DeleteLists(matched, "", true, nil); err != nil {
				return err
			}
		}
	}

	return c.JSON(http.StatusOK, okResp{true})
}
