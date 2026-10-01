package main

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/utils"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
	"gopkg.in/volatiletech/null.v6"
)

var (
	reUsername = regexp.MustCompile(`^[a-zA-Z0-9_\-\.@]+$`)
)

// GetUser retrieves a single user by ID.
func (a *App) GetUser(c echo.Context) error {
	// Get the user from the DB.
	id := getID(c)
	out, err := a.core.GetUser(id, "", "")
	if err != nil {
		return err
	}

	// Blank out the password hash in the response.
	out.Password = null.String{}

	return c.JSON(http.StatusOK, okResp{out})
}

// GetUsers retrieves all users.
func (a *App) GetUsers(c echo.Context) error {
	// Get all users from the DB.
	out, err := a.core.GetUsers()
	if err != nil {
		return err
	}

	// Blank out the password hash in the response.
	for n := range out {
		out[n].Password = null.String{}
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// CreateUser handles user creation.
func (a *App) CreateUser(c echo.Context) error {
	var u auth.User
	if err := c.Bind(&u); err != nil {
		return err
	}

	u.Username = strings.TrimSpace(u.Username)
	u.Name = strings.TrimSpace(u.Name)
	email := strings.ToLower(strings.TrimSpace(u.Email.String))

	// Validate fields.
	if !strHasLen(u.Username, 3, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "username"))
	}
	if !reUsername.MatchString(u.Username) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "username"))
	}
	if u.Type != auth.UserTypeAPI {
		if !utils.ValidateEmail(email) {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "email"))
		}
		if u.PasswordLogin {
			// Fork (password policy, PASSWORD-POLICY-SPEC D3) -- the rule, not strHasLen(…, 8, …).
			if !validatePassword(u.Password.String) {
				return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.passwordPolicy"))
			}
		}

		u.Email = null.String{String: email, Valid: true}
	}

	if u.Name == "" {
		u.Name = u.Username
	}

	// Create the user in the DB.
	user, err := a.core.CreateUser(u)
	if err != nil {
		return err
	}

	// Blank out the password hash in the response.
	if user.Type != auth.UserTypeAPI {
		user.Password = null.String{}
	}

	// Cache the API token for in-memory, off-DB /api/* request auth.
	if _, err := cacheUsers(a.core, a.auth); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{user})
}

