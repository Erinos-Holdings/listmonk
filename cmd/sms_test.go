package main

// Fork (SMS Admin) -- integrations listmonk/SMS-LINKS-ADMIN-SPEC I4, I5 and I6 (the proxy half), no
// database: the link handlers against an httptest admin API with static AWS credentials. I3 and
// the promo half of I6 are sms_db_test.go.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/i18n"
	"github.com/labstack/echo/v4"
)

const smsTestRegion = "us-west-2"

var smsTestNow = time.Date(2026, 11, 23, 19, 30, 0, 0, time.UTC)

// smsSeen is one request the fake admin API received.
type smsSeen struct {
	method, path, host string
	header             http.Header
	body               []byte
}

type smsFake struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	seen   []smsSeen
	status int
	reply  string
	delay  time.Duration
}

func newSMSFake(t *testing.T) *smsFake {
	f := &smsFake{t: t, status: http.StatusOK, reply: `{"status":"ok","slug":"tg-nov","target":"https://example.test/","short_url":"https://yq.link/tg-nov","resolved_target":"https://example.test/"}`}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.seen = append(f.seen, smsSeen{method: r.Method, path: r.URL.RequestURI(), host: r.Host, header: r.Header.Clone(), body: b})
		status, reply, delay := f.status, f.reply, f.delay
		f.mu.Unlock()
		if delay > 0 {
			time.Sleep(delay)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, reply)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *smsFake) last() smsSeen {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.seen) == 0 {
		f.t.Fatal("the admin API received no request")
	}
	return f.seen[len(f.seen)-1]
}

func (f *smsFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.seen)
}

func smsTestI18n(t *testing.T) *i18n.I18n {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "i18n", "en.json"))
	if err != nil {
		t.Fatal(err)
	}
	i, err := i18n.New(b)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

var smsTestCreds = credentials.NewStaticCredentialsProvider("AKIDSMSTEST", "secret-sms-test", "session-token-sms-test")

// smsTestApp is an App whose proxy targets the fake (the configured URL's shape is parsed by
// TestSMSAdminURL; here the endpoint is the fake's).
func smsTestApp(t *testing.T, f *smsFake, creds aws.CredentialsProvider) (*App, *bytes.Buffer) {
	t.Helper()
	var logged bytes.Buffer
	a := &App{i18n: smsTestI18n(t), log: log.New(&logged, "", 0)}
	if f != nil {
		s := newSMSAdmin(f.srv.URL+"/", smsTestRegion, creds)
		s.now = func() time.Time { return smsTestNow }
		a.smsAdmin = s
	}
	return a, &logged
}

// smsCall runs one handler as user "marketing" -- the status and the body (or the error message).
func smsCall(t *testing.T, fn echo.HandlerFunc, method, target, body, slug string) (int, string) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if slug != "" {
		c.SetParamNames("slug")
		c.SetParamValues(slug)
	}
	c.Set(auth.UserHTTPCtxKey, auth.User{Username: "marketing"})
	if err := fn(c); err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("handler: %v", err)
	}
	return rec.Code, rec.Body.String()
}

func jsonMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not a JSON object: %s", b)
	}
	return m
}

func TestSMSRoutes(t *testing.T) {
	b, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, re := range []string{
		`g\.GET\("/api/sms/links", pm\(a\.GetSMSLinks, "sms:manage"\)\)`,
		`g\.POST\("/api/sms/links", pm\(a\.CreateSMSLink, "sms:manage"\)\)`,
		`g\.PUT\("/api/sms/links/:slug", pm\(a\.UpdateSMSLink, "sms:manage"\)\)`,
		`g\.DELETE\("/api/sms/links/:slug", pm\(a\.DeleteSMSLink, "sms:manage"\)\)`,
		`g\.GET\("/api/sms/promo-codes", pm\(a\.GetSMSPromoCodes, "sms:manage"\)\)`,
		`g\.POST\("/api/sms/promo-codes", pm\(a\.CreateSMSPromoCode, "sms:manage"\)\)`,
		`g\.DELETE\("/api/sms/promo-codes/:id", pm\(hasID\(a\.DeleteSMSPromoCode\), "sms:manage"\)\)`,
	} {
		if !regexp.MustCompile(re).Match(b) {
			t.Fatalf("handlers.go does not register %s", re)
		}
	}
	// Every /api/sms/ route is one of the seven above (no route registered under another permission).
	if n := len(regexp.MustCompile(`"/api/sms/`).FindAll(b, -1)); n != 7 {
		t.Fatalf("handlers.go registers %d /api/sms/ routes, want 7", n)
	}
}

