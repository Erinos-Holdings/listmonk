package core

// Fork (campaign review) -- integrations ALT-AUTOGEN-SPEC I10: POST /api/campaigns/:id/alts' poster.
// Unconfigured (empty app.review_url) is ErrReviewNotConfigured (the handler's 503); the body
// `{"mode":"alts","campaignId":N}` is signed with review.Sign over ts + "." + body (the pinned
// vector's recipe, the same as a job); a 200 is relayed verbatim; a non-200, a transport error and a
// non-JSON body are errors (the handler's 502); the client timeout is 28 s. No database.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/review"
)

func TestRequestAltsUnconfigured(t *testing.T) {
	if _, err := RequestAlts(ReviewConfig{}, 179); !errors.Is(err, ErrReviewNotConfigured) {
		t.Fatalf("empty URL: %v, want ErrReviewNotConfigured", err)
	}
}

func TestRequestAltsSignsAndRelays(t *testing.T) {
	const secret = "review-test-secret"
	at := time.Unix(1790000000, 0)
	const answer = `{"status":"ok","fixes":[{"kind":"alt.set","block":"a","from":"","to":"Shop","undecided":true,"src":"https://e/a.jpg"}],"keep":{},"rejected":0}`

	var status int
	var reply string
	var gotTS, gotSig, gotCT, gotMethod string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotTS = r.Header.Get("X-Review-Timestamp")
		gotSig = r.Header.Get("X-Review-Signature")
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	defer srv.Close()
	cfg := ReviewConfig{URL: srv.URL, Secret: secret, Now: func() time.Time { return at }}

	status, reply = http.StatusOK, answer
	out, err := RequestAlts(cfg, 179)
	if err != nil {
		t.Fatalf("200: %v", err)
	}
	if string(out) != answer {
		t.Fatalf("relayed %s, want the Lambda's JSON verbatim", out)
	}
	if gotMethod != http.MethodPost || gotCT != "application/json" {
		t.Fatalf("method %s content-type %s", gotMethod, gotCT)
	}
	var b struct {
		Mode       string `json:"mode"`
		CampaignID int    `json:"campaignId"`
	}
	if err := json.Unmarshal(gotBody, &b); err != nil || b.Mode != "alts" || b.CampaignID != 179 {
		t.Fatalf("body %s (%v)", gotBody, err)
	}
	if gotTS != strconv.FormatInt(at.Unix(), 10) {
		t.Fatalf("X-Review-Timestamp = %q", gotTS)
	}
	if want := review.Sign(secret, at.Unix(), gotBody); gotSig != want {
		t.Fatalf("X-Review-Signature = %q, want review.Sign = %q", gotSig, want)
	}

	for _, s := range []int{http.StatusAccepted, http.StatusBadRequest, http.StatusUnauthorized, http.StatusInternalServerError, http.StatusBadGateway} {
		status, reply = s, `{"error":"x"}`
		if _, err := RequestAlts(cfg, 179); err == nil {
			t.Fatalf("%d: no error", s)
		}
	}
	status, reply = http.StatusOK, "not json"
	if _, err := RequestAlts(cfg, 179); err == nil {
		t.Fatalf("a 200 with no JSON: no error")
	}
}

// integrations ALT-ON-INSERT-SPEC I13: the snapshot body `{"mode":"alts","snapshot":{...}}` is
// signed and relayed exactly as the campaign-id body -- by the one helper, postAlts: unconfigured is
// ErrReviewNotConfigured, the snapshot is forwarded verbatim, the signature is review.Sign over the
// body, a non-200 and a non-JSON answer are errors; and both RequestAlts and RequestAltsSnapshot
// call postAlts, which is the only alts code that sets the signature header.
func TestRequestAltsSnapshotOneSigner(t *testing.T) {
	const secret = "review-test-secret"
	at := time.Unix(1790000000, 0)
	snapshot := json.RawMessage(`{"content_type":"visual","body_source":"{}","body":"<p>x</p>","attribs":null,"status":"draft","name":"C"}`)
	if _, err := RequestAltsSnapshot(ReviewConfig{}, snapshot); !errors.Is(err, ErrReviewNotConfigured) {
		t.Fatalf("empty URL: %v, want ErrReviewNotConfigured", err)
	}

	var status int
	var gotTS, gotSig string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTS = r.Header.Get("X-Review-Timestamp")
		gotSig = r.Header.Get("X-Review-Signature")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"status":"none","fixes":[],"keep":{},"rejected":0,"lang":"en"}`))
	}))
	defer srv.Close()
	cfg := ReviewConfig{URL: srv.URL, Secret: secret, Now: func() time.Time { return at }}

	status = http.StatusOK
	out, err := RequestAltsSnapshot(cfg, snapshot)
	if err != nil || string(out) != `{"status":"none","fixes":[],"keep":{},"rejected":0,"lang":"en"}` {
		t.Fatalf("200: %s %v", out, err)
	}
	var b struct {
		Mode     string          `json:"mode"`
		Snapshot json.RawMessage `json:"snapshot"`
	}
	if err := json.Unmarshal(gotBody, &b); err != nil || b.Mode != "alts" || string(b.Snapshot) != string(snapshot) {
		t.Fatalf("body %s (%v)", gotBody, err)
	}
	if gotTS != strconv.FormatInt(at.Unix(), 10) || gotSig != review.Sign(secret, at.Unix(), gotBody) {
		t.Fatalf("headers %q %q", gotTS, gotSig)
	}
	status = http.StatusBadRequest
	if _, err := RequestAltsSnapshot(cfg, snapshot); err == nil {
		t.Fatal("400: no error")
	}

	src, err := os.ReadFile("reviews.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, fn := range []string{"func RequestAlts(", "func RequestAltsSnapshot("} {
		i := strings.Index(s, fn)
		if i < 0 {
			t.Fatalf("%s not found", fn)
		}
		end := strings.Index(s[i:], "\n}\n")
		if body := s[i : i+end]; !strings.Contains(body, "return postAlts(cfg, body)") || strings.Contains(body, "X-Review-Signature") {
			t.Fatalf("%s does not sign through postAlts", fn)
		}
	}
	if n := strings.Count(s, `"X-Review-Signature"`); n != 2 {
		t.Fatalf("X-Review-Signature set in %d places, want 2 (the job poster and postAlts)", n)
	}
}

func TestRequestAltsTransportErrorAndTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cfg := ReviewConfig{URL: srv.URL, Secret: "s", Client: &http.Client{Timeout: 20 * time.Millisecond}}
	if _, err := RequestAlts(cfg, 1); err == nil {
		t.Fatal("a timed-out POST: no error")
	}
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := closed.URL
	closed.Close()
	if _, err := RequestAlts(ReviewConfig{URL: url, Secret: "s"}, 1); err == nil {
		t.Fatal("a refused connection: no error")
	}
	if AltsTimeout != 28*time.Second || altsClient.Timeout != AltsTimeout {
		t.Fatalf("client timeout %v / %v, want 28 s (under CloudFront's 30 s origin timeout)", AltsTimeout, altsClient.Timeout)
	}
}
