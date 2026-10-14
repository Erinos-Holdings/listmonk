package main

// Fork (campaign review) -- integrations LIST-START-GLYPH-SPEC I1/I2 against a real database,
// through GET /api/campaigns: with the gate on, a draft/paused non-optin row carries
// review_passed, true exactly when reviewGate(id, "running") passes; every other row, and every
// row with the gate off, carries no field. Shares newLinkHarness (LISTMONK_TEST_PG opt-in).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/review"
	"github.com/knadh/listmonk/models"
	"github.com/knadh/paginator"
)

func TestCampaignsListReviewPassed(t *testing.T) {
	h := newLinkHarness(t)
	ensureReviewManager(h)
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})

	var list, optinList int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme', 'public', 'single', '{brand:acme,"from:Acme <hello@acme.test>"}') RETURNING id`)
	h.db.Get(&optinList, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Double', 'public', 'double') RETURNING id`)

	passed := newReviewCampaign(t, h, models.CampaignTypeRegular, list).ID // draft, passing review
	paused := newReviewCampaign(t, h, models.CampaignTypeRegular, list).ID // paused, no review
	optin := newReviewCampaign(t, h, models.CampaignTypeOptin, optinList).ID
	others := map[string]int{}
	for _, st := range []string{"running", "scheduled", "finished", "cancelled"} {
		id := newReviewCampaign(t, h, models.CampaignTypeRegular, list).ID
		h.db.MustExec(`UPDATE campaigns SET status = $2::campaign_status, send_at = CASE WHEN $2 = 'scheduled' THEN NOW() + INTERVAL '2 days' END WHERE id = $1`, id, st)
		others[st] = id
	}
	h.db.MustExec(`UPDATE campaigns SET status = 'paused' WHERE id = $1`, paused)

	// A passing complete review of the fetched row's current bundle hash.
	cm, err := h.app.core.GetCampaign(passed, "", "")
	if err != nil {
		t.Fatal(err)
	}
	hash := review.BundleHash(cm, core.CampaignListIDs(cm))
	insertReview(t, h, passed, hash, "complete", reviewReport(hash, passItems), 2)

	// rows is the list response keyed by id, each row as raw JSON keys (absence is visible).
	rows := func() map[int]map[string]json.RawMessage {
		t.Helper()
		code, body := baCall(t, reviewSender, "/api/campaigns?per_page=all&no_body=true", h.app.GetCampaigns)
		if code != http.StatusOK {
			t.Fatalf("GetCampaigns: %d %s", code, body)
		}
		var resp struct {
			Data struct {
				Results []map[string]json.RawMessage `json:"results"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("GetCampaigns: %s: %v", body, err)
		}
		out := map[int]map[string]json.RawMessage{}
		for _, r := range resp.Data.Results {
			var id int
			json.Unmarshal(r["id"], &id)
			out[id] = r
		}
		return out
	}
	field := func(r map[string]json.RawMessage) (bool, bool) {
		raw, ok := r["review_passed"]
		if !ok {
			return false, false
		}
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("review_passed %s: %v", raw, err)
		}
		return v, true
	}
	// agree asserts the row's field equals the gate's verdict for a Start, and equals want.
	agree := func(name string, rs map[int]map[string]json.RawMessage, id int, want bool) {
		t.Helper()
		v, ok := field(rs[id])
		if !ok {
			t.Fatalf("%s: review_passed absent on a gated row", name)
		}
		gateOK := h.app.reviewGate(id, models.CampaignStatusRunning) == nil
		if v != gateOK {
			t.Fatalf("%s: review_passed %v disagrees with reviewGate pass=%v", name, v, gateOK)
		}
		if v != want {
			t.Fatalf("%s: review_passed %v, want %v", name, v, want)
		}
	}
	absent := func(name string, rs map[int]map[string]json.RawMessage, id int) {
		t.Helper()
		if _, ok := field(rs[id]); ok {
			t.Fatalf("%s: review_passed present: %s", name, rs[id]["review_passed"])
		}
	}

	// I2: gate off -- absent on every row.
	setReviewGate(t, "")
	off := rows()
	if len(off) != 3+len(others) {
		t.Fatalf("gate off: %d rows", len(off))
	}
	for id := range off {
		absent(fmt.Sprintf("gate off, campaign %d", id), off, id)
	}

	lambda := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) }))
	defer lambda.Close()
	setReviewGate(t, lambda.URL)

	// I1: a passing complete review of the current hash -> true; no review -> false.
	on := rows()
	agree("passed draft", on, passed, true)
	agree("paused without review", on, paused, false)

	// I2: gate on -- absent on optin and every non-draft/paused status.
	absent("optin", on, optin)
	for st, id := range others {
		absent(st, on, id)
	}

	// I1: a save that changes the bundle hash -> false.
	body := fmt.Sprintf(`{"lists": [%d], "subject": "Changed"}`, list)
	if code, msg := putCampaign(t, h, passed, body); code != http.StatusOK {
		t.Fatalf("save: %d %s", code, msg)
	}
	agree("edited since the pass", rows(), passed, false)
}
