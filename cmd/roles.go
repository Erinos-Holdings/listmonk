package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
	null "gopkg.in/volatiletech/null.v6"
)

// GetUserRoles retrieves roles.
func (a *App) GetUserRoles(c echo.Context) error {
	// Get all roles.
	out, err := a.core.GetRoles()
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// GeListRoles retrieves roles.
func (a *App) GeListRoles(c echo.Context) error {
	// Get all roles.
	out, err := a.core.GetListRoles()
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// CreateUserRole handles role creation.
func (a *App) CreateUserRole(c echo.Context) error {
	var r auth.Role
	if err := c.Bind(&r); err != nil {
		return err
	}
	if err := a.validateUserRole(r); err != nil {
		return err
	}

	// Create the role in the DB.
	out, err := a.core.CreateRole(r)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// CreateListRole handles role creation.
func (a *App) CreateListRole(c echo.Context) error {
	r, err := a.parseListRole(c)
	if err != nil {
		return err
	}

	// Create the role in the DB.
	out, err := a.core.CreateListRole(r)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateUserRole handles role modification.
func (a *App) UpdateUserRole(c echo.Context) error {
	id := getID(c)

	// ID 1 is reserved for the Super Admin user role.
	if id == auth.SuperAdminRoleID {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidID"))
	}

	// Incoming params.
	var r auth.Role
	if err := c.Bind(&r); err != nil {
		return err
	}
	if err := a.validateUserRole(r); err != nil {
		return err
	}

	// Validate.
	r.Name.String = strings.TrimSpace(r.Name.String)

	// Update the role in the DB.
	out, err := a.core.UpdateUserRole(id, r)
	if err != nil {
		return err
	}

	// Cache API tokens for in-memory, off-DB /api/* request auth.
	if _, err := cacheUsers(a.core, a.auth); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateListRole handles role modification.
func (a *App) UpdateListRole(c echo.Context) error {
	// Get the role ID.
	id := getID(c)

	// ID 1 is reserved for the Super Admin user role.
	if id == auth.SuperAdminRoleID {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidID"))
	}

	// Incoming params (fork, brand roles, BRAND-ROLES-SPEC D5: brands, never lists).
	r, err := a.parseListRole(c)
	if err != nil {
		return err
	}

	// Update the role in the DB (a non-list role id is a 400 there).
	out, err := a.core.UpdateListRole(id, r)
	if err != nil {
		return err
	}

	// Cache API tokens for in-memory, off-DB /api/* request auth.
	if _, err := cacheUsers(a.core, a.auth); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// DeleteRole handles (user|list) role deletion.
func (a *App) DeleteRole(c echo.Context) error {
	// Get the role ID.
	id := getID(c)

	// ID 1 is reserved for the Super Admin user role.
	if id == auth.SuperAdminRoleID {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidID"))
	}

	// Delete the role from the DB.
	if err := a.core.DeleteRole(int(id)); err != nil {
		return err
	}

	// Cache API tokens for in-memory, off-DB /api/* request auth.
	if _, err := cacheUsers(a.core, a.auth); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

func (a *App) validateUserRole(r auth.Role) error {
	if !strHasLen(r.Name.String, 1, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "name"))
	}

	for _, p := range r.Permissions {
		if _, ok := a.cfg.Permissions[p]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", fmt.Sprintf("permission: %s", p)))
		}
	}

	return nil
}

// listRoleReq (fork, brand roles, integrations listmonk/BRAND-ROLES-SPEC D5) is the body of POST
// /api/roles/lists and PUT /api/roles/lists/:id: a name and the brand grants. Lists is read only to
// refuse it -- a brand role grants by brand, never by list.
type listRoleReq struct {
	Name   string            `json:"name"`
	Brands []auth.BrandGrant `json:"brands"`
	Lists  []json.RawMessage `json:"lists"`
}

// parseListRole binds and validates a list-role write (D5): a non-empty lists field, a bad name,
// any brandGrantsProblem and an unknown brand slug are each a 400.
func (a *App) parseListRole(c echo.Context) (auth.ListRole, error) {
	var req listRoleReq
	if err := c.Bind(&req); err != nil {
		return auth.ListRole{}, err
	}
	bad := func(field string) (auth.ListRole, error) {
		return auth.ListRole{}, echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", field))
	}

	name := strings.TrimSpace(req.Name)
	if !strHasLen(name, 1, stdInputMaxLen) {
		return bad("name")
	}
	if len(req.Lists) > 0 {
		return bad("lists (a brand role grants brands)")
	}
	if p := brandGrantsProblem(req.Brands); p != "" {
		return bad(p)
	}

	var slugs []string
	for _, g := range req.Brands {
		if g.Brand != nil {
			slugs = append(slugs, *g.Brand)
		}
	}
	if len(slugs) > 0 {
		known, err := a.core.BrandSlugsIn(slugs)
		if err != nil {
			return auth.ListRole{}, err
		}
		for _, s := range slugs {
			if !known[s] {
				return bad(fmt.Sprintf("brands: unknown brand %s", s))
			}
		}
	}

	grants := req.Brands
	if grants == nil {
		grants = []auth.BrandGrant{}
	}
	return auth.ListRole{Name: null.StringFrom(name), Brands: grants}, nil
}

// brandGrantsProblem (fork, brand roles, BRAND-ROLES-SPEC D5) is the pure half of the grant
// validation: "" when the set is acceptable, else the field text for the 400. Refused: a level
// other than manage or analytics; an empty brand slug; All brands (brand null) at manage; the same
// brand at both levels; a duplicate entry.
func brandGrantsProblem(grants []auth.BrandGrant) string {
	seen := map[string]string{}
	for _, g := range grants {
		if g.Level != auth.BrandLevelManage && g.Level != auth.BrandLevelAnalytics {
			return fmt.Sprintf("brands: level %q", g.Level)
		}
		key := "*"
		if g.Brand != nil {
			if strings.TrimSpace(*g.Brand) == "" || *g.Brand == "*" {
				return "brands: empty brand"
			}
			key = *g.Brand
		} else if g.Level == auth.BrandLevelManage {
			return "brands: All brands is read-only (analytics)"
		}
		if prev, ok := seen[key]; ok {
			if prev == g.Level {
				return fmt.Sprintf("brands: duplicate %s", key)
			}
			return fmt.Sprintf("brands: %s at both levels", key)
		}
		seen[key] = g.Level
	}
	return ""
}
