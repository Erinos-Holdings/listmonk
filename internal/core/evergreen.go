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
	return c.flagsStmt(stmt, SessionFlags{Backfill: backfill})
}

// SessionFlags are the transaction-local session settings a subscriber_lists write may carry.
// Backfill: listmonk.backfill (not new -- no confirmed_at, no welcome; see backfillStmt).
// ReleaseFinal (fork, sunset v2 -- integrations SUNSET-V2-SPEC D6): listmonk.release_final, the
// only way the hold-release trigger (v6.2.22) lets a FINAL hold (meta.hold.final_at) be released
// by a status change. Sent only by the person's own authenticated actions (the mirror's storefront
// join, the manage-preferences page, the sunset Lambda's click release); every other writer leaves
// a final hold in place.
type SessionFlags struct {
	Backfill     bool
	ReleaseFinal bool
}

// flagsStmt is backfillStmt for a set of session flags. With no flag set it is the plain prepared
// statement; with any flag set the statement is bound to a transaction that SET LOCALs each one
// (a SET LOCAL outside a transaction is a no-op, so a flagged write ALWAYS runs in one).
func (c *Core) flagsStmt(stmt *sqlx.Stmt, f SessionFlags) (stmtRunner, func(error) error, error) {
	if !f.Backfill && !f.ReleaseFinal {
		return stmt, func(err error) error { return err }, nil
	}

	tx, err := c.db.Beginx()
	if err != nil {
		return nil, nil, err
	}
	for _, set := range []struct {
		on  bool
		sql string
	}{
		{f.Backfill, "SET LOCAL listmonk.backfill = 'true'"},
		{f.ReleaseFinal, "SET LOCAL listmonk.release_final = 'true'"},
	} {
		if !set.on {
			continue
		}
		if _, err := tx.Exec(set.sql); err != nil {
			tx.Rollback()
			return nil, nil, err
		}
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
