package core

// Fork (erinos evergreen campaigns) -- see queries/evergreen.sql and
// internal/manager/evergreen.go.

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// evergreenCollision reports whether another running evergreen on any of cm's lists
// with the same delay (and not the same non-null variant group) exists.
func (c *Core) evergreenCollision(cm models.Campaign) (string, bool, error) {
	// Campaign.Lists is the JSON aggregate from campaign_lists.
	var lists []struct {
		ID int `json:"id"`
	}
	if len(cm.Lists) > 0 {
		if err := json.Unmarshal(cm.Lists, &lists); err != nil {
			c.log.Printf("error reading campaign lists: %v", err)
			return "", false, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	listIDs := make([]int, 0, len(lists))
	for _, l := range lists {
		listIDs = append(listIDs, l.ID)
	}

	var (
		vg  = sql.NullString{String: cm.VariantGroupID.String, Valid: cm.VariantGroupID.Valid}
		row struct {
			ID   int    `db:"id"`
			Name string `db:"name"`
		}
	)
	// Fork (multi-language campaigns) -- NULL when the campaign targets everyone.
	lang := sql.NullString{String: cm.Lang(), Valid: cm.Lang() != ""}
	err := c.q.GetEvergreenCollision.Get(&row, cm.ID, cm.SendDelaySecs, pq.Array(listIDs), vg, lang)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		c.log.Printf("error checking evergreen collision: %v", err)
		return "", false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
	}
	return row.Name, true, nil
}

// stmtRunner is the subset of *sqlx.Stmt the backfill-aware writers use.
type stmtRunner interface {
	Get(dest interface{}, args ...interface{}) error
	Exec(args ...interface{}) (sql.Result, error)
}

// backfillStmt returns the statement to run and a finalizer to call with the write's
// error. With backfill=false it is the plain prepared statement and the finalizer is
// the identity. With backfill=true the statement is bound to a transaction that has
// SET LOCAL listmonk.backfill = 'true', so the subscriber_lists trigger leaves
// confirmed_at NULL on every row the write creates or flips -- these people are not
// new (bulk import, archive replay) and no evergreen campaign may welcome them.
func (c *Core) backfillStmt(stmt *sqlx.Stmt, backfill bool) (stmtRunner, func(error) error, error) {
	if !backfill {
		return stmt, func(err error) error { return err }, nil
	}

	tx, err := c.db.Beginx()
	if err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec("SET LOCAL listmonk.backfill = 'true'"); err != nil {
		tx.Rollback()
		return nil, nil, err
	}

	return tx.Stmtx(stmt), func(err error) error {
		if err != nil {
			tx.Rollback()
			return err
		}
		return tx.Commit()
	}, nil
}

// EvergreenLockedChange reports whether an update would change a started campaign's
// evergreen flag or its target lists. Both are frozen once started_at is set: a paused
// evergreen flipped to regular is claimed by next-campaigns on resume and mails the
// whole list; a swapped list makes its entire post-watermark membership "new".
func EvergreenLockedChange(cm models.Campaign, evergreen bool, listIDs []int) bool {
	if !cm.StartedAt.Valid {
		return false
	}
	if cm.Evergreen != evergreen {
		return true
	}
	if !cm.Evergreen {
		return false
	}

	var lists []struct {
		ID int `json:"id"`
	}
	if len(cm.Lists) > 0 {
		if err := json.Unmarshal(cm.Lists, &lists); err != nil {
			return true // unreadable = do not risk it
		}
	}
	have := map[int]bool{}
	for _, l := range lists {
		have[l.ID] = true
	}
	if len(have) != len(listIDs) {
		return true
	}
	for _, id := range listIDs {
		if !have[id] {
			return true
		}
	}
	return false
}
