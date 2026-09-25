package main

// Fork (campaign review) -- integrations CAMPAIGN-INSPECT-SPEC I1 against a real database, through
// the handlers: the gate refuses a transition into running/scheduled with 400 unless the newest
// COMPLETE review for the campaign's CURRENT bundle hash has verdict pass; a save that changes the
// hash of a scheduled campaign is refused; it is a no-op while app.review_url is empty and for
// type=optin; test sends are never gated. Shares newLinkHarness (LISTMONK_TEST_PG opt-in).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/manager"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// reviewSender is every permission the gated paths need (Start needs campaigns:send).
var reviewSender = auth.User{PermissionsMap: map[string]struct{}{
	auth.PermCampaignsManageAll: {}, auth.PermCampaignsGetAll: {}, auth.PermCampaignsSend: {},
	auth.PermListGetAll: {}, auth.PermListManageAll: {}, auth.PermSubscribersGetAll: {},
}}

// ensureReviewManager is ensureManager with the app's URL config, so the footer guard sees the
// rendered unsubscribe link (the plain test campaigns carry {{ UnsubscribeURL }}).
func ensureReviewManager(h *linkHarness) {
	if h.app.manager == nil {
		h.app.manager = manager.New(manager.Config{
			UnsubURL:     "https://lm.test/subscription/%s/%s",
			OptinURL:     "https://lm.test/subscription/optin/%s?%s",
			LinkTrackURL: "https://lm.test/link/%s/%s/%s",
			ViewTrackURL: "https://lm.test/campaign/%s/%s/px.png",
			MessageURL:   "https://lm.test/campaign/%s/%s",
			ArchiveURL:   "https://lm.test/archive",
			RootURL:      "https://lm.test",
		}, &store{queries: h.q, core: h.app.core}, h.app.i18n, h.app.log)
		h.app.manager.AddMessenger(stubMessenger{})
	}
}

