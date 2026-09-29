package main

// Fork (inspect scope) -- integrations INSPECT-SCOPE-SPEC:
//
//   I7  accepting an `R#…` key (D4.2's structure finding) or the legacy `R` without
//       campaigns:review_structure is a 403 for the whole batch; with it (or as Super Admin), 200;
//       other dispositions are unchanged. PUT structure-verifications requires the permission.
//   I15 a stage-1 and a stage-2 record for one fingerprint coexist (append-only) and both come back
//       from the GET, so coverage can combine them.
//
// Shares newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/review"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// callAs is callCampaignHandler with an explicit user (the permission under test).
func callAs(t *testing.T, h *linkHarness, handler echo.HandlerFunc, method string, id int, body string, u auth.User) (int, string) {
	t.Helper()
	ensureReviewManager(h)
	e := echo.New()
	req := httptest.NewRequest(method, "/api/campaigns/"+itoa(id), strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("id", id)
	c.Set(auth.UserHTTPCtxKey, u)
	if err := handler(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("handler: %v", err)
	}
	return rec.Code, rec.Body.String()
}

func withPerms(username string, roleID int, extra ...string) auth.User {
	u := auth.User{PermissionsMap: map[string]struct{}{}}
	for k := range reviewSender.PermissionsMap {
		u.PermissionsMap[k] = struct{}{}
	}
	for _, p := range extra {
		u.PermissionsMap[p] = struct{}{}
	}
	u.Username = username
	u.UserRoleID = roleID
	return u
}

func TestStructureAcceptIsAdminOnly(t *testing.T) {
	h := newLinkHarness(t)
	ensureReviewManager(h)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme', 'public', 'single', '{brand:acme,"from:Acme <hello@acme.test>"}') RETURNING id`)
	camp := newReviewCampaign(t, h, models.CampaignTypeRegular, list)
	hash := currentHash(t, h, camp.ID)
	items := `{"id": "D4.2", "tier": "D", "verdict": "fail", "acceptable": true, "findings": [{"key": "R#0123456789abcdef"}]},
		{"id": "D2.3", "tier": "D", "verdict": "fail", "acceptable": true, "findings": [{"key": "D2.3#aa"}]}`
	insertReview(t, h, camp.ID, hash, "complete", reviewReport(hash, items), 1)

	marketing := withPerms("kbrophy", 5)                               // no campaigns:review_structure
	admin := withPerms("robbie", 5, auth.PermCampaignsReviewStructure) // holds it
	superAdmin := withPerms("root", auth.SuperAdminRoleID)             // implicit
	post := func(u auth.User, body string) (int, string) {
		return callAs(t, h, h.app.PostReviewDispositions, http.MethodPost, camp.ID, body, u)
	}
	count := func() int {
		var n int
		h.db.Get(&n, `SELECT COUNT(*) FROM campaign_review_dispositions`)
		return n
	}

	// Without the permission: an accept on R# or R is 403, and it refuses the WHOLE batch.
	for _, body := range []string{
		`{"items": [{"key": "R#0123456789abcdef", "action": "accept"}]}`,
		`{"items": [{"key": "R", "action": "accept"}]}`,
		`{"items": [{"key": "D2.3#aa", "action": "accept"}, {"key": "R#0123456789abcdef", "action": "accept"}]}`,
	} {
		if code, msg := post(marketing, body); code != http.StatusForbidden {
			t.Fatalf("marketing %s: want 403, got %d %s", body, code, msg)
		}
	}
	if n := count(); n != 0 {
		t.Fatalf("refused batches wrote %d rows", n)
	}

	// Other dispositions are unchanged for the same user -- including a non-accept on R#.
	for _, body := range []string{
		`{"items": [{"key": "D2.3#aa", "action": "accept"}]}`,
		`{"items": [{"key": "R#0123456789abcdef", "action": "fixme"}]}`,
	} {
		if code, msg := post(marketing, body); code != http.StatusOK {
			t.Fatalf("marketing %s: want 200, got %d %s", body, code, msg)
		}
	}

	// The gate still blocks on the structure key until an admin accepts it.
	state, _ := h.app.core.GetLatestReview(freshCampaign(t, h, camp.ID))
	if state.Gate == nil || state.Gate.Verdict.Verdict != review.VerdictBlocked {
		t.Fatalf("verdict before the structure accept = %+v", state.Gate)
	}

	// With the permission: 200 (a role holding it, and Super Admin implicitly).
	if code, msg := post(admin, `{"items": [{"key": "R#0123456789abcdef", "action": "accept", "note": "acknowledged"}]}`); code != http.StatusOK {
		t.Fatalf("admin accept: %d %s", code, msg)
	}
	if code, msg := post(superAdmin, `{"items": [{"key": "R", "action": "accept"}]}`); code != http.StatusOK {
		t.Fatalf("super admin accept R: %d %s", code, msg)
	}
	state, _ = h.app.core.GetLatestReview(freshCampaign(t, h, camp.ID))
	if state.Gate == nil || state.Gate.Verdict.Verdict != review.VerdictPass {
		t.Fatalf("verdict after the structure accept = %+v", state.Gate)
	}

	// PUT structure-verifications requires the permission (403 without, 200 with).
	put := func(u auth.User, body string) (int, string) {
		return callAs(t, h, h.app.PutStructureVerification, http.MethodPut, 0, body, u)
	}
	rec := `{"fingerprint": "` + strings.Repeat("c", 64) + `", "test_id": "T1", "components": {"blocks": []}, "canary": {"Text": "x"},
		"clients": ["a", "a_dm"], "roster": ["a", "a_dm", "b"], "stage": "1", "modes": ["light", "dark"], "campaign_id": 108}`
	if code, _ := put(marketing, rec); code != http.StatusForbidden {
		t.Fatalf("marketing PUT: want 403, got %d", code)
	}
	withReview := withPerms("reviewer", 5, auth.PermCampaignsReview)
	if code, _ := put(withReview, rec); code != http.StatusForbidden {
		t.Fatalf("campaigns:review alone PUT: want 403, got %d", code)
	}
	if code, msg := put(admin, rec); code != http.StatusOK {
		t.Fatalf("admin PUT: %d %s", code, msg)
	}
	if code, _ := put(admin, strings.Replace(rec, `"stage": "1"`, `"stage": "3"`, 1)); code != http.StatusBadRequest {
		t.Fatal("a bogus stage must be refused")
	}
	if code, _ := put(admin, strings.Replace(rec, `"clients": ["a", "a_dm"]`, `"clients": []`, 1)); code != http.StatusBadRequest {
		t.Fatal("a record with no completed clients must be refused")
	}
}

func TestStructureRecordsCoexist(t *testing.T) {
	h := newLinkHarness(t)
	ensureReviewManager(h)
	admin := withPerms("robbie", 5, auth.PermCampaignsReviewStructure)
	fp := strings.Repeat("d", 64)
	put := func(body string) {
		if code, msg := callAs(t, h, h.app.PutStructureVerification, http.MethodPut, 0, body, admin); code != http.StatusOK {
			t.Fatalf("PUT: %d %s", code, msg)
		}
	}
	put(`{"fingerprint": "` + fp + `", "test_id": "S1", "clients": ["f", "f_dm"], "roster": ["f", "f_dm", "x"], "stage": "1", "modes": ["light", "dark"], "canary": {"Text": "h"}}`)
	put(`{"fingerprint": "` + fp + `", "test_id": "S2", "clients": ["x"], "roster": ["f", "f_dm", "x"], "stage": "2", "modes": ["light", "dark"], "canary": {"Text": "h"}, "verified_at": "2026-09-25T23:57:50Z"}`)

	code, body := callAs(t, h, func(c echo.Context) error {
		c.SetParamNames("fingerprint")
		c.SetParamValues(fp)
		return h.app.GetStructureVerification(c)
	}, http.MethodGet, 0, "", admin)
	if code != http.StatusOK {
		t.Fatalf("GET: %d %s", code, body)
	}
	var resp struct {
		Data struct {
			Match   *models.StructureVerification  `json:"match"`
			Records []models.StructureRecord       `json:"records"`
			Legacy  []models.StructureVerification `json:"legacy_records"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	stages := map[string]models.StructureRecord{}
	for _, r := range resp.Data.Records {
		if r.Fingerprint == fp {
			stages[r.Stage] = r
		}
	}
	if len(stages) != 2 || stages["1"].TestID != "S1" || stages["2"].TestID != "S2" {
		t.Fatalf("records = %+v, want a stage-1 and a stage-2 row for one fingerprint", resp.Data.Records)
	}
	if strings.Join(stages["1"].Clients, ",") != "f,f_dm" || strings.Join(stages["2"].Roster, ",") != "f,f_dm,x" || stages["1"].VerifiedBy != "robbie" {
		t.Fatalf("stage rows = %+v", stages)
	}
	if got := stages["2"].VerifiedAt.UTC().Format("2006-01-02T15:04:05Z"); got != "2026-09-25T23:57:50Z" {
		t.Fatalf("verified_at = %s, want the supplied date", got)
	}
	if resp.Data.Match != nil || resp.Data.Legacy == nil {
		t.Fatalf("legacy half = %+v / %+v", resp.Data.Match, resp.Data.Legacy)
	}
}
