package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/knadh/listmonk/internal/review"
)

// Fork (on-demand brand-health refresh, integrations BRANDHEALTH-REFRESH-SPEC D1/D2). The fork
// asks the integrations BrandHealth Lambda to re-run its daily computation now: one signed POST to
// the Lambda's Function URL. The Lambda answers 202 and runs asynchronously; the fork computes
// nothing and stores nothing -- the run writes the usual documents back through the API.

// HealthRefreshConfig is the live configuration, read from koanf by the handler on every call
// (the go-live switch is a settings save + reload).
type HealthRefreshConfig struct {
	URL    string
	Secret string
	// Client posts the request; nil = the 8 s client (the 202 leg never computes, Stage 2 S7).
	Client *http.Client
	// Now stamps X-Health-Timestamp and requestedAt; nil = time.Now.
	Now func() time.Time
}

// HealthRefreshResult is what POST /api/system/health/refresh answers on success. Busy = the
// Lambda's single reserved-concurrency slot is taken (a run is already in progress) -- a success
// state, not an error (Stage 2 S2).
type HealthRefreshResult struct {
	Started bool `json:"started"`
	Busy    bool `json:"busy,omitempty"`
}

// healthRefreshBody is the signed body.
type healthRefreshBody struct {
	RequestedAt string `json:"requestedAt"`
}

var healthRefreshClient = &http.Client{Timeout: 8 * time.Second}

// RequestHealthRefresh signs and POSTs the refresh request (the review job's recipe,
// internal/review.Sign, in X-Health-Signature beside X-Health-Timestamp) and maps the Lambda's
// answer: 202 -> started; 429 (a synchronous Function URL call throttled by reserved concurrency
// 1) -> busy; anything else, or a transport error/timeout -> an error carrying a terse reason for
// the caller's log (never returned to the client).
func RequestHealthRefresh(cfg HealthRefreshConfig) (HealthRefreshResult, error) {
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	client := healthRefreshClient
	if cfg.Client != nil {
		client = cfg.Client
	}
	t := now()
	body, err := json.Marshal(healthRefreshBody{RequestedAt: t.UTC().Format(time.RFC3339)})
	if err != nil {
		return HealthRefreshResult{}, err
	}
	ts := t.Unix()
	req, err := http.NewRequest(http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return HealthRefreshResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Health-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Health-Signature", review.Sign(cfg.Secret, ts, body))
	resp, err := client.Do(req)
	if err != nil {
		return HealthRefreshResult{}, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch resp.StatusCode {
	case http.StatusAccepted:
		return HealthRefreshResult{Started: true}, nil
	case http.StatusTooManyRequests:
		return HealthRefreshResult{Started: false, Busy: true}, nil
	}
	return HealthRefreshResult{}, fmt.Errorf("HTTP %d", resp.StatusCode)
}
