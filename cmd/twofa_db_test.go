package main

// Fork (two-factor) -- integrations PASSKEY-2FA-SPEC I3 to I5, I7 to I13 and I22 to I26 against a
// real database, through the real router (initHTTPHandlers), so the route wiring, the session
// middleware and the permission checks are the production ones. Shares newPasswordHarness (a real
// auth.Auth and a capturing renderer; LISTMONK_TEST_PG opt-in, see link_redirect_db_test.go).
// Passkey ceremonies use a software authenticator (github.com/descope/virtualwebauthn); the
// passkey-specific invariants are in passkey_db_test.go.

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/tmptokens"
	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
)

const (
	twofaRoot    = "https://lm.example.test"
	twofaRPID    = "lm.example.test"
	twofaPw      = "twofa-test-pw"
	twofaSecret  = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	twofaNextURI = "/admin/campaigns"
)

type twofaHarness struct {
	*ppHarness
	e          *echo.Echo
	rp         virtualwebauthn.RelyingParty
	readerRole int
	writerRole int
}

func newTwofaHarness(t *testing.T) *twofaHarness {
	h := newPasswordHarness(t)
	totpGuesses.reset()
	t.Cleanup(totpGuesses.reset)

	h.app.urlCfg = &UrlConfig{RootURL: twofaRoot, LoginURL: uriLogin}
	h.app.cfg.SiteName = "listmonk test"
	h.app.webAuthn, h.app.webAuthnErr = initWebAuthn(twofaRoot, h.app.cfg.SiteName)
	if h.app.webAuthn == nil {
		t.Fatalf("initWebAuthn: %v", h.app.webAuthnErr)
	}
	h.app.chReload = make(chan os.Signal, 16)

	th := &twofaHarness{
		ppHarness: h,
		rp:        virtualwebauthn.RelyingParty{ID: twofaRPID, Name: "listmonk test", Origin: twofaRoot},
	}
	h.db.Get(&th.readerRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Reader', '{campaigns:get,campaigns:get_analytics,settings:get}') RETURNING id`)
	h.db.Get(&th.writerRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Writer', '{campaigns:get,campaigns:manage}') RETURNING id`)
	th.setSwitch(false)
	return th
}

// setSwitch rebuilds core and auth as a process started with security.require_twofa = on, and the
// router on top of them (initHTTPHandlers binds the auth middleware at registration).
func (h *twofaHarness) setSwitch(on bool) {
	h.t.Helper()
	h.app.core = core.New(&core.Opt{Constants: core.Constants{RequireTwofa: on}, Queries: h.q, DB: h.db, I18n: h.app.i18n, Log: h.app.log}, &core.Hooks{})
	cb := &auth.Callbacks{
		GetCookie: func(name string, r any) (*http.Cookie, error) { return r.(echo.Context).Cookie(name) },
		SetCookie: func(cookie *http.Cookie, w any) error {
			cookie.SameSite = http.SameSiteLaxMode
			w.(echo.Context).SetCookie(cookie)
			return nil
		},
		GetUser: func(id int) (auth.User, error) { return h.app.core.GetUser(id, "", "") },
	}
	a, err := auth.New(auth.Config{RequireTwofa: on}, h.db.DB, cb, log.New(io.Discard, "", 0))
	if err != nil {
		h.t.Fatalf("auth.New: %v", err)
	}
	h.app.auth = a
	if _, err := cacheUsers(h.app.core, h.app.auth); err != nil {
		h.t.Fatalf("cacheUsers: %v", err)
	}
	h.e = echo.New()
	initHTTPHandlers(h.e, h.app)
	h.e.Renderer = h.render
}

// user inserts an enabled password user (cost-6 hash of twofaPw) with the given user role.
func (h *twofaHarness) user(username string, roleID int) int {
	h.t.Helper()
	var id int
	if err := h.db.Get(&id, `INSERT INTO users (username, password_login, password, email, name, type, user_role_id, status)
		VALUES ($1, true, CRYPT($2, GEN_SALT('bf')), $1 || '@example.test', $1, 'user', $3, 'enabled') RETURNING id`, username, twofaPw, roleID); err != nil {
		h.t.Fatalf("user %s: %v", username, err)
	}
	return id
}

func (h *twofaHarness) setTOTP(id int) {
	h.db.MustExec(`UPDATE users SET twofa_type = 'totp', twofa_key = $2 WHERE id = $1`, id, twofaSecret)
}

func (h *twofaHarness) sessionsOf(id int) int {
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM sessions WHERE data->>'user_id' = $1`, strconv.Itoa(id))
	return n
}

func (h *twofaHarness) passkeysOf(id int) int {
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM user_passkeys WHERE user_id = $1`, id)
	return n
}

func (h *twofaHarness) twofaType(id int) (string, bool) {
	var (
		typ string
		key *string
	)
	row := h.db.QueryRow(`SELECT twofa_type, twofa_key FROM users WHERE id = $1`, id)
	if err := row.Scan(&typ, &key); err != nil {
		h.t.Fatal(err)
	}
	return typ, key != nil
}

// twofaClient is a cookie-carrying client of the harness router.
type twofaClient struct {
	h   *twofaHarness
	jar map[string]string
	hdr map[string]string
}

func (h *twofaHarness) client() *twofaClient {
	return &twofaClient{h: h, jar: map[string]string{}, hdr: map[string]string{}}
}

// sessionClient is a client holding a fresh cookie session for the user (no login round trip).
func (h *twofaHarness) sessionClient(id int) *twofaClient {
	h.t.Helper()
	u, err := h.app.core.GetUser(id, "", "")
	if err != nil {
		h.t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	if err := h.app.auth.SaveSession(u, "", c); err != nil {
		h.t.Fatal(err)
	}
	cl := h.client()
	cl.keep(rec)
	if cl.jar["session"] == "" {
		h.t.Fatal("no session cookie")
	}
	return cl
}

func (cl *twofaClient) keep(rec *httptest.ResponseRecorder) {
	for _, ck := range rec.Result().Cookies() {
		if ck.MaxAge < 0 || ck.Value == "" {
			delete(cl.jar, ck.Name)
		} else {
			cl.jar[ck.Name] = ck.Value
		}
	}
}

func (cl *twofaClient) do(method, target, ctype, body string) *httptest.ResponseRecorder {
	rec := cl.fire(method, target, ctype, body)
	cl.keep(rec)
	return rec
}

// fire sends one request with the client's cookies and leaves the jar alone, so one client can
// fire from many goroutines at once.
func (cl *twofaClient) fire(method, target, ctype, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if ctype != "" {
		req.Header.Set(echo.HeaderContentType, ctype)
	}
	for k, v := range cl.jar {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	for k, v := range cl.hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	cl.h.e.ServeHTTP(rec, req)
	return rec
}

// burst fires n copies of one request concurrently from the client and returns the response bodies.
func (cl *twofaClient) burst(n int, method, target, ctype, body string) []string {
	out := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out[i] = cl.fire(method, target, ctype, body).Body.String()
		}(i)
	}
	wg.Wait()
	return out
}

// syncBuffer is a bytes.Buffer safe for the concurrent writes of a burst's log lines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func countContaining(bodies []string, sub string) int {
	n := 0
	for _, b := range bodies {
		if strings.Contains(b, sub) {
			n++
		}
	}
	return n
}

func (cl *twofaClient) form(method, target string, v url.Values) *httptest.ResponseRecorder {
	return cl.do(method, target, echo.MIMEApplicationForm, v.Encode())
}

func (cl *twofaClient) json(method, target string, body any) *httptest.ResponseRecorder {
	b := []byte("{}")
	if body != nil {
		var err error
		if b, err = json.Marshal(body); err != nil {
			cl.h.t.Fatal(err)
		}
	}
	return cl.do(method, target, echo.MIMEApplicationJSON, string(b))
}

// login posts the login form with twofaPw.
func (cl *twofaClient) login(username string) *httptest.ResponseRecorder {
	return cl.form(http.MethodPost, uriLogin, url.Values{"username": {username}, "password": {twofaPw}, "next": {twofaNextURI}})
}

func (cl *twofaClient) stepUpPassword() *httptest.ResponseRecorder {
	return cl.form(http.MethodPost, "/api/profile/twofa/stepup", url.Values{"password": {twofaPw}})
}

func (cl *twofaClient) stepUpTOTP() *httptest.ResponseRecorder {
	code, _ := totp.GenerateCode(twofaSecret, time.Now())
	return cl.form(http.MethodPost, "/api/profile/twofa/stepup", url.Values{"totp_code": {code}})
}

