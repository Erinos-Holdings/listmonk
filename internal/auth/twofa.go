package auth

// Fork (two-factor, integrations PASSKEY-2FA-SPEC D2/D3/D6). Who must hold a second factor, the
// relying party derivation, the stored passkey row, and the session step-up stamp.

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/zerodha/simplesessions/v3"
	null "gopkg.in/volatiletech/null.v6"
)

const (
	// StepUpTTL is how long a step-up stamp on a session stays valid.
	StepUpTTL = 5 * time.Minute

	// StepUpMaxFails is the number of failed step-up attempts that destroys the session.
	StepUpMaxFails = 5

	sessKeyStepUpAt    = "stepup_at"
	sessKeyStepUpFails = "stepup_fails"

	// twofaTypeTOTP mirrors models.TwofaTypeTOTP (auth does not import models).
	twofaTypeTOTP = "totp"
)

// Passkey is one row of user_passkeys. Credential is the library's whole webauthn.Credential as
// JSON; it never leaves the server (json:"-").
type Passkey struct {
	ID           int       `db:"id" json:"id"`
	UserID       int       `db:"user_id" json:"-"`
	CredentialID []byte    `db:"credential_id" json:"-"`
	Credential   []byte    `db:"credential" json:"-"`
	Name         string    `db:"name" json:"name"`
	CreatedAt    null.Time `db:"created_at" json:"created_at"`
	LastUsedAt   null.Time `db:"last_used_at" json:"last_used_at"`
}

// IsReadPerm reports whether perm is a read permission: true only when the action after the
// colon is exactly get, get_all or get_analytics. Everything else -- every manage*, import,
// sql_query, send, maintain, review*, post_bounce, and any action not in that list -- is more
// than read, so a new upstream permission is enforced until someone classifies it.
func IsReadPerm(perm string) bool {
	_, action, ok := strings.Cut(perm, ":")
	if !ok {
		return false
	}
	switch action {
	case "get", "get_all", "get_analytics":
		return true
	}
	return false
}

// IsTwofaEnforced is the role-based answer to "must this user hold a second factor": a
// password-login user (never an API user) whose user role is Super Admin, or carries any
// permission that is not a read permission, or whose list role carries anything but list:get.
// The role id is read from UserRole.ID: setupUserFields copies UserRoleID there and zeroes it, so
// UserRoleID is always 0 on a loaded user. A login-user row has no role at all -- load the user
// with core.GetUser before asking.
func (u *User) IsTwofaEnforced() bool {
	if u.Type != UserTypeUser || !u.PasswordLogin {
		return false
	}
	if u.UserRole.ID == SuperAdminRoleID {
		return true
	}
	for _, p := range u.UserRole.Permissions {
		if !IsReadPerm(p) {
			return true
		}
	}
	if u.ListRole != nil {
		for _, l := range u.ListRole.Lists {
			for _, p := range l.Permissions {
				if p != PermListGet {
					return true
				}
			}
		}
	}
	return false
}

// HasTwofaFactor reports whether the user holds a second factor (TOTP on, or a passkey).
func (u *User) HasTwofaFactor() bool {
	return u.TwofaType == twofaTypeTOTP || u.PasskeyCount > 0
}

// RelyingPartyFromRootURL derives the WebAuthn relying party from app.root_url: the RP ID is the
// hostname and the single allowed origin is scheme + host + port. Passkeys are bound to that
// hostname, so changing the root URL's host invalidates every passkey.
func RelyingPartyFromRootURL(root string) (rpID, origin string, err error) {
	u, err := url.Parse(strings.TrimSpace(root))
	if err != nil {
		return "", "", err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", "", errors.New("app.root_url is not an absolute http(s) URL")
	}
	return strings.ToLower(u.Hostname()), u.Scheme + "://" + strings.ToLower(u.Host), nil
}

// RequireTwofa returns the running process's security.require_twofa.
func (o *Auth) RequireTwofa() bool {
	return o.cfg.RequireTwofa
}

// session returns the cookie session on the request, nil for a token (API) request.
func session(c echo.Context) *simplesessions.Session {
	sess, _ := c.Get(SessionKey).(*simplesessions.Session)
	return sess
}

// StampStepUp records a successful step-up on the request's session and clears its failure count.
func (o *Auth) StampStepUp(c echo.Context) error {
	sess := session(c)
	if sess == nil {
		return errors.New("no session")
	}
	return sess.SetMulti(map[string]any{sessKeyStepUpAt: time.Now().Unix(), sessKeyStepUpFails: 0})
}

// HasStepUp reports whether the request's session carries a step-up stamp younger than
// StepUpTTL. A request with no cookie session (an API token) never does.
func (o *Auth) HasStepUp(c echo.Context) bool {
	sess := session(c)
	if sess == nil {
		return false
	}
	at, err := o.sessStore.Int64(sess.Get(sessKeyStepUpAt))
	if err != nil || at <= 0 {
		return false
	}
	return time.Since(time.Unix(at, 0)) <= StepUpTTL
}

// StepUpFailed counts a failed step-up on the request's session and destroys the session on the
// StepUpMaxFails-th failure. It reports whether the session was destroyed.
func (o *Auth) StepUpFailed(c echo.Context) bool {
	sess := session(c)
	if sess == nil {
		return false
	}
	n, _ := o.sessStore.Int(sess.Get(sessKeyStepUpFails))
	n++
	if n >= StepUpMaxFails {
		if err := sess.Destroy(); err != nil {
			o.log.Printf("error destroying session after failed step-ups: %v", err)
		}
		return true
	}
	if err := sess.Set(sessKeyStepUpFails, n); err != nil {
		o.log.Printf("error counting a failed step-up: %v", err)
	}
	return false
}
