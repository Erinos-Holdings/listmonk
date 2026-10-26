package main

// Fork (on-demand brand-health refresh) -- integrations BRANDHEALTH-REFRESH-SPEC I3 (the handler
// half) and I7 (the secret setting), against a real database through the real router
// (twofaHarness, built by initHTTPHandlers), so pm(), the session middleware and the settings
// handlers are the production ones. LISTMONK_TEST_PG opt-in. The non-DB half (the signing recipe,
// the 202/429 mapping, the 8 s client) is internal/core/health_refresh_test.go.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/review"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

const hrRoute = "/api/system/health/refresh"

// hrSettings sets the two live keys for the test (the handler reads koanf on every call).
func hrSettings(t *testing.T, url, secret string) {
	t.Helper()
	ko.Set("app.health_refresh_url", url)
	ko.Set("app.health_refresh_hmac_secret", secret)
	t.Cleanup(func() {
		ko.Set("app.health_refresh_url", "")
		ko.Set("app.health_refresh_hmac_secret", "")
	})
}

// hrUser is a cookie session of a user whose role carries exactly perms.
func hrUser(t *testing.T, h *twofaHarness, name string, perms ...string) *twofaClient {
	t.Helper()
	var role int
	h.db.Get(&role, `INSERT INTO roles (type, name, permissions) VALUES ('user', $1, $2) RETURNING id`, name+" role", pq.StringArray(perms))
	id := h.user(name, role)
	if _, err := cacheUsers(h.app.core, h.app.auth); err != nil {
		t.Fatal(err)
	}
	return h.sessionClient(id)
}