// mint issues a challenge or enrol token for the user as doLogin would, and gives this client the
// matching nonce cookie.
func (cl *twofaClient) mint(userID int, purpose string, ttl time.Duration) string {
	token, _ := generateRandomString(tmpAuthTokenLen)
	nonce, _ := generateRandomString(32)
	tmptokens.Set(token, ttl, twofaToken{UserID: userID, Purpose: purpose, Nonce: nonce})
	cl.jar[twofaNonceCookie] = nonce
	return token
}

func location(rec *httptest.ResponseRecorder) string {
	return rec.Result().Header.Get("Location")
}

func tokenOf(t *testing.T, loc string) string {
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("token")
}

func dataOf(t *testing.T, rec *httptest.ResponseRecorder) json.RawMessage {
	t.Helper()
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Data) == 0 {
		t.Fatalf("no data in %d %s", rec.Code, rec.Body.String())
	}
	return out.Data
}

// vkey is one software passkey.
type vkey struct {
	auth virtualwebauthn.Authenticator
	cred virtualwebauthn.Credential
}

func newVKey(userID int) *vkey {
	a := virtualwebauthn.NewAuthenticatorWithOptions(virtualwebauthn.AuthenticatorOptions{UserHandle: []byte(strconv.Itoa(userID))})
	c := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	a.AddCredential(c)
	return &vkey{auth: a, cred: c}
}

func (k *vkey) attest(t *testing.T, rp virtualwebauthn.RelyingParty, opts json.RawMessage) json.RawMessage {
	t.Helper()
	ao, err := virtualwebauthn.ParseAttestationOptions(string(opts))
	if err != nil {
		t.Fatalf("attestation options %s: %v", opts, err)
	}
	return json.RawMessage(virtualwebauthn.CreateAttestationResponse(rp, k.auth, k.cred, *ao))
}

func (k *vkey) assert(t *testing.T, rp virtualwebauthn.RelyingParty, opts json.RawMessage) json.RawMessage {
	t.Helper()
	ao, err := virtualwebauthn.ParseAssertionOptions(string(opts))
	if err != nil {
		t.Fatalf("assertion options %s: %v", opts, err)
	}
	k.cred.Counter++
	return json.RawMessage(virtualwebauthn.CreateAssertionResponse(rp, k.auth, k.cred, *ao))
}

// addPasskey registers k from the profile (the caller holds a stamp) and returns the finish
// response.
func (cl *twofaClient) addPasskey(t *testing.T, k *vkey, name string) *httptest.ResponseRecorder {
	t.Helper()
	rec := cl.json(http.MethodPost, "/api/profile/passkeys/begin", nil)
	if rec.Code != http.StatusOK {
		return rec
	}
	return cl.json(http.MethodPost, "/api/profile/passkeys/finish", map[string]any{"name": name, "credential": k.attest(t, cl.h.rp, dataOf(t, rec))})
}

// passkeyUser creates a user with role roleID holding one passkey registered through the real
// in-session route (so the stored credential went through the database), and returns the key.
func (h *twofaHarness) passkeyUser(username string, roleID int) (int, *vkey) {
	h.t.Helper()
	id := h.user(username, roleID)
	cl := h.sessionClient(id)
	if rec := cl.stepUpPassword(); rec.Code != http.StatusOK {
		h.t.Fatalf("step-up for %s: %d %s", username, rec.Code, rec.Body.String())
	}
	k := newVKey(id)
	if rec := cl.addPasskey(h.t, k, "laptop"); rec.Code != http.StatusOK {
		h.t.Fatalf("add passkey for %s: %d %s", username, rec.Code, rec.Body.String())
	}
	h.db.MustExec(`DELETE FROM sessions WHERE data->>'user_id' = $1`, strconv.Itoa(id))
	return id, k
}

// I3 -- switch ON: a Super Admin with no factor, loaded through the real login-user query (role 1
// stored with an EMPTY permission list, the D3 trap), gets no session from doLogin, only a
// redirect to the enrolment page with a browser-bound token.
func TestLoginEnforcedNoFactorRedirectsToEnroll(t *testing.T) {
	h := newTwofaHarness(t)
	h.setSwitch(true)
	id := h.user("boss", auth.SuperAdminRoleID)

	var perms string
	h.db.Get(&perms, `SELECT array_to_string(permissions, ',') FROM roles WHERE id = 1`)
	if perms != "" {
		t.Fatalf("fixture: role 1 permissions %q, want empty", perms)
	}

	cl := h.client()
	rec := cl.login("boss")
	loc := location(rec)
	if rec.Code != http.StatusFound || !strings.HasPrefix(loc, "/admin/login/enroll?") {
		t.Fatalf("login: want 302 to /admin/login/enroll, got %d %q", rec.Code, loc)
	}
	if h.sessionsOf(id) != 0 || cl.jar["session"] != "" {
		t.Fatalf("login gave a session: rows %d, cookie %q", h.sessionsOf(id), cl.jar["session"])
	}
	tk := tokenOf(t, loc)
	if len(tk) != tmpAuthTokenLen {
		t.Fatalf("enrol token %q is not %d characters", tk, tmpAuthTokenLen)
	}
	data, err := tmptokens.Check(tk)
	if p, ok := data.(twofaToken); err != nil || !ok || p.Purpose != twofaPurposeEnroll || p.UserID != id || p.Nonce == "" || p.Nonce != cl.jar[twofaNonceCookie] {
		t.Fatalf("enrol token payload %#v (%v), nonce cookie %q", data, err, cl.jar[twofaNonceCookie])
	}
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == twofaNonceCookie && (ck.Path != uriLogin || !ck.HttpOnly || !ck.Secure || ck.SameSite != http.SameSiteLaxMode) {
			t.Fatalf("nonce cookie attributes %+v", ck)
		}
	}
}

// I4 -- switch OFF or absent: the same Super Admin gets a session (the release is dark).
func TestLoginSwitchOffIsUnchanged(t *testing.T) {
	h := newTwofaHarness(t)

	// initAuth reads the switch; absent is false.
	absent := koanf.New(".")
	if _, a := initAuth(h.app.core, h.db.DB, absent); a.RequireTwofa() {
		t.Fatal("initAuth with security.require_twofa absent: switch on")
	}
	off := koanf.New(".")
	off.Set("security.require_twofa", false)
	if _, a := initAuth(h.app.core, h.db.DB, off); a.RequireTwofa() {
		t.Fatal("initAuth with security.require_twofa false: switch on")
	}
	on := koanf.New(".")
	on.Set("security.require_twofa", true)
	if _, a := initAuth(h.app.core, h.db.DB, on); !a.RequireTwofa() {
		t.Fatal("initAuth with security.require_twofa true: switch off")
	}

	id := h.user("boss", auth.SuperAdminRoleID)
	cl := h.client()
	rec := cl.login("boss")
	if rec.Code != http.StatusFound || location(rec) != twofaNextURI {
		t.Fatalf("login, switch off: want 302 to %s, got %d %q", twofaNextURI, rec.Code, location(rec))
	}
	if h.sessionsOf(id) != 1 || cl.jar["session"] == "" {
		t.Fatalf("login, switch off: sessions %d, cookie %q", h.sessionsOf(id), cl.jar["session"])
	}
	// The session works, and a factorless enforced session is left alone with the switch off.
	if rec := cl.do(http.MethodGet, "/api/profile", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/profile: %d %s", rec.Code, rec.Body.String())
	}
	var users []auth.User
	rec = cl.do(http.MethodGet, "/api/users", "", "")
	json.Unmarshal(dataOf(t, rec), &users)
	for _, u := range users {
		if u.ID == id && (!u.TwofaEnforced || u.TwofaRequired) {
			t.Fatalf("GET /api/users, switch off: twofa_enforced %v twofa_required %v", u.TwofaEnforced, u.TwofaRequired)
		}
	}
}

// I5 -- a user with a passkey and no TOTP is challenged, never given a session by doLogin, whatever
// the switch.
func TestLoginPasskeyOnlyUserIsChallenged(t *testing.T) {
	h := newTwofaHarness(t)
	id, _ := h.passkeyUser("pkey", h.readerRole)
	if typ, _ := h.twofaType(id); typ != "none" || h.passkeysOf(id) != 1 {
		t.Fatalf("fixture: twofa_type %q passkeys %d", typ, h.passkeysOf(id))
	}

	for _, on := range []bool{false, true} {
		h.setSwitch(on)
		cl := h.client()
		rec := cl.login("pkey")
		loc := location(rec)
		if rec.Code != http.StatusFound || !strings.HasPrefix(loc, "/admin/login/twofa?") {
			t.Fatalf("switch %v: want 302 to the challenge, got %d %q", on, rec.Code, loc)
		}
		if h.sessionsOf(id) != 0 || cl.jar["session"] != "" {
			t.Fatalf("switch %v: doLogin gave a passkey-only user a session", on)
		}
		// The challenge page offers the passkey and not the TOTP form.
		h.render.name, h.render.data = "", nil
		if rec := cl.do(http.MethodGet, loc, "", ""); rec.Code != http.StatusOK || h.render.name != "admin-twofa" {
			t.Fatalf("switch %v: challenge page %d %q", on, rec.Code, h.render.name)
		}
		if tpl, ok := h.render.data.(twofaTpl); !ok || !tpl.Passkey || tpl.TOTP {
			t.Fatalf("switch %v: challenge page data %#v", on, h.render.data)
		}
	}
}