// D3 -- the accepted shape, group 1 the region; empty or ill-shaped is off with one log line.
func TestSMSAdminURL(t *testing.T) {
	for raw, want := range map[string]string{
		"https://exampleexampleexampleexampleexa.lambda-url.us-west-2.on.aws/": "us-west-2",
		"https://abc123.lambda-url.eu-central-1.on.aws":                        "eu-central-1",
	} {
		if got, reason := parseSMSAdminURL(raw); got != want || reason != "" {
			t.Errorf("%s: region %q (%s), want %q", raw, got, reason, want)
		}
	}
	for _, raw := range []string{
		"",
		"http://abc.lambda-url.us-west-2.on.aws/",
		"https://abc.lambda-url.us-west-2.on.aws/admin",
		"https://abc.lambda-url.us-west-2.on.aws.evil.test/",
		"https://ABC.lambda-url.us-west-2.on.aws/",
		"https://example.test/",
		" https://abc.lambda-url.us-west-2.on.aws/",
	} {
		if region, reason := parseSMSAdminURL(raw); region != "" || reason == "" {
			t.Errorf("%q accepted (region %q)", raw, region)
		}
	}

	var logged bytes.Buffer
	lo := log.New(&logged, "", 0)
	if s := initSMSAdmin("", lo); s != nil || !strings.Contains(logged.String(), "sms admin: off (sms.admin_url is empty)") {
		t.Fatalf("empty: %v %q", s, logged.String())
	}
	logged.Reset()
	if s := initSMSAdmin("https://example.test/", lo); s != nil || !strings.Contains(logged.String(), "sms admin: off (sms.admin_url is ill-shaped") {
		t.Fatalf("ill-shaped: %v %q", s, logged.String())
	}
	logged.Reset()
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDBOOT")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	s := initSMSAdmin("https://abc123.lambda-url.us-west-2.on.aws/", lo)
	if s == nil || s.region != "us-west-2" || s.creds == nil || !strings.Contains(logged.String(), "sms admin: configured, region us-west-2") {
		t.Fatalf("configured: %+v %q", s, logged.String())
	}
	if strings.Count(logged.String(), "\n") != 1 {
		t.Fatalf("want exactly one boot line, got %q", logged.String())
	}
}