// UpdateUser handles user modification.
func (a *App) UpdateUser(c echo.Context) error {
	// Incoming params.
	var u auth.User
	if err := c.Bind(&u); err != nil {
		return err
	}

	u.Username = strings.TrimSpace(u.Username)
	u.Name = strings.TrimSpace(u.Name)
	email := strings.ToLower(strings.TrimSpace(u.Email.String))

	// Validate fields.
	if !strHasLen(u.Username, 3, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "username"))
	}
	if !reUsername.MatchString(u.Username) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "username"))
	}

	// Get the user ID.
	id := getID(c)

	// Fork (two-factor, integrations PASSKEY-2FA-SPEC D6) -- editing YOUR OWN account here must not
	// bypass the profile route's step-up: on a cookie session, a request for the caller's own id
	// that sets a password, changes the email, or changes password_login or type needs the stamp.
	// The last two are gated because flipping either first would switch this very gate off (and
	// password_login=false also drops the password). Edits of other users (user administration)
	// and API-token callers stay ungated (a stated non-goal).
	if caller, ok := c.Get(auth.UserHTTPCtxKey).(auth.User); ok && id == caller.ID && auth.GetSessionID(c) != "" {
		if u.Password.String != "" ||
			(u.Email.Valid && email != strings.ToLower(strings.TrimSpace(caller.Email.String))) ||
			u.PasswordLogin != caller.PasswordLogin ||
			(u.Type != "" && u.Type != caller.Type) {
			if err := a.requireStepUp(c); err != nil {
				return err
			}
		}
	}

	if u.Type != auth.UserTypeAPI {
		if !utils.ValidateEmail(email) {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "email"))
		}

		// Validate password if password login is enabled.
		if u.PasswordLogin {
			if u.Password.String != "" {
				// If a password is sent, validate it before updating in the DB. If it's not set, leave the password in the DB untouched.
				// Fork (password policy, PASSWORD-POLICY-SPEC D3) -- the rule, not strHasLen(…, 8, …).
				if !validatePassword(u.Password.String) {
					return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.passwordPolicy"))
				}
			} else {
				// Get the user from the DB.
				user, err := a.core.GetUser(id, "", "")
				if err != nil {
					return err
				}

				// If password login is enabled, but there's no password in the DB and there's no incoming
				// password, throw an error.
				if !user.HasPassword {
					return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "password"))
				}
			}
		}

		u.Email = null.String{String: email, Valid: true}
	} else if u.PasswordLogin && u.Password.String != "" {
		// Fork (password policy, PASSWORD-POLICY-SPEC D3) -- update-user hashes a sent password
		// whenever the STORED row is not api, so the stored type, not the request's, decides.
		stored, err := a.core.GetUser(id, "", "")
		if err != nil {
			return err
		}
		if stored.Type != auth.UserTypeAPI && !validatePassword(u.Password.String) {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.passwordPolicy"))
		}
	}

	// Default the name to username if not set.
	if u.Name == "" {
		u.Name = u.Username
	}

	// Update the user in the DB.
	user, err := a.core.UpdateUser(id, u)
	if err != nil {
		return err
	}

	// Blank out the password hash in the response.
	user.Password = null.String{}

	// If password was changed by admin, destroy all sessions for the given user.
	if u.Password.String != "" {
		if err := a.core.DeleteUserSessions(id, ""); err != nil {
			a.log.Printf("error destroying sessions on admin password change for user_id=%d: %v", id, err)
		}
	}

	// Cache the API token for in-memory, off-DB /api/* request auth.
	if _, err := cacheUsers(a.core, a.auth); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{user})
}

// DeleteUser handles the deletion of a single user by ID.
func (a *App) DeleteUser(c echo.Context) error {
	// Delete the user(s) from the DB.
	id := getID(c)
	if err := a.core.DeleteUsers([]int{id}); err != nil {
		return err
	}

	// Cache the API token for in-memory, off-DB /api/* request auth.
	if _, err := cacheUsers(a.core, a.auth); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// DeleteUsers handles user deletion, either a single one (ID in the URI), or a list.
func (a *App) DeleteUsers(c echo.Context) error {
	ids, err := getQueryInts("id", c.QueryParams())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidID"))
	}

	// Delete the user(s) from the DB.
	if err := a.core.DeleteUsers(ids); err != nil {
		return err
	}

	// Cache the API token for in-memory, off-DB /api/* request auth.
	if _, err := cacheUsers(a.core, a.auth); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// GetUserProfile fetches the uesr profile for the currently logged in user.
func (a *App) GetUserProfile(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Blank out the password hash in the response.
	user.Password.String = ""
	user.Password.Valid = false

	return c.JSON(http.StatusOK, okResp{user})
}

// UpdateUserProfile update's the current user's profile.
func (a *App) UpdateUserProfile(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Incoming params.
	u := auth.User{}
	if err := c.Bind(&u); err != nil {
		return err
	}
	u.PasswordLogin = user.PasswordLogin
	u.Name = strings.TrimSpace(u.Name)
	email := strings.TrimSpace(u.Email.String)

	// Validate fields.
	if user.PasswordLogin {
		if !utils.ValidateEmail(email) {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "email"))
		}
		u.Email = null.String{String: email, Valid: true}
	}

	// Fork (two-factor, integrations PASSKEY-2FA-SPEC D6) -- a session alone may change the name,
	// but replacing the password or the email (the reset mailbox) needs a step-up stamp. Only for
	// a password-login USER: setupUserFields marks an API user password_login too (its token
	// hash), and an API token's profile save stays as upstream has it.
	if user.Type == auth.UserTypeUser && user.PasswordLogin && (u.Password.String != "" || email != user.Email.String) {
		if err := a.requireStepUp(c); err != nil {
			return err
		}
	}

	if u.PasswordLogin && u.Password.String != "" {
		// Fork (password policy, PASSWORD-POLICY-SPEC D3) -- the rule, not strHasLen(…, 8, …).
		if !validatePassword(u.Password.String) {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.passwordPolicy"))
		}
	}

	// Update the user in the DB.
	out, err := a.core.UpdateUserProfile(user.ID, u)
	if err != nil {
		return err
	}

	// If password was changed, destroy all existing sessions for the user except for the current one.
	if u.Password.String != "" {
		if err := a.core.DeleteUserSessions(user.ID, auth.GetSessionID(c)); err != nil {
			a.log.Printf("error destroying sessions after profile password change for user_id=%d: %v", user.ID, err)
		}
	}

	// Blank out the password hash in the response.
	out.Password = null.String{}

	return c.JSON(http.StatusOK, okResp{out})
}