// I7 -- enrolment by TOTP and by passkey each store the factor, delete the token and yield a
// session; every enrol step is refused once the user has a factor; a challenge token, and an enrol
// token without its nonce cookie, are refused on enrol routes; begin never re-Sets the token (its
// age and try count are not reset -- each request counts one try, as on every token route).
func TestEnrolFlow(t *testing.T) {
	h := newTwofaHarness(t)
	h.setSwitch(true)

	// TOTP, through a real login.
	id := h.user("boss", auth.SuperAdminRoleID)
	other := h.sessionClient(id) // a session from before the flip (the middleware would end it)
	_ = other
	cl := h.client()
	loc := location(cl.login("boss"))
	tk := tokenOf(t, loc)

	h.render.name, h.render.data = "", nil
	if rec := cl.do(http.MethodGet, loc, "", ""); rec.Code != http.StatusOK || h.render.name != "admin-enroll" {
		t.Fatalf("enrol page: %d %q", rec.Code, h.render.name)
	}
	page, _ := h.render.data.(enrollTpl)
	if page.Secret == "" || !strings.HasPrefix(string(page.QR), "data:image/png;base64,") {
		t.Fatalf("enrol page data %#v", h.render.data)
	}

	// A wrong code re-renders the SAME secret.
	h.render.data = nil
	rec := cl.form(http.MethodPost, "/admin/login/enroll", url.Values{"token": {tk}, "next": {twofaNextURI}, "secret": {page.Secret}, "totp_code": {"000000"}})
	if again, _ := h.render.data.(enrollTpl); rec.Code != http.StatusOK || again.Secret != page.Secret || again.Error == "" {
		t.Fatalf("wrong code: %d, secret kept %v, error %q", rec.Code, again.Secret == page.Secret, again.Error)
	}
	if h.sessionsOf(id) != 1 {
		t.Fatal("a wrong enrolment code changed the sessions")
	}

	code, _ := totp.GenerateCode(page.Secret, time.Now())
	rec = cl.form(http.MethodPost, "/admin/login/enroll", url.Values{"token": {tk}, "next": {twofaNextURI}, "secret": {page.Secret}, "totp_code": {code}})
	if rec.Code != http.StatusFound || location(rec) != twofaNextURI {
		t.Fatalf("enrol TOTP: want 302 to %s, got %d %q %s", twofaNextURI, rec.Code, location(rec), rec.Body.String())
	}
	if typ, _ := h.twofaType(id); typ != "totp" {
		t.Fatalf("enrol TOTP: twofa_type %q", typ)
	}
	if _, err := tmptokens.Check(tk); err == nil {
		t.Fatal("enrol TOTP: the token survived")
	}
	if cl.jar["session"] == "" || h.sessionsOf(id) != 1 {
		t.Fatalf("enrol TOTP: session cookie %q, rows %d (the pre-flip session must be gone, the new one present)", cl.jar["session"], h.sessionsOf(id))
	}
	if rec := cl.do(http.MethodGet, "/api/profile", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("enrol TOTP: the new session does not work: %d", rec.Code)
	}

	// Every enrol step is refused once the user has a factor: an enrol token minted for them now.
	cl2 := h.client()
	tk2 := cl2.mint(id, twofaPurposeEnroll, enrollTokenTTL)
	if rec := cl2.do(http.MethodGet, "/admin/login/enroll?token="+tk2, "", ""); rec.Code != http.StatusFound || location(rec) != uriLogin {
		t.Fatalf("enrol page for a user with a factor: %d %q", rec.Code, location(rec))
	}
	tk2 = cl2.mint(id, twofaPurposeEnroll, enrollTokenTTL)
	if rec := cl2.json(http.MethodPost, "/admin/login/enroll/passkey/begin", map[string]string{"token": tk2}); rec.Code != http.StatusBadRequest {
		t.Fatalf("enrol passkey begin for a user with a factor: %d %s", rec.Code, rec.Body.String())
	}
	if rec := cl2.json(http.MethodPost, "/admin/login/enroll/passkey/finish", map[string]any{"token": tk2, "name": "x", "credential": json.RawMessage(`{}`)}); rec.Code != http.StatusBadRequest {
		t.Fatalf("enrol passkey finish for a user with a factor: %d %s", rec.Code, rec.Body.String())
	}
	// ... and the core write itself refuses (the one-transaction check).
	if err := h.app.core.EnableTOTPFactor(id, twofaSecret, core.FactorRule{Enrol: true}); err == nil {
		t.Fatal("core enrol write over an existing factor succeeded")
	}
	if _, err := h.app.core.AddPasskey(id, []byte{9}, []byte(`{}`), "x", core.FactorRule{Enrol: true}); err == nil {
		t.Fatal("core enrol passkey over an existing factor succeeded")
	}

	// Passkey enrolment for a second Super Admin.
	pid := h.user("boss2", auth.SuperAdminRoleID)
	cl3 := h.client()
	tk3 := cl3.mint(pid, twofaPurposeEnroll, enrollTokenTTL)

	// A challenge token is refused on enrol routes, and an enrol token without its nonce cookie.
	ch := h.client()
	chTk := ch.mint(pid, twofaPurposeChallenge, twofaTokenTTL)
	if rec := ch.do(http.MethodGet, "/admin/login/enroll?token="+chTk, "", ""); rec.Code != http.StatusFound || location(rec) != uriLogin {
		t.Fatalf("challenge token on the enrol page: %d %q", rec.Code, location(rec))
	}
	if rec := ch.json(http.MethodPost, "/admin/login/enroll/passkey/begin", map[string]string{"token": chTk}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("challenge token on enrol begin: %d", rec.Code)
	}
	bare := h.client()
	if rec := bare.json(http.MethodPost, "/admin/login/enroll/passkey/begin", map[string]string{"token": tk3}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("enrol token without its nonce cookie: %d", rec.Code)
	}
	bare.jar[twofaNonceCookie] = "not-the-nonce"
	if rec := bare.do(http.MethodGet, "/admin/login/enroll?token="+tk3, "", ""); rec.Code != http.StatusFound || location(rec) != uriLogin {
		t.Fatalf("enrol token with the wrong nonce cookie: %d %q", rec.Code, location(rec))
	}

	k := newVKey(pid)
	rec = cl3.json(http.MethodPost, "/admin/login/enroll/passkey/begin", map[string]string{"token": tk3})
	if rec.Code != http.StatusOK {
		t.Fatalf("enrol passkey begin: %d %s", rec.Code, rec.Body.String())
	}
	cred := k.attest(t, h.rp, dataOf(t, rec))
	rec = cl3.json(http.MethodPost, "/admin/login/enroll/passkey/finish", map[string]any{"token": tk3, "next": twofaNextURI, "name": "  phone  ", "credential": cred})
	if rec.Code != http.StatusOK {
		t.Fatalf("enrol passkey finish: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Redirect string `json:"redirect"`
	}
	json.Unmarshal(dataOf(t, rec), &out)
	if out.Redirect != twofaNextURI {
		t.Fatalf("enrol passkey finish redirect %q", out.Redirect)
	}
	var name string
	h.db.Get(&name, `SELECT name FROM user_passkeys WHERE user_id = $1`, pid)
	if h.passkeysOf(pid) != 1 || name != "phone" {
		t.Fatalf("enrol passkey: %d rows, name %q", h.passkeysOf(pid), name)
	}
	if _, err := tmptokens.Check(tk3); err == nil {
		t.Fatal("enrol passkey: the token survived")
	}
	if cl3.jar["session"] == "" || h.sessionsOf(pid) != 1 {
		t.Fatal("enrol passkey: no session")
	}

	// begin never re-Sets the live token. Age: a token minted with a 1.5 s TTL and begun at 1 s
	// still expires at 1.5 s (a re-Set would have moved its expiry to 2.5 s).
	aid := h.user("boss3", auth.SuperAdminRoleID)
	cl4 := h.client()
	short := cl4.mint(aid, twofaPurposeEnroll, 1500*time.Millisecond)
	time.Sleep(time.Second)
	if rec := cl4.json(http.MethodPost, "/admin/login/enroll/passkey/begin", map[string]string{"token": short}); rec.Code != http.StatusOK {
		t.Fatalf("begin within the TTL: %d %s", rec.Code, rec.Body.String())
	}
	time.Sleep(600 * time.Millisecond)
	if rec := cl4.json(http.MethodPost, "/admin/login/enroll/passkey/begin", map[string]string{"token": short}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("begin after the ORIGINAL TTL: want 401 (the token expired on schedule), got %d", rec.Code)
	}
	// Try count: begin does not reset it, so the 16th use of one token is refused (a re-Set
	// would reset the count to zero on every begin and never exhaust it).
	many := cl4.mint(aid, twofaPurposeEnroll, enrollTokenTTL)
	for i := 1; i <= 15; i++ {
		if rec := cl4.json(http.MethodPost, "/admin/login/enroll/passkey/begin", map[string]string{"token": many}); rec.Code != http.StatusOK {
			t.Fatalf("begin %d of 15: %d", i, rec.Code)
		}
	}
	if rec := cl4.json(http.MethodPost, "/admin/login/enroll/passkey/begin", map[string]string{"token": many}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("begin 16: want 401 (try count exhausted), got %d", rec.Code)
	}
	if rec := cl4.do(http.MethodGet, "/admin/login/enroll?token="+many, "", ""); rec.Code != http.StatusFound || location(rec) != uriLogin {
		t.Fatalf("an exhausted token: want the login page, got %d %q", rec.Code, location(rec))
	}
}

// I8 -- doResetPassword stores the password, leaves zero sessions for the user, and redirects to
// the login page.
func TestResetPasswordCreatesNoSession(t *testing.T) {
	h := newTwofaHarness(t)
	id := h.user("resetme", h.writerRole)
	h.sessionClient(id)
	h.sessionClient(id)
	before := h.hash("resetme")

	const tk = "reset-token-for-the-twofa-test"
	tmptokens.Set("resetme@example.test", passwordResetTTL, tk)
	cl := h.client()
	rec := cl.form(http.MethodPost, "/admin/reset?token="+tk+"&email=resetme@example.test", url.Values{"password": {ppGoodPassword}, "password2": {ppGoodPassword}})
	if rec.Code != http.StatusFound || location(rec) != uriLogin {
		t.Fatalf("reset: want 302 to %s, got %d %q", uriLogin, rec.Code, location(rec))
	}
	if h.hash("resetme") == before {
		t.Fatal("reset: the password was not stored")
	}
	if _, err := h.app.core.LoginUser("resetme", ppGoodPassword); err != nil {
		t.Fatalf("reset: the new password does not verify: %v", err)
	}
	if n := h.sessionsOf(id); n != 0 || cl.jar["session"] != "" {
		t.Fatalf("reset: %d sessions left, session cookie %q", n, cl.jar["session"])
	}
}

// I9 -- without a fresh step-up stamp, add/delete passkey, enable/disable TOTP and the admin reset
// are all 403; with a stamp older than 5 minutes, also 403.
func TestFactorChangesNeedStepUp(t *testing.T) {
	h := newTwofaHarness(t)
	id := h.user("boss", auth.SuperAdminRoleID)
	target := h.user("target", h.writerRole)
	h.setTOTP(target)
	h.db.MustExec(`INSERT INTO user_passkeys (user_id, credential_id, credential, name) VALUES ($1, '\x01', '{}', 'k')`, id)
	var pkID int
	h.db.Get(&pkID, `SELECT id FROM user_passkeys WHERE user_id = $1`, id)

	cl := h.sessionClient(id)
	gated := func(label string) {
		t.Helper()
		code, _ := totp.GenerateCode(twofaSecret, time.Now())
		for _, r := range []struct {
			name string
			rec  func() *httptest.ResponseRecorder
		}{
			{"add passkey begin", func() *httptest.ResponseRecorder { return cl.json(http.MethodPost, "/api/profile/passkeys/begin", nil) }},
			{"add passkey finish", func() *httptest.ResponseRecorder {
				return cl.json(http.MethodPost, "/api/profile/passkeys/finish", map[string]any{"name": "x", "credential": json.RawMessage(`{}`)})
			}},
			{"delete passkey", func() *httptest.ResponseRecorder {
				return cl.do(http.MethodDelete, "/api/profile/passkeys/"+strconv.Itoa(pkID), "", "")
			}},
			{"enable TOTP", func() *httptest.ResponseRecorder {
				return cl.form(http.MethodPut, "/api/users/"+strconv.Itoa(id)+"/twofa", url.Values{"secret": {twofaSecret}, "code": {code}})
			}},
			{"disable TOTP", func() *httptest.ResponseRecorder {
				return cl.do(http.MethodDelete, "/api/users/"+strconv.Itoa(id)+"/twofa", "", "")
			}},
			{"admin reset", func() *httptest.ResponseRecorder {
				return cl.do(http.MethodDelete, "/api/users/"+strconv.Itoa(target)+"/factors", "", "")
			}},
		} {
			if rec := r.rec(); rec.Code != http.StatusForbidden {
				t.Fatalf("%s, %s: want 403, got %d %s", label, r.name, rec.Code, rec.Body.String())
			}
		}
		if h.passkeysOf(id) != 1 || h.passkeysOf(target) != 0 {
			t.Fatalf("%s: a refused request changed passkeys", label)
		}
		if typ, _ := h.twofaType(id); typ != "none" {
			t.Fatalf("%s: a refused request enabled TOTP", label)
		}
		if typ, _ := h.twofaType(target); typ != "totp" {
			t.Fatalf("%s: a refused request reset the target", label)
		}
	}

	gated("no stamp")

	// A stamp older than 5 minutes. The user holds a passkey, so step up with TOTP after setting
	// it... simpler: stamp the session directly, aged 301 s.
	h.db.MustExec(`UPDATE sessions SET data = data || jsonb_build_object('stepup_at', EXTRACT(EPOCH FROM NOW())::BIGINT - 301) WHERE id = $1`, cl.jar["session"])
	gated("stale stamp")

	// A fresh stamp (sanity: the same stamp, 10 s old, opens the gate).
	h.db.MustExec(`UPDATE sessions SET data = data || jsonb_build_object('stepup_at', EXTRACT(EPOCH FROM NOW())::BIGINT - 10) WHERE id = $1`, cl.jar["session"])
	if rec := cl.do(http.MethodDelete, "/api/users/"+strconv.Itoa(target)+"/factors", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("admin reset with a fresh stamp: %d %s", rec.Code, rec.Body.String())
	}
}

// I10 -- step-up by password is accepted only for a user with no factor; a user with TOTP or a
// passkey is refused a password step-up.
func TestStepUpPasswordOnlyWithoutFactor(t *testing.T) {
	h := newTwofaHarness(t)

	none := h.user("nofactor", h.writerRole)
	cl := h.sessionClient(none)
	if rec := cl.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("password step-up, no factor: %d %s", rec.Code, rec.Body.String())
	}
	if rec := cl.form(http.MethodPost, "/api/profile/twofa/stepup", url.Values{"password": {"wrong-password"}}); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong password step-up: %d", rec.Code)
	}

	withTOTP := h.user("totpuser", h.writerRole)
	h.setTOTP(withTOTP)
	cl = h.sessionClient(withTOTP)
	if rec := cl.stepUpPassword(); rec.Code != http.StatusForbidden {
		t.Fatalf("password step-up, TOTP user: want 403, got %d", rec.Code)
	}
	if h.app.auth.HasStepUp(h.contextFor(cl)) {
		t.Fatal("a refused password step-up stamped the session")
	}
	if rec := cl.stepUpTOTP(); rec.Code != http.StatusOK {
		t.Fatalf("TOTP step-up: %d %s", rec.Code, rec.Body.String())
	}

	withPasskey, k := h.passkeyUser("pkuser", h.writerRole)
	cl = h.sessionClient(withPasskey)
	if rec := cl.stepUpPassword(); rec.Code != http.StatusForbidden {
		t.Fatalf("password step-up, passkey user: want 403, got %d", rec.Code)
	}
	rec := cl.json(http.MethodPost, "/api/profile/twofa/stepup/passkey/begin", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("passkey step-up begin: %d %s", rec.Code, rec.Body.String())
	}
	if rec := cl.json(http.MethodPost, "/api/profile/twofa/stepup/passkey/finish", map[string]any{"credential": k.assert(t, h.rp, dataOf(t, rec))}); rec.Code != http.StatusOK {
		t.Fatalf("passkey step-up finish: %d %s", rec.Code, rec.Body.String())
	}
	if !h.app.auth.HasStepUp(h.contextFor(cl)) {
		t.Fatal("passkey step-up did not stamp the session")
	}
}