// I4 -- only D6's typed fields reach the Lambda; created_by is the session user's on create and
// absent on update; each route maps to its admin action.
func TestSMSProxyForwardsTypedFields(t *testing.T) {
	f := newSMSFake(t)
	a, _ := smsTestApp(t, f, smsTestCreds)

	hostile := `"created_by": "forged", "clicks": 999, "previews": 5, "action": "delete", "mode": "update",
		"last_click_at": "2026-01-01T00:00:00Z", "unknown_key": {"x": 1}, "short_url": "https://evil.test/"`

	cases := []struct {
		name   string
		fn     echo.HandlerFunc
		method string
		slug   string
		body   string
		want   map[string]any
		status int
	}{
		{"POST with a slug -> put mode create", a.CreateSMSLink, http.MethodPost, "",
			`{"slug": "tg-nov", "target": "https://example.test/x", "utm_source": "sms", "utm_medium": "clerk", "utm_campaign": "bfcm",
			  "utm_term": "t", "utm_content": "c", "brand": "thirstygirl", "campaign": "BFCM", "notes": "n", "active": false, ` + hostile + `}`,
			map[string]any{"action": "put", "mode": "create", "slug": "tg-nov", "target": "https://example.test/x", "utm_source": "sms",
				"utm_medium": "clerk", "utm_campaign": "bfcm", "utm_term": "t", "utm_content": "c", "brand": "thirstygirl", "campaign": "BFCM",
				"notes": "n", "created_by": "listmonk:marketing"}, http.StatusCreated},
		{"POST without a slug -> create", a.CreateSMSLink, http.MethodPost, "",
			`{"target": "https://example.test/x", "utm_source": "sms", ` + hostile + `}`,
			map[string]any{"action": "create", "target": "https://example.test/x", "utm_source": "sms", "created_by": "listmonk:marketing"}, http.StatusCreated},
		{"POST with a blank slug -> create", a.CreateSMSLink, http.MethodPost, "",
			`{"slug": "  ", "target": "https://example.test/x"}`,
			map[string]any{"action": "create", "target": "https://example.test/x", "created_by": "listmonk:marketing"}, http.StatusCreated},
		{"PUT -> put mode update, the path's slug, no created_by", a.UpdateSMSLink, http.MethodPut, "tg-nov",
			`{"slug": "other", "target": "https://example.test/y", "utm_source": "sms", "active": false, ` + hostile + `}`,
			map[string]any{"action": "put", "mode": "update", "slug": "tg-nov", "target": "https://example.test/y", "utm_source": "sms", "active": false}, http.StatusOK},
		{"PUT reactivate", a.UpdateSMSLink, http.MethodPut, "tg-nov",
			`{"target": "https://example.test/y", "active": true}`,
			map[string]any{"action": "put", "mode": "update", "slug": "tg-nov", "target": "https://example.test/y", "active": true}, http.StatusOK},
		{"DELETE -> delete", a.DeleteSMSLink, http.MethodDelete, "tg-nov", ``,
			map[string]any{"action": "delete", "slug": "tg-nov"}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f.status = http.StatusOK
			if tc.status == http.StatusCreated {
				f.status = http.StatusCreated
			}
			code, body := smsCall(t, tc.fn, tc.method, "/api/sms/links", tc.body, tc.slug)
			if code != tc.status {
				t.Fatalf("status %d (%s), want %d", code, body, tc.status)
			}
			seen := f.last()
			if got := jsonMap(t, seen.body); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("forwarded %s\nwant      %v", seen.body, tc.want)
			}
			if seen.method != http.MethodPost || seen.path != "/" || seen.header.Get("Content-Type") != "application/json" {
				t.Fatalf("request %s %s %q", seen.method, seen.path, seen.header.Get("Content-Type"))
			}
			// The record comes back as okResp{data}, the Lambda's outcome word dropped.
			var out struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal([]byte(body), &out); err != nil || out.Data["short_url"] != "https://yq.link/tg-nov" {
				t.Fatalf("response %s", body)
			}
			if _, ok := out.Data["status"]; ok {
				t.Fatalf("response carries the Lambda status word: %s", body)
			}
		})
	}

	// GET -> list; the data is the Lambda's links array.
	f.status = http.StatusOK
	f.reply = `{"status":"ok","count":2,"links":[{"slug":"a1","clicks":3},{"slug":"b2","clicks":0}]}`
	code, body := smsCall(t, a.GetSMSLinks, http.MethodGet, "/api/sms/links", "", "")
	if code != http.StatusOK {
		t.Fatalf("list: %d %s", code, body)
	}
	if got := jsonMap(t, f.last().body); !reflect.DeepEqual(got, map[string]any{"action": "list"}) {
		t.Fatalf("list forwarded %v", got)
	}
	if body != `{"data":[{"slug":"a1","clicks":3},{"slug":"b2","clicks":0}]}`+"\n" {
		t.Fatalf("list body %q", body)
	}

	// A body that is not the typed shape is a 400 and nothing is forwarded.
	before := f.count()
	if code, _ := smsCall(t, a.CreateSMSLink, http.MethodPost, "/api/sms/links", `{"target": 7}`, ""); code != http.StatusBadRequest {
		t.Fatalf("a non-string target: %d", code)
	}
	if code, _ := smsCall(t, a.CreateSMSLink, http.MethodPost, "/api/sms/links", `not json`, ""); code != http.StatusBadRequest {
		t.Fatalf("not JSON: %d", code)
	}
	if f.count() != before {
		t.Fatal("a refused body reached the admin API")
	}
}

