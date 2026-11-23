package core

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// GetRoles retrieves all roles.
func (c *Core) GetRoles() ([]auth.Role, error) {
	out := []auth.Role{}
	if err := c.q.GetUserRoles.Select(&out, nil); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "role", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetRole retrieves a role.
func (c *Core) GetRole(id int) (auth.Role, error) {
	out := []auth.Role{}
	if err := c.q.GetUserRoles.Select(&out, id); err != nil {
		return auth.Role{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "role", "error", pqErrMsg(err)))
	}

	// Role does not exist.
	if len(out) == 0 {
		return auth.Role{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "role", "error", "role not found"))
	}

	return out[0], nil
}

// GetListRoles retrieves all list roles.
func (c *Core) GetListRoles() ([]auth.ListRole, error) {
	out := []auth.ListRole{}
	if err := c.q.GetListRoles.Select(&out); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "role", "error", pqErrMsg(err)))
	}

	// Unmarshall the resolved lists and the brand grants (fork, brand roles, BRAND-ROLES-SPEC D5).
	for n, r := range out {
		out[n].Lists = []auth.ListPermission{}
		out[n].Brands = []auth.BrandGrant{}
		if r.ListsRaw != nil {
			if err := json.Unmarshal(r.ListsRaw, &out[n].Lists); err != nil {
				c.log.Printf("error unmarshalling list permissions for role %d: %v", r.ID, err)
			}
		}
		if r.BrandsRaw != nil {
			if err := json.Unmarshal(r.BrandsRaw, &out[n].Brands); err != nil {
				c.log.Printf("error unmarshalling brand grants for role %d: %v", r.ID, err)
			}
		}
	}

	return out, nil
}

// BrandSlugsIn (fork, brand roles, BRAND-ROLES-SPEC D5) returns which of the given slugs are brands
// rows -- the role handlers' unknown-brand check.
func (c *Core) BrandSlugsIn(slugs []string) (map[string]bool, error) {
	var found []string
	if err := c.q.GetBrandSlugsIn.Select(&found, pq.StringArray(slugs)); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "brands", "error", pqErrMsg(err)))
	}
	out := make(map[string]bool, len(found))
	for _, s := range found {
		out[s] = true
	}
	return out, nil
}

// replaceRoleBrands (fork, brand roles, BRAND-ROLES-SPEC D5) replaces a role's brand grants inside
// tx: the old rows deleted, the given ones inserted. The grants are validated by the handler.
func (c *Core) replaceRoleBrands(tx *sqlx.Tx, roleID int, grants []auth.BrandGrant) error {
	if _, err := tx.Stmtx(c.q.DeleteRoleBrands).Exec(roleID); err != nil {
		return err
	}
	brands := make([]string, 0, len(grants))
	levels := make([]string, 0, len(grants))
	for _, g := range grants {
		b := ""
		if g.Brand != nil {
			b = *g.Brand
		}
		brands = append(brands, b)
		levels = append(levels, g.Level)
	}
	if len(grants) == 0 {
		return nil
	}
	_, err := tx.Stmtx(c.q.InsertRoleBrands).Exec(roleID, pq.StringArray(brands), pq.StringArray(levels))
	return err
}

// CreateRole creates a new role.
func (c *Core) CreateRole(r auth.Role) (auth.Role, error) {
	var out auth.Role

	if err := c.q.CreateRole.Get(&out, r.Name, auth.RoleTypeUser, pq.Array(r.Permissions)); err != nil {
		return out, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", pqErrMsg(err)))
	}

	return out, nil
}

// CreateListRole creates a new list role.
func (c *Core) CreateListRole(r auth.ListRole) (auth.ListRole, error) {
	var out auth.ListRole

	// Fork (brand roles, integrations listmonk/BRAND-ROLES-SPEC D5) -- the role row and its brand
	// grants in one transaction; the per-list child rows are never written.
	fail := func(err error) (auth.ListRole, error) {
		return auth.ListRole{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{users.role}", "error", pqErrMsg(err)))
	}
	tx, err := c.db.Beginx()
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback()

	// Unsafe: the statement returns roles.* (type, permissions) which ListRole does not carry; a
	// transaction-bound statement loses the app handle's Unsafe flag.
	if err := tx.Stmtx(c.q.CreateRole).Unsafe().Get(&out, r.Name, auth.RoleTypeList, pq.Array([]string{})); err != nil {
		return fail(err)
	}
	if err := c.replaceRoleBrands(tx, out.ID, r.Brands); err != nil {
		return fail(err)
	}
	if err := tx.Commit(); err != nil {
		return fail(err)
	}

	out.Brands = r.Brands
	return out, nil
}

// UpdateUserRole updates a given role.
func (c *Core) UpdateUserRole(id int, r auth.Role) (auth.Role, error) {
	var out auth.Role

	if err := c.q.UpdateRole.Get(&out, id, r.Name, pq.Array(r.Permissions)); err != nil {
		return out, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{users.userRole}", "error", pqErrMsg(err)))
	}

	if out.ID == 0 {
		return out, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.notFound", "name", "{users.userRole}"))
	}

	return out, nil
}

// UpdateListRole updates a given role.
func (c *Core) UpdateListRole(id int, r auth.ListRole) (auth.ListRole, error) {
	var out auth.ListRole

	// Fork (brand roles, integrations listmonk/BRAND-ROLES-SPEC D5) -- update-list-role (a list role
	// only: any other id updates nothing and is a 400) and the grant replacement in ONE
	// transaction, so a failed write leaves the role and its grants exactly as they were.
	fail := func(err error) (auth.ListRole, error) {
		return auth.ListRole{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{users.listRole}", "error", pqErrMsg(err)))
	}
	tx, err := c.db.Beginx()
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback()

	if err := tx.Stmtx(c.q.UpdateListRole).Unsafe().Get(&out, id, r.Name); err != nil {
		if err == sql.ErrNoRows {
			return out, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.notFound", "name", "{users.listRole}"))
		}
		return fail(err)
	}
	if out.ID == 0 {
		return out, echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.notFound", "name", "{users.listRole}"))
	}
	if err := c.replaceRoleBrands(tx, out.ID, r.Brands); err != nil {
		return fail(err)
	}
	if err := tx.Commit(); err != nil {
		return fail(err)
	}

	out.Brands = r.Brands
	return out, nil
}

// DeleteRole deletes a given role.
func (c *Core) DeleteRole(id int) error {
	if _, err := c.q.DeleteRole.Exec(id); err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Constraint == "users_role_id_fkey" {
			return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("users.cantDeleteRole"))
		}
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{users.role}", "error", pqErrMsg(err)))
	}

	return nil
}
