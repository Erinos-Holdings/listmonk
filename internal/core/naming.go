package core

// Fork (naming standard, integrations listmonk/NAMING-STANDARD-SPEC D10/D13 §4.2): the name-only
// rename and the former-name rule. One transaction per rename -- the former-name statement runs
// FIRST (it reads the old name from the record's own table), then the name write. The general
// update paths run the same former-name statement in their own transaction (UpdateCampaign,
// UpdateList, UpdateTemplate), so a form rename records the former name too.

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// GetFormerName returns the record's former_names name, and whether a row exists.
func (c *Core) GetFormerName(kind string, id int) (string, bool, error) {
	var name string
	if err := c.q.GetFormerName.Get(&name, kind, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "former name", "error", pqErrMsg(err)))
	}
	return name, true, nil
}

// RenameRecord writes a campaign's, list's or template's name ONLY (D13), with the former-name
// rule, in one transaction. The caller has applied every guard and the name rule. updated_at is
// not bumped; a list's campaign_lists.list_name follows. A missing record is a 404.
func (c *Core) RenameRecord(kind string, id int, name string) error {
	var stmt *sqlx.Stmt
	switch kind {
	case models.NameKindCampaign:
		stmt = c.q.RenameCampaign
	case models.NameKindList:
		stmt = c.q.RenameList
	case models.NameKindTemplate:
		stmt = c.q.RenameTemplate
	default:
		return echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("globals.messages.invalidFields", "name", "kind"))
	}

	tx, err := c.db.Beginx()
	if err != nil {
		return c.renameErr(err)
	}
	defer tx.Rollback()

	if _, err := tx.Stmtx(c.q.RecordFormerName).Exec(kind, id, name); err != nil {
		return c.renameErr(err)
	}
	var got int
	if err := tx.Stmtx(stmt).Get(&got, id, name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, c.i18n.Ts("globals.messages.notFound", "name", kind))
		}
		return c.renameErr(err)
	}
	if err := tx.Commit(); err != nil {
		return c.renameErr(err)
	}
	return nil
}

func (c *Core) renameErr(err error) error {
	c.log.Printf("error renaming: %v", err)
	return echo.NewHTTPError(http.StatusInternalServerError,
		c.i18n.Ts("globals.messages.errorUpdating", "name", "name", "error", pqErrMsg(err)))
}

// recordFormerNameTx runs the former-name rule inside a general update's transaction. An empty
// name (the list and template updates keep the stored name for "") records nothing.
func (c *Core) recordFormerNameTx(tx *sqlx.Tx, kind string, id int, name string) error {
	if name == "" {
		return nil
	}
	_, err := tx.Stmtx(c.q.RecordFormerName).Exec(kind, id, name)
	return err
}