// contextFor runs the session middleware for the client's cookies and returns the context it set
// up (user + session), for direct auth assertions.
func (h *twofaHarness) contextFor(cl *twofaClient) echo.Context {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for k, v := range cl.jar {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	c := echo.New().NewContext(req, httptest.NewRecorder())
	h.app.auth.Middleware(func(echo.Context) error { return nil })(c)
	return c
}

// I11 -- switch ON: an enforced user cannot remove their last factor; with two factors one can be
// removed; switch OFF allows it.
func TestLastFactorRule(t *testing.T) {
	h := newTwofaHarness(t)
	id, k := h.passkeyUser("boss", h.writerRole)
	h.setTOTP(id)
	h.setSwitch(true)

	var pk int
	h.db.Get(&pk, `SELECT id FROM user_passkeys WHERE user_id = $1`, id)

	cl := h.sessionClient(id)
	if rec := cl.stepUpTOTP(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d %s", rec.Code, rec.Body.String())
	}
	// Two factors: TOTP can go.
	if rec := cl.do(http.MethodDelete, "/api/users/"+strconv.Itoa(id)+"/twofa", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("disable TOTP with a passkey left: %d %s", rec.Code, rec.Body.String())
	}
	// The passkey is now the last factor.
	if rec := cl.do(http.MethodDelete, "/api/profile/passkeys/"+strconv.Itoa(pk), "", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete the last passkey, switch on: want 400, got %d", rec.Code)
	}
	if h.passkeysOf(id) != 1 {
		t.Fatal("the last passkey was deleted")
	}
	_ = k

	// TOTP as the only factor, switch on: refused.
	h.db.MustExec(`DELETE FROM user_passkeys WHERE user_id = $1`, id)
	h.setTOTP(id)
	cl = h.sessionClient(id)
	cl.stepUpTOTP()
	if rec := cl.do(http.MethodDelete, "/api/users/"+strconv.Itoa(id)+"/twofa", "", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("disable the only TOTP, switch on: want 400, got %d", rec.Code)
	}
	if typ, _ := h.twofaType(id); typ != "totp" {
		t.Fatal("the last factor (TOTP) was removed")
	}

	// Switch OFF: allowed.
	h.setSwitch(false)
	cl = h.sessionClient(id)
	cl.stepUpTOTP()
	if rec := cl.do(http.MethodDelete, "/api/users/"+strconv.Itoa(id)+"/twofa", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("disable the only TOTP, switch off: %d %s", rec.Code, rec.Body.String())
	}
	if typ, hasKey := h.twofaType(id); typ != "none" || hasKey {
		t.Fatalf("disable TOTP: twofa_type %q, key kept %v (want NULL)", typ, hasKey)
	}
}

// I12 -- switch ON: a cookie session of an enforced user with no factor is destroyed on its next
// request, including a read-only user whose role gains a write permission mid-session; an
// API-token request and a read-only user's session are untouched.
func TestMiddlewareEndsFactorlessEnforcedSession(t *testing.T) {
	h := newTwofaHarness(t)
	boss := h.user("boss", auth.SuperAdminRoleID)
	reader := h.user("reader", h.readerRole)
	bossCl := h.sessionClient(boss) // sessions from before the flip
	readerCl := h.sessionClient(reader)

	api, err := h.app.core.CreateUser(auth.User{Type: auth.UserTypeAPI, Username: "bot", Name: "bot", UserRoleID: auth.SuperAdminRoleID, Status: auth.UserStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}

	h.setSwitch(true)

	rec := bossCl.do(http.MethodGet, "/api/profile", "", "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "invalid session") {
		t.Fatalf("factorless Super Admin session, switch on: want 403 invalid session, got %d %s", rec.Code, rec.Body.String())
	}
	if h.sessionsOf(boss) != 0 {
		t.Fatal("the factorless enforced session was not destroyed")
	}

	if rec := readerCl.do(http.MethodGet, "/api/profile", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("read-only user's session: %d", rec.Code)
	}

	// The reader's role gains a write permission mid-session.
	h.db.MustExec(`UPDATE roles SET permissions = permissions || '{subscribers:sql_query}' WHERE id = $1`, h.readerRole)
	if rec := readerCl.do(http.MethodGet, "/api/profile", "", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("reader upgraded past read: want 403, got %d", rec.Code)
	}
	if h.sessionsOf(reader) != 0 {
		t.Fatal("the upgraded reader's session was not destroyed")
	}

	// An API-token request is never checked.
	tok := h.client()
	tok.hdr["Authorization"] = "token bot:" + api.Password.String
	if rec := tok.do(http.MethodGet, "/api/profile", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("API token request, switch on: %d %s", rec.Code, rec.Body.String())
	}

	// A user who holds a factor keeps the session.
	h.setTOTP(boss)
	cl := h.sessionClient(boss)
	if rec := cl.do(http.MethodGet, "/api/profile", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("enforced user with TOTP: %d", rec.Code)
	}

	// GET /api/users carries the running process's answer.
	var users []auth.User
	json.Unmarshal(dataOf(t, cl.do(http.MethodGet, "/api/users", "", "")), &users)
	seen := 0
	for _, u := range users {
		switch u.ID {
		case boss:
			seen++
			if !u.TwofaEnforced || !u.TwofaRequired {
				t.Fatalf("GET /api/users, switch on, Super Admin: enforced %v required %v", u.TwofaEnforced, u.TwofaRequired)
			}
		case api.ID:
			seen++
			if u.TwofaEnforced || u.TwofaRequired {
				t.Fatal("GET /api/users: an API user is enforced")
			}
		}
	}
	if seen != 2 {
		t.Fatalf("GET /api/users: saw %d of the 2 users", seen)
	}
}

// I13 -- the admin reset clears TOTP, passkeys and every session of the target; refuses the
// caller's own id; needs users:manage.
func TestAdminResetFactors(t *testing.T) {
	h := newTwofaHarness(t)
	admin := h.user("admin1", auth.SuperAdminRoleID)
	target, _ := h.passkeyUser("target", h.writerRole)
	h.setTOTP(target)
	h.sessionClient(target)
	h.sessionClient(target)

	cl := h.sessionClient(admin)
	if rec := cl.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d", rec.Code)
	}
	if rec := cl.do(http.MethodDelete, "/api/users/"+strconv.Itoa(admin)+"/factors", "", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("reset own factors: want 400, got %d", rec.Code)
	}
	if rec := cl.do(http.MethodDelete, "/api/users/"+strconv.Itoa(target)+"/factors", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", rec.Code, rec.Body.String())
	}
	if typ, hasKey := h.twofaType(target); typ != "none" || hasKey {
		t.Fatalf("reset: twofa_type %q, key kept %v", typ, hasKey)
	}
	if h.passkeysOf(target) != 0 || h.sessionsOf(target) != 0 {
		t.Fatalf("reset: %d passkeys, %d sessions left", h.passkeysOf(target), h.sessionsOf(target))
	}
	if h.sessionsOf(admin) != 1 {
		t.Fatal("reset touched the caller's session")
	}

	// users:manage is required: a role with users:get only, stamped, is refused.
	var viewRole int
	h.db.Get(&viewRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Viewer', '{users:get}') RETURNING id`)
	viewer := h.user("viewer", viewRole)
	h.setTOTP(target)
	vcl := h.sessionClient(viewer)
	if rec := vcl.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("viewer step-up: %d", rec.Code)
	}
	if rec := vcl.do(http.MethodDelete, "/api/users/"+strconv.Itoa(target)+"/factors", "", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("reset without users:manage: want 403, got %d", rec.Code)
	}
	if typ, _ := h.twofaType(target); typ != "totp" {
		t.Fatal("a refused reset cleared the target's TOTP")
	}
}

// I22 -- 5 failed step-ups destroy the session; 10 failed TOTP codes for one user inside 15
// minutes refuse further TOTP attempts (challenge and step-up) while a passkey assertion still
// succeeds.
func TestSecondFactorAttemptLimits(t *testing.T) {
	h := newTwofaHarness(t)

	none := h.user("nofactor", h.writerRole)
	cl := h.sessionClient(none)
	for i := 1; i <= 4; i++ {
		rec := cl.form(http.MethodPost, "/api/profile/twofa/stepup", url.Values{"password": {"wrong-password"}})
		if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "invalid session") || h.sessionsOf(none) != 1 {
			t.Fatalf("failed step-up %d: %d %s, sessions %d", i, rec.Code, rec.Body.String(), h.sessionsOf(none))
		}
	}
	rec := cl.form(http.MethodPost, "/api/profile/twofa/stepup", url.Values{"password": {"wrong-password"}})
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "invalid session") || h.sessionsOf(none) != 0 {
		t.Fatalf("5th failed step-up: want the session destroyed, got %d %s, sessions %d", rec.Code, rec.Body.String(), h.sessionsOf(none))
	}

	// A user with TOTP and a passkey.
	id, k := h.passkeyUser("both", h.writerRole)
	h.setTOTP(id)

	lcl := h.client()
	tk := lcl.mint(id, twofaPurposeChallenge, twofaTokenTTL)
	for i := 1; i <= totpMaxFails; i++ {
		h.render.data = nil
		rec := lcl.form(http.MethodPost, "/admin/login/twofa", url.Values{"token": {tk}, "next": {twofaNextURI}, "totp_code": {"000000"}})
		if tpl, _ := h.render.data.(twofaTpl); rec.Code != http.StatusOK || tpl.Error == "" {
			t.Fatalf("wrong code %d: %d %#v", i, rec.Code, h.render.data)
		}
	}
	code, _ := totp.GenerateCode(twofaSecret, time.Now())
	h.render.data = nil
	rec = lcl.form(http.MethodPost, "/admin/login/twofa", url.Values{"token": {tk}, "next": {twofaNextURI}, "totp_code": {code}})
	if tpl, _ := h.render.data.(twofaTpl); rec.Code != http.StatusOK || tpl.Error != h.app.i18n.T("users.totpLimited") || lcl.jar["session"] != "" {
		t.Fatalf("correct code after 10 failures at the challenge: want refused (%q), got %d %#v", h.app.i18n.T("users.totpLimited"), rec.Code, h.render.data)
	}

	// Step-up by TOTP is refused too.
	scl := h.sessionClient(id)
	if rec := scl.stepUpTOTP(); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), h.app.i18n.T("users.totpLimited")) {
		t.Fatalf("TOTP step-up after 10 failures: want refused, got %d %s", rec.Code, rec.Body.String())
	}

	// A passkey assertion still succeeds at the challenge.
	rec = lcl.json(http.MethodPost, "/admin/login/twofa/passkey/begin", map[string]string{"token": tk})
	if rec.Code != http.StatusOK {
		t.Fatalf("passkey begin while TOTP is limited: %d %s", rec.Code, rec.Body.String())
	}
	rec = lcl.json(http.MethodPost, "/admin/login/twofa/passkey/finish", map[string]any{"token": tk, "next": twofaNextURI, "credential": k.assert(t, h.rp, dataOf(t, rec))})
	if rec.Code != http.StatusOK || lcl.jar["session"] == "" {
		t.Fatalf("passkey finish while TOTP is limited: %d %s", rec.Code, rec.Body.String())
	}

	// (a) A concurrent burst of wrong step-up passwords on one session evaluates at most
	// StepUpMaxFails of them: every other answer is the invalid-session one, and the session is gone.
	// Evaluations are counted from the app log: stepUpFail logs one line per EVALUATED failure
	// (the last evaluated one answers invalid-session too, so the answers alone cannot tell).
	var logBuf syncBuffer
	appLog := h.app.log
	h.app.log = log.New(&logBuf, "", 0)
	burster := h.user("burster", h.writerRole)
	bcl := h.sessionClient(burster)
	bodies := bcl.burst(40, http.MethodPost, "/api/profile/twofa/stepup", echo.MIMEApplicationForm, "password=wrong-password")
	h.app.log = appLog
	wrong, ended := countContaining(bodies, h.app.i18n.T("users.invalidPassword")), countContaining(bodies, "invalid session")
	evaluated := strings.Count(logBuf.String(), "2FA: failed step-up attempt")
	if evaluated > auth.StepUpMaxFails || wrong > auth.StepUpMaxFails || wrong+ended != len(bodies) {
		t.Fatalf("burst of 40 wrong step-up passwords: %d evaluated (log), %d answered wrong-password, %d invalid-session (want <= %d evaluated, the rest invalid-session)", evaluated, wrong, ended, auth.StepUpMaxFails)
	}
	if h.sessionsOf(burster) != 0 {
		t.Fatal("burst of wrong step-up passwords: the session survived")
	}

	// A malformed step-up (neither field) consumes no attempt: 10 of them, then a correct
	// password still works on a fresh session.
	mcl := h.sessionClient(h.user("malformed", h.writerRole))
	for i := 0; i < 10; i++ {
		if rec := mcl.form(http.MethodPost, "/api/profile/twofa/stepup", url.Values{}); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed step-up %d: %d", i, rec.Code)
		}
	}
	if rec := mcl.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("step-up after 10 malformed requests: %d %s", rec.Code, rec.Body.String())
	}

	// (b) A concurrent burst of wrong TOTP codes for ONE user, one per session (so no session
	// limit applies), evaluates at most totpMaxFails; the rest are refused by the per-user limit.
	tuser := h.user("totpburst", h.writerRole)
	h.setTOTP(tuser)
	clients := make([]*twofaClient, 40)
	for i := range clients {
		clients[i] = h.sessionClient(tuser)
	}
	tbodies := make([]string, len(clients))
	var wg sync.WaitGroup
	for i, c := range clients {
		wg.Add(1)
		go func(i int, c *twofaClient) {
			defer wg.Done()
			tbodies[i] = c.fire(http.MethodPost, "/api/profile/twofa/stepup", echo.MIMEApplicationForm, "totp_code=000000").Body.String()
		}(i, c)
	}
	wg.Wait()
	evaluated, refused := countContaining(tbodies, h.app.i18n.T("users.invalidTOTPCode")), countContaining(tbodies, h.app.i18n.T("users.totpLimited"))
	if evaluated > totpMaxFails || evaluated+refused != len(tbodies) {
		t.Fatalf("burst of 40 wrong TOTP codes for one user: %d evaluated, %d refused (want <= %d evaluated, the rest refused)", evaluated, refused, totpMaxFails)
	}

	// (c) Those wrong codes were made AT STEP-UP, and they count toward the per-user limit: the
	// login challenge now refuses even the correct code for that user.
	ccl := h.client()
	ctk := ccl.mint(tuser, twofaPurposeChallenge, twofaTokenTTL)
	code, _ = totp.GenerateCode(twofaSecret, time.Now())
	h.render.data = nil
	rec = ccl.form(http.MethodPost, "/admin/login/twofa", url.Values{"token": {ctk}, "next": {twofaNextURI}, "totp_code": {code}})
	if tpl, _ := h.render.data.(twofaTpl); rec.Code != http.StatusOK || tpl.Error != h.app.i18n.T("users.totpLimited") || ccl.jar["session"] != "" {
		t.Fatalf("challenge after wrong step-up codes: want the TOTP limit, got %d %#v", rec.Code, h.render.data)
	}

	// A correct code is never counted: a fresh user, 15 correct step-ups across sessions, and
	// the user is not limited.
	ok := h.user("correct", h.writerRole)
	h.setTOTP(ok)
	for i := 0; i < 15; i++ {
		if rec := h.sessionClient(ok).stepUpTOTP(); rec.Code != http.StatusOK {
			t.Fatalf("correct step-up %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	if totpGuesses.limited(ok) {
		t.Fatal("correct TOTP codes were counted toward the guess limit")
	}

	// A wrong code while ENROLLING TOTP is not counted: another user, 12 wrong enrolment codes,
	// then the challenge's correct code still works.
	other := h.user("enroller", auth.SuperAdminRoleID)
	h.setSwitch(true)
	ecl := h.client()
	etk := ecl.mint(other, twofaPurposeEnroll, enrollTokenTTL)
	for i := 0; i < 12; i++ {
		ecl.form(http.MethodPost, "/admin/login/enroll", url.Values{"token": {etk}, "secret": {twofaSecret}, "totp_code": {"000000"}})
	}
	if totpGuesses.limited(other) {
		t.Fatal("wrong enrolment codes were counted toward the TOTP guess limit")
	}
}

// I23 -- PUT /api/profile changing the password or the email is 403 without a stamp and succeeds
// with one; a name-only save needs none; step-up by password leaves loggedin_at unchanged.
func TestProfileCredentialChangeNeedsStepUp(t *testing.T) {
	h := newTwofaHarness(t)
	id := h.user("me", h.writerRole)
	h.db.MustExec(`UPDATE users SET loggedin_at = NOW() - INTERVAL '1 day' WHERE id = $1`, id)
	var before string
	h.db.Get(&before, `SELECT loggedin_at::TEXT FROM users WHERE id = $1`, id)

	cl := h.sessionClient(id)
	profile := func(email, password string) *httptest.ResponseRecorder {
		return cl.json(http.MethodPut, "/api/profile", map[string]string{"name": "Me Renamed", "email": email, "password": password})
	}

	if rec := profile("me@example.test", ""); rec.Code != http.StatusOK {
		t.Fatalf("name-only save: %d %s", rec.Code, rec.Body.String())
	}
	before = h.hash("me")
	if rec := profile("me@example.test", ppGoodPassword); rec.Code != http.StatusForbidden {
		t.Fatalf("password change without a stamp: want 403, got %d", rec.Code)
	}
	if rec := profile("new@example.test", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("email change without a stamp: want 403, got %d", rec.Code)
	}
	var email string
	h.db.Get(&email, `SELECT email FROM users WHERE id = $1`, id)
	if email != "me@example.test" || h.hash("me") != before {
		t.Fatal("a refused profile save changed the email or the password")
	}

	if rec := cl.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d", rec.Code)
	}
	var after string
	h.db.Get(&after, `SELECT loggedin_at::TEXT FROM users WHERE id = $1`, id)
	var dayOld bool
	h.db.Get(&dayOld, `SELECT loggedin_at < NOW() - INTERVAL '23 hours' FROM users WHERE id = $1`, id)
	if !dayOld {
		t.Fatalf("step-up by password rewrote loggedin_at (now %s)", after)
	}

	if rec := profile("new@example.test", ppGoodPassword); rec.Code != http.StatusOK {
		t.Fatalf("password + email change with a stamp: %d %s", rec.Code, rec.Body.String())
	}
	h.db.Get(&email, `SELECT email FROM users WHERE id = $1`, id)
	if email != "new@example.test" || h.hash("me") == before {
		t.Fatal("the stamped profile save did not land")
	}
}

