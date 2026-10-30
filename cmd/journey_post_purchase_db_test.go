package main

// Fork (integrations LIFECYCLE-SERIES-SPEC I13) -- every step-list guard that names no_purchase
// covers the Post-purchase series step kind post_purchase too (one helper, isStepTriggerKind),
// against a real database through the real router (stepFixture, journey_step_lists_db_test.go):
// a session add is refused and writes nothing, an API token passes, the list is never public, and
// a delayed automation is refused from both sides of the pair. LISTMONK_TEST_PG opt-in. Fixture
// names are acme.test / example.test only.

import (
	"encoding/json"
	"net/http"
	"testing"
)

const juPostPurchase = `{"kind":"post_purchase","days":14}`

// newPostStepFixture is newStepFixture with its step list switched to a post_purchase trigger.
func newPostStepFixture(t *testing.T) *stepFixture {
	t.Helper()
	s := newStepFixture(t)
	s.juTrigger(s.step, juPostPurchase)
	return s
}

func TestJourneyPostPurchaseStepGuards(t *testing.T) {
	s := newPostStepFixture(t)
	other := s.sub("other@example.test", s.plain)
	before := s.state()

	// A session may not add a member (get-step-list-add, refuseStepListAdd).
	s.refused("POST subscriber on the post-purchase step list", s.admin.json(http.MethodPost, "/api/subscribers",
		map[string]any{"email": "new@example.test", "name": "New", "lists": []int{s.plain, s.step}}))
	s.refused("manage add onto the post-purchase step list", s.admin.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{other}, "action": "add", "target_list_ids": []int{s.step}, "status": "confirmed"}))
	if got := s.state(); got != before {
		t.Fatalf("a refused write changed the rows:\n%s\n->\n%s", before, got)
	}
	// An API token (the sweep's writer) passes.
	s.ok("token manage add", s.token.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{other}, "action": "add", "target_list_ids": []int{s.step}, "status": "confirmed"}))
	if st := s.member(other, s.step); st != "confirmed" {
		t.Fatalf("token add: membership %s", st)
	}

	// Never public, from either side of the pair.
	s.wantErr("type public on the post-purchase step list", s.admin.json(http.MethodPut, "/api/lists/"+itoa(s.step),
		map[string]any{"name": "Acme step", "type": "public"}), http.StatusBadRequest, "lists.noPurchasePublic")
	s.wantErr("type public + post_purchase together", s.admin.json(http.MethodPut, "/api/lists/"+itoa(s.plain),
		map[string]any{"name": "Acme news", "type": "public", "join_trigger": json.RawMessage(juPostPurchase)}),
		http.StatusBadRequest, "lists.noPurchasePublic")
	if s.juStoredTrigger(s.plain) != "<nil>" {
		t.Fatal("refused write stored the trigger")
	}

	// The campaign side: an automation on the step list must send on join (delay 0).
	s.wantErr("POST delayed evergreen on post_purchase", s.juEvergreen(s.step, juWeek), http.StatusBadRequest,
		"campaigns.evergreenNoPurchaseDelay")
	s.campaignOf("POST delay-0 evergreen on post_purchase", s.juEvergreen(s.step, 0))

	// The list side: a post_purchase trigger over a delayed automation is refused.
	c := s.campaignOf("POST delayed evergreen on the plain list", s.juEvergreen(s.plain, juWeek))
	s.wantErr("post_purchase over a delayed evergreen", s.juPutList(s.plain, "Acme news", juPostPurchase),
		http.StatusBadRequest, "lists.triggerNoPurchaseDelayedEvergreen", "name", "C", "id", itoa(c.ID))
	if s.juStoredTrigger(s.plain) != "<nil>" {
		t.Fatal("refused write stored the trigger")
	}
}
