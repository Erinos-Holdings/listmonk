package main

// Fork (two-factor, integrations PASSKEY-2FA-SPEC). Passkeys (WebAuthn) as a second factor beside
// TOTP, the login challenge and enrolment pages, session step-up for every factor change, and the
// admin "Reset two-factor". Who is enforced lives in internal/auth (IsTwofaEnforced); the
// transactional factor writes in internal/core/twofa.go.

import (
	"bytes"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"image/png"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/tmptokens"
	"github.com/knadh/listmonk/internal/utils"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/pquerna/otp/totp"
)

const (
	twofaPurposeChallenge = "challenge"
	twofaPurposeEnroll    = "enroll"

	// enrollTokenTTL is the enrol token's lifetime (twofaTokenTTL, 5 minutes, is the challenge's).
	enrollTokenTTL = 15 * time.Minute

	// ceremonyTTL is the lifetime of a WebAuthn ceremony's state between begin and finish.
	ceremonyTTL = 5 * time.Minute

	// twofaNonceCookie binds a challenge/enrol token to the browser that logged in (D5).
	twofaNonceCookie = "twofa_nonce"

	// settingRequireTwofa is the enforcement switch (D4).
	settingRequireTwofa = "security.require_twofa"

	// Per-user TOTP guess limit (D5): totpMaxFails failures inside totpFailWindow refuse further
	// TOTP attempts for that user until the window passes.
	totpMaxFails   = 10
	totpFailWindow = 15 * time.Minute

	passkeyNameMaxLen = 64

	uriLogin = uriAdmin + "/login"
)

// twofaToken is the payload of a challenge or enrol temp token. Any other payload (a reset entry,
// a ceremony entry) is refused on both kinds of route.
type twofaToken struct {
	UserID  int
	Purpose string
	Nonce   string
}

type enrollTpl struct {
	Title       string
	Description string
	Token       string
	NextURI     string
	Secret      string
	QR          template.URL
	Error       string
}

// passkeyReq is the JSON body of every passkey begin/finish route.
type passkeyReq struct {
	Token      string          `json:"token"`
	Next       string          `json:"next"`
	Name       string          `json:"name"`
	Credential json.RawMessage `json:"credential"`
}

// totpGuessLimiter counts failed TOTP codes per user in memory.
type totpGuessLimiter struct {
	mu    sync.Mutex
	fails map[int][]time.Time
}

var totpGuesses = &totpGuessLimiter{fails: map[int][]time.Time{}}

func (l *totpGuessLimiter) prune(userID int) []time.Time {
	var keep []time.Time
	for _, t := range l.fails[userID] {
		if time.Since(t) < totpFailWindow {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(l.fails, userID)
	} else {
		l.fails[userID] = keep
	}
	return keep
}

// limited reports whether the user's TOTP attempts are currently refused.
func (l *totpGuessLimiter) limited(userID int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(userID)) >= totpMaxFails
}

// reserve counts one TOTP guess for the user BEFORE it is evaluated, under one lock, refusing
// when totpMaxFails guesses are already inside the window. A correct code hands the reservation
// back with release, so only wrong codes count -- and however concurrent the guesses, no more than
// totpMaxFails are ever evaluated per window.
func (l *totpGuessLimiter) reserve(userID int) (time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	keep := l.prune(userID)
	if len(keep) >= totpMaxFails {
		return time.Time{}, false
	}
	t := time.Now()
	l.fails[userID] = append(keep, t)
	return t, true
}

// release removes the reservation made at t (the guess was correct).
func (l *totpGuessLimiter) release(userID int, t time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fails := l.fails[userID]
	for i, f := range fails {
		if f.Equal(t) {
			l.fails[userID] = append(fails[:i:i], fails[i+1:]...)
			break
		}
	}
	l.prune(userID)
}

// checkTOTPGuess evaluates one TOTP code against the user's enabled TOTP under the per-user guess
// limit (D5), logging every wrong code and every refusal by user id (never the code). where names
// the page for the log.
func (a *App) checkTOTPGuess(u auth.User, code, where string) (valid, limited bool) {
	at, ok := totpGuesses.reserve(u.ID)
	if !ok {
		a.log.Printf("2FA: TOTP attempt refused at %s for user_id=%d: per-user guess limit reached", where, u.ID)
		return false, true
	}
	if strHasLen(code, 6, 6) && totp.Validate(code, u.TwofaKey.String) {
		totpGuesses.release(u.ID, at)
		return true, false
	}
	a.log.Printf("2FA: wrong TOTP code at %s for user_id=%d", where, u.ID)
	return false, false
}

