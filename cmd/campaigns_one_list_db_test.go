package main

// Fork (campaign lists) -- integrations listmonk/CAMPAIGN-LISTS-SPEC I3 and I4 through the real
// router (bpFixture, brands_db_test.go): a campaign targets exactly one list -- create, update,
// the opt-in path and the test send refuse an otherwise-valid multi-list body 400
// campaigns.oneList (evergreens included), a two-brand pair keeps its brandFromConflict error --
// and the server still accepts a single hidden system list (D3: the selector hides it, the server
// never refuses it). LISTMONK_TEST_PG opt-in (newLinkHarness skips without it).

import (
	"html/template"
	"net/http"
	"testing"

	"github.com/knadh/listmonk/internal/notifs"
	"github.com/lib/pq"
)

// I3 -- one list per campaign on the server.
func TestCampaignOneList(t *testing.T) {
	f, tagged, untagged := newPersonaFixture(t)
	tagged2 := f.sqlList("Acme list two", "brand:acme", "from:"+pfBrandFrom)
	untagged2 := f.sqlList("Seed list two", "seed")
	beta := f.sqlList("Beta list", "brand:beta", "from:Beta <hello@beta.test>")

	two := func(a, b int, extra map[string]any) map[string]any {
		body := pfBody(a, "", extra)
		body["lists"] = []int{a, b}
		return body
	}

	// Create: two lists of one brand, two untagged lists, an evergreen on two -- each refused, and
	// nothing written.
	f.wantErr("create two acme lists", f.admin.json(http.MethodPost, "/api/campaigns", two(tagged, tagged2, nil)),
		http.StatusBadRequest, "campaigns.oneList")
	f.wantErr("create two untagged lists", f.admin.json(http.MethodPost, "/api/campaigns", two(untagged, untagged2, nil)),
		http.StatusBadRequest, "campaigns.oneList")
	f.wantErr("create evergreen on two", f.admin.json(http.MethodPost, "/api/campaigns",
		two(tagged, tagged2, map[string]any{"evergreen": true})), http.StatusBadRequest, "campaigns.oneList")
	// A two-brand pair keeps its brand error (the brand mapping runs first).
	f.wantErr("create two brands", f.admin.json(http.MethodPost, "/api/campaigns", two(tagged, beta, nil)),
		http.StatusBadRequest, "campaigns.brandFromConflict", "brands", "acme, beta")
	if n := f.count(`SELECT COUNT(*) FROM campaigns`); n != 0 {
		t.Fatalf("%d campaigns written by refused creates", n)
	}

	// One list passes, tagged and untagged; an evergreen on one list passes.
	c := f.campaignOf("create on one acme list", f.postCampaign(tagged, "", nil))
	f.campaignOf("create on one untagged list", f.postCampaign(untagged, "", nil))
	f.campaignOf("create evergreen on one list", f.postCampaign(tagged2, "", map[string]any{"evergreen": true}))

	// Update: two lists refused and the stored list kept; one list saves.
	f.wantErr("update onto two", f.admin.json(http.MethodPut, "/api/campaigns/"+itoa(c.ID), two(tagged, tagged2, nil)),
		http.StatusBadRequest, "campaigns.oneList")
	if n := f.count(`SELECT COUNT(*) FROM campaign_lists WHERE campaign_id = $1`, c.ID); n != 1 {
		t.Fatalf("the refused update left %d list rows", n)
	}
	f.campaignOf("update onto one", f.putCampaign(c.ID, tagged2, "", nil))

	// The test send runs the same validation: two lists refused before any subscriber is read.
	f.wantErr("test send on two", f.admin.json(http.MethodPost, "/api/campaigns/"+itoa(c.ID)+"/test",
		two(tagged, tagged2, map[string]any{"subscribers": []string{"someone@example.test"}})),
		http.StatusBadRequest, "campaigns.oneList")

	// The opt-in path: makeOptinCampaignMessage renders the body (the harness does not load the
	// notification templates, so a stub stands in), then the same validateCampaignFields refuses.
	prev := notifs.Tpls
	notifs.Tpls = template.Must(template.New("optin-campaign").Parse(`{{ range .Lists }}{{ .Name }}{{ end }}`))
	t.Cleanup(func() { notifs.Tpls = prev })
	var d1, d2 int
	for i, id := range []*int{&d1, &d2} {
		f.h.db.Get(id, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), $1, 'public', 'double', $2) RETURNING id`,
			"Acme double "+itoa(i), pq.StringArray{"brand:acme", "from:" + pfBrandFrom})
	}
	before := f.count(`SELECT COUNT(*) FROM campaigns`)
	f.wantErr("create opt-in on two", f.admin.json(http.MethodPost, "/api/campaigns",
		two(d1, d2, map[string]any{"type": "optin"})), http.StatusBadRequest, "campaigns.oneList")
	if n := f.count(`SELECT COUNT(*) FROM campaigns`); n != before {
		t.Fatalf("the refused opt-in create wrote a campaign")
	}
	f.campaignOf("create opt-in on one", f.postCampaign(d1, "", map[string]any{"type": "optin"}))
}

// I4 -- the server still accepts a hidden system list: a create and an update carrying exactly one
// hidden step list (and a final queue as an evergreen) pass validation and are stored.
func TestCreateCampaignOnSystemList(t *testing.T) {
	f, tagged, _ := newPersonaFixture(t)
	from := "from:" + pfBrandFrom
	step := f.sqlList("Acme No purchase 2 days", "brand:acme", from)
	f.juTrigger(step, `{"kind":"no_purchase","days":2}`)
	fo := f.sqlList("Acme first order", "brand:acme", from)
	f.juTrigger(fo, `{"kind":"first_order"}`)
	queue4 := f.sqlList("Acme re-engagement 4", "brand:acme", from, "reengage:5:4")

	for _, id := range []int{step, fo, queue4} {
		l, err := f.h.app.core.GetList(id, "")
		if err != nil || !isHiddenSystemList(l) {
			t.Fatalf("fixture list %d is not hidden (%v)", id, err)
		}
	}

	stored := func(campID, listID int) {
		t.Helper()
		if n := f.count(`SELECT COUNT(*) FROM campaign_lists WHERE campaign_id = $1 AND list_id = $2`, campID, listID); n != 1 {
			t.Fatalf("campaign %d is not stored on list %d", campID, listID)
		}
	}

	c := f.campaignOf("create evergreen on the step list", f.postCampaign(step, "", map[string]any{"evergreen": true}))
	stored(c.ID, step)
	r := f.campaignOf("create regular on the first-order list", f.postCampaign(fo, "", nil))
	stored(r.ID, fo)
	q := f.campaignOf("create evergreen on queue 4", f.postCampaign(queue4, "", map[string]any{"evergreen": true}))
	stored(q.ID, queue4)

	// Update: a draft on an ordinary list re-pointed at the hidden step list saves and is stored.
	d := f.campaignOf("create draft", f.postCampaign(tagged, "", nil))
	f.campaignOf("update onto the step list", f.putCampaign(d.ID, step, "", map[string]any{"evergreen": true}))
	stored(d.ID, step)
}