// I3 -- brands:manage is enforced (a brands:get holder is 403, the Lambda never called); the
// route is not step-up gated; not configured -> 400 notConfigured; configured -> one signed POST
// whose X-Health-Signature is review.Sign over ts.body; 202 -> {started: true}; 429 ->
// {started: false, busy: true}; 401/503/500/timeout -> 502 refreshUnavailable with no reason
// leaked; serverConfig carries only the boolean.
func TestHealthRefreshEndpoint(t *testing.T) {
	h := newTwofaHarness(t)
	const secret = "health-db-test-secret"

	var calls atomic.Int32
	var status atomic.Int32
	var stall atomic.Bool
	var lastSigOK atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body [512]byte
		n, _ := r.Body.Read(body[:])
		ts := r.Header.Get("X-Health-Timestamp")
		var tsn int64
		if err := json.Unmarshal([]byte(ts), &tsn); err == nil {
			lastSigOK.Store(r.Header.Get("X-Health-Signature") == review.Sign(secret, tsn, body[:n]))
		}
		if stall.Load() {
			time.Sleep(300 * time.Millisecond)
		}
		w.WriteHeader(int(status.Load()))
	}))
	defer srv.Close()

	if needsStepUp("brands:manage") {
		t.Fatal("brands:manage is step-up gated; the refresh must not be (D2)")
	}

	reader := hrUser(t, h, "hrreader", "brands:get")
	admin := hrUser(t, h, "hradmin", "brands:get", "brands:manage")

	// Not configured: 400, for the URL empty and for the secret empty.
	for _, c := range []struct{ url, secret string }{{"", ""}, {"", secret}, {srv.URL, ""}} {
		hrSettings(t, c.url, c.secret)
		rec := admin.do(http.MethodPost, hrRoute, echo.MIMEApplicationJSON, "{}")
		if rec.Code != http.StatusBadRequest || message(rec) != h.app.i18n.T("dashboard.ses.refreshNotConfigured") {
			t.Fatalf("not configured (%q, secret %v): %d %s", c.url, c.secret != "", rec.Code, rec.Body.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("the Lambda was called %d times while not configured", calls.Load())
	}

	hrSettings(t, srv.URL, secret)
	status.Store(http.StatusAccepted)

	// brands:get alone: 403, nothing sent.
	if rec := reader.do(http.MethodPost, hrRoute, echo.MIMEApplicationJSON, "{}"); rec.Code != http.StatusForbidden {
		t.Fatalf("brands:get holder: %d %s, want 403", rec.Code, rec.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatal("a refused request reached the Lambda")
	}

	ok := func(what string, want core.HealthRefreshResult) {
		t.Helper()
		before := calls.Load()
		rec := admin.do(http.MethodPost, hrRoute, echo.MIMEApplicationJSON, "{}")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", what, rec.Code, rec.Body.String())
		}
		var got core.HealthRefreshResult
		if err := json.Unmarshal(dataOf(t, rec), &got); err != nil || got != want {
			t.Fatalf("%s: %s, want %+v", what, rec.Body.String(), want)
		}
		if calls.Load() != before+1 || !lastSigOK.Load() {
			t.Fatalf("%s: calls %d -> %d, signature ok %v", what, before, calls.Load(), lastSigOK.Load())
		}
	}
	ok("202", core.HealthRefreshResult{Started: true})
	status.Store(http.StatusTooManyRequests)
	ok("429", core.HealthRefreshResult{Started: false, Busy: true})

	for _, s := range []int{http.StatusUnauthorized, http.StatusServiceUnavailable, http.StatusInternalServerError} {
		status.Store(int32(s))
		rec := admin.do(http.MethodPost, hrRoute, echo.MIMEApplicationJSON, "{}")
		if rec.Code != http.StatusBadGateway || message(rec) != h.app.i18n.T("dashboard.ses.refreshUnavailable") {
			t.Fatalf("Lambda %d: %d %s, want 502 refreshUnavailable", s, rec.Code, rec.Body.String())
		}
	}

	// A stalled Lambda: the client's timeout ends it as a 502.
	healthRefreshHTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
	t.Cleanup(func() { healthRefreshHTTPClient = nil })
	stall.Store(true)
	status.Store(http.StatusAccepted)
	if rec := admin.do(http.MethodPost, hrRoute, echo.MIMEApplicationJSON, "{}"); rec.Code != http.StatusBadGateway {
		t.Fatalf("timeout: %d %s, want 502", rec.Code, rec.Body.String())
	}
	stall.Store(false)

	// serverConfig carries the boolean only: healthRefreshEnabled() follows the two keys, and the
	// marshalled struct holds neither the URL nor the secret. (GET /api/config itself needs the
	// app's static filesystem, which the harness does not build.)
	if !healthRefreshEnabled() {
		t.Fatal("healthRefreshEnabled() false while configured")
	}
	b, _ := json.Marshal(serverConfig{HealthRefreshEnabled: healthRefreshEnabled()})
	if !strings.Contains(string(b), `"health_refresh_enabled":true`) || strings.Contains(string(b), srv.URL) || strings.Contains(string(b), secret) {
		t.Fatalf("serverConfig JSON: %s", b)
	}
	hrSettings(t, "", "")
	if healthRefreshEnabled() {
		t.Fatal("healthRefreshEnabled() true with the URL empty")
	}
}

// I7 -- the refresh HMAC secret (and, retro-pinned, the review HMAC secret) is a SECRET setting at
// all three cmd/settings.go sites: masked on GET /api/settings (a settings:get reader never sees
// it), a full-settings save that round-trips the GET body keeps the stored secret (the mask is
// never persisted), a per-key PUT of the mask keeps it, and a per-key PUT of a new value stores it.
func TestHealthRefreshSecretIsMasked(t *testing.T) {
	f := newSUFixture(t)
	h := f.h
	const refreshSecret, reviewSecret = "refresh-s3cret-value", "review-s3cret-value"
	h.db.MustExec(`UPDATE settings SET value = to_jsonb($1::TEXT) WHERE key = 'app.health_refresh_hmac_secret'`, refreshSecret)
	h.db.MustExec(`UPDATE settings SET value = to_jsonb($1::TEXT) WHERE key = 'app.review_hmac_secret'`, reviewSecret)
	stored := func(key string) string {
		var v string
		h.db.Get(&v, `SELECT value #>> '{}' FROM settings WHERE key = $1`, key)
		return v
	}

	// GET as the Super Admin and as a settings:get-only reader: masked, never the plaintext.
	reader := h.sessionClient(h.user("hrsettingsreader", h.readerRole))
	boss := h.sessionClient(f.boss)
	var body string
	for name, cl := range map[string]*twofaClient{"reader": reader, "boss": boss} {
		rec := cl.do(http.MethodGet, "/api/settings", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/settings as %s: %d %s", name, rec.Code, rec.Body.String())
		}
		if s := rec.Body.String(); strings.Contains(s, refreshSecret) || strings.Contains(s, reviewSecret) {
			t.Fatalf("GET /api/settings as %s leaks a secret: %s", name, s)
		}
		var m map[string]json.RawMessage
		json.Unmarshal(dataOf(t, rec), &m)
		for key, secret := range map[string]string{"app.health_refresh_hmac_secret": refreshSecret, "app.review_hmac_secret": reviewSecret} {
			var got string
			json.Unmarshal(m[key], &got)
			if got != core.MaskSecret(secret) {
				t.Fatalf("GET as %s: %s = %q, want the mask", name, key, got)
			}
		}
		if name == "boss" {
			body = string(dataOf(t, rec))
		}
	}

	if rec := boss.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d %s", rec.Code, rec.Body.String())
	}

	// Full save of the GET body (the mask round-trips): stored secrets unchanged.
	f.drainReload()
	if rec := boss.do(http.MethodPut, "/api/settings", echo.MIMEApplicationJSON, body); rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings: %d %s", rec.Code, rec.Body.String())
	}
	if stored("app.health_refresh_hmac_secret") != refreshSecret || stored("app.review_hmac_secret") != reviewSecret {
		t.Fatalf("full save persisted the mask: refresh %q review %q", stored("app.health_refresh_hmac_secret"), stored("app.review_hmac_secret"))
	}

	// Per-key save of the mask: unchanged; of a new value: stored.
	mask, _ := json.Marshal(core.MaskSecret(refreshSecret))
	if rec := boss.do(http.MethodPut, "/api/settings/app.health_refresh_hmac_secret", echo.MIMEApplicationJSON, string(mask)); rec.Code != http.StatusOK {
		t.Fatalf("PUT per-key mask: %d %s", rec.Code, rec.Body.String())
	}
	if got := stored("app.health_refresh_hmac_secret"); got != refreshSecret {
		t.Fatalf("per-key save persisted the mask: %q", got)
	}
	if rec := boss.do(http.MethodPut, "/api/settings/app.health_refresh_hmac_secret", echo.MIMEApplicationJSON, `"rotated-value"`); rec.Code != http.StatusOK {
		t.Fatalf("PUT per-key new value: %d %s", rec.Code, rec.Body.String())
	}
	if got := stored("app.health_refresh_hmac_secret"); got != "rotated-value" {
		t.Fatalf("per-key new value not stored: %q", got)
	}
	f.drainReload()
}
