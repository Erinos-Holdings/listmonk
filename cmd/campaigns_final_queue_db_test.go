package main

// Fork (final-queue guard) -- integrations FINAL-QUEUE-GUARD-SPEC K2-K4 through the real router
// (bpFixture, brands_db_test.go): a non-evergreen campaign on a reengage:<n>:3|4 list is refused on
// create, update, Start and Schedule; an evergreen there and a regular campaign on queue 1/2 or any
// other list pass the guard; a list update keeps the list's reengage: tags. LISTMONK_TEST_PG opt-in.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newFinalQueueFixture is a bpFixture with a manager and the four queue lists of brand list 23
// plus an untagged list.
func newFinalQueueFixture(t *testing.T) (f *bpFixture, q map[int]int, plain int) {
	t.Helper()
	f = newBPFixture(t)
	// The URL-configured manager, so the footer guard sees a rendered unsubscribe link and the
	// status pass-throughs below are answered by the rule after it.
	ensureReviewManager(f.h.linkHarness)
	q = map[int]int{
		1: f.sqlList("Brand re-engagement 1", "reengage:23:1"),
		2: f.sqlList("Brand re-engagement 2", "reengage:23:2"),
		3: f.sqlList("Brand re-engagement 3", "reengage:23:3", "keep"),
		4: f.sqlList("Brand re-engagement 4", "reengage:23:4"),
	}
	plain = f.sqlList("Seed list", "seed")
	return f, q, plain
}

func (f *bpFixture) putStatus(id int, status string) *httptest.ResponseRecorder {
	return f.admin.json(http.MethodPut, "/api/campaigns/"+itoa(id)+"/status", map[string]any{"status": status})
}

func (f *bpFixture) campaignStatus(id int) string {
	f.h.t.Helper()
	var s string
	if err := f.h.db.Get(&s, `SELECT status FROM campaigns WHERE id = $1`, id); err != nil {
		f.h.t.Fatal(err)
	}
	return s
}

// K2 + K3 (create, update) -- a regular campaign naming a final queue list is refused 400 with
// the list's name and nothing is written; the same body as an evergreen, and a regular campaign
// on queue 1, queue 2 or an untagged list, are created.
func TestCampaignFinalQueueCreateUpdate(t *testing.T) {
	f, q, plain := newFinalQueueFixture(t)

	for _, n := range []int{3, 4} {
		name := "Brand re-engagement " + itoa(n)
		f.wantErr("create regular on queue "+itoa(n), f.postCampaign(q[n], "", nil),
			http.StatusBadRequest, "campaigns.finalQueueNoBroadcast", "list", name)
		// Mixed with an allowed list: still refused.
		f.wantErr("create regular on plain+queue "+itoa(n),
			f.admin.json(http.MethodPost, "/api/campaigns", pfBody(plain, "", map[string]any{"lists": []int{plain, q[n]}})),
			http.StatusBadRequest, "campaigns.finalQueueNoBroadcast", "list", name)
	}
	if n := f.count(`SELECT COUNT(*) FROM campaigns`); n != 0 {
		t.Fatalf("%d campaigns written by refused creates", n)
	}

	// K3 -- evergreens on the final queues, regular campaigns elsewhere.
	for _, n := range []int{3, 4} {
		f.campaignOf("create evergreen on queue "+itoa(n), f.postCampaign(q[n], "", map[string]any{"evergreen": true}))
	}
	for _, l := range []int{q[1], q[2], plain} {
		f.campaignOf("create regular on list "+itoa(l), f.postCampaign(l, "", nil))
	}

	// Update: a regular draft on the untagged list re-pointed at queue 4 is refused and its lists
	// stay as stored; re-pointed at queue 2 it saves.
	c := f.campaignOf("create draft", f.postCampaign(plain, "", nil))
	f.wantErr("update onto queue 4", f.putCampaign(c.ID, q[4], "", nil),
		http.StatusBadRequest, "campaigns.finalQueueNoBroadcast", "list", "Brand re-engagement 4")
	if n := f.count(`SELECT COUNT(*) FROM campaign_lists WHERE campaign_id = $1 AND list_id = $2`, c.ID, plain); n != 1 {
		t.Fatalf("the refused update changed the stored lists")
	}
	f.campaignOf("update onto queue 2", f.putCampaign(c.ID, q[2], "", nil))

	// An evergreen on queue 3 saves its update (the stored flag carries through the bind).
	e := f.campaignOf("create evergreen", f.postCampaign(q[3], "", map[string]any{"evergreen": true}))
	f.campaignOf("update evergreen on queue 3", f.putCampaign(e.ID, q[3], "", map[string]any{"evergreen": true, "name": "E2"}))
	f.campaignOf("update evergreen on queue 3, flag absent", f.putCampaign(e.ID, q[3], "", map[string]any{"name": "E3"}))
}

