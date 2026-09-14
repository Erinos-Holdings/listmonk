package main

import (
	"encoding/json"
	"testing"
)

// Fork (TEST-SEND-WARNINGS-SPEC I3): GetSubscribersByEmail returns an empty result and no error
// when nothing matches, so TestCampaign can name the skipped addresses. Opt-in on
// LISTMONK_TEST_PG via newLinkHarness; not run in CI (G2b is the release-time backstop).
func TestGetSubscribersByEmailEmpty(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db

	var listID, subID int
	if err := db.Get(&listID, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'L', 'public', 'single', '{}') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&subID, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 'known@x', 'K', '{}') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, subID, listID)

	out, err := h.app.core.GetSubscribersByEmail([]string{"unknown@example.invalid"})
	if err != nil {
		t.Fatalf("unknown address: err %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("unknown address: got %d rows", len(out))
	}

	out, err = h.app.core.GetSubscribersByEmail([]string{"known@x", "unknown@example.invalid"})
	if err != nil {
		t.Fatalf("known address: err %v", err)
	}
	if len(out) != 1 || out[0].ID != subID {
		t.Fatalf("known address: got %+v", out)
	}
	var lists []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(out[0].Lists, &lists); err != nil {
		t.Fatalf("lists: %v (%s)", err, out[0].Lists)
	}
	if len(lists) != 1 || lists[0].ID != listID {
		t.Fatalf("lists not loaded: %s", out[0].Lists)
	}
}