func callCampaignHandler(t *testing.T, h *linkHarness, handler echo.HandlerFunc, method string, id int, body string, params ...string) (int, string) {
	t.Helper()
	ensureReviewManager(h)
	e := echo.New()
	req := httptest.NewRequest(method, "/api/campaigns/"+itoa(id), strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("id", id)
	if len(params) > 0 {
		names, vals := []string{}, []string{}
		for i := 0; i+1 < len(params); i += 2 {
			names = append(names, params[i])
			vals = append(vals, params[i+1])
		}
		c.SetParamNames(names...)
		c.SetParamValues(vals...)
	}
	u := reviewSender
	u.Username = "robbie"
	c.Set(auth.UserHTTPCtxKey, u)
	if err := handler(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("handler: %v", err)
	}
	return rec.Code, rec.Body.String()
}

func statusReq(t *testing.T, h *linkHarness, id int, status string) (int, string) {
	return callCampaignHandler(t, h, h.app.UpdateCampaignStatus, http.MethodPut, id, `{"status": "`+status+`"}`)
}

// newReviewCampaign is a plain campaign the footer guard accepts (it renders an unsubscribe link).
func newReviewCampaign(t *testing.T, h *linkHarness, typ string, listID int) models.Campaign {
	t.Helper()
	c, err := h.app.core.CreateCampaign(models.Campaign{
		Type:        typ,
		Name:        "Gate_EN",
		Subject:     "Hello",
		FromEmail:   "Acme <hello@acme.test>",
		Body:        "Hi. Unsubscribe: {{ UnsubscribeURL }}",
		ContentType: models.CampaignContentTypePlain,
		Messenger:   "email",
		Attribs:     models.JSON{"lang": "en", "preheader": "p"},
		// What a UI save stores (validateCampaignFields merges the brand tag header), so an
		// unchanged re-save reproduces the stored bundle hash.
		Headers:     models.Headers{{"X-SES-MESSAGE-TAGS": "brand=acme"}},
		ArchiveMeta: json.RawMessage("{}"),
	}, []int{listID}, nil)
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	return c
}

func currentHash(t *testing.T, h *linkHarness, id int) string {
	t.Helper()
	cm, err := h.app.core.GetCampaign(id, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return core.CampaignBundleHash(cm)
}

func reviewReport(hash string, items string) string {
	return `{"v": 1, "status": "complete", "bundleHash": "` + hash + `", "items": [` + items + `], "ai": {"status": "ok"}}`
}

const passItems = `{"id": "D1.1", "tier": "D", "verdict": "pass", "acceptable": false, "findings": []}`
const failItems = `{"id": "D2.3", "tier": "D", "verdict": "fail", "acceptable": true, "findings": [{"key": "D2.3#aa"}]}`

// insertReview writes a review row directly, `ago` minutes old.
func insertReview(t *testing.T, h *linkHarness, campID int, hash, status, report string, ago int) {
	t.Helper()
	var rep any
	if report != "" {
		rep = report
	}
	h.db.MustExec(`INSERT INTO campaign_reviews (campaign_id, bundle_hash, job_id, status, report, requested_at)
		VALUES ($1, $2, gen_random_uuid(), $3, $4, NOW() - ($5 || ' minutes')::INTERVAL)`, campID, hash, status, rep, fmt.Sprint(ago))
}

func storedStatus(t *testing.T, h *linkHarness, id int) string {
	var s string
	h.db.Get(&s, `SELECT status FROM campaigns WHERE id = $1`, id)
	return s
}

func resetDraft(h *linkHarness, id int) {
	h.db.MustExec(`UPDATE campaigns SET status = 'draft', send_at = NULL WHERE id = $1`, id)
}

// setReviewGate turns the gate on (url != "") against a stub Lambda that accepts every job.
func setReviewGate(t *testing.T, url string) {
	ko.Set("app.review_url", url)
	ko.Set("app.review_hmac_secret", "gate-test-secret")
	t.Cleanup(func() { ko.Set("app.review_url", ""); ko.Set("app.review_hmac_secret", "") })
}

func TestReviewGate(t *testing.T) {
	h := newLinkHarness(t)
	ensureReviewManager(h)
	var list, optinList int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme', 'public', 'single', '{brand:acme,"from:Acme <hello@acme.test>"}') RETURNING id`)
	h.db.Get(&optinList, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Double', 'public', 'double') RETURNING id`)
	camp := newReviewCampaign(t, h, models.CampaignTypeRegular, list)
	id := camp.ID

	// Gate off: the pre-release Start path, no review needed.
	setReviewGate(t, "")
	if code, msg := statusReq(t, h, id, models.CampaignStatusRunning); code != http.StatusOK {
		t.Fatalf("gate off: start refused %d %s", code, msg)
	}
	if storedStatus(t, h, id) != "running" {
		t.Fatal("gate off: status did not change")
	}
	resetDraft(h, id)

	lambda := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) }))
	defer lambda.Close()
	setReviewGate(t, lambda.URL)

	refused := func(name, want string) {
		t.Helper()
		code, msg := statusReq(t, h, id, models.CampaignStatusRunning)
		if code != http.StatusBadRequest || !strings.Contains(msg, want) {
			t.Fatalf("%s: want 400 containing %q, got %d %s", name, want, code, msg)
		}
		if storedStatus(t, h, id) != "draft" {
			t.Fatalf("%s: a refused start changed the status", name)
		}
	}

	// No review.
	refused("no review", "not inspected")

	// A passing review of an OLDER hash: edited since.
	insertReview(t, h, id, "0000000000000000000000000000000000000000000000000000000000000000", "complete", reviewReport("x", passItems), 3)
	refused("stale hash", "edited since")

	// A running review of the current hash.
	hash := currentHash(t, h, id)
	insertReview(t, h, id, hash, "running", "", 0)
	refused("running", "inspection running")
	h.db.MustExec(`DELETE FROM campaign_reviews WHERE campaign_id = $1`, id)

	// A blocked review of the current hash.
	insertReview(t, h, id, hash, "complete", reviewReport(hash, failItems), 2)
	refused("blocked", "blocked: 1 item(s)")
	// Accepting the finding opens the gate (and Schedule is gated the same way first).
	h.db.MustExec(`INSERT INTO campaign_review_dispositions (campaign_id, bundle_hash, item_key, rubric_id, action) VALUES ($1, $2, 'D2.3#aa', 'D2.3', 'accept')`, id, hash)
	if code, msg := statusReq(t, h, id, models.CampaignStatusRunning); code != http.StatusOK {
		t.Fatalf("accepted: start refused %d %s", code, msg)
	}
	resetDraft(h, id)
	h.db.MustExec(`DELETE FROM campaign_reviews WHERE campaign_id = $1`, id)
	h.db.MustExec(`DELETE FROM campaign_review_dispositions WHERE campaign_id = $1`, id)

	// A pass, then a NEWER failed row for the same hash: the complete row still opens the gate.
	insertReview(t, h, id, hash, "complete", reviewReport(hash, passItems), 5)
	insertReview(t, h, id, hash, "failed", "", 1)
	if code, msg := statusReq(t, h, id, models.CampaignStatusRunning); code != http.StatusOK {
		t.Fatalf("pass under a newer failed row: %d %s", code, msg)
	}
	if storedStatus(t, h, id) != "running" {
		t.Fatal("pass: status did not change")
	}
	resetDraft(h, id)

	// Pause/cancel are never gated.
	h.db.MustExec(`DELETE FROM campaign_reviews WHERE campaign_id = $1`, id)
	h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE id = $1`, id)
	if code, msg := statusReq(t, h, id, models.CampaignStatusPaused); code != http.StatusOK {
		t.Fatalf("pause gated: %d %s", code, msg)
	}
	// Resume (paused -> running) IS gated.
	if code, _ := statusReq(t, h, id, models.CampaignStatusRunning); code != http.StatusBadRequest {
		t.Fatalf("resume must be gated, got %d", code)
	}
	resetDraft(h, id)

	// Opt-in confirmation campaigns are never gated.
	opt := newReviewCampaign(t, h, models.CampaignTypeOptin, optinList)
	if code, msg := statusReq(t, h, opt.ID, models.CampaignStatusRunning); code != http.StatusOK {
		t.Fatalf("optin gated: %d %s", code, msg)
	}

	// Test sends are never gated.
	var subID int
	h.db.Get(&subID, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 'tester@acme.test', 'T', '{}') RETURNING id`)
	h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, subID, list)
	body := fmt.Sprintf(`{"name": "Gate_EN", "subject": "Hello", "lists": [%d], "from_email": "Acme <hello@acme.test>", "messenger": "email", "type": "regular", "content_type": "plain", "body": "Hi. Unsubscribe: {{ UnsubscribeURL }}", "attribs": {"lang": "en"}, "subscribers": ["tester@acme.test"]}`, list)
	if code, msg := callCampaignHandler(t, h, h.app.TestCampaign, http.MethodPost, id, body); code != http.StatusOK {
		t.Fatalf("test send gated: %d %s", code, msg)
	}
}

