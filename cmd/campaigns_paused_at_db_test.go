package main

// Fork (campaign pause time) -- integrations listmonk/CAMPAIGN-STATUS-FILTER-SPEC I1/I2 against a
// real database: update-campaign-status stamps campaigns.paused_at on every transition to paused
// and leaves it untouched on any other transition (I1, through the API path
// core.UpdateCampaignStatus); the sender's auto-pause path, the manager store's
// UpdateCampaignStatus (cmd/manager_store.go, called from internal/manager/pipe.go), stamps it too
// (I2 -- driven through a real store, not by asserting statement identity). Shares newLinkHarness
// (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go). The migration is
// internal/migrations/v6_2_39_db_test.go.

import (
	"database/sql"
	"io"
	"log"
	"testing"
	"time"

	"github.com/knadh/listmonk/models"
)

func (h *linkHarness) pausedAtCampaign(name string, status string) int {
	h.t.Helper()
	var id int
	if err := h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, status, attribs)
		VALUES (gen_random_uuid(), $1, 's', 'Acme <hello@example.test>', '<p>b</p>', 'email', $2, '{"lang":"en"}') RETURNING id`, name, status); err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *linkHarness) pausedAt(id int) sql.NullTime {
	h.t.Helper()
	var ts sql.NullTime
	if err := h.db.Get(&ts, `SELECT paused_at FROM campaigns WHERE id = $1`, id); err != nil {
		h.t.Fatal(err)
	}
	return ts
}

func (h *linkHarness) dbNow() time.Time {
	h.t.Helper()
	var ts time.Time
	if err := h.db.Get(&ts, `SELECT clock_timestamp()`); err != nil {
		h.t.Fatal(err)
	}
	return ts
}

func (h *linkHarness) campaignStatus(id int) string {
	h.t.Helper()
	var s string
	if err := h.db.Get(&s, `SELECT status FROM campaigns WHERE id = $1`, id); err != nil {
		h.t.Fatal(err)
	}
	return s
}

// TestCampaignPausedAtWriter is I1 (the API path) and I2 (the sender's store path).
func TestCampaignPausedAtWriter(t *testing.T) {
	h := newLinkHarness(t)

	// I1 -- running -> paused stamps; paused -> running keeps the value; draft -> running leaves NULL.
	start := h.dbNow()
	run := h.pausedAtCampaign("Paused via API", models.CampaignStatusRunning)
	if _, err := h.app.core.UpdateCampaignStatus(run, models.CampaignStatusPaused); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if got := h.campaignStatus(run); got != models.CampaignStatusPaused {
		t.Fatalf("status after pause = %s", got)
	}
	stamped := h.pausedAt(run)
	if !stamped.Valid || stamped.Time.Before(start) {
		t.Fatalf("pause: paused_at %v, want non-null and >= %v", stamped, start)
	}

	if _, err := h.app.core.UpdateCampaignStatus(run, models.CampaignStatusRunning); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if got := h.campaignStatus(run); got != models.CampaignStatusRunning {
		t.Fatalf("status after resume = %s", got)
	}
	if kept := h.pausedAt(run); !kept.Valid || !kept.Time.Equal(stamped.Time) {
		t.Fatalf("resume changed paused_at: %v, want %v", kept, stamped.Time)
	}

	// Cancel (another non-paused transition) keeps it as well.
	if _, err := h.app.core.UpdateCampaignStatus(run, models.CampaignStatusCancelled); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if kept := h.pausedAt(run); !kept.Valid || !kept.Time.Equal(stamped.Time) {
		t.Fatalf("cancel changed paused_at: %v, want %v", kept, stamped.Time)
	}

	draft := h.pausedAtCampaign("Started draft", models.CampaignStatusDraft)
	if _, err := h.app.core.UpdateCampaignStatus(draft, models.CampaignStatusRunning); err != nil {
		t.Fatalf("start: %v", err)
	}
	if got := h.campaignStatus(draft); got != models.CampaignStatusRunning {
		t.Fatalf("status after start = %s", got)
	}
	if ts := h.pausedAt(draft); ts.Valid {
		t.Fatalf("start stamped paused_at: %v", ts.Time)
	}

	// The model carries it on every c.* read.
	cm, err := h.app.core.GetCampaign(run, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !cm.PausedAt.Valid || !cm.PausedAt.Time.Equal(stamped.Time) {
		t.Fatalf("GetCampaign PausedAt = %v, want %v", cm.PausedAt, stamped.Time)
	}

	// I2 -- the sender's too-many-errors auto-pause: the manager store's UpdateCampaignStatus.
	st := newManagerStore(h.q, h.app.core, nil, h.db, log.New(io.Discard, "", 0))
	auto := h.pausedAtCampaign("Auto-paused", models.CampaignStatusRunning)
	start = h.dbNow()
	if err := st.UpdateCampaignStatus(auto, models.CampaignStatusPaused); err != nil {
		t.Fatalf("store pause: %v", err)
	}
	if got := h.campaignStatus(auto); got != models.CampaignStatusPaused {
		t.Fatalf("status after store pause = %s", got)
	}
	first := h.pausedAt(auto)
	if !first.Valid || first.Time.Before(start) {
		t.Fatalf("store pause: paused_at %v, want non-null and >= %v", first, start)
	}

	// A repeated transition to paused re-stamps (D2).
	time.Sleep(5 * time.Millisecond)
	if err := st.UpdateCampaignStatus(auto, models.CampaignStatusPaused); err != nil {
		t.Fatalf("store re-pause: %v", err)
	}
	if again := h.pausedAt(auto); !again.Valid || !again.Time.After(first.Time) {
		t.Fatalf("re-pause: paused_at %v, want after %v", again, first.Time)
	}
}
