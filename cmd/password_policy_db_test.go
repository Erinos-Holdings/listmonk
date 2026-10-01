package main

// Fork (password policy) -- integrations PASSWORD-POLICY-SPEC I2, I3, I5, I6, I7 against a real
// database, through the handlers. The five SET sites (CreateUser, UpdateUser, UpdateUserProfile,
// doFirstTimeSetup, doResetPassword) refuse a 16+ character password missing one class with the
// users.passwordPolicy text and accept a compliant one; the two VERIFY sites (doLogin,
// DisableTOTP) still take an 8-character password seeded by SQL at bcrypt cost 6; an API user is
// created with no rule on its token; every set query stores a cost-12 hash; doLogin answers no
// faster than loginMinDuration for a wrong password and for an unknown username.
// Shares newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go), plus a real
// auth.Auth (the success paths call SaveSession) and a capturing renderer (stubRenderer discards
// template data, which would hide the reset page's error line).

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/tmptokens"
	"github.com/labstack/echo/v4"
)

const (
	// 19 characters, all four classes.
	ppGoodPassword = "Compliant-Pass-2026"
	// 18 characters, no upper-case letter: a length check alone would accept it.
	ppBadPassword = "noupper-pass-2026x"
)

// capturingRenderer records the last template name and data, so a re-rendered form's error
// line is visible to the test.
type capturingRenderer struct {
	name string
	data any
}

func (r *capturingRenderer) Render(w io.Writer, name string, data any, c echo.Context) error {
	r.name, r.data = name, data
	_, err := w.Write([]byte("rendered:" + name))
	return err
}

type ppHarness struct {
	*linkHarness
	render *capturingRenderer
	policy string
}

func newPasswordHarness(t *testing.T) *ppHarness {
	h := newLinkHarness(t)

	// initAuth's callbacks, against the test database.
	cb := &auth.Callbacks{
		GetCookie: func(name string, r any) (*http.Cookie, error) {
			return r.(echo.Context).Cookie(name)
		},
		SetCookie: func(cookie *http.Cookie, w any) error {
			cookie.SameSite = http.SameSiteLaxMode
			w.(echo.Context).SetCookie(cookie)
			return nil
		},
		GetUser: func(id int) (auth.User, error) {
			return h.app.core.GetUser(id, "", "")
		},
	}
	a, err := auth.New(auth.Config{}, h.db.DB, cb, log.New(os.Stderr, "", 0))
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	h.app.auth = a

	// The Super Admin role (id 1) every fixture user holds.
	var roleID int
	if err := h.db.Get(&roleID, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Super Admin', '{}') RETURNING id`); err != nil || roleID != 1 {
		t.Fatalf("seed role: id %d, %v", roleID, err)
	}

	return &ppHarness{linkHarness: h, render: &capturingRenderer{}, policy: h.app.i18n.T("users.passwordPolicy")}
}

// call runs handler on a request with body of the given content type, after setup, and returns
// the status, the message (an HTTPError's message, else the body) and the response headers.
func (h *ppHarness) call(handler echo.HandlerFunc, method, ctype, body string, setup func(echo.Context)) (int, string, http.Header) {
	h.t.Helper()
	e := echo.New()
	e.Renderer = h.render
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, ctype)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if setup != nil {
		setup(c)
	}
	if err := handler(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message), rec.Header()
		}
		h.t.Fatalf("handler: %v", err)
	}
	return rec.Code, rec.Body.String(), rec.Header()
}

func (h *ppHarness) json(handler echo.HandlerFunc, method, body string, setup func(echo.Context)) (int, string) {
	code, msg, _ := h.call(handler, method, echo.MIMEApplicationJSON, body, setup)
	return code, msg
}

func (h *ppHarness) form(handler echo.HandlerFunc, v url.Values, setup func(echo.Context)) (int, string, http.Header) {
	return h.call(handler, http.MethodPost, echo.MIMEApplicationForm, v.Encode(), setup)
}