// I24 -- a full settings PUT whose body lacks security.require_twofa leaves the stored value;
// changing the value is 403 without a stamp; a save that keeps the value needs none.
func TestRequireTwofaSettingGuard(t *testing.T) {
	h := newTwofaHarness(t)
	ensureManager(h.linkHarness)
	id := h.user("boss", auth.SuperAdminRoleID)
	cl := h.sessionClient(id)

	stored := func() string {
		var v string
		h.db.Get(&v, `SELECT value::TEXT FROM settings WHERE key = 'security.require_twofa'`)
		return v
	}
	if stored() != "false" {
		t.Fatalf("fixture: stored %q", stored())
	}

	// The schema seed's SMTP blocks carry no uuid, so a masked password could not be matched back
	// to its stored secret on save; give them one, as the settings UI's first save would.
	h.db.MustExec(`UPDATE settings SET value = (SELECT jsonb_agg(e || jsonb_build_object('uuid', gen_random_uuid()::TEXT)) FROM jsonb_array_elements(value) e) WHERE key = 'smtp'`)

	rec := cl.do(http.MethodGet, "/api/settings", "", "")
	var set map[string]json.RawMessage
	if err := json.Unmarshal(dataOf(t, rec), &set); err != nil {
		t.Fatal(err)
	}
	if string(set[settingRequireTwofa]) != "false" {
		t.Fatalf("GET /api/settings: %s = %s", settingRequireTwofa, set[settingRequireTwofa])
	}
	put := func(body map[string]json.RawMessage) *httptest.ResponseRecorder {
		return cl.json(http.MethodPut, "/api/settings", body)
	}

	// Keeping the value needs no stamp.
	if rec := put(set); rec.Code != http.StatusOK {
		t.Fatalf("full PUT keeping false: %d %s", rec.Code, rec.Body.String())
	}

	// Changing it needs one, on both routes.
	set[settingRequireTwofa] = json.RawMessage(`true`)
	if rec := put(set); rec.Code != http.StatusForbidden || stored() != "false" {
		t.Fatalf("full PUT to true without a stamp: %d, stored %s", rec.Code, stored())
	}
	if rec := cl.do(http.MethodPut, "/api/settings/"+settingRequireTwofa, echo.MIMEApplicationJSON, "true"); rec.Code != http.StatusForbidden || stored() != "false" {
		t.Fatalf("PUT by key to true without a stamp: %d, stored %s", rec.Code, stored())
	}
	if rec := cl.do(http.MethodPut, "/api/settings/"+settingRequireTwofa, echo.MIMEApplicationJSON, "false"); rec.Code != http.StatusOK {
		t.Fatalf("PUT by key keeping false: %d %s", rec.Code, rec.Body.String())
	}
	if rec := cl.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d", rec.Code)
	}
	if rec := put(set); rec.Code != http.StatusOK || stored() != "true" {
		t.Fatalf("full PUT to true with a stamp: %d %s, stored %s", rec.Code, rec.Body.String(), stored())
	}

	// A body that lacks the key (a tab loaded before the release) keeps the stored true.
	delete(set, settingRequireTwofa)
	cl2 := h.sessionClient(id)
	if rec := cl2.json(http.MethodPut, "/api/settings", set); rec.Code != http.StatusOK || stored() != "true" {
		t.Fatalf("full PUT without the key: %d %s, stored %s (want true kept)", rec.Code, rec.Body.String(), stored())
	}
}