// I5 -- SigV4 for service lambda in the URL's region, the session token, the payload hash of the
// exact bytes sent; re-signing the received request with the same credential and time reproduces
// the Authorization header.
func TestSMSProxySigV4(t *testing.T) {
	f := newSMSFake(t)
	f.status = http.StatusCreated
	a, _ := smsTestApp(t, f, smsTestCreds)
	if code, body := smsCall(t, a.CreateSMSLink, http.MethodPost, "/api/sms/links", `{"slug":"tg-nov","target":"https://example.test/é"}`, ""); code != http.StatusCreated {
		t.Fatalf("%d %s", code, body)
	}
	seen := f.last()
	authz := seen.header.Get("Authorization")
	wantPrefix := "AWS4-HMAC-SHA256 Credential=AKIDSMSTEST/20261123/" + smsTestRegion + "/lambda/aws4_request, SignedHeaders="
	if !strings.HasPrefix(authz, wantPrefix) {
		t.Fatalf("Authorization %q, want prefix %q", authz, wantPrefix)
	}
	if got := seen.header.Get("X-Amz-Date"); got != "20261123T193000Z" {
		t.Fatalf("X-Amz-Date %q", got)
	}
	if got := seen.header.Get("X-Amz-Security-Token"); got != "session-token-sms-test" {
		t.Fatalf("X-Amz-Security-Token %q", got)
	}
	for _, h := range []string{"host", "x-amz-date", "x-amz-security-token", "content-type"} {
		if !strings.Contains(authz, h) {
			t.Fatalf("%s is not signed: %s", h, authz)
		}
	}
	if !strings.HasPrefix(f.srv.URL, "http://"+seen.host) {
		t.Fatalf("sent to %s, configured %s", seen.host, f.srv.URL)
	}

	// Re-sign what arrived (the exact bytes) and compare.
	req, _ := http.NewRequest(http.MethodPost, "http://"+seen.host+"/", bytes.NewReader(seen.body))
	req.Header.Set("Content-Type", "application/json")
	cr, _ := smsTestCreds.Retrieve(context.Background())
	sum := sha256.Sum256(seen.body)
	if err := v4.NewSigner().SignHTTP(context.Background(), cr, req, hex.EncodeToString(sum[:]), "lambda", smsTestRegion, smsTestNow); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Authorization"); got != authz {
		t.Fatalf("signature does not cover the sent bytes\n sent     %s\n expected %s", authz, got)
	}

	// A credential without a session token sends no X-Amz-Security-Token.
	a2, _ := smsTestApp(t, f, credentials.NewStaticCredentialsProvider("AKIDNOTOKEN", "s", ""))
	smsCall(t, a2.GetSMSLinks, http.MethodGet, "/api/sms/links", "", "")
	if seen := f.last(); seen.header.Get("X-Amz-Security-Token") != "" || !strings.Contains(seen.header.Get("Authorization"), "Credential=AKIDNOTOKEN/") {
		t.Fatalf("no-token request: %v", seen.header)
	}
}

type smsFailingCreds struct{}

func (smsFailingCreds) Retrieve(context.Context) (aws.Credentials, error) {
	return aws.Credentials{}, errors.New("imds-secret-cause: no role")
}

