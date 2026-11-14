package core

import (
	"database/sql"
	"net/http"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	null "gopkg.in/volatiletech/null.v6"
)

// GetTemplates retrieves all templates.
func (c *Core) GetTemplates(status string, noBody bool) ([]models.Template, error) {
	out := []models.Template{}
	if err := c.q.GetTemplates.Select(&out, 0, noBody, status); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.templates}", "error", pqErrMsg(err)))
	}

	return out, nil
}

// GetTemplate retrieves a given template.
func (c *Core) GetTemplate(id int, noBody bool) (models.Template, error) {
	var out []models.Template
	if err := c.q.GetTemplates.Select(&out, id, noBody, ""); err != nil {
		return models.Template{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.templates}", "error", pqErrMsg(err)))
	}

	if len(out) == 0 {
		return models.Template{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.template}"))
	}

	return out[0], nil
}

// CreateTemplate creates a new template.
func (c *Core) CreateTemplate(name, typ, subject string, body []byte, bodySource null.String, brand, lang string) (models.Template, error) {
	var newID int
	if err := c.q.CreateTemplate.Get(&newID, name, typ, subject, body, bodySource, brand, lang); err != nil {
		return models.Template{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
	}

	return c.GetTemplate(newID, false)
}

// UpdateTemplate updates a given template.
func (c *Core) UpdateTemplate(id int, name, subject string, body []byte, bodySource null.String, brand, lang string) (models.Template, error) {
	errUpd := func(err error) error {
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
	}
	// Fork (naming standard, NAMING-STANDARD-SPEC §4.2) -- the former-name rule in the same
	// transaction, before the write; a no-op for an unchanged or empty (kept) name.
	tx, err := c.db.Beginx()
	if err != nil {
		return models.Template{}, errUpd(err)
	}
	defer tx.Rollback()
	if err := c.recordFormerNameTx(tx, models.NameKindTemplate, id, name); err != nil {
		return models.Template{}, errUpd(err)
	}
	res, err := tx.Stmtx(c.q.UpdateTemplate).Exec(id, name, subject, body, bodySource, brand, lang)
	if err != nil {
		return models.Template{}, errUpd(err)
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return models.Template{}, echo.NewHTTPError(http.StatusBadRequest,
			c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.template}"))
	}
	if err := tx.Commit(); err != nil {
		return models.Template{}, errUpd(err)
	}

	return c.GetTemplate(id, false)
}

// SetDefaultTemplate sets a template as default.
func (c *Core) SetDefaultTemplate(id int) error {
	if _, err := c.q.SetDefaultTemplate.Exec(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
	}

	return nil
}

// DeleteTemplate deletes a given template.
func (c *Core) DeleteTemplate(id int) error {
	var delID int
	if err := c.q.DeleteTemplate.Get(&delID, id); err != nil && err != sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "{globals.terms.template}", "error", pqErrMsg(err)))
	}
	if delID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("templates.cantDeleteDefault"))
	}

	return nil
}