// I25 -- after a factor is added or removed, the user's other sessions are gone and the current
// one remains.
func TestFactorChangeEndsOtherSessions(t *testing.T) {
	h := newTwofaHarness(t)
	id := h.user("me", h.writerRole)
	cl := h.sessionClient(id)
	others := func(n int) {
		for i := 0; i < n; i++ {
			h.sessionClient(id)
		}
	}
	current := func(label string) {
		t.Helper()
		var n int
		h.db.Get(&n, `SELECT COUNT(*) FROM sessions WHERE id = $1`, cl.jar["session"])
		if n != 1 || h.sessionsOf(id) != 1 {
			t.Fatalf("%s: current session present %v, %d sessions for the user (want only the current)", label, n == 1, h.sessionsOf(id))
		}
	}

	cl.stepUpPassword()
	others(2)
	k := newVKey(id)
	if rec := cl.addPasskey(t, k, "laptop"); rec.Code != http.StatusOK {
		t.Fatalf("add passkey: %d %s", rec.Code, rec.Body.String())
	}
	current("add passkey")

	// Enable TOTP (step up with the passkey first: the user now holds a factor).
	stepUpPasskey := func() {
		t.Helper()
		rec := cl.json(http.MethodPost, "/api/profile/twofa/stepup/passkey/begin", nil)
		if rec := cl.json(http.MethodPost, "/api/profile/twofa/stepup/passkey/finish", map[string]any{"credential": k.assert(t, h.rp, dataOf(t, rec))}); rec.Code != http.StatusOK {
			t.Fatalf("passkey step-up: %d %s", rec.Code, rec.Body.String())
		}
	}
	stepUpPasskey()
	others(2)
	code, _ := totp.GenerateCode(twofaSecret, time.Now())
	if rec := cl.form(http.MethodPut, "/api/users/"+strconv.Itoa(id)+"/twofa", url.Values{"secret": {twofaSecret}, "code": {code}}); rec.Code != http.StatusOK {
		t.Fatalf("enable TOTP: %d %s", rec.Code, rec.Body.String())
	}
	current("enable TOTP")

	others(2)
	if rec := cl.do(http.MethodDelete, "/api/users/"+strconv.Itoa(id)+"/twofa", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("disable TOTP: %d %s", rec.Code, rec.Body.String())
	}
	current("disable TOTP")

	others(2)
	var pk int
	h.db.Get(&pk, `SELECT id FROM user_passkeys WHERE user_id = $1`, id)
	if rec := cl.do(http.MethodDelete, "/api/profile/passkeys/"+strconv.Itoa(pk), "", ""); rec.Code != http.StatusOK {
		t.Fatalf("delete passkey: %d %s", rec.Code, rec.Body.String())
	}
	current("delete passkey")
}

