package main

// Fork (integrations SKU-JOURNEYS-SPEC I8) -- the two SKU journey kinds, replenish and first_sku,
// are sweep-filled (isSweepFilledKind): every step-list GUARD covers them, against a real database
// through the real router (stepFixture, journey_step_lists_db_test.go) -- a session add is refused
// via get-step-list-add and writes nothing, an API token passes, the list is never public, a delayed
// automation is refused from both sides of the pair. The journey LOCK and the hide rule do NOT apply
// (TestLockSkuJourneyKindsUnlocked, lists_journey_lock_test.go; the systemListCases rows).
// LISTMONK_TEST_PG opt-in. Fixture names are acme.test / example.test only.

import (
	"encoding/json"
	"net/http"
	"testing"
)

const (
	juReplenish = `{"kind":"replenish","skus":["JoyBelle-BrushOnGlue"],"days":90}`
	juFirstSku  = `{"kind":"first_sku","skus":["joybelle-brushonglue"],"days":30}`
)

func TestJourneySkuStepGuards(t *testing.T) {
	for _, jt := range []string{juReplenish, juFirstSku} {
		t.Run(jt, func(t *testing.T) { skuStepGuards(t, jt) })
	}
}

// skuStepGuards is one kind's case (a subtest: each fixture owns the scratch database).
func skuStepGuards(t *testing.T, jt string) {
	{
		s := newStepFixture(t)
		s.juTrigger(s.step, jt)
		other := s.sub("other@example.test", s.plain)
		before := s.state()

		s.refused("POST subscriber on the SKU list "+jt, s.admin.json(http.MethodPost, "/api/subscribers",
			map[string]any{"email": "new@example.test", "name": "New", "lists": []int{s.plain, s.step}}))
		s.refused("manage add onto the SKU list "+jt, s.admin.json(http.MethodPut, "/api/subscribers/lists",
			map[string]any{"ids": []int{other}, "action": "add", "target_list_ids": []int{s.step}, "status": "confirmed"}))
		if got := s.state(); got != before {
			t.Fatalf("%s: a refused write changed the rows:\n%s\n->\n%s", jt, before, got)
		}
		s.ok("token manage add "+jt, s.token.json(http.MethodPut, "/api/subscribers/lists",
			map[string]any{"ids": []int{other}, "action": "add", "target_list_ids": []int{s.step}, "status": "confirmed"}))
		if st := s.member(other, s.step); st != "confirmed" {
			t.Fatalf("%s: token add: membership %s", jt, st)
		}

		s.wantErr("type public on the SKU list "+jt, s.admin.json(http.MethodPut, "/api/lists/"+itoa(s.step),
			map[string]any{"name": "Acme step", "type": "public"}), http.StatusBadRequest, "lists.noPurchasePublic")
		s.wantErr("type public + SKU kind together "+jt, s.admin.json(http.MethodPut, "/api/lists/"+itoa(s.plain),
			map[string]any{"name": "Acme news", "type": "public", "join_trigger": json.RawMessage(jt)}),
			http.StatusBadRequest, "lists.noPurchasePublic")

		s.wantErr("POST delayed evergreen on the SKU list "+jt, s.juEvergreen(s.step, juWeek), http.StatusBadRequest,
			"campaigns.evergreenNoPurchaseDelay")
		s.campaignOf("POST delay-0 evergreen on the SKU list "+jt, s.juEvergreen(s.step, 0))

		c := s.campaignOf("POST delayed evergreen on the plain list "+jt, s.juEvergreen(s.plain, juWeek))
		s.wantErr("SKU kind over a delayed evergreen "+jt, s.juPutList(s.plain, "Acme news", jt),
			http.StatusBadRequest, "lists.triggerNoPurchaseDelayedEvergreen", "name", "C", "id", itoa(c.ID))
		if s.juStoredTrigger(s.plain) != "<nil>" {
			t.Fatalf("%s: refused write stored the trigger", jt)
		}
	}
}