// EnableTOTP enables TOTP 2FA for a user after verifying the code.
func (a *App) EnableTOTP(c echo.Context) error {
	// Fork (two-factor, integrations PASSKEY-2FA-SPEC D6) -- a session alone never changes a factor.
	if err := a.requireStepUp(c); err != nil {
		return err
	}

	var (
		u      = c.Get(auth.UserHTTPCtxKey).(auth.User)
		secret = strings.TrimSpace(c.FormValue("secret"))
		code   = strings.TrimSpace(c.FormValue("code"))
	)

	if secret == "" || code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidFields"))
	}

	// If password login is disabled, can't enable TOTP.
	if !u.PasswordLogin {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("public.invalidFeature"))
	}

	// If TOTP is already enabled, don't allow re-enabling.
	if u.TwofaType == models.TwofaTypeTOTP {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.twoFAAlreadyEnabled"))
	}

	// Verify the TOTP code.
	valid := totp.Validate(code, secret)
	if !valid {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidTOTPCode"))
	}

	// Enable TOTP in the DB (fork: in the per-user factor transaction), then end the user's other
	// sessions.
	if err := a.core.EnableTOTPFactor(u.ID, secret, core.FactorRule{}); err != nil {
		return err
	}
	a.endOtherSessions(u.ID, auth.GetSessionID(c))

	return c.JSON(http.StatusOK, okResp{true})
}

// DisableTOTP disables TOTP 2FA for a user.
// Fork (two-factor, integrations PASSKEY-2FA-SPEC D6/D7/D9) -- upstream asked only for the
// password, so the password undid the factor meant to back it up. It now needs a step-up stamp,
// takes no password, and (switch ON, enforced user) refuses to remove the last factor.
func (a *App) DisableTOTP(c echo.Context) error {
	if err := a.requireStepUp(c); err != nil {
		return err
	}

	u := c.Get(auth.UserHTTPCtxKey).(auth.User)

	// TOTP isn't enabled.
	if u.TwofaType != models.TwofaTypeTOTP {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.twoFANotEnabled"))
	}

	// Disable TOTP in the DB.
	if err := a.core.DisableTOTPFactor(u.ID, a.keepLastRule(u)); err != nil {
		return err
	}
	a.endOtherSessions(u.ID, auth.GetSessionID(c))

	return c.JSON(http.StatusOK, okResp{true})
}

// cacheUsers fetches (API) users and caches them in the auth module.
// It also returns a bool indicating whether there are any actual users in the DB at all,
// which if there aren't, the first time user setup needs to be run.
func cacheUsers(co *core.Core, a *auth.Auth) (bool, error) {
	users, err := co.GetUsers()
	if err != nil {
		return false, err
	}

	hasUser := false
	apiUsers := make([]auth.User, 0, len(users))
	for _, u := range users {
		if u.Type == auth.UserTypeAPI && u.Status == auth.UserStatusEnabled {
			apiUsers = append(apiUsers, u)
		}

		if u.Type == auth.UserTypeUser {
			hasUser = true
		}
	}

	a.CacheAPIUsers(apiUsers)
	return hasUser, nil
}
