package core

// Fork (two-factor, integrations PASSKEY-2FA-SPEC D5 to D8). Every write that adds or removes a
// second factor runs in one transaction that locks the user row first (lock-user-factors), so a
// "has no factor" / "is not the last factor" / "under the passkey cap" check and its write can
// never interleave with another factor change for the same user.

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// MaxPasskeys is the most passkeys one user may hold.
const MaxPasskeys = 10

// FactorRule says which refusal applies to a factor write.
type FactorRule struct {
	// Enrol: the write must be the user's FIRST factor (an enrol token never adds a factor to an
	// account that already has one).
	Enrol bool

	// KeepLast: a removal that would leave the user with no factor is refused (switch ON and the
	// user is enforced, D7).
	KeepLast bool
}

type userFactors struct {
	TwofaType    string `db:"twofa_type"`
	PasskeyCount int    `db:"passkey_count"`
}

func (f userFactors) count() int {
	n := f.PasskeyCount
	if f.TwofaType == models.TwofaTypeTOTP {
		n++
	}
	return n
}

// factorTx locks the user row, reads the user's factors, and runs fn inside the transaction.
func (c *Core) factorTx(userID int, fn func(tx *sqlx.Tx, f userFactors) error) error {
	tx, err := c.db.Beginx()
	if err != nil {
		return c.twofaErr(err)
	}
	defer tx.Rollback()

	var id int
	if err := tx.Stmtx(c.q.LockUserFactors).Get(&id, userID); err != nil {
		if err == sql.ErrNoRows {
			return echo.NewHTTPError(http.StatusNotFound, c.i18n.Ts("globals.messages.notFound", "name", "{globals.terms.user}"))
		}
		return c.twofaErr(err)
	}
	var f userFactors
	if err := tx.Stmtx(c.q.GetUserFactors).Get(&f, userID); err != nil {
		return c.twofaErr(err)
	}

	if err := fn(tx, f); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return c.twofaErr(err)
	}
	return nil
}

func (c *Core) twofaErr(err error) error {
	return echo.NewHTTPError(http.StatusInternalServerError,
		c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.user}", "error", pqErrMsg(err)))
}

// GetPasskeys returns a user's passkeys, oldest first.
func (c *Core) GetPasskeys(userID int) ([]auth.Passkey, error) {
	out := []auth.Passkey{}
	if err := c.q.GetUserPasskeys.Select(&out, userID); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.user}", "error", pqErrMsg(err)))
	}
	return out, nil
}

// AddPasskey stores a registered credential (credJSON is the library's webauthn.Credential as
// JSON). It refuses the (MaxPasskeys+1)-th passkey, and under rule.Enrol any user who already
// holds a factor.
func (c *Core) AddPasskey(userID int, credID, credJSON []byte, name string, rule FactorRule) (int, error) {
	var id int
	err := c.factorTx(userID, func(tx *sqlx.Tx, f userFactors) error {
		if rule.Enrol && f.count() > 0 {
			return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("users.twoFAHasFactor"))
		}
		if f.PasskeyCount >= MaxPasskeys {
			return echo.NewHTTPError(http.StatusBadRequest, c.i18n.Ts("users.passkeyLimit", "num", strconv.Itoa(MaxPasskeys)))
		}
		if err := tx.Stmtx(c.q.InsertUserPasskey).Get(&id, userID, credID, string(credJSON), name); err != nil {
			return c.twofaErr(err)
		}
		return nil
	})
	return id, err
}

// UpdatePasskeyLogin stores the credential as a successful assertion left it (sign counter,
// flags) and stamps last_used_at.
func (c *Core) UpdatePasskeyLogin(id, userID int, credJSON []byte) error {
	if _, err := c.q.UpdateUserPasskeyLogin.Exec(id, userID, string(credJSON)); err != nil {
		return c.twofaErr(err)
	}
	return nil
}

// DeletePasskey deletes one of the user's OWN passkeys (the row is matched on id AND user_id).
func (c *Core) DeletePasskey(userID, passkeyID int, rule FactorRule) error {
	return c.factorTx(userID, func(tx *sqlx.Tx, f userFactors) error {
		if rule.KeepLast && f.count() <= 1 {
			return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("users.twoFALastFactor"))
		}
		res, err := tx.Stmtx(c.q.DeleteUserPasskey).Exec(passkeyID, userID)
		if err != nil {
			return c.twofaErr(err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return echo.NewHTTPError(http.StatusNotFound, c.i18n.Ts("globals.messages.notFound", "name", "passkey"))
		}
		return nil
	})
}

// EnableTOTPFactor turns TOTP on with the given secret. Under rule.Enrol it refuses a user who
// already holds any factor.
func (c *Core) EnableTOTPFactor(userID int, secret string, rule FactorRule) error {
	return c.factorTx(userID, func(tx *sqlx.Tx, f userFactors) error {
		if rule.Enrol && f.count() > 0 {
			return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("users.twoFAHasFactor"))
		}
		if f.TwofaType == models.TwofaTypeTOTP {
			return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("users.twoFAAlreadyEnabled"))
		}
		if _, err := tx.Stmtx(c.q.SetUserTwoFA).Exec(userID, models.TwofaTypeTOTP, secret); err != nil {
			return c.twofaErr(err)
		}
		return nil
	})
}

// DisableTOTPFactor turns TOTP off. Under rule.KeepLast it refuses when TOTP is the user's only
// factor.
func (c *Core) DisableTOTPFactor(userID int, rule FactorRule) error {
	return c.factorTx(userID, func(tx *sqlx.Tx, f userFactors) error {
		if f.TwofaType != models.TwofaTypeTOTP {
			return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("users.twoFANotEnabled"))
		}
		if rule.KeepLast && f.count() <= 1 {
			return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("users.twoFALastFactor"))
		}
		if _, err := tx.Stmtx(c.q.SetUserTwoFA).Exec(userID, models.TwofaTypeNone, ""); err != nil {
			return c.twofaErr(err)
		}
		return nil
	})
}

// ResetTwofa is the admin recovery (D8): TOTP off with a NULL key, every passkey deleted and every
// session of the user deleted, in one transaction.
func (c *Core) ResetTwofa(userID int) error {
	return c.factorTx(userID, func(tx *sqlx.Tx, f userFactors) error {
		if _, err := tx.Stmtx(c.q.SetUserTwoFA).Exec(userID, models.TwofaTypeNone, ""); err != nil {
			return c.twofaErr(err)
		}
		if _, err := tx.Stmtx(c.q.DeleteUserPasskeys).Exec(userID); err != nil {
			return c.twofaErr(err)
		}
		if _, err := tx.Stmtx(c.q.DeleteUserSessions).Exec(strconv.Itoa(userID), ""); err != nil {
			return c.twofaErr(err)
		}
		return nil
	})
}

// VerifyUserPassword checks a password for step-up. Read-only: unlike LoginUser it never touches
// loggedin_at. It returns false for a wrong password and for a user without password login.
func (c *Core) VerifyUserPassword(userID int, password string) (bool, error) {
	var id int
	if err := c.q.VerifyUserPassword.Get(&id, userID, password); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, c.twofaErr(err)
	}
	return id == userID, nil
}
