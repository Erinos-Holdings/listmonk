package main

// Fork (journey form UX) -- integrations JOURNEY-UX-SPEC I6 (D5: validateCampaignFields refuses an
// evergreen with a nonzero delay on a no_purchase list) and I6.1 (D5.1: the list write refuses a
// no_purchase trigger while a non-cancelled evergreen with a nonzero delay targets the list),
// against a real database through the real router (bpFixture, brands_db_test.go).
// LISTMONK_TEST_PG opt-in. Fixture names are acme.test only.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	juNoPurchase = `{"kind":"no_purchase","days":7}`
	juFirstOrder = `{"kind":"first_order"}`
	juWeek       = 7 * 86400
)

// juTrigger sets a list's stored join_trigger directly ("" = NULL).
func (f *bpFixture) juTrigger(list int, jt string) {
	f.h.t.Helper()
	f.h.db.MustExec(`UPDATE lists SET join_trigger = NULLIF($2, '')::JSONB WHERE id = $1`, list, jt)
}

func (f *bpFixture) juEvergreen(list int, delay int) *httptest.ResponseRecorder {
	return f.postCampaign(list, "", map[string]any{"evergreen": true, "send_delay_secs": delay})
}

func (f *bpFixture) juPutList(list int, name, jt string) *httptest.ResponseRecorder {
	body := map[string]any{"name": name}
	if jt != "" {
		body["join_trigger"] = json.RawMessage(jt)
	}
	return f.admin.json(http.MethodPut, "/api/lists/"+itoa(list), body)
}

func (f *bpFixture) juStoredTrigger(list int) string {
	f.h.t.Helper()
	var s *string
	if err := f.h.db.Get(&s, `SELECT join_trigger::TEXT FROM lists WHERE id = $1`, list); err != nil {
		f.h.t.Fatal(err)
	}
	if s == nil {
		return "<nil>"
	}
	return *s
}

// I6 -- D5. Create and update refuse an evergreen with a nonzero delay on a no_purchase list and
// write nothing; delay 0 there is accepted (the live shape: campaign 229, a running delay-0
// evergreen on a no_purchase list); a nonzero delay on a purchase list or a trigger-less list is
// accepted; a broadcast to the no_purchase list is legal (D3 warns only).
func TestJourneyNoPurchaseDelay(t *testing.T) {
	f, np, _ := newPersonaFixture(t)
	f.juTrigger(np, juNoPurchase)
	po := f.sqlList("Acme first order", "brand:acme", "from:"+pfBrandFrom)
	f.juTrigger(po, juFirstOrder)
	none := f.sqlList("Acme welcome", "brand:acme", "from:"+pfBrandFrom)

	f.wantErr("POST delayed evergreen on no_purchase", f.juEvergreen(np, juWeek), http.StatusBadRequest,
		"campaigns.evergreenNoPurchaseDelay")
	f.wantErr("POST 1s-delayed evergreen on no_purchase", f.juEvergreen(np, 1), http.StatusBadRequest,
		"campaigns.evergreenNoPurchaseDelay")
	if n := f.count(`SELECT COUNT(*) FROM campaigns`); n != 0 {
		t.Fatalf("%d campaigns written by refused creates", n)
	}

	// Delay 0 on the no_purchase list: accepted on create, and on a re-save.
	c := f.campaignOf("POST delay-0 evergreen on no_purchase", f.juEvergreen(np, 0))
	f.campaignOf("PUT delay-0 evergreen on no_purchase",
		f.putCampaign(c.ID, np, "", map[string]any{"evergreen": true, "send_delay_secs": 0}))

	// Update to a nonzero delay: refused, the stored delay unchanged.
	f.wantErr("PUT delayed evergreen on no_purchase",
		f.putCampaign(c.ID, np, "", map[string]any{"evergreen": true, "send_delay_secs": juWeek}),
		http.StatusBadRequest, "campaigns.evergreenNoPurchaseDelay")
	if d := f.count(`SELECT send_delay_secs FROM campaigns WHERE id = $1`, c.ID); d != 0 {
		t.Fatalf("refused update stored delay %d", d)
	}

	// A nonzero delay on a purchase list and on a trigger-less list is accepted.
	f.campaignOf("POST delayed evergreen on first_order", f.juEvergreen(po, juWeek))
	f.campaignOf("POST delayed evergreen on no trigger", f.juEvergreen(none, juWeek))

	// A broadcast to the no_purchase list stays legal.
	f.campaignOf("POST broadcast on no_purchase", f.postCampaign(np, "", nil))
}

// I6.1 -- D5.1. Setting no_purchase on a list hosting a nonzero-delay evergreen in any status but
// cancelled is refused and leaves the trigger unset; it is accepted once that campaign is
// cancelled, on a list whose evergreen has delay 0, and on a list with no campaign. Other kinds
// are not affected.
func TestJourneyNoPurchaseTriggerVsDelayedEvergreen(t *testing.T) {
	f, delayed, _ := newPersonaFixture(t)
	c := f.campaignOf("POST delayed evergreen", f.juEvergreen(delayed, juWeek))

	for _, status := range []string{"draft", "running", "paused", "finished"} {
		f.h.db.MustExec(`UPDATE campaigns SET status = $2::campaign_status WHERE id = $1`, c.ID, status)
		f.wantErr("no_purchase over a "+status+" delayed evergreen", f.juPutList(delayed, "Acme list", juNoPurchase),
			http.StatusBadRequest, "lists.triggerNoPurchaseDelayedEvergreen", "name", "C", "id", itoa(c.ID))
		if got := f.juStoredTrigger(delayed); got != "<nil>" {
			t.Fatalf("%s: refused write stored %s", status, got)
		}
	}

	// Another kind is not affected.
	f.ok("first_order over a delayed evergreen", f.juPutList(delayed, "Acme list", juFirstOrder))

	// Cancelled: accepted.
	f.h.db.MustExec(`UPDATE campaigns SET status = 'cancelled' WHERE id = $1`, c.ID)
	f.ok("no_purchase over a cancelled delayed evergreen", f.juPutList(delayed, "Acme list", juNoPurchase))
	if got := f.juStoredTrigger(delayed); got == "<nil>" {
		t.Fatal("accepted write stored no trigger")
	}

	// A delay-0 evergreen: accepted.
	zero := f.sqlList("Acme step", "brand:acme", "from:"+pfBrandFrom)
	f.campaignOf("POST delay-0 evergreen", f.juEvergreen(zero, 0))
	f.ok("no_purchase over a delay-0 evergreen", f.juPutList(zero, "Acme step", juNoPurchase))

	// No campaign: accepted.
	empty := f.sqlList("Acme empty", "brand:acme", "from:"+pfBrandFrom)
	f.ok("no_purchase on a list with no campaign", f.juPutList(empty, "Acme empty", juNoPurchase))
}