func (l *totpGuessLimiter) reset() {
	l.mu.Lock()
	l.fails = map[int][]time.Time{}
	l.mu.Unlock()
}

// webauthnUser adapts a listmonk user and its stored passkeys to webauthn.User. The user handle
// is the bytes of the decimal user id, never the email.
type webauthnUser struct {
	user  auth.User
	rows  []auth.Passkey
	creds []webauthn.Credential
}

func (w *webauthnUser) WebAuthnID() []byte                         { return []byte(strconv.Itoa(w.user.ID)) }
func (w *webauthnUser) WebAuthnName() string                       { return w.user.Username }
func (w *webauthnUser) WebAuthnDisplayName() string                { return w.user.Name }
func (w *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return w.creds }

func (w *webauthnUser) descriptors() []protocol.CredentialDescriptor {
	out := make([]protocol.CredentialDescriptor, 0, len(w.creds))
	for _, c := range w.creds {
		out = append(out, c.Descriptor())
	}
	return out
}

// rowFor returns the stored row of a credential id.
func (w *webauthnUser) rowFor(credID []byte) (auth.Passkey, bool) {
	for _, r := range w.rows {
		if bytes.Equal(r.CredentialID, credID) {
			return r, true
		}
	}
	return auth.Passkey{}, false
}

// initWebAuthn builds the relying party from app.root_url (D2). An unbuildable relying party is
// logged and returned as an error; boot never fails on it, the passkey routes answer 503 and TOTP
// is unaffected.
func initWebAuthn(rootURL, siteName string) (*webauthn.WebAuthn, error) {
	rpID, origin, err := auth.RelyingPartyFromRootURL(rootURL)
	if err != nil {
		lo.Printf("passkeys disabled: cannot derive the relying party from app.root_url %q: %v", rootURL, err)
		return nil, err
	}
	if siteName == "" {
		siteName = rpID
	}
	w, err := webauthn.New(&webauthn.Config{
		RPID:                  rpID,
		RPDisplayName:         siteName,
		RPOrigins:             []string{origin},
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		},
	})
	if err != nil {
		lo.Printf("passkeys disabled: error building the relying party: %v", err)
		return nil, err
	}
	return w, nil
}

// passkeysAvailable answers 503 when there is no relying party.
func (a *App) passkeysAvailable() error {
	if a.webAuthn == nil {
		a.log.Printf("passkey request refused: no relying party (%v)", a.webAuthnErr)
		return echo.NewHTTPError(http.StatusServiceUnavailable, a.i18n.T("users.passkeyUnavailable"))
	}
	return nil
}

// loadWebAuthnUser loads the user's stored passkeys as library credentials.
func (a *App) loadWebAuthnUser(u auth.User) (*webauthnUser, error) {
	rows, err := a.core.GetPasskeys(u.ID)
	if err != nil {
		return nil, err
	}
	w := &webauthnUser{user: u, rows: rows}
	for _, r := range rows {
		var cr webauthn.Credential
		if err := json.Unmarshal(r.Credential, &cr); err != nil {
			a.log.Printf("error decoding passkey %d of user %d: %v", r.ID, u.ID, err)
			continue
		}
		w.creds = append(w.creds, cr)
	}
	return w, nil
}

// beginPasskeyLogin starts an assertion ceremony for the user and stores its state under key.
func (a *App) beginPasskeyLogin(u auth.User, key string) (*protocol.CredentialAssertion, error) {
	w, err := a.loadWebAuthnUser(u)
	if err != nil {
		return nil, err
	}
	if len(w.creds) == 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.passkeyNone"))
	}
	opts, sd, err := a.webAuthn.BeginLogin(w,
		webauthn.WithAllowedCredentials(w.descriptors()),
		webauthn.WithUserVerification(protocol.VerificationPreferred))
	if err != nil {
		a.log.Printf("error beginning passkey login for user %d: %v", u.ID, err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}
	tmptokens.Set(key, ceremonyTTL, *sd)
	return opts, nil
}

