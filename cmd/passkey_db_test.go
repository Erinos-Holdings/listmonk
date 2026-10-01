package main

// Fork (two-factor) -- integrations PASSKEY-2FA-SPEC I6, I14, I15, I27 (the 503 half) and I28:
// the passkey ceremonies end to end with a software authenticator, every credential stored in and
// re-loaded from the database. Harness and helpers: twofa_db_test.go (LISTMONK_TEST_PG opt-in).

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/tmptokens"
	"github.com/pquerna/otp/totp"
)

const (
	challengeBegin  = "/admin/login/twofa/passkey/begin"
	challengeFinish = "/admin/login/twofa/passkey/finish"
)

func (h *twofaHarness) storedSignCount(id int) (int, bool) {
	var (
		raw  string
		used bool
	)
	h.db.QueryRow(`SELECT credential::TEXT, last_used_at IS NOT NULL FROM user_passkeys WHERE user_id = $1`, id).Scan(&raw, &used)
	var c struct {
		Authenticator struct {
			SignCount int `json:"signCount"`
		} `json:"authenticator"`
	}
	json.Unmarshal([]byte(raw), &c)
	return c.Authenticator.SignCount, used
}

// I6 -- the passkey challenge, with the credential stored in and re-loaded from the database.
func TestPasskeyChallenge(t *testing.T) {
	h := newTwofaHarness(t)
	id, k := h.passkeyUser("pkey", h.writerRole)
	_, otherKey := h.passkeyUser("other", h.writerRole)

	count0, used0 := h.storedSignCount(id)
	if used0 {
		t.Fatal("fixture: last_used_at already set")
	}

	// A real login hands out the challenge token and its nonce cookie.
	cl := h.client()
	tk := tokenOf(t, location(cl.login("pkey")))
	begin := func(cl *twofaClient, token string) json.RawMessage {
		t.Helper()
		rec := cl.json(http.MethodPost, challengeBegin, map[string]string{"token": token})
		if rec.Code != http.StatusOK {
			t.Fatalf("begin: %d %s", rec.Code, rec.Body.String())
		}
		return dataOf(t, rec)
	}
	finish := func(cl *twofaClient, token string, cred json.RawMessage) int {
		return cl.json(http.MethodPost, challengeFinish, map[string]any{"token": token, "next": twofaNextURI, "credential": cred}).Code
	}
	noSession := func(label string) {
		t.Helper()
		if h.sessionsOf(id) != 0 || cl.jar["session"] != "" {
			t.Fatalf("%s gave a session", label)
		}
	}

	// A wrong-origin assertion is refused.
	opts := begin(cl, tk)
	evil := h.rp
	evil.Origin = "https://evil.example.test"
	if code := finish(cl, tk, k.assert(t, evil, opts)); code != http.StatusForbidden {
		t.Fatalf("wrong origin: want 403, got %d", code)
	}
	noSession("a wrong-origin assertion")

	// The ceremony state is single-use: a valid assertion for the challenge the failed finish
	// consumed is refused (a replay of a challenge).
	if code := finish(cl, tk, k.assert(t, h.rp, opts)); code != http.StatusBadRequest {
		t.Fatalf("assertion for a consumed challenge: want 400, got %d", code)
	}
	noSession("a consumed challenge")

	// Another user's credential is refused.
	opts = begin(cl, tk)
	if code := finish(cl, tk, otherKey.assert(t, h.rp, opts)); code != http.StatusForbidden {
		t.Fatalf("another user's credential: want 403, got %d", code)
	}
	noSession("another user's credential")

	// Tokens that are not this browser's challenge token: an enrol token, a reset-token entry, a
	// ceremony entry, a missing or wrong nonce cookie.
	ecl := h.client()
	etk := ecl.mint(id, twofaPurposeEnroll, enrollTokenTTL)
	if rec := ecl.json(http.MethodPost, challengeBegin, map[string]string{"token": etk}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("enrol-purpose token on the challenge: %d", rec.Code)
	}
	resetKey := "a-reset-entry-keyed-by-a-long-email-address-0123456789@example.test"
	tmptokens.Set(resetKey, passwordResetTTL, "reset-token-value")
	if rec := cl.json(http.MethodPost, challengeBegin, map[string]string{"token": resetKey}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("reset-token entry on the challenge: %d", rec.Code)
	}
	if rec := cl.do(http.MethodGet, "/admin/login/twofa?token="+url.QueryEscape(resetKey), "", ""); rec.Code != http.StatusFound || location(rec) != uriLogin {
		t.Fatalf("reset-token entry on the challenge page: %d %q", rec.Code, location(rec))
	}
	begin(cl, tk) // leaves a ceremony entry under wa:<token>
	if rec := cl.json(http.MethodPost, challengeBegin, map[string]string{"token": "wa:" + tk}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("ceremony entry as a token: %d", rec.Code)
	}
	nonce := cl.jar[twofaNonceCookie]
	delete(cl.jar, twofaNonceCookie)
	if rec := cl.json(http.MethodPost, challengeBegin, map[string]string{"token": tk}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing nonce cookie: %d", rec.Code)
	}
	cl.jar[twofaNonceCookie] = "wrong-" + nonce
	if rec := cl.json(http.MethodPost, challengeFinish, map[string]any{"token": tk, "credential": json.RawMessage(`{}`)}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong nonce cookie: %d", rec.Code)
	}
	cl.jar[twofaNonceCookie] = nonce

	// A disabled user is refused at begin and at finish.
	opts = begin(cl, tk)
	h.db.MustExec(`UPDATE users SET status = 'disabled' WHERE id = $1`, id)
	if code := finish(cl, tk, k.assert(t, h.rp, opts)); code != http.StatusForbidden {
		t.Fatalf("disabled user, finish: want 403, got %d", code)
	}
	if rec := cl.json(http.MethodPost, challengeBegin, map[string]string{"token": tk}); rec.Code != http.StatusForbidden {
		t.Fatalf("disabled user, begin: want 403, got %d", rec.Code)
	}
	noSession("a disabled user")
	h.db.MustExec(`UPDATE users SET status = 'enabled' WHERE id = $1`, id)

	// A valid assertion yields a session, deletes the token, and updates the stored credential
	// (the authenticator's counter) and last_used_at.
	opts = begin(cl, tk)
	good := k.assert(t, h.rp, opts)
	rec := cl.json(http.MethodPost, challengeFinish, map[string]any{"token": tk, "next": twofaNextURI, "credential": good})
	if rec.Code != http.StatusOK {
		t.Fatalf("valid assertion: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Redirect string `json:"redirect"`
	}
	json.Unmarshal(dataOf(t, rec), &out)
	if out.Redirect != twofaNextURI || cl.jar["session"] == "" || h.sessionsOf(id) != 1 {
		t.Fatalf("valid assertion: redirect %q, cookie %q, sessions %d", out.Redirect, cl.jar["session"], h.sessionsOf(id))
	}
	if _, err := tmptokens.Check(tk); err == nil {
		t.Fatal("the challenge token survived a successful finish")
	}
	count1, used1 := h.storedSignCount(id)
	if !used1 || count1 != int(k.cred.Counter) || count1 == count0 {
		t.Fatalf("stored credential after login: signCount %d (was %d, authenticator at %d), last_used_at set %v", count1, count0, k.cred.Counter, used1)
	}

	// Replaying the successful assertion against a new challenge is refused.
	cl2 := h.client()
	tk2 := cl2.mint(id, twofaPurposeChallenge, twofaTokenTTL)
	begin(cl2, tk2)
	if code := finish(cl2, tk2, good); code != http.StatusForbidden {
		t.Fatalf("replayed assertion: want 403, got %d", code)
	}
	if cl2.jar["session"] != "" {
		t.Fatal("a replayed assertion gave a session")
	}
}

// I14 -- a passkey delete touches only the caller's own rows.
func TestDeletePasskeyIsOwnerScoped(t *testing.T) {
	h := newTwofaHarness(t)
	a, _ := h.passkeyUser("alice", h.writerRole)
	b, _ := h.passkeyUser("bob", h.writerRole)
	var bobPK, alicePK int
	h.db.Get(&bobPK, `SELECT id FROM user_passkeys WHERE user_id = $1`, b)
	h.db.Get(&alicePK, `SELECT id FROM user_passkeys WHERE user_id = $1`, a)

	h.setTOTP(a)
	cl := h.sessionClient(a)
	if rec := cl.stepUpTOTP(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d %s", rec.Code, rec.Body.String())
	}
	if rec := cl.do(http.MethodDelete, "/api/profile/passkeys/"+strconv.Itoa(bobPK), "", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete another user's passkey: want 404, got %d", rec.Code)
	}
	if h.passkeysOf(b) != 1 {
		t.Fatal("another user's passkey was deleted")
	}
	if err := h.app.core.DeletePasskey(a, bobPK, core.FactorRule{}); err == nil || h.passkeysOf(b) != 1 {
		t.Fatal("core.DeletePasskey deleted a row the caller does not own")
	}
	if rec := cl.do(http.MethodDelete, "/api/profile/passkeys/"+strconv.Itoa(alicePK), "", ""); rec.Code != http.StatusOK {
		t.Fatalf("delete own passkey: %d %s", rec.Code, rec.Body.String())
	}
	if h.passkeysOf(a) != 0 || h.passkeysOf(b) != 1 {
		t.Fatalf("after deleting own: alice %d, bob %d", h.passkeysOf(a), h.passkeysOf(b))
	}
}

// I15 -- an assertion carrying the library's clone warning (the sign counter went backwards) is
// refused.
func TestCloneWarningRefused(t *testing.T) {
	h := newTwofaHarness(t)
	id, k := h.passkeyUser("pkey", h.writerRole)

	login := func(counter uint32) int {
		t.Helper()
		cl := h.client()
		tk := cl.mint(id, twofaPurposeChallenge, twofaTokenTTL)
		rec := cl.json(http.MethodPost, challengeBegin, map[string]string{"token": tk})
		if rec.Code != http.StatusOK {
			t.Fatalf("begin: %d", rec.Code)
		}
		k.cred.Counter = counter - 1 // assert increments it
		return cl.json(http.MethodPost, challengeFinish, map[string]any{"token": tk, "credential": k.assert(t, h.rp, dataOf(t, rec))}).Code
	}

	if code := login(10); code != http.StatusOK {
		t.Fatalf("counter 10: %d", code)
	}
	if n, _ := h.storedSignCount(id); n != 10 {
		t.Fatalf("stored counter %d, want 10", n)
	}
	sessions := h.sessionsOf(id)
	if code := login(5); code != http.StatusForbidden {
		t.Fatalf("counter regressed to 5: want 403, got %d", code)
	}
	if h.sessionsOf(id) != sessions {
		t.Fatal("a cloned-credential assertion gave a session")
	}
	if n, _ := h.storedSignCount(id); n != 10 {
		t.Fatalf("a refused assertion changed the stored counter to %d", n)
	}
}

// I27 (routes) -- with no relying party every passkey route answers 503, and a TOTP challenge
// still works.
func TestPasskeyRoutes503WithoutRelyingParty(t *testing.T) {
	h := newTwofaHarness(t)
	id := h.user("totpuser", h.writerRole)
	h.setTOTP(id)

	h.app.webAuthn, h.app.webAuthnErr = initWebAuthn("not a url", "x")
	if h.app.webAuthn != nil || h.app.webAuthnErr == nil {
		t.Fatal("initWebAuthn accepted an unparseable root URL")
	}

	cl := h.sessionClient(id)
	if rec := cl.stepUpTOTP(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d", rec.Code)
	}
	lcl := h.client()
	ctk := lcl.mint(id, twofaPurposeChallenge, twofaTokenTTL)
	etk := lcl.mint(id, twofaPurposeEnroll, enrollTokenTTL)
	for _, r := range []struct {
		cl   *twofaClient
		path string
		body map[string]any
	}{
		{lcl, challengeBegin, map[string]any{"token": ctk}},
		{lcl, challengeFinish, map[string]any{"token": ctk, "credential": json.RawMessage(`{}`)}},
		{lcl, "/admin/login/enroll/passkey/begin", map[string]any{"token": etk}},
		{lcl, "/admin/login/enroll/passkey/finish", map[string]any{"token": etk, "name": "x", "credential": json.RawMessage(`{}`)}},
		{cl, "/api/profile/twofa/stepup/passkey/begin", nil},
		{cl, "/api/profile/twofa/stepup/passkey/finish", map[string]any{"credential": json.RawMessage(`{}`)}},
		{cl, "/api/profile/passkeys/begin", nil},
		{cl, "/api/profile/passkeys/finish", map[string]any{"name": "x", "credential": json.RawMessage(`{}`)}},
	} {
		if rec := r.cl.json(http.MethodPost, r.path, r.body); rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s without a relying party: want 503, got %d %s", r.path, rec.Code, rec.Body.String())
		}
	}

	// TOTP is unaffected: a real login and the challenge's code.
	tcl := h.client()
	loc := location(tcl.login("totpuser"))
	if !strings.HasPrefix(loc, "/admin/login/twofa?") {
		t.Fatalf("login: %q", loc)
	}
	code, _ := totp.GenerateCode(twofaSecret, time.Now())
	rec := tcl.form(http.MethodPost, "/admin/login/twofa", url.Values{"token": {tokenOf(t, loc)}, "next": {twofaNextURI}, "totp_code": {code}})
	if rec.Code != http.StatusFound || location(rec) != twofaNextURI || tcl.jar["session"] == "" {
		t.Fatalf("TOTP challenge without a relying party: %d %q, cookie %q", rec.Code, location(rec), tcl.jar["session"])
	}
}