// I6 (proxy half) -- the error mapping.
func TestSMSProxyErrors(t *testing.T) {
	unavailable := smsTestI18n(t).T("sms.adminUnavailable")
	notConfigured := smsTestI18n(t).T("sms.notConfigured")
	if unavailable == "sms.adminUnavailable" || notConfigured == "sms.notConfigured" {
		t.Fatal("the i18n keys are missing from en.json")
	}

	long := strings.Repeat("é", 300)
	for _, tc := range []struct {
		status   int
		reply    string
		want     int
		wantBody string
	}{
		{400, `{"status":400,"error":"target must be https"}`, 400, "target must be https"},
		{400, `{"status":400,"error":"` + long + `"}`, 400, strings.Repeat("é", 200)},
		{404, `{"status":404,"error":"not found"}`, 404, "not found"},
		{409, `{"status":409,"error":"slug exists"}`, 409, "slug exists"},
		{403, `{"Message":"cause-403-forbidden"}`, 502, unavailable},
		{500, `cause-500-internal`, 502, unavailable},
		{503, `{"status":503,"error":"cause-503-no-free-code"}`, 502, unavailable},
		{200, `cause-not-json`, 502, unavailable},
	} {
		f := newSMSFake(t)
		f.status, f.reply = tc.status, tc.reply
		a, logged := smsTestApp(t, f, smsTestCreds)
		code, body := smsCall(t, a.CreateSMSLink, http.MethodPost, "/api/sms/links", `{"slug":"tg-nov","target":"https://example.test/"}`, "")
		if code != tc.want || body != tc.wantBody {
			t.Errorf("Lambda %d %s: got %d %q, want %d %q", tc.status, tc.reply, code, body, tc.want, tc.wantBody)
		}
		if tc.want == 502 {
			if strings.Contains(body, "cause") {
				t.Errorf("Lambda %d: the cause reached the body: %q", tc.status, body)
			}
			if !strings.Contains(logged.String(), "sms admin: put failed") {
				t.Errorf("Lambda %d: the cause was not logged: %q", tc.status, logged.String())
			}
		}
	}

	// A timeout.
	f := newSMSFake(t)
	f.delay = 300 * time.Millisecond
	a, logged := smsTestApp(t, f, smsTestCreds)
	a.smsAdmin.client.Timeout = 50 * time.Millisecond
	if code, body := smsCall(t, a.GetSMSLinks, http.MethodGet, "/api/sms/links", "", ""); code != 502 || body != unavailable {
		t.Errorf("timeout: %d %q", code, body)
	}
	if !strings.Contains(logged.String(), "sms admin: list failed") {
		t.Errorf("timeout not logged: %q", logged.String())
	}
	if smsAdminTimeout != 10*time.Second || newSMSAdmin("x", "r", nil).client.Timeout != 10*time.Second {
		t.Error("the proxy timeout is not 10 s")
	}

	// A transport error (nothing listening).
	dead := newSMSFake(t)
	a, _ = smsTestApp(t, dead, smsTestCreds)
	dead.srv.Close()
	if code, body := smsCall(t, a.DeleteSMSLink, http.MethodDelete, "/api/sms/links/x", "", "tg-nov"); code != 502 || body != unavailable {
		t.Errorf("transport: %d %q", code, body)
	}

	// A credential error: nothing is sent, the cause is logged and never returned.
	f = newSMSFake(t)
	a, logged = smsTestApp(t, f, smsFailingCreds{})
	if code, body := smsCall(t, a.UpdateSMSLink, http.MethodPut, "/api/sms/links/x", `{"target":"https://example.test/"}`, "tg-nov"); code != 502 || body != unavailable {
		t.Errorf("credentials: %d %q", code, body)
	}
	if f.count() != 0 {
		t.Error("a request was sent without credentials")
	}
	if !strings.Contains(logged.String(), "imds-secret-cause") {
		t.Errorf("credential cause not logged: %q", logged.String())
	}

	// Not configured: every link route is 503 sms.notConfigured and nothing is sent.
	a, _ = smsTestApp(t, nil, nil)
	for name, call := range map[string]func() (int, string){
		"GET":    func() (int, string) { return smsCall(t, a.GetSMSLinks, http.MethodGet, "/api/sms/links", "", "") },
		"POST":   func() (int, string) { return smsCall(t, a.CreateSMSLink, http.MethodPost, "/api/sms/links", `{"target":"https://example.test/"}`, "") },
		"PUT":    func() (int, string) { return smsCall(t, a.UpdateSMSLink, http.MethodPut, "/api/sms/links/x", `{"target":"https://example.test/"}`, "x") },
		"DELETE": func() (int, string) { return smsCall(t, a.DeleteSMSLink, http.MethodDelete, "/api/sms/links/x", "", "x") },
	} {
		if code, body := call(); code != http.StatusServiceUnavailable || body != notConfigured {
			t.Errorf("%s not configured: %d %q", name, code, body)
		}
	}
}