func (h *ppHarness) hash(username string) string {
	var pw string
	if err := h.db.Get(&pw, `SELECT COALESCE(password, '') FROM users WHERE username = $1`, username); err != nil {
		h.t.Fatalf("hash of %s: %v", username, err)
	}
	return pw
}

func (h *ppHarness) userID(username string) int {
	var id int
	if err := h.db.Get(&id, `SELECT id FROM users WHERE username = $1`, username); err != nil {
		h.t.Fatalf("id of %s: %v", username, err)
	}
	return id
}

func (h *ppHarness) sessions() int {
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM sessions`)
	return n
}

func (h *ppHarness) userJSON(username, email, password string) string {
	return fmt.Sprintf(`{"username": %q, "name": %q, "email": %q, "type": "user", "user_role_id": 1, "status": "enabled", "password_login": true, "password": %q}`,
		username, username, email, password)
}

var reCost12 = regexp.MustCompile(`^\$2a\$12\$`)

func TestPasswordPolicySetPaths(t *testing.T) {
	h := newPasswordHarness(t)
	app := h.app

	// I2 + I6 -- CreateUser.
	if code, msg := h.json(app.CreateUser, http.MethodPost, h.userJSON("createme", "create@example.test", ppBadPassword), nil); code != http.StatusBadRequest || msg != h.policy {
		t.Fatalf("CreateUser, no upper case: want 400 %q, got %d %q", h.policy, code, msg)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM users WHERE username = 'createme'`)
	if n != 0 {
		t.Fatal("CreateUser: a refused password created the user")
	}
	if code, msg := h.json(app.CreateUser, http.MethodPost, h.userJSON("createme", "create@example.test", ppGoodPassword), nil); code != http.StatusOK {
		t.Fatalf("CreateUser, compliant: want 200, got %d %s", code, msg)
	}
	if pw := h.hash("createme"); !reCost12.MatchString(pw) {
		t.Fatalf("create-user stored %q, want a cost-12 bcrypt hash", pw)
	}

	// I2 + I6 -- UpdateUser.
	editID := h.userID("createme")
	setID := func(c echo.Context) { c.Set("id", editID) }
	const edited = "Edited-Password-77"
	before := h.hash("createme")
	if code, msg := h.json(app.UpdateUser, http.MethodPut, h.userJSON("createme", "create@example.test", ppBadPassword), setID); code != http.StatusBadRequest || msg != h.policy {
		t.Fatalf("UpdateUser, no upper case: want 400 %q, got %d %q", h.policy, code, msg)
	}
	if h.hash("createme") != before {
		t.Fatal("UpdateUser: a refused password changed the hash")
	}
	if code, msg := h.json(app.UpdateUser, http.MethodPut, h.userJSON("createme", "create@example.test", edited), setID); code != http.StatusOK {
		t.Fatalf("UpdateUser, compliant: want 200, got %d %s", code, msg)
	}
	if pw := h.hash("createme"); !reCost12.MatchString(pw) || pw == before {
		t.Fatalf("update-user stored %q (before %q), want a new cost-12 bcrypt hash", pw, before)
	}
	if _, err := app.core.LoginUser("createme", edited); err != nil {
		t.Fatalf("UpdateUser: the new password does not verify: %v", err)
	}

	// I2 + I6 -- UpdateUserProfile, as the user themself.
	asSelf := func(c echo.Context) {
		u, err := app.core.GetUser(editID, "", "")
		if err != nil {
			t.Fatal(err)
		}
		c.Set(auth.UserHTTPCtxKey, u)
	}
	const profiled = "Profile Pass 2026"
	before = h.hash("createme")
	profile := func(pw string) string {
		return fmt.Sprintf(`{"name": "createme", "email": "create@example.test", "password": %q}`, pw)
	}
	if code, msg := h.json(app.UpdateUserProfile, http.MethodPut, profile(ppBadPassword), asSelf); code != http.StatusBadRequest || msg != h.policy {
		t.Fatalf("UpdateUserProfile, no upper case: want 400 %q, got %d %q", h.policy, code, msg)
	}
	if h.hash("createme") != before {
		t.Fatal("UpdateUserProfile: a refused password changed the hash")
	}
	if code, msg := h.json(app.UpdateUserProfile, http.MethodPut, profile(profiled), asSelf); code != http.StatusOK {
		t.Fatalf("UpdateUserProfile, compliant: want 200, got %d %s", code, msg)
	}
	if pw := h.hash("createme"); !reCost12.MatchString(pw) || pw == before {
		t.Fatalf("update-user-profile stored %q (before %q), want a new cost-12 bcrypt hash", pw, before)
	}
	if _, err := app.core.LoginUser("createme", profiled); err != nil {
		t.Fatalf("UpdateUserProfile: the new password does not verify: %v", err)
	}

	// I2 + I6 -- doFirstTimeSetup (create-user again, through the setup form).
	setup := func(pw string) url.Values {
		return url.Values{"email": {"setup@example.test"}, "username": {"setupadmin"}, "password": {pw}, "password2": {pw}}
	}
	if code, msg, _ := h.form(app.doFirstTimeSetup, setup(ppBadPassword), nil); code != http.StatusBadRequest || msg != h.policy {
		t.Fatalf("doFirstTimeSetup, no upper case: want 400 %q, got %d %q", h.policy, code, msg)
	}
	h.db.Get(&n, `SELECT COUNT(*) FROM users WHERE username = 'setupadmin'`)
	if n != 0 {
		t.Fatal("doFirstTimeSetup: a refused password created the user")
	}
	sess := h.sessions()
	if code, msg, _ := h.form(app.doFirstTimeSetup, setup(ppGoodPassword), nil); code != http.StatusOK {
		t.Fatalf("doFirstTimeSetup, compliant: want 200, got %d %s", code, msg)
	}
	if pw := h.hash("setupadmin"); !reCost12.MatchString(pw) {
		t.Fatalf("doFirstTimeSetup stored %q, want a cost-12 bcrypt hash", pw)
	}
	if h.sessions() != sess+1 {
		t.Fatal("doFirstTimeSetup, compliant: no session saved")
	}

	// I2 + I6 -- doResetPassword, with the token doForgotPassword would have issued.
	const (
		resetEmail = "create@example.test"
		resetToken = "reset-token-for-the-password-policy-test"
		resetPw    = "Reset-Password-99"
	)
	tmptokens.Set(resetEmail, passwordResetTTL, resetToken)
	reset := func(c echo.Context) error { return app.doResetPassword(c, resetToken, resetEmail) }
	pwForm := func(pw string) url.Values { return url.Values{"password": {pw}, "password2": {pw}} }

	before = h.hash("createme")
	h.render.name, h.render.data = "", nil
	code, _, _ := h.form(reset, pwForm(ppBadPassword), nil)
	if code != http.StatusOK || h.render.name != "admin-reset-password" {
		t.Fatalf("doResetPassword, no upper case: want the reset page re-rendered, got %d %q", code, h.render.name)
	}
	if tpl, ok := h.render.data.(resetPasswordTpl); !ok || tpl.Error != h.policy {
		t.Fatalf("doResetPassword, no upper case: want error line %q, got %#v", h.policy, h.render.data)
	}
	if h.hash("createme") != before {
		t.Fatal("doResetPassword: a refused password changed the hash")
	}
	if _, err := tmptokens.Check(resetEmail); err != nil {
		t.Fatalf("doResetPassword: a refused password consumed the reset token: %v", err)
	}
	sess = h.sessions()
	code, _, hdr := h.form(reset, pwForm(resetPw), nil)
	if code != http.StatusFound || hdr.Get("Location") != uriAdmin {
		t.Fatalf("doResetPassword, compliant: want 302 to %s, got %d %q", uriAdmin, code, hdr.Get("Location"))
	}
	if pw := h.hash("createme"); !reCost12.MatchString(pw) || pw == before {
		t.Fatalf("doResetPassword stored %q (before %q), want a new cost-12 bcrypt hash", pw, before)
	}
	if _, err := app.core.LoginUser("createme", resetPw); err != nil {
		t.Fatalf("doResetPassword: the new password does not verify: %v", err)
	}
	if h.sessions() != sess+1 {
		t.Fatal("doResetPassword, compliant: no session saved")
	}

	// I5 -- an API user is created with no rule on its token (a short "password" in the body
	// is not checked, and the stored value is the token's SHA-256, not a bcrypt hash).
	api := `{"username": "apibot", "name": "apibot", "type": "api", "user_role_id": 1, "status": "enabled", "password_login": true, "password": "x"}`
	if code, msg := h.json(app.CreateUser, http.MethodPost, api, nil); code != http.StatusOK {
		t.Fatalf("CreateUser type=api: want 200, got %d %s", code, msg)
	}
	if pw := h.hash("apibot"); !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(pw) {
		t.Fatalf("CreateUser type=api stored %q, want a SHA-256 token hash", pw)
	}

	// I3 + I6 -- a user whose stored password is 8 characters at bcrypt cost 6, seeded by SQL.
	const legacyPw = "short8pw"
	h.db.MustExec(`INSERT INTO users (username, password_login, password, email, name, type, user_role_id, status)
		VALUES ('legacy', true, CRYPT($1, GEN_SALT('bf')), 'legacy@example.test', 'Legacy', 'user', 1, 'enabled')`, legacyPw)
	if pw := h.hash("legacy"); !strings.HasPrefix(pw, "$2a$06$") {
		t.Fatalf("legacy fixture: %q is not a cost-6 hash", pw)
	}
	login := func(username, password string) (int, string, time.Duration) {
		start := time.Now()
		code, msg, _ := h.form(app.doLogin, url.Values{"username": {username}, "password": {password}}, nil)
		return code, msg, time.Since(start)
	}
	sess = h.sessions()
	if code, msg, _ := login("legacy", legacyPw); code != http.StatusOK {
		t.Fatalf("doLogin with the 8-character cost-6 password: want 200, got %d %s", code, msg)
	}
	if h.sessions() != sess+1 {
		t.Fatal("doLogin with the 8-character password: no session saved")
	}

	legacyID := h.userID("legacy")
	h.db.MustExec(`UPDATE users SET twofa_type = 'totp', twofa_key = 'JBSWY3DPEHPK3PXP' WHERE id = $1`, legacyID)
	asLegacy := func(c echo.Context) {
		u, err := app.core.GetUser(legacyID, "", "")
		if err != nil {
			t.Fatal(err)
		}
		c.Set(auth.UserHTTPCtxKey, u)
	}
	if code, msg, _ := h.form(app.DisableTOTP, url.Values{"password": {legacyPw}}, asLegacy); code != http.StatusOK {
		t.Fatalf("DisableTOTP with the 8-character password: want 200, got %d %s", code, msg)
	}
	var twofa string
	h.db.Get(&twofa, `SELECT twofa_type FROM users WHERE id = $1`, legacyID)
	if twofa != "none" {
		t.Fatalf("DisableTOTP: twofa_type is %q, want none", twofa)
	}

	// I7 -- the login floor, for a wrong password on a real username and for an unknown one.
	if code, _, took := login("legacy", "wrong-password"); code != http.StatusForbidden || took < loginMinDuration {
		t.Fatalf("doLogin, wrong password on a real username: want 403 in >= %s, got %d in %s", loginMinDuration, code, took)
	}
	if code, _, took := login("no-such-user", "wrong-password"); code != http.StatusForbidden || took < loginMinDuration {
		t.Fatalf("doLogin, unknown username: want 403 in >= %s, got %d in %s", loginMinDuration, code, took)
	}
}
