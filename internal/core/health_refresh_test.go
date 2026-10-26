package core

// Fork (on-demand brand-health refresh) -- integrations BRANDHEALTH-REFRESH-SPEC I3 (the signing
// half): the POST carries X-Health-Timestamp (unix seconds) and X-Health-Signature built by the
// shared internal/review.Sign over ts + "." + body -- the recipe the Lambda verifies -- with the
// {"requestedAt"} body; 202 maps to started, 429 to busy (Stage 2 S2), anything else and a timeout
// to an error; the default client times out at 8 s (Stage 2 S7). No database.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/review"
)

func TestHealthRefreshSignsAndMaps(t *testing.T) {
	const secret = "health-test-secret"
	at := time.Unix(1790000000, 0)

	var status int
	var gotTS, gotSig, gotCT, gotMethod string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotTS = r.Header.Get("X-Health-Timestamp")
		gotSig = r.Header.Get("X-Health-Signature")
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
	}))
	defer srv.Close()

	cfg := HealthRefreshConfig{URL: srv.URL, Secret: secret, Now: func() time.Time { return at }}

	status = http.StatusAccepted
	res, err := RequestHealthRefresh(cfg)
	if err != nil || !res.Started || res.Busy {
		t.Fatalf("202: %+v %v, want started", res, err)
	}
	if gotMethod != http.MethodPost || gotCT != "application/json" {
		t.Fatalf("method %s content-type %s", gotMethod, gotCT)
	}
	if gotTS != strconv.FormatInt(at.Unix(), 10) {
		t.Fatalf("X-Health-Timestamp = %q", gotTS)
	}
	var b struct {
		RequestedAt string `json:"requestedAt"`
	}
	if err := json.Unmarshal(gotBody, &b); err != nil || b.RequestedAt != "2026-09-21T14:13:20Z" {
		t.Fatalf("body %s (%v)", gotBody, err)
	}
	// The shared helper, and the recipe it implements, independently recomputed.
	if want := review.Sign(secret, at.Unix(), gotBody); gotSig != want {
		t.Fatalf("X-Health-Signature = %q, want review.Sign = %q", gotSig, want)
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(gotTS + "." + string(gotBody)))
	if want := "sha256=" + hex.EncodeToString(m.Sum(nil)); gotSig != want {
		t.Fatalf("signature is not HMAC-SHA256(secret, ts.body): %q vs %q", gotSig, want)
	}

	status = http.StatusTooManyRequests
	if res, err := RequestHealthRefresh(cfg); err != nil || res.Started || !res.Busy {
		t.Fatalf("429: %+v %v, want busy", res, err)
	}
	for _, s := range []int{http.StatusUnauthorized, http.StatusServiceUnavailable, http.StatusInternalServerError, http.StatusOK} {
		status = s
		if _, err := RequestHealthRefresh(cfg); err == nil {
			t.Fatalf("%d: no error", s)
		}
	}
}

func TestHealthRefreshTimeout(t *testing.T) {
	if healthRefreshClient.Timeout != 8*time.Second {
		t.Fatalf("default client timeout = %s, want 8s", healthRefreshClient.Timeout)
	}
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	defer close(release)
	_, err := RequestHealthRefresh(HealthRefreshConfig{URL: srv.URL, Secret: "s", Client: &http.Client{Timeout: 50 * time.Millisecond}})
	if err == nil {
		t.Fatal("a stalled Lambda did not time out")
	}
}
