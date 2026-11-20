package main

// Fork (integrations SAFE-ACTION-BODY-SPEC I4, I12) -- POST /api/campaigns/actions is
// models.BodyActions and nothing else: 200 with the report for a valid request, 400 for a body
// that is not JSON, and 200 with the one `error` finding for a body over the 4 MiB cap (R8).
// The route's permissions and the absence of any query are read off the diff (gate G2):
// handlers.go is a registration list with no test seam. No database: the App holds only a
// manager (canaryApp), and the handler touches nothing else.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

func postActions(t *testing.T, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/actions", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	err := canaryApp().PostCampaignActions(e.NewContext(req, rec))
	return rec, err
}

func actionsReport(t *testing.T, rec *httptest.ResponseRecorder) models.BodyActionReport {
	t.Helper()
	var out struct {
		Data models.BodyActionReport `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("answer %s: %v", rec.Body.String(), err)
	}
	return out.Data
}

func TestPostCampaignActions(t *testing.T) {
	// A valid request: the report, as the function gives it.
	rec, err := postActions(t, `{"content_type":"visual","body":"<p>{{ .Subscriber.FirstName }}</p>"}`)
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("plain: %v %d", err, rec.Code)
	}
	if r := actionsReport(t, rec); r.V != 1 || r.Total != 0 || r.Actions == nil || len(r.Actions) != 0 || r.Digest != "" {
		t.Fatalf("plain report %+v (%s)", r, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"actions":[]`) {
		t.Fatalf("actions not an empty array: %s", rec.Body.String())
	}

	rec, err = postActions(t, `{"content_type":"visual","body":"<p>{{ if true }}x{{ end }}</p>"}`)
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("if: %v %d", err, rec.Code)
	}
	if r := actionsReport(t, rec); r.Total != 1 || r.Actions[0].Kind != "if" || len(r.Digest) != 64 {
		t.Fatalf("if report %+v", r)
	}

	// Not JSON: 400.
	_, err = postActions(t, `not json`)
	var he *echo.HTTPError
	if he2, ok := err.(*echo.HTTPError); ok {
		he = he2
	}
	if he == nil || he.Code != http.StatusBadRequest {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Over the cap: 200 with the one error finding, digested as any finding.
	big := `{"content_type":"visual","body":"` + strings.Repeat("a", maxBodyActionsBytes) + `"}`
	rec, err = postActions(t, big)
	if err != nil || rec.Code != http.StatusOK {
		t.Fatalf("over the cap: %v %d", err, rec.Code)
	}
	r := actionsReport(t, rec)
	if r.Total != 1 || r.Actions[0].Kind != "error" || r.Actions[0].Text != "body over 4 MiB" || r.Digest != models.BodyActionsFailure("body over 4 MiB").Digest || r.Digest == "" {
		t.Fatalf("over-cap report %+v", r)
	}
}
