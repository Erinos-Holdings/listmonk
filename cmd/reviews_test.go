package main

// Fork (naming standard) -- integrations listmonk/NAMING-STANDARD-SPEC I6, through the handlers
// against a real database (newLinkHarness, LISTMONK_TEST_PG opt-in): the Start gate,
// reviews/latest (gate, current) and AddDispositionsAt all read core.CurrentReview, so a review
// written BEFORE the release (the legacy hash, name included) stays current after a rename --
// the renamed campaign starts -- while an edited one does not.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/review"
	"github.com/knadh/listmonk/models"
)

func TestCurrentReviewAcceptsLegacyHash(t *testing.T) {
	h := newLinkHarness(t)
	ensureReviewManager(h)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme', 'public', 'single', '{brand:acme,"from:Acme <hello@acme.test>"}') RETURNING id`)
	camp := newReviewCampaign(t, h, models.CampaignTypeRegular, list)
	id := camp.ID

	lambda := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) }))
	defer lambda.Close()
	setReviewGate(t, lambda.URL)

	// A pre-release passing review: the legacy formula over the campaign's name at review.
	cm, _ := h.app.core.GetCampaign(id, "", "")
	legacy := review.LegacyBundleHash(cm, core.CampaignListIDs(cm), cm.Name)
	if legacy == core.CampaignBundleHash(cm) {
		t.Fatal("fixture: the two formulas agree")
	}
	insertReview(t, h, id, legacy, "complete", reviewReport(legacy, passItems), 3)

	latest := func() core.ReviewState {
		t.Helper()
		cm, err := h.app.core.GetCampaign(id, "", "")
		if err != nil {
			t.Fatal(err)
		}
		st, err := h.app.core.GetLatestReview(cm)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	if st := latest(); !st.Current || st.Gate == nil {
		t.Fatalf("unrenamed: current %v gate %v", st.Current, st.Gate)
	}

	// Rename (twice: the ORIGINAL name stays in former_names) -- still current, and it starts.
	for _, n := range []string{"Gate renamed", "Gate renamed again"} {
		if code, msg := nameCall(t, h, h.app.RenameCampaign, http.MethodPut, id, renameBody(n), renamer); code != http.StatusOK {
			t.Fatalf("rename %q: %d %s", n, code, msg)
		}
	}
	st := latest()
	if !st.Current || st.Gate == nil || st.Gate.Review.BundleHash != legacy {
		t.Fatalf("renamed: current %v gate %+v", st.Current, st.Gate)
	}
	if st.CurrentHash == legacy {
		t.Fatal("current_hash must stay the new formula")
	}

	// AddDispositionsAt validates against the same row (its keys are accepted).
	cm, _ = h.app.core.GetCampaign(id, "", "")
	// (The A pseudo-key is accepted against any report; the row it is recorded against is the point.)
	if _, err := h.app.core.AddDispositions(cm, 0, "robbie", []core.DispositionIn{{Key: review.KeyAI, Action: models.DispositionAccept}}, nil); err != nil {
		t.Fatalf("dispositions against the legacy row: %v", err)
	}
	var dispHash string
	h.db.Get(&dispHash, `SELECT bundle_hash FROM campaign_review_dispositions WHERE campaign_id = $1 ORDER BY id DESC LIMIT 1`, id)
	if dispHash != legacy {
		t.Fatalf("the disposition was recorded against %q, want the legacy row's hash", dispHash)
	}

	if code, msg := statusReq(t, h, id, models.CampaignStatusRunning); code != http.StatusOK {
		t.Fatalf("a renamed campaign with a pre-release passing review must start: %d %s", code, msg)
	}
	resetDraft(h, id)

	// An EDITED campaign does not: content changed, neither hash matches.
	h.db.MustExec(`UPDATE campaigns SET subject = 'Edited' WHERE id = $1`, id)
	if st := latest(); st.Current || st.Gate != nil {
		t.Fatalf("edited: current %v gate %v", st.Current, st.Gate)
	}
	code, msg := statusReq(t, h, id, models.CampaignStatusRunning)
	if code != http.StatusBadRequest || !strings.Contains(msg, "edited since") {
		t.Fatalf("edited: want 400 edited since, got %d %s", code, msg)
	}

	// A new review stores the new hash and is current too.
	h.db.MustExec(`UPDATE campaigns SET subject = 'Hello' WHERE id = $1`, id)
	cm, _ = h.app.core.GetCampaign(id, "", "")
	fresh := core.CampaignBundleHash(cm)
	insertReview(t, h, id, fresh, "complete", reviewReport(fresh, passItems), 0)
	if st := latest(); !st.Current || st.Gate == nil || st.Gate.Review.BundleHash != fresh {
		t.Fatalf("new-formula review: current %v gate %+v", st.Current, st.Gate)
	}
}
