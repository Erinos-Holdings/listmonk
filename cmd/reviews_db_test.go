package main

// Fork (campaign review) -- integrations CAMPAIGN-INSPECT-SPEC I4 (AddDispositions refuses accept on
// a non-acceptable item and any key absent from the newest report except the pseudo-key A; every
// row is appended, never updated -- the log), plus the start/write-back endpoints: the signed job,
// the 409 on a live running row, the 6-minute expiry, the 502 on a Lambda that does not accept,
// and the PATCH rules (shape-validated complete, 409 once not running).

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/review"
	"github.com/knadh/listmonk/models"
)

func TestReviewDispositionsLog(t *testing.T) {
	h := newLinkHarness(t)
	ensureReviewManager(h)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme', 'public', 'single', '{brand:acme,"from:Acme <hello@acme.test>"}') RETURNING id`)
	camp := newReviewCampaign(t, h, models.CampaignTypeRegular, list)
	hash := currentHash(t, h, camp.ID)

	// No completed inspection yet: nothing can be decided.
	if _, err := h.app.core.AddDispositions(camp.ID, 0, "robbie", []core.DispositionIn{{Key: "A", Action: "accept"}}); err == nil {
		t.Fatal("a disposition without a completed report must be refused")
	}

	items := `{"id": "D2.6", "tier": "D", "verdict": "fail", "acceptable": false, "findings": [{"key": "D2.6#nope"}]},
		{"id": "D2.3", "tier": "D", "verdict": "fail", "acceptable": true, "findings": [{"key": "D2.3#aa"}]},
		{"id": "A1.1", "tier": "A", "verdict": "finding", "acceptable": true, "findings": [{"key": "A1.1#cc", "severity": "critical"}]}`
	insertReview(t, h, camp.ID, hash, "complete", reviewReport(hash, items), 1)

	for name, in := range map[string][]core.DispositionIn{
		"accept on a non-acceptable item": {{Key: "D2.6#nope", Action: "accept"}},
		"unknown key":                     {{Key: "D9.9#zz", Action: "accept"}},
		"bad action":                      {{Key: "D2.3#aa", Action: "ignore"}},
		"one bad row refuses the batch":   {{Key: "D2.3#aa", Action: "accept"}, {Key: "D2.6#nope", Action: "accept"}},
	} {
		if _, err := h.app.core.AddDispositions(camp.ID, 0, "robbie", in); err == nil {
			t.Fatalf("%s: want a refusal", name)
		}
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM campaign_review_dispositions`)
	if n != 0 {
		t.Fatalf("refused batches wrote %d rows", n)
	}

	// fixme on a non-acceptable item is fine; A and R are accepted without a report key.
	for _, in := range []core.DispositionIn{
		{Key: "D2.6#nope", Action: "fixme"},
		{Key: "D2.3#aa", Action: "accept", Note: "known"},
		{Key: "A", Action: "accept"},
		{Key: "R", Action: "accept"},
		{Key: "A1.1#cc", RubricID: "spoofed", Action: "accept"},
	} {
		if _, err := h.app.core.AddDispositions(camp.ID, 7, "robbie", []core.DispositionIn{in}); err != nil {
			t.Fatalf("%s: %v", in.Key, err)
		}
	}
	// The rubric id comes from the REPORT, never the client.
	var rid string
	h.db.Get(&rid, `SELECT rubric_id FROM campaign_review_dispositions WHERE item_key = 'A1.1#cc'`)
	if rid != "A1.1" {
		t.Fatalf("rubric_id = %q, want A1.1", rid)
	}

	// Append-only: a later fixme adds a row; the accept row is untouched and the latest wins.
	if _, err := h.app.core.AddDispositions(camp.ID, 7, "robbie", []core.DispositionIn{{Key: "D2.3#aa", Action: "fixme"}}); err != nil {
		t.Fatal(err)
	}
	var actions []string
	h.db.Select(&actions, `SELECT action FROM campaign_review_dispositions WHERE item_key = 'D2.3#aa' ORDER BY id`)
	if strings.Join(actions, ",") != "accept,fixme" {
		t.Fatalf("log for D2.3#aa = %v, want accept then fixme", actions)
	}
	state, err := h.app.core.GetLatestReview(camp)
	if err != nil {
		t.Fatal(err)
	}
	latest := review.LatestDispositions(state.Dispositions)
	if latest["D2.3#aa"].Action != "fixme" || latest["D2.3#aa"].Username != "robbie" || !latest["D2.3#aa"].UserID.Valid {
		t.Fatalf("latest D2.3#aa = %+v", latest["D2.3#aa"])
	}
	// D2.6 (non-acceptable) and D2.3 (fixme) block.
	if state.Verdict.Verdict != review.VerdictBlocked || len(state.Verdict.Blockers) != 2 {
		t.Fatalf("verdict = %+v", state.Verdict)
	}

	// Stats aggregate per rubric id.
	stats, err := h.app.core.ReviewStats()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]models.ReviewStat{}
	for _, s := range stats {
		byID[s.RubricID] = s
	}
	if byID["D2.3"].Accepts != 1 || byID["D2.3"].Fixmes != 1 || byID["A"].Accepts != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestReviewStartAndWriteBack(t *testing.T) {
	h := newLinkHarness(t)
	ensureReviewManager(h)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme', 'public', 'single', '{brand:acme,"from:Acme <hello@acme.test>"}') RETURNING id`)
	camp := newReviewCampaign(t, h, models.CampaignTypeRegular, list)

	// A stub Lambda that verifies the signature exactly as the TS handler does.
	var got core.ReviewJob
	accept := http.StatusAccepted
	lambda := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts, _ := strconv.ParseInt(r.Header.Get("X-Review-Timestamp"), 10, 64)
		if r.Header.Get("X-Review-Signature") != review.Sign("gate-test-secret", ts, body) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.Unmarshal(body, &got)
		w.WriteHeader(accept)
	}))
	defer lambda.Close()

	// Not configured -> 503.
	setReviewGate(t, "")
	if code, _ := callCampaignHandler(t, h, h.app.StartCampaignReview, http.MethodPost, camp.ID, ""); code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured: want 503, got %d", code)
	}

	setReviewGate(t, lambda.URL)
	code, resp := callCampaignHandler(t, h, h.app.StartCampaignReview, http.MethodPost, camp.ID, "")
	if code != http.StatusOK {
		t.Fatalf("start: %d %s", code, resp)
	}
	hash := currentHash(t, h, camp.ID)
	if got.CampaignID != camp.ID || got.BundleHash != hash || got.RequestedBy != "robbie" || got.JobID == "" {
		t.Fatalf("job = %+v (hash %s)", got, hash)
	}
	job := got.JobID

	// A second start while that one runs -> 409.
	if code, _ := callCampaignHandler(t, h, h.app.StartCampaignReview, http.MethodPost, camp.ID, ""); code != http.StatusConflict {
		t.Fatalf("second start: want 409, got %d", code)
	}

	patch := func(jobID, body string) (int, string) {
		return callCampaignHandler(t, h, h.app.PatchCampaignReview, http.MethodPatch, camp.ID, body, "jobId", jobID)
	}
	if code, msg := patch(job, `{"status": "running", "progress": {"stage": "deterministic"}}`); code != http.StatusOK {
		t.Fatalf("progress: %d %s", code, msg)
	}
	if code, _ := patch(job, `{"status": "complete", "report": `+reviewReport("ffff", passItems)+`}`); code != http.StatusBadRequest {
		t.Fatal("a complete report for another bundle hash must be refused")
	}
	if code, _ := patch(job, `{"status": "complete", "report": {"v": 1, "bundleHash": "`+hash+`", "items": []}}`); code != http.StatusBadRequest {
		t.Fatal("a complete report with no items must be refused")
	}
	if code, msg := patch(job, `{"status": "complete", "report": `+reviewReport(hash, passItems)+`}`); code != http.StatusOK {
		t.Fatalf("complete: %d %s", code, msg)
	}
	// Once complete, any further write is a 409 (a replayed job never writes a second report).
	for _, b := range []string{`{"status": "running", "progress": {}}`, `{"status": "failed", "error": "x"}`, `{"status": "complete", "report": ` + reviewReport(hash, passItems) + `}`} {
		if code, _ := patch(job, b); code != http.StatusConflict {
			t.Fatalf("write after complete: want 409, got %d for %s", code, b)
		}
	}
	state, _ := h.app.core.GetLatestReview(camp)
	if state.Review == nil || state.Review.Status != "complete" || state.Verdict.Verdict != review.VerdictPass || state.CurrentHash != hash {
		t.Fatalf("latest = %+v verdict %+v", state.Review, state.Verdict)
	}

	// A running row older than 6 minutes is expired by the next start (and on read).
	h.db.MustExec(`UPDATE campaign_reviews SET status = 'running', requested_at = NOW() - INTERVAL '7 minutes' WHERE job_id = $1`, job)
	state, _ = h.app.core.GetLatestReview(camp)
	if state.Review.Status != "failed" || state.Review.Error != "timeout" {
		t.Fatalf("expiry on read: %+v", state.Review)
	}
	h.db.MustExec(`UPDATE campaign_reviews SET status = 'running' WHERE job_id = $1`, job)
	if code, msg := callCampaignHandler(t, h, h.app.StartCampaignReview, http.MethodPost, camp.ID, ""); code != http.StatusOK {
		t.Fatalf("start over a dead row: %d %s", code, msg)
	}

	// A Lambda that does not accept -> 502, and the row is failed (never left running).
	h.db.MustExec(`UPDATE campaign_reviews SET status = 'complete' WHERE status = 'running'`)
	accept = http.StatusInternalServerError
	if code, _ := callCampaignHandler(t, h, h.app.StartCampaignReview, http.MethodPost, camp.ID, ""); code != http.StatusBadGateway {
		t.Fatalf("lambda 500: want 502, got %d", code)
	}
	var st, errMsg string
	h.db.QueryRow(`SELECT status, error FROM campaign_reviews WHERE campaign_id = $1 ORDER BY id DESC LIMIT 1`, camp.ID).Scan(&st, &errMsg)
	if st != "failed" || !strings.Contains(errMsg, "HTTP 500") {
		t.Fatalf("row after a refused job = %s %q", st, errMsg)
	}

	// Structure verifications: upsert + {match, records}.
	fp := strings.Repeat("a", 64)
	if _, err := h.app.core.PutStructureVerification(fp, "test-1", json.RawMessage(`{"blocks": ["Text"]}`), "robbie"); err != nil {
		t.Fatal(err)
	}
	match, all, err := h.app.core.GetStructureVerification(fp)
	if err != nil || match == nil || match.TestID != "test-1" || len(all) != 1 {
		t.Fatalf("structure: %+v %+v %v", match, all, err)
	}
	if m, _, _ := h.app.core.GetStructureVerification(strings.Repeat("b", 64)); m != nil {
		t.Fatal("an unknown fingerprint must not match")
	}
}