func TestReviewGateScheduledSave(t *testing.T) {
	h := newLinkHarness(t)
	ensureReviewManager(h)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme', 'public', 'single', '{brand:acme,"from:Acme <hello@acme.test>"}') RETURNING id`)
	camp := newReviewCampaign(t, h, models.CampaignTypeRegular, list)
	h.db.MustExec(`UPDATE campaigns SET status = 'scheduled', send_at = NOW() + INTERVAL '2 days' WHERE id = $1`, camp.ID)

	lambda := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) }))
	defer lambda.Close()
	setReviewGate(t, lambda.URL)

	lists := fmt.Sprintf(`"lists": [%d]`, list)
	before := currentHash(t, h, camp.ID)
	if code, msg := putCampaign(t, h, camp.ID, `{`+lists+`, "subject": "Changed"}`); code != http.StatusBadRequest || !strings.Contains(msg, "Unschedule") {
		t.Fatalf("scheduled + changed hash: want 400 unschedule-first, got %d %s", code, msg)
	}
	if currentHash(t, h, camp.ID) != before {
		t.Fatal("a refused save changed the campaign")
	}
	if code, msg := putCampaign(t, h, camp.ID, `{`+lists+`}`); code != http.StatusOK {
		t.Fatalf("scheduled + same hash: want 200, got %d %s", code, msg)
	}

	// With the gate off the same edit is allowed (the pre-release save path).
	setReviewGate(t, "")
	if code, msg := putCampaign(t, h, camp.ID, `{`+lists+`, "subject": "Changed"}`); code != http.StatusOK {
		t.Fatalf("gate off: scheduled edit refused %d %s", code, msg)
	}
}