// I26 -- GET /api/profile/twofa carries no credential, public key or TOTP secret; an API-token
// caller gets 403 on every stamp-gated route.
func TestTwofaReadAndTokenCallers(t *testing.T) {
	h := newTwofaHarness(t)
	id, _ := h.passkeyUser("me", h.writerRole)
	h.setTOTP(id)
	h.setSwitch(true)

	cl := h.sessionClient(id)
	rec := cl.do(http.MethodGet, "/api/profile/twofa", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/profile/twofa: %d", rec.Code)
	}
	body := rec.Body.String()
	var credJSON string
	h.db.Get(&credJSON, `SELECT credential::TEXT FROM user_passkeys WHERE user_id = $1`, id)
	var stored map[string]any
	json.Unmarshal([]byte(credJSON), &stored)
	for _, bad := range []string{twofaSecret, "publicKey", "public_key", "credential", "twofa_key", stored["publicKey"].(string), stored["id"].(string)} {
		if strings.Contains(body, bad) {
			t.Fatalf("GET /api/profile/twofa leaks %q: %s", bad, body)
		}
	}
	var out struct {
		TOTP     bool             `json:"totp"`
		Passkeys []map[string]any `json:"passkeys"`
		Required bool             `json:"required"`
		Enforced bool             `json:"enforced"`
	}
	json.Unmarshal(dataOf(t, rec), &out)
	if !out.TOTP || len(out.Passkeys) != 1 || out.Passkeys[0]["name"] != "laptop" || !out.Required || !out.Enforced {
		t.Fatalf("GET /api/profile/twofa: %+v", out)
	}
	for k := range out.Passkeys[0] {
		if k != "id" && k != "name" && k != "created_at" && k != "last_used_at" {
			t.Fatalf("passkey row carries %q", k)
		}
	}

	api, err := h.app.core.CreateUser(auth.User{Type: auth.UserTypeAPI, Username: "bot", Name: "bot", UserRoleID: auth.SuperAdminRoleID, Status: auth.UserStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}
	cacheUsers(h.app.core, h.app.auth)
	tok := h.client()
	tok.hdr["Authorization"] = "token bot:" + api.Password.String
	code, _ := totp.GenerateCode(twofaSecret, time.Now())
	for _, r := range []struct{ method, path, ctype, body string }{
		{http.MethodPost, "/api/profile/twofa/stepup", echo.MIMEApplicationForm, "password=" + twofaPw},
		{http.MethodPost, "/api/profile/twofa/stepup", echo.MIMEApplicationForm, "totp_code=" + code},
		{http.MethodPost, "/api/profile/passkeys/begin", echo.MIMEApplicationJSON, "{}"},
		{http.MethodPost, "/api/profile/passkeys/finish", echo.MIMEApplicationJSON, `{"name":"x","credential":{}}`},
		{http.MethodDelete, "/api/profile/passkeys/1", "", ""},
		{http.MethodPut, "/api/users/" + strconv.Itoa(api.ID) + "/twofa", echo.MIMEApplicationForm, "secret=" + twofaSecret + "&code=" + code},
		{http.MethodDelete, "/api/users/" + strconv.Itoa(api.ID) + "/twofa", "", ""},
		{http.MethodDelete, "/api/users/" + strconv.Itoa(id) + "/factors", "", ""},
		{http.MethodPut, "/api/settings/" + settingRequireTwofa, echo.MIMEApplicationJSON, "true"},
	} {
		if rec := tok.do(r.method, r.path, r.ctype, r.body); rec.Code != http.StatusForbidden {
			t.Fatalf("API token %s %s: want 403, got %d %s", r.method, r.path, rec.Code, rec.Body.String())
		}
	}
	// A full settings PUT that changes the switch is refused to a token caller too.
	h.db.MustExec(`UPDATE settings SET value = (SELECT jsonb_agg(e || jsonb_build_object('uuid', gen_random_uuid()::TEXT)) FROM jsonb_array_elements(value) e) WHERE key = 'smtp'`)
	var set map[string]json.RawMessage
	if err := json.Unmarshal(dataOf(t, tok.do(http.MethodGet, "/api/settings", "", "")), &set); err != nil {
		t.Fatal(err)
	}
	set[settingRequireTwofa] = json.RawMessage(`true`)
	if rec := tok.json(http.MethodPut, "/api/settings", set); rec.Code != http.StatusForbidden {
		t.Fatalf("API token full settings PUT changing the switch: want 403, got %d %s", rec.Code, rec.Body.String())
	}
	var sw string
	h.db.Get(&sw, `SELECT value::TEXT FROM settings WHERE key = 'security.require_twofa'`)
	if sw != "false" {
		t.Fatalf("a token caller changed the switch to %s", sw)
	}

	if h.passkeysOf(id) != 1 {
		t.Fatal("a token caller changed a factor")
	}
	if typ, _ := h.twofaType(id); typ != "totp" {
		t.Fatal("a token caller reset a factor")
	}

	// A token caller's PUT /api/profile with a name and an email answers as upstream does (200):
	// the step-up gate is for password-login USERS only.
	if rec := tok.json(http.MethodPut, "/api/profile", map[string]string{"name": "bot renamed", "email": "bot@example.test"}); rec.Code != http.StatusOK {
		t.Fatalf("API token PUT /api/profile: want 200 as at base, got %d %s", rec.Code, rec.Body.String())
	}
	if rec := tok.do(http.MethodGet, "/api/profile", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("the token stopped working after its profile save: %d", rec.Code)
	}
}

// F5 of the stage-4 review (D6): editing YOUR OWN account through PUT /api/users/:id needs the
// same step-up as the profile route when it changes the password or the email; editing another
// user stays ungated (user administration is a stated non-goal).
func TestSelfUpdateViaUsersRouteNeedsStepUp(t *testing.T) {
	h := newTwofaHarness(t)
	me := h.user("selfadmin", auth.SuperAdminRoleID)
	h.setTOTP(me)
	other := h.user("otheruser", h.writerRole)

	cl := h.sessionClient(me)
	put := func(cl *twofaClient, id int, username, email, password string) *httptest.ResponseRecorder {
		return cl.do(http.MethodPut, "/api/users/"+strconv.Itoa(id), echo.MIMEApplicationJSON, h.userJSON(username, email, password))
	}
	before := h.hash("selfadmin")
	if rec := put(cl, me, "selfadmin", "selfadmin@example.test", ppGoodPassword); rec.Code != http.StatusForbidden {
		t.Fatalf("own password via /api/users without a stamp: want 403, got %d %s", rec.Code, rec.Body.String())
	}
	if rec := put(cl, me, "selfadmin", "new-self@example.test", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("own email via /api/users without a stamp: want 403, got %d %s", rec.Code, rec.Body.String())
	}
	var email string
	h.db.Get(&email, `SELECT email FROM users WHERE id = $1`, me)
	if email != "selfadmin@example.test" || h.hash("selfadmin") != before {
		t.Fatal("a refused self-edit changed the email or the password")
	}
	// Neither changed: no stamp needed.
	if rec := put(cl, me, "selfadmin", "selfadmin@example.test", ""); rec.Code != http.StatusOK {
		t.Fatalf("own save with no credential change: %d %s", rec.Code, rec.Body.String())
	}

	// A differently cased copy of the same email is no change.
	if rec := put(cl, me, "selfadmin", "SelfAdmin@Example.test", ""); rec.Code != http.StatusOK {
		t.Fatalf("own save with the same email in another case: %d %s", rec.Code, rec.Body.String())
	}

	// The gate cannot be switched off first: flipping password_login or type on your own account
	// needs the stamp too (either would open the way to a new email and password in a second
	// request, and password_login=false alone drops the password).
	raw := func(body string) *httptest.ResponseRecorder {
		return cl.do(http.MethodPut, "/api/users/"+strconv.Itoa(me), echo.MIMEApplicationJSON, body)
	}
	if rec := raw(`{"username": "selfadmin", "name": "selfadmin", "email": "selfadmin@example.test", "type": "user", "user_role_id": 1, "status": "enabled", "password_login": false}`); rec.Code != http.StatusForbidden {
		t.Fatalf("own password_login flip without a stamp: want 403, got %d %s", rec.Code, rec.Body.String())
	}
	if rec := raw(`{"username": "selfadmin", "name": "selfadmin", "type": "api", "user_role_id": 1, "status": "enabled", "password_login": true}`); rec.Code != http.StatusForbidden {
		t.Fatalf("own type flip without a stamp: want 403, got %d %s", rec.Code, rec.Body.String())
	}
	var typ string
	h.db.Get(&typ, `SELECT type FROM users WHERE id = $1`, me)
	h.db.Get(&email, `SELECT email FROM users WHERE id = $1`, me)
	if typ != "user" || email != "selfadmin@example.test" || h.hash("selfadmin") != before {
		t.Fatalf("a refused self-edit changed the account: type=%s email=%s", typ, email)
	}

	// Editing ANOTHER user needs no stamp, password included.
	if rec := put(cl, other, "otheruser", "otheruser@example.test", ppGoodPassword); rec.Code != http.StatusOK {
		t.Fatalf("edit another user without a stamp: want 200, got %d %s", rec.Code, rec.Body.String())
	}

	if rec := cl.stepUpTOTP(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d %s", rec.Code, rec.Body.String())
	}
	if rec := put(cl, me, "selfadmin", "new-self@example.test", ""); rec.Code != http.StatusOK {
		t.Fatalf("own email with a stamp: %d %s", rec.Code, rec.Body.String())
	}
	if rec := put(cl, me, "selfadmin", "new-self@example.test", ppGoodPassword); rec.Code != http.StatusOK {
		t.Fatalf("own password with a stamp: %d %s", rec.Code, rec.Body.String())
	}
	h.db.Get(&email, `SELECT email FROM users WHERE id = $1`, me)
	if email != "new-self@example.test" || h.hash("selfadmin") == before {
		t.Fatal("the stamped self-edit did not land")
	}
}