// I28 -- the 11th passkey is refused; an empty or 65-character name is refused.
func TestPasskeyCapAndName(t *testing.T) {
	h := newTwofaHarness(t)
	id := h.user("many", h.writerRole)
	cl := h.sessionClient(id)
	if rec := cl.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d", rec.Code)
	}

	// Names: checked before the ceremony state is consumed.
	k := newVKey(id)
	rec := cl.json(http.MethodPost, "/api/profile/passkeys/begin", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("begin: %d %s", rec.Code, rec.Body.String())
	}
	cred := k.attest(t, h.rp, dataOf(t, rec))
	for _, name := range []string{"", "   ", strings.Repeat("n", 65)} {
		if rec := cl.json(http.MethodPost, "/api/profile/passkeys/finish", map[string]any{"name": name, "credential": cred}); rec.Code != http.StatusBadRequest {
			t.Fatalf("name %q: want 400, got %d", name, rec.Code)
		}
	}
	if h.passkeysOf(id) != 0 {
		t.Fatal("a refused name stored a passkey")
	}
	if rec := cl.json(http.MethodPost, "/api/profile/passkeys/finish", map[string]any{"name": strings.Repeat("n", 64), "credential": cred}); rec.Code != http.StatusOK {
		t.Fatalf("64-character name: %d %s", rec.Code, rec.Body.String())
	}

	// Up to 10 (the other 9 inserted directly), then the 11th is refused at begin and in core.
	for i := 2; i <= 10; i++ {
		h.db.MustExec(`INSERT INTO user_passkeys (user_id, credential_id, credential, name) VALUES ($1, $2, '{}', $3)`, id, []byte{byte(i), 0xAA}, "k"+strconv.Itoa(i))
	}
	if h.passkeysOf(id) != core.MaxPasskeys {
		t.Fatalf("fixture: %d passkeys", h.passkeysOf(id))
	}
	if rec := cl.json(http.MethodPost, "/api/profile/passkeys/begin", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("11th passkey begin: want 400, got %d", rec.Code)
	}
	if _, err := h.app.core.AddPasskey(id, []byte{0xFF}, []byte(`{}`), "eleventh", core.FactorRule{}); err == nil {
		t.Fatal("core.AddPasskey stored an 11th passkey")
	}
	if h.passkeysOf(id) != core.MaxPasskeys {
		t.Fatalf("%d passkeys after the refusals", h.passkeysOf(id))
	}
}