// finishPasskeyLogin consumes the ceremony state under key and validates the assertion against
// the user's stored credentials. On success the stored credential and last_used_at are updated.
// An assertion carrying the library's clone warning is refused and logged.
func (a *App) finishPasskeyLogin(u auth.User, key string, assertion []byte) error {
	data, err := tmptokens.Get(key)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidRequest"))
	}
	sd, ok := data.(webauthn.SessionData)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidRequest"))
	}
	w, err := a.loadWebAuthnUser(u)
	if err != nil {
		return err
	}
	if !bytes.Equal(sd.UserID, w.WebAuthnID()) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidRequest"))
	}

	parsed, err := protocol.ParseCredentialRequestResponseBytes(assertion)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.passkeyFailed"))
	}
	cred, err := a.webAuthn.ValidateLogin(w, sd, parsed)
	if err != nil {
		a.log.Printf("passkey assertion refused for user %d: %v", u.ID, err)
		return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("users.passkeyFailed"))
	}
	if cred.Authenticator.CloneWarning {
		a.log.Printf("passkey assertion refused for user %d: the authenticator's sign counter went backwards (possible cloned credential)", u.ID)
		return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("users.passkeyFailed"))
	}

	row, ok := w.rowFor(cred.ID)
	if !ok {
		return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("users.passkeyFailed"))
	}
	b, err := json.Marshal(cred)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}
	return a.core.UpdatePasskeyLogin(row.ID, u.ID, b)
}

// beginPasskeyRegistration starts a creation ceremony and stores its state under key.
func (a *App) beginPasskeyRegistration(u auth.User, key string) (*protocol.CredentialCreation, error) {
	w, err := a.loadWebAuthnUser(u)
	if err != nil {
		return nil, err
	}
	if len(w.rows) >= core.MaxPasskeys {
		return nil, echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("users.passkeyLimit", "num", strconv.Itoa(core.MaxPasskeys)))
	}
	opts, sd, err := a.webAuthn.BeginRegistration(w,
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		}),
		webauthn.WithExclusions(w.descriptors()))
	if err != nil {
		a.log.Printf("error beginning passkey registration for user %d: %v", u.ID, err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}
	tmptokens.Set(key, ceremonyTTL, *sd)
	return opts, nil
}

// finishPasskeyRegistration consumes the ceremony state under key, validates the attestation and
// stores the credential under the given (already validated) name.
func (a *App) finishPasskeyRegistration(u auth.User, key, name string, attestation []byte, rule core.FactorRule) (int, error) {
	data, err := tmptokens.Get(key)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidRequest"))
	}
	sd, ok := data.(webauthn.SessionData)
	if !ok {
		return 0, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidRequest"))
	}
	w, err := a.loadWebAuthnUser(u)
	if err != nil {
		return 0, err
	}
	if !bytes.Equal(sd.UserID, w.WebAuthnID()) {
		return 0, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.invalidRequest"))
	}

	parsed, err := protocol.ParseCredentialCreationResponseBytes(attestation)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.passkeyFailed"))
	}
	cred, err := a.webAuthn.CreateCredential(w, sd, parsed)
	if err != nil {
		a.log.Printf("passkey registration refused for user %d: %v", u.ID, err)
		return 0, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.passkeyFailed"))
	}
	b, err := json.Marshal(cred)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}
	return a.core.AddPasskey(u.ID, cred.ID, b, name, rule)
}

// cleanPasskeyName trims a passkey name and checks its length (1 to 64 characters).
func (a *App) cleanPasskeyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > passkeyNameMaxLen {
		return "", echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("users.passkeyName", "max", strconv.Itoa(passkeyNameMaxLen)))
	}
	return name, nil
}

// =================================================================
// Login: temp tokens bound to the browser.

