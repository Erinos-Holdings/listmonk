package main

// Fork (campaign review) -- integrations ALT-ON-INSERT-SPEC I13: POST /api/campaigns/alts, the
// in-editor alt generation's relay. The route's permission (campaigns:manage_all or
// campaigns:manage, read off handlers.go -- a registration list with no test seam); 503 while the
// gate (app.review_url) is off; 413 over 2 MiB; 400 when `snapshot` is not a JSON object; a 200
// relays the Lambda's JSON as {data: ...} with the snapshot signed and forwarded verbatim; a Lambda
// failure is a 502; nothing is stored (the App holds no database: a store access would panic) and
// nothing of the body is logged. The one signing helper is internal/core/reviews_alts_test.go.

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/internal/review"
	"github.com/labstack/echo/v4"
)

func TestAltsDraftRoute(t *testing.T) {
	b, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	re := `g\.POST\("/api/campaigns/alts", pm\(a\.GenerateAltsDraft, "campaigns:manage_all", "campaigns:manage"\)\)`
	if !regexp.MustCompile(re).Match(b) {
		t.Fatalf("handlers.go does not register %s", re)
	}
}

func altsDraftApp(t *testing.T) (*App, *bytes.Buffer) {
	t.Helper()
	i, err := i18n.New([]byte(`{"_.code":"en","_.name":"English","campaigns.reviewNotConfigured":"off","campaigns.altsUnavailable":"unavailable","campaigns.altsTooLarge":"too large","globals.messages.invalidData":"invalid"}`))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	return &App{i18n: i, log: log.New(&buf, "", 0)}, &buf
}

func postAltsDraft(t *testing.T, a *App, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/alts", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return rec, a.GenerateAltsDraft(e.NewContext(req, rec))
}

func httpCode(err error) int {
	if he, ok := err.(*echo.HTTPError); ok {
		return he.Code
	}
	return 0
}

func TestGenerateAltsDraft(t *testing.T) {
	const secret = "review-test-secret"
	const snapshot = `{"content_type":"visual","body_source":"{\"root\":{}}","body":"<p>SECRET-BODY-TEXT</p>","attribs":{"lang":"es"},"status":"draft","name":"C"}`
	const answer = `{"status":"ok","fixes":[],"keep":{},"rejected":0,"lang":"es"}`

	var status int
	var gotBody []byte
	var gotTS, gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotTS = r.Header.Get("X-Review-Timestamp")
		gotSig = r.Header.Get("X-Review-Signature")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	t.Cleanup(func() {
		ko.Set("app.review_url", "")
		ko.Set("app.review_hmac_secret", "")
	})

	a, logs := altsDraftApp(t)

	// The gate off: 503, nothing sent.
	ko.Set("app.review_url", "")
	if _, err := postAltsDraft(t, a, `{"snapshot":`+snapshot+`}`); httpCode(err) != http.StatusServiceUnavailable {
		t.Fatalf("gate off: %v, want 503", err)
	}
	if gotBody != nil {
		t.Fatal("gate off: the Lambda was called")
	}

	ko.Set("app.review_url", srv.URL)
	ko.Set("app.review_hmac_secret", secret)

	// Over 2 MiB: 413, nothing sent.
	big := `{"snapshot":{"body":"` + strings.Repeat("a", core.MaxAltsSnapshotBytes) + `"}}`
	if _, err := postAltsDraft(t, a, big); httpCode(err) != http.StatusRequestEntityTooLarge {
		t.Fatalf("over 2 MiB: %v, want 413", err)
	}
	if gotBody != nil {
		t.Fatal("over 2 MiB: the Lambda was called")
	}

	// `snapshot` not an object (or not JSON at all): 400, nothing sent.
	for _, bad := range []string{`not json`, `{}`, `{"snapshot":null}`, `{"snapshot":[]}`, `{"snapshot":"x"}`, `{"snapshot":3}`} {
		if _, err := postAltsDraft(t, a, bad); httpCode(err) != http.StatusBadRequest {
			t.Fatalf("%s: %v, want 400", bad, err)
		}
	}
	if gotBody != nil {
		t.Fatal("a bad body: the Lambda was called")
	}

	// A 200: relayed as {data: <the Lambda's JSON>}; the snapshot signed and forwarded verbatim.
	status = http.StatusOK
	rec, err := postAltsDraft(t, a, `{"snapshot":`+snapshot+`}`)
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("200: %v %d", err, rec.Code)
	}
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || string(out.Data) != answer {
		t.Fatalf("relayed %s (%v), want {data: the Lambda's JSON}", rec.Body.String(), err)
	}
	var sent struct {
		Mode       string          `json:"mode"`
		Snapshot   json.RawMessage `json:"snapshot"`
		CampaignID *int            `json:"campaignId"`
	}
	if err := json.Unmarshal(gotBody, &sent); err != nil || sent.Mode != "alts" || string(sent.Snapshot) != snapshot || sent.CampaignID != nil {
		t.Fatalf("sent %s (%v)", gotBody, err)
	}
	ts, _ := strconv.ParseInt(gotTS, 10, 64)
	if want := review.Sign(secret, ts, gotBody); gotSig != want {
		t.Fatalf("X-Review-Signature = %q, want review.Sign = %q", gotSig, want)
	}

	// A Lambda failure: 502.
	for _, s := range []int{http.StatusBadRequest, http.StatusInternalServerError, http.StatusBadGateway} {
		status = s
		if _, err := postAltsDraft(t, a, `{"snapshot":`+snapshot+`}`); httpCode(err) != http.StatusBadGateway {
			t.Fatalf("Lambda %d: %v, want 502", s, err)
		}
	}

	// Nothing of the body is logged.
	if strings.Contains(logs.String(), "SECRET-BODY-TEXT") {
		t.Fatalf("the log carries the snapshot: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "draft alt generation failed") {
		t.Fatalf("the failure was not logged: %q", logs.String())
	}
}