// K2 + K3 (Start, Schedule) -- the stored campaign's lists are read: a regular campaign saved on an
// untagged list that is LATER tagged reengage:23:4 is refused at running and scheduled with its
// status unchanged; a regular campaign on queue 1 or 2 enters running / scheduled, and an evergreen
// on queue 4 is answered by the next rule (the automation feature flag), not this one.
func TestCampaignFinalQueueStatus(t *testing.T) {
	f, q, plain := newFinalQueueFixture(t)

	c := f.campaignOf("create draft", f.postCampaign(plain, "", nil))
	f.h.db.MustExec(`UPDATE lists SET tags = '{reengage:23:4}' WHERE id = $1`, plain)
	for _, st := range []string{"running", "scheduled"} {
		f.wantErr("status "+st, f.putStatus(c.ID, st), http.StatusBadRequest,
			"campaigns.finalQueueNoBroadcast", "list", "Seed list")
		if s := f.campaignStatus(c.ID); s != "draft" {
			t.Fatalf("status %s refusal changed the stored status to %q", st, s)
		}
	}

	// K3 -- a body the footer guard accepts (the fixture's manager renders the unsubscribe URL)
	// and no review_url: a regular campaign on queue 1 or queue 2 actually enters running /
	// scheduled.
	body := `hello <a href="{{ UnsubscribeURL }}">unsubscribe</a>`
	for _, n := range []int{1, 2} {
		for _, st := range []string{"running", "scheduled"} {
			extra := map[string]any{"body": body}
			if st == "scheduled" {
				extra["send_at"] = time.Now().Add(48 * time.Hour).Format(time.RFC3339)
			}
			r := f.campaignOf("create on queue "+itoa(n), f.postCampaign(q[n], "", extra))
			f.ok("queue "+itoa(n)+" "+st, f.putStatus(r.ID, st))
			if s := f.campaignStatus(r.ID); s != st {
				t.Fatalf("queue %d campaign at %s: stored status %q", n, st, s)
			}
		}
	}

	// An evergreen on queue 4 passes this guard; the next rule to answer is core's automation
	// feature flag (EvergreenEnabled is off in the harness), named exactly so the test knows
	// which rule refused.
	e := f.campaignOf("create evergreen on queue 4", f.postCampaign(q[4], "", map[string]any{"evergreen": true, "body": body}))
	f.wantErr("evergreen on queue 4 running", f.putStatus(e.ID, "running"), http.StatusBadRequest, "campaigns.evergreenDisabled")
}

// K4 -- a list update that omits, alters or repeats its reengage: tag saves the list with the tag
// kept (with brand absent, "" or a slug); create with a typed reengage: tag stays allowed.
func TestListReengageTagKept(t *testing.T) {
	f, _, _ := newFinalQueueFixture(t)
	f.brandRow("acme", "Acme <hello@acme.test>", "")

	var l listResp
	json.Unmarshal(f.ok("create with reengage tag", f.admin.json(http.MethodPost, "/api/lists",
		map[string]any{"name": "Acme re-engagement 4", "type": "private", "optin": "single", "tags": []string{"reengage:23:4", "x"}})), &l)
	if got := f.tags(l.ID); got != "reengage:23:4|x" {
		t.Fatalf("created: %q", got)
	}

	put := func(what string, body map[string]any, want string) {
		t.Helper()
		body["name"] = "Acme re-engagement 4"
		f.ok(what, f.admin.json(http.MethodPut, "/api/lists/"+itoa(l.ID), body))
		if got := f.tags(l.ID); got != want {
			t.Fatalf("%s: %q want %q", what, got, want)
		}
	}
	put("tags []", map[string]any{"tags": []string{}}, "reengage:23:4")
	put("tags absent", map[string]any{}, "reengage:23:4")
	put("repeated", map[string]any{"tags": []string{"reengage:23:4", "y"}}, "reengage:23:4|y")
	put("brand acme", map[string]any{"tags": []string{"z"}, "brand": "acme"}, "brand:acme|from:Acme <hello@acme.test>|reengage:23:4|z")
	put("brand cleared", map[string]any{"tags": []string{}, "brand": ""}, "reengage:23:4")
	// Altered: a reengage: tag the list does not carry is refused on update; the stored tag stays.
	f.wantErr("altered", f.admin.json(http.MethodPut, "/api/lists/"+itoa(l.ID),
		map[string]any{"name": "Acme re-engagement 4", "tags": []string{"reengage:23:1"}}),
		http.StatusBadRequest, "lists.reservedTag", "tag", "reengage:23:1")
	if got := f.tags(l.ID); got != "reengage:23:4" {
		t.Fatalf("altered: %q", got)
	}
}