// issueTwofaToken creates a challenge or enrol token for the user, sets the browser-binding nonce
// cookie, and redirects to the challenge or enrol page.
func (a *App) issueTwofaToken(c echo.Context, userID int, purpose string) error {
	token, err := generateRandomString(tmpAuthTokenLen)
	if err != nil {
		a.log.Printf("error generating 2FA token: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}
	nonce, err := generateRandomString(32)
	if err != nil {
		a.log.Printf("error generating 2FA nonce: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}

	ttl, page := twofaTokenTTL, "/login/twofa"
	if purpose == twofaPurposeEnroll {
		ttl, page = enrollTokenTTL, "/login/enroll"
	}
	tmptokens.Set(token, ttl, twofaToken{UserID: userID, Purpose: purpose, Nonce: nonce})
	a.setTwofaNonce(c, nonce, int(ttl.Seconds()))

	next := utils.SanitizeURI(c.FormValue("next"))
	return c.Redirect(http.StatusFound, fmt.Sprintf("%s%s?token=%s&next=%s", uriAdmin, page, token, url.QueryEscape(next)))
}

// setTwofaNonce sets (maxAge > 0) or clears (maxAge < 0) the nonce cookie, scoped to /admin/login.
func (a *App) setTwofaNonce(c echo.Context, nonce string, maxAge int) {
	c.SetCookie(&http.Cookie{
		Name:     twofaNonceCookie,
		Value:    nonce,
		Path:     uriLogin,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   a.urlCfg != nil && strings.HasPrefix(a.urlCfg.RootURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

// checkTwofaToken validates a challenge or enrol token: it must exist (a Check, which counts a
// try), carry a twofaToken of the given purpose, and match the nonce cookie of this browser.
func (a *App) checkTwofaToken(c echo.Context, token, purpose string) (twofaToken, bool) {
	if len(token) < tmpAuthTokenLen {
		return twofaToken{}, false
	}
	data, err := tmptokens.Check(token)
	if err != nil {
		return twofaToken{}, false
	}
	tk, ok := data.(twofaToken)
	if !ok || tk.Purpose != purpose || tk.UserID < 1 {
		return twofaToken{}, false
	}
	ck, err := c.Cookie(twofaNonceCookie)
	if err != nil || ck.Value == "" || subtle.ConstantTimeCompare([]byte(ck.Value), []byte(tk.Nonce)) != 1 {
		return twofaToken{}, false
	}
	return tk, true
}

// twofaUser reloads the token's user for a finish step and refuses a disabled one.
func (a *App) twofaUser(userID int) (auth.User, error) {
	u, err := a.core.GetUser(userID, "", "")
	if err != nil {
		return auth.User{}, echo.NewHTTPError(http.StatusForbidden, a.i18n.T("users.invalidRequest"))
	}
	if u.Status == auth.UserStatusDisabled {
		return auth.User{}, echo.NewHTTPError(http.StatusForbidden, a.i18n.T("users.invalidLogin"))
	}
	return u, nil
}

// completeTwofaLogin ends a challenge or enrolment: the token is deleted, the nonce cookie cleared
// and the session saved.
func (a *App) completeTwofaLogin(c echo.Context, token string, u auth.User) error {
	tmptokens.Delete(token)
	a.setTwofaNonce(c, "", -1)
	return a.auth.SaveSession(u, "", c)
}

// twofaNext is the local path a finished challenge or enrolment redirects to. Anything that is
// not a plain local path -- a backslash (browsers read one as a slash), a leading "//" (before or
// after SanitizeURI), or no leading "/" -- falls back to the admin home.
func twofaNext(next string) string {
	if raw := strings.TrimSpace(next); strings.HasPrefix(raw, "//") || strings.Contains(raw, `\`) {
		return uriAdmin
	}
	next = utils.SanitizeURI(next)
	if next == "" || next == "/" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.Contains(next, `\`) {
		return uriAdmin
	}
	// A control character (a tab or newline decoded from %09/%0A) is stripped by browsers when
	// they parse a URL, which would turn "/<tab>/host" into "//host". A "%" is refused as well --
	// this runs at page render and again at finish, and each pass decodes one more layer.
	for i := 0; i < len(next); i++ {
		if next[i] < 0x20 || next[i] == 0x7f || next[i] == '%' {
			return uriAdmin
		}
	}
	return next
}

// twofaJSONErr is a JSON route's answer to a missing, expired, used-up or foreign token: 401, which
// webauthn.js turns into a redirect to the login page.
func (a *App) twofaJSONErr() error {
	return echo.NewHTTPError(http.StatusUnauthorized, a.i18n.T("users.invalidRequest"))
}

func redirectResp(c echo.Context, next string) error {
	return c.JSON(http.StatusOK, okResp{struct {
		Redirect string `json:"redirect"`
	}{twofaNext(next)}})
}

// TwofaPasskeyBegin starts a passkey assertion for the login challenge.
func (a *App) TwofaPasskeyBegin(c echo.Context) error {
	if err := a.passkeysAvailable(); err != nil {
		return err
	}
	var req passkeyReq
	if err := c.Bind(&req); err != nil {
		return err
	}
	token := strings.TrimSpace(req.Token)
	tk, ok := a.checkTwofaToken(c, token, twofaPurposeChallenge)
	if !ok {
		return a.twofaJSONErr()
	}
	u, err := a.twofaUser(tk.UserID)
	if err != nil {
		return err
	}
	// The ceremony state lives under its own key: re-Setting the live token would reset its TTL
	// and its try counter.
	opts, err := a.beginPasskeyLogin(u, "wa:"+token)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{opts})
}

// TwofaPasskeyFinish verifies the assertion and, on success, saves the session.
func (a *App) TwofaPasskeyFinish(c echo.Context) error {
	if err := a.passkeysAvailable(); err != nil {
		return err
	}
	var req passkeyReq
	if err := c.Bind(&req); err != nil {
		return err
	}
	token := strings.TrimSpace(req.Token)
	tk, ok := a.checkTwofaToken(c, token, twofaPurposeChallenge)
	if !ok {
		return a.twofaJSONErr()
	}
	u, err := a.twofaUser(tk.UserID)
	if err != nil {
		return err
	}
	if err := a.finishPasskeyLogin(u, "wa:"+token, req.Credential); err != nil {
		return err
	}
	if err := a.completeTwofaLogin(c, token, u); err != nil {
		return err
	}
	return redirectResp(c, req.Next)
}

// EnrollPage renders the enrolment page (an enforced user with no factor, switch ON) and handles
// its TOTP form.
func (a *App) EnrollPage(c echo.Context) error {
	var token, next string
	if c.Request().Method == http.MethodPost {
		token = strings.TrimSpace(c.FormValue("token"))
		next = twofaNext(c.FormValue("next"))
	} else {
		token = strings.TrimSpace(c.QueryParam("token"))
		next = twofaNext(c.QueryParam("next"))
	}

	tk, ok := a.checkTwofaToken(c, token, twofaPurposeEnroll)
	if !ok {
		return c.Redirect(http.StatusFound, uriLogin)
	}
	u, err := a.twofaUser(tk.UserID)
	if err != nil {
		return c.Redirect(http.StatusFound, uriLogin)
	}

	// An enrol token never adds a factor to an account that has one (that path is step-up).
	if u.HasTwofaFactor() {
		tmptokens.Delete(token)
		return c.Redirect(http.StatusFound, uriLogin)
	}

	if c.Request().Method != http.MethodPost {
		return a.renderEnrollPage(c, u, token, next, "", "")
	}

	// The secret travels in a hidden field so a wrong code re-renders the SAME secret and QR.
	var (
		secret = strings.TrimSpace(c.FormValue("secret"))
		code   = strings.TrimSpace(c.FormValue("totp_code"))
	)
	if _, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret); err != nil || secret == "" {
		return a.renderEnrollPage(c, u, token, next, "", a.i18n.T("users.invalidRequest"))
	}
	if !strHasLen(code, 6, 6) || !totp.Validate(code, secret) {
		return a.renderEnrollPage(c, u, token, next, secret, a.i18n.T("users.invalidTOTPCode"))
	}
	if err := a.core.EnableTOTPFactor(u.ID, secret, core.FactorRule{Enrol: true}); err != nil {
		tmptokens.Delete(token)
		return c.Redirect(http.StatusFound, uriLogin)
	}
	a.endOtherSessions(u.ID, "")
	if err := a.completeTwofaLogin(c, token, u); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, next)
}

// renderEnrollPage renders enroll.html with a TOTP secret (a new one when secret is empty).
func (a *App) renderEnrollPage(c echo.Context, u auth.User, token, next, secret, errMsg string) error {
	opts := totp.GenerateOpts{Issuer: a.cfg.SiteName, AccountName: u.Email.String}
	if opts.Issuer == "" {
		opts.Issuer = "listmonk"
	}
	if opts.AccountName == "" {
		opts.AccountName = u.Username
	}
	if secret != "" {
		raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
		if err == nil {
			opts.Secret = raw
		}
	}
	key, err := totp.Generate(opts)
	if err != nil {
		a.log.Printf("error generating TOTP key: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}
	img, err := key.Image(200, 200)
	if err != nil {
		a.log.Printf("error generating QR code: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		a.log.Printf("error encoding QR code: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
	}

	return c.Render(http.StatusOK, "admin-enroll", enrollTpl{
		Title:   a.i18n.T("users.twoFAEnroll"),
		Token:   token,
		NextURI: next,
		Secret:  key.Secret(),
		// A data: URL; html/template would otherwise neutralise it in an src attribute.
		QR:    template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())),
		Error: errMsg,
	})
}

// EnrollPasskeyBegin starts a passkey registration for enrolment.
func (a *App) EnrollPasskeyBegin(c echo.Context) error {
	if err := a.passkeysAvailable(); err != nil {
		return err
	}
	var req passkeyReq
	if err := c.Bind(&req); err != nil {
		return err
	}
	token := strings.TrimSpace(req.Token)
	tk, ok := a.checkTwofaToken(c, token, twofaPurposeEnroll)
	if !ok {
		return a.twofaJSONErr()
	}
	u, err := a.twofaUser(tk.UserID)
	if err != nil {
		return err
	}
	if u.HasTwofaFactor() {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.twoFAHasFactor"))
	}
	opts, err := a.beginPasskeyRegistration(u, "wa:"+token)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{opts})
}

// EnrollPasskeyFinish stores the enrolment passkey and saves the session.
func (a *App) EnrollPasskeyFinish(c echo.Context) error {
	if err := a.passkeysAvailable(); err != nil {
		return err
	}
	var req passkeyReq
	if err := c.Bind(&req); err != nil {
		return err
	}
	token := strings.TrimSpace(req.Token)
	tk, ok := a.checkTwofaToken(c, token, twofaPurposeEnroll)
	if !ok {
		return a.twofaJSONErr()
	}
	u, err := a.twofaUser(tk.UserID)
	if err != nil {
		return err
	}
	name, err := a.cleanPasskeyName(req.Name)
	if err != nil {
		return err
	}
	if u.HasTwofaFactor() {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.twoFAHasFactor"))
	}
	if _, err := a.finishPasskeyRegistration(u, "wa:"+token, name, req.Credential, core.FactorRule{Enrol: true}); err != nil {
		return err
	}
	a.endOtherSessions(u.ID, "")
	if err := a.completeTwofaLogin(c, token, u); err != nil {
		return err
	}
	return redirectResp(c, req.Next)
}

// =================================================================
// In-session: step-up and factor changes (D6).

// requireStepUp refuses a request whose session carries no fresh step-up stamp. A token (API)
// request has no session and is always refused.
func (a *App) requireStepUp(c echo.Context) error {
	if !a.auth.HasStepUp(c) {
		return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("users.stepUpRequired"))
	}
	return nil
}

// stepUpPerms are the permissions whose routes need a fresh step-up stamp on a cookie session
// (integrations STEPUP-ADMIN-SPEC D1): every user, role and settings write. An API token has no
// session, so it is refused on them.
var stepUpPerms = map[string]bool{
	"users:manage":    true,
	"roles:manage":    true,
	"settings:manage": true,
}

// needsStepUp reports whether a route registered under perms is behind step-up: true when any of
// them is in stepUpPerms.
func needsStepUp(perms ...string) bool {
	for _, p := range perms {
		if stepUpPerms[p] {
			return true
		}
	}
	return false
}

// stepUpGate wraps a handler with the step-up check (integrations STEPUP-ADMIN-SPEC D1/D2). It
// runs after the permission check (the caller wraps the result in auth.Perm), so a stamp grants
// nothing. A request with no cookie session (an API token) gets its own message.
func (a *App) stepUpGate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if a.auth.HasStepUp(c) {
			return next(c)
		}
		if auth.GetSessionID(c) == "" {
			return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("users.stepUpNoToken"))
		}
		return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("users.stepUpRequired"))
	}
}

// keepLastRule is D7: with the switch ON, an enforced user cannot remove their last factor.
func (a *App) keepLastRule(u auth.User) core.FactorRule {
	return core.FactorRule{KeepLast: a.auth.RequireTwofa() && u.IsTwofaEnforced()}
}

// endOtherSessions deletes the user's sessions except keepID (all of them when keepID is "").
func (a *App) endOtherSessions(userID int, keepID string) {
	if err := a.core.DeleteUserSessions(userID, keepID); err != nil {
		a.log.Printf("error destroying sessions after a factor change for user_id=%d: %v", userID, err)
	}
}

// stepUpAttempt reserves a step-up attempt on the session before the guess is evaluated
// (auth.StepUpAttempt). When it is refused the session is gone, and the answer is the
// invalid-session error, which the admin UI turns into a redirect to the login page.
func (a *App) stepUpAttempt(c echo.Context, userID int) (int, error) {
	n, ok := a.auth.StepUpAttempt(c)
	if !ok {
		// Over the limit (the first such attempt is the one that destroys the session; later ones
		// in the same burst find it already condemned), or the session row is already gone or
		// unreadable (auth logs a database error itself).
		if n == auth.StepUpMaxFails+1 {
			a.log.Printf("2FA: session destroyed by the step-up attempt limit for user_id=%d", userID)
		} else {
			a.log.Printf("2FA: step-up attempt refused for user_id=%d (no usable session)", userID)
		}
		return n, echo.NewHTTPError(http.StatusForbidden, "invalid session")
	}
	return n, nil
}

// stepUpFail answers failed step-up attempt n. The StepUpMaxFails-th failure destroys the session
// and answers as an invalid session.
func (a *App) stepUpFail(c echo.Context, userID, n int, msg string) error {
	a.log.Printf("2FA: failed step-up attempt %d for user_id=%d", n, userID)
	if a.auth.StepUpFailed(c, n) {
		a.log.Printf("2FA: session destroyed by the step-up attempt limit for user_id=%d", userID)
		return echo.NewHTTPError(http.StatusForbidden, "invalid session")
	}
	return echo.NewHTTPError(http.StatusForbidden, msg)
}

// GetProfileTwofa returns the caller's factor state. Never any key material. stepup_ttl is the
// seconds left on the session's step-up stamp (integrations STEPUP-ADMIN-SPEC D4), 0 for none or
// for an API token; the browser skips the dialog on it, the server still checks every request.
func (a *App) GetProfileTwofa(c echo.Context) error {
	u := auth.GetUser(c)
	passkeys, err := a.core.GetPasskeys(u.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{struct {
		TOTP      bool           `json:"totp"`
		Passkeys  []auth.Passkey `json:"passkeys"`
		Required  bool           `json:"required"`
		Enforced  bool           `json:"enforced"`
		StepUpTTL int            `json:"stepup_ttl"`
	}{u.TwofaType == models.TwofaTypeTOTP, passkeys, u.TwofaRequired, u.TwofaEnforced, a.auth.StepUpRemaining(c)}})
}

// StepUp stamps the session after a TOTP code or, for a user with no factor at all, the password.
func (a *App) StepUp(c echo.Context) error {
	if auth.GetSessionID(c) == "" {
		return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("users.stepUpRequired"))
	}
	var (
		u        = auth.GetUser(c)
		code     = strings.TrimSpace(c.FormValue("totp_code"))
		password = c.FormValue("password")
	)

	// A malformed request consumes no attempt.
	if code == "" && password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "totp_code"))
	}

	// The attempt is counted before the guess is evaluated (at most StepUpMaxFails per session).
	n, err := a.stepUpAttempt(c, u.ID)
	if err != nil {
		return err
	}

	if code != "" {
		if u.TwofaType != models.TwofaTypeTOTP {
			return a.stepUpFail(c, u.ID, n, a.i18n.T("users.twoFANotEnabled"))
		}
		// Wrong codes here count toward the per-user TOTP limit too.
		valid, limited := a.checkTOTPGuess(u, code, "step-up")
		if limited {
			return a.stepUpFail(c, u.ID, n, a.i18n.T("users.totpLimited"))
		}
		if !valid {
			return a.stepUpFail(c, u.ID, n, a.i18n.T("users.invalidTOTPCode"))
		}
	} else {
		// A password proves nothing a session does not, once the user holds a factor.
		if u.HasTwofaFactor() {
			return a.stepUpFail(c, u.ID, n, a.i18n.T("users.stepUpNeedsFactor"))
		}
		// A VERIFY site (PASSWORD-POLICY-SPEC I3 as amended by PASSKEY-2FA-SPEC D9) -- upstream's
		// 8-character check, never validatePassword.
		if !strHasLen(password, 8, stdInputMaxLen) {
			return a.stepUpFail(c, u.ID, n, a.i18n.T("users.invalidPassword"))
		}
		ok, err := a.core.VerifyUserPassword(u.ID, password)
		if err != nil {
			return err
		}
		if !ok {
			return a.stepUpFail(c, u.ID, n, a.i18n.T("users.invalidPassword"))
		}
	}

	if err := a.auth.StampStepUp(c); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{true})
}

// StepUpPasskeyBegin starts a passkey assertion for step-up.
func (a *App) StepUpPasskeyBegin(c echo.Context) error {
	if err := a.passkeysAvailable(); err != nil {
		return err
	}
	sessID := auth.GetSessionID(c)
	if sessID == "" {
		return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("users.stepUpRequired"))
	}
	opts, err := a.beginPasskeyLogin(auth.GetUser(c), "wa:stepup:"+sessID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{opts})
}

// StepUpPasskeyFinish verifies the assertion and stamps the session.
func (a *App) StepUpPasskeyFinish(c echo.Context) error {
	if err := a.passkeysAvailable(); err != nil {
		return err
	}
	sessID := auth.GetSessionID(c)
	if sessID == "" {
		return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("users.stepUpRequired"))
	}
	var req passkeyReq
	if err := c.Bind(&req); err != nil {
		return err
	}
	u := auth.GetUser(c)
	n, err := a.stepUpAttempt(c, u.ID)
	if err != nil {
		return err
	}
	if err := a.finishPasskeyLogin(u, "wa:stepup:"+sessID, req.Credential); err != nil {
		return a.stepUpFail(c, u.ID, n, a.i18n.T("users.passkeyFailed"))
	}
	if err := a.auth.StampStepUp(c); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{true})
}

// AddPasskeyBegin starts registering an additional passkey from the profile (stamp required).
func (a *App) AddPasskeyBegin(c echo.Context) error {
	if err := a.requireStepUp(c); err != nil {
		return err
	}
	if err := a.passkeysAvailable(); err != nil {
		return err
	}
	opts, err := a.beginPasskeyRegistration(auth.GetUser(c), "wa:add:"+auth.GetSessionID(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{opts})
}

// AddPasskeyFinish stores the passkey (stamp required) and ends the user's other sessions.
func (a *App) AddPasskeyFinish(c echo.Context) error {
	if err := a.requireStepUp(c); err != nil {
		return err
	}
	if err := a.passkeysAvailable(); err != nil {
		return err
	}
	var req passkeyReq
	if err := c.Bind(&req); err != nil {
		return err
	}
	name, err := a.cleanPasskeyName(req.Name)
	if err != nil {
		return err
	}
	u := auth.GetUser(c)
	id, err := a.finishPasskeyRegistration(u, "wa:add:"+auth.GetSessionID(c), name, req.Credential, core.FactorRule{})
	if err != nil {
		return err
	}
	a.endOtherSessions(u.ID, auth.GetSessionID(c))
	return c.JSON(http.StatusOK, okResp{struct {
		ID int `json:"id"`
	}{id}})
}

// DeletePasskey deletes one of the caller's own passkeys (stamp required).
func (a *App) DeletePasskey(c echo.Context) error {
	if err := a.requireStepUp(c); err != nil {
		return err
	}
	u := auth.GetUser(c)
	if err := a.core.DeletePasskey(u.ID, getID(c), a.keepLastRule(u)); err != nil {
		return err
	}
	a.endOtherSessions(u.ID, auth.GetSessionID(c))
	return c.JSON(http.StatusOK, okResp{true})
}

// ResetUserTwofa is the admin recovery (D8): users:manage, a step-up stamp, never the caller's own
// account. It clears TOTP, every passkey and every session of the target.
func (a *App) ResetUserTwofa(c echo.Context) error {
	if err := a.requireStepUp(c); err != nil {
		return err
	}
	id := getID(c)
	if id == auth.GetUser(c).ID {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("users.twoFAResetSelf"))
	}
	if err := a.core.ResetTwofa(id); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{true})
}
