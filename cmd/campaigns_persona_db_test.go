package main

// Fork (persona From) -- integrations PERSONA-FROM-SPEC I3, I4, I7 and I13 against a real database
// through the real router (bpFixture, brands_db_test.go), so pm(), the session middleware and the
// campaign handlers are the production ones. LISTMONK_TEST_PG opt-in. The harness configures one
// SMTP from address, hello@acme.test; fixture names are acme.test / example.test only.

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/notifs"
	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
)

const (
	pfBrandFrom = "Acme <hello@acme.test>"
	pfNat       = "Nat at Acme <hello@acme.test>"
	pfJo        = "Jo at Acme <hello@acme.test>"
)

type pfCampaign struct {
	ID        int                 `json:"id"`
	FromEmail string              `json:"from_email"`
	Status    string              `json:"status"`
	Headers   []map[string]string `json:"headers"`
}

// newPersonaFixture is a bpFixture with a manager (the campaign handlers ask it for the messenger
// roster and the template funcs), the brand acme with a tagged list, and an untagged list.
func newPersonaFixture(t *testing.T) (f *bpFixture, tagged, untagged int) {
	t.Helper()
	f = newBPFixture(t)
	ensureManager(f.h.linkHarness)
	f.brandRow("acme", pfBrandFrom, "")
	tagged = f.sqlList("Acme list", "brand:acme", "from:"+pfBrandFrom)
	untagged = f.sqlList("Seed list", "seed")
	return f, tagged, untagged
}

func (f *bpFixture) setPersonas(slug string, personas ...string) {
	f.h.t.Helper()
	if personas == nil {
		personas = []string{}
	}
	f.h.db.MustExec(`UPDATE brands SET personas = $2 WHERE slug = $1`, slug, pq.StringArray(personas))
}

func pfBody(list int, from string, extra map[string]any) map[string]any {
	b := map[string]any{
		"name": "C", "subject": "s", "lists": []int{list}, "type": "regular",
		"content_type": "plain", "body": "hello", "messenger": "email",
	}
	if from != "" {
		b["from_email"] = from
	}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

func (f *bpFixture) postCampaign(list int, from string, extra map[string]any) *httptest.ResponseRecorder {
	return f.admin.json(http.MethodPost, "/api/campaigns", pfBody(list, from, extra))
}

func (f *bpFixture) putCampaign(id, list int, from string, extra map[string]any) *httptest.ResponseRecorder {
	return f.admin.json(http.MethodPut, "/api/campaigns/"+itoa(id), pfBody(list, from, extra))
}

func (f *bpFixture) campaignOf(what string, rec *httptest.ResponseRecorder) pfCampaign {
	f.h.t.Helper()
	var c pfCampaign
	if err := json.Unmarshal(f.ok(what, rec), &c); err != nil {
		f.h.t.Fatalf("%s: %v", what, err)
	}
	return c
}

// brandTagOf is the campaign's X-SES-MESSAGE-TAGS value.
func brandTagOf(c pfCampaign) string {
	for _, h := range c.Headers {
		for k, v := range h {
			if strings.EqualFold(k, sesTagHeader) {
				return v
			}
		}
	}
	return ""
}

// I3 + I13 -- create and update on a tagged list accept the brand From and every canonical persona
// From (the SES brand tag still derived from the list mapping); every other From is refused with
// the named key, and with NO personas on the row the message is today's brandFromMismatch.
func TestCampaignPersonaFrom(t *testing.T) {
	f, tagged, _ := newPersonaFixture(t)

	// No personas on the row: a persona-shaped From is refused with the unchanged message.
	f.wantErr("no personas", f.postCampaign(tagged, pfNat, nil), http.StatusBadRequest,
		"campaigns.brandFromMismatch", "from", pfNat, "expected", pfBrandFrom, "list", "Acme list")
	if got, want := message(f.postCampaign(tagged, pfNat, nil)),
		`From address "Nat at Acme <hello@acme.test>" does not match "Acme <hello@acme.test>", the From address of list "Acme list".`; got != want {
		t.Fatalf("the no-persona message changed:\n got  %s\n want %s", got, want)
	}
	if n := f.count(`SELECT COUNT(*) FROM campaigns`); n != 0 {
		t.Fatalf("%d campaigns written by refused creates", n)
	}

	f.setPersonas("acme", "Nat at Acme", "Jo at Acme")

	// Accepted on create: the brand From and each canonical persona From, stored verbatim, with
	// brand=acme in the headers whatever the From (I13).
	var ids []int
	for _, from := range []string{pfBrandFrom, pfNat, pfJo} {
		c := f.campaignOf("POST "+from, f.postCampaign(tagged, from, nil))
		if c.FromEmail != from {
			t.Fatalf("POST %q stored %q", from, c.FromEmail)
		}
		if brandTagOf(c) != "brand=acme" {
			t.Fatalf("POST %q: brand tag %q", from, brandTagOf(c))
		}
		ids = append(ids, c.ID)
	}
	// An omitted From still derives the brand From, never a persona (S7).
	if c := f.campaignOf("POST no From", f.postCampaign(tagged, "", nil)); c.FromEmail != pfBrandFrom {
		t.Fatalf("omitted From derived %q", c.FromEmail)
	}

	// Accepted on update: brand From -> persona -> other persona -> brand From.
	id := ids[0]
	for _, from := range []string{pfNat, pfJo, pfBrandFrom, pfNat} {
		c := f.campaignOf("PUT "+from, f.putCampaign(id, tagged, from, nil))
		if c.FromEmail != from || brandTagOf(c) != "brand=acme" {
			t.Fatalf("PUT %q stored %q, tag %q", from, c.FromEmail, brandTagOf(c))
		}
	}

	// Refused, on create and on update, each naming the accepted Froms.
	accepted := strings.Join([]string{pfBrandFrom, pfNat, pfJo}, ", ")
	before := f.count(`SELECT COUNT(*) FROM campaigns`)
	for name, from := range map[string]string{
		"a persona not on the row":    "Kim at Acme <hello@acme.test>",
		"a different bare address":    "Nat at Acme <hello@beta.test>",
		"quoted display name":         `"Nat at Acme" <hello@acme.test>`,
		"double space before address": "Nat at Acme  <hello@acme.test>",
		"double space in the name":    "Nat  at Acme <hello@acme.test>",
		"a bare address":              "hello@acme.test",
		"case variant":                "nat at acme <hello@acme.test>",
		"another display name":        "Acme Shop <hello@acme.test>",
	} {
		f.wantErr("POST "+name, f.postCampaign(tagged, from, nil), http.StatusBadRequest,
			"campaigns.brandFromNotPersona", "from", from, "list", "Acme list", "accepted", accepted)
		f.wantErr("PUT "+name, f.putCampaign(id, tagged, from, nil), http.StatusBadRequest,
			"campaigns.brandFromNotPersona", "from", from, "list", "Acme list", "accepted", accepted)
	}
	if n := f.count(`SELECT COUNT(*) FROM campaigns`); n != before {
		t.Fatalf("refused creates wrote %d campaign(s)", n-before)
	}
	if n := f.count(`SELECT COUNT(*) FROM campaigns WHERE id = $1 AND from_email = $2`, id, pfNat); n != 1 {
		t.Fatal("a refused update changed the stored From")
	}

	// A persona belongs to one brand: another brand's list refuses acme's persona From.
	f.brandRow("beta", "Beta <hello@acme.test>", "")
	other := f.sqlList("Beta list", "brand:beta", "from:Beta <hello@acme.test>")
	f.wantErr("persona on the wrong brand", f.postCampaign(other, pfNat, nil), http.StatusBadRequest,
		"campaigns.brandFromMismatch", "from", pfNat, "expected", "Beta <hello@acme.test>", "list", "Beta list")

	// SQL-written tags with no brands row: no personas, so the exact match and today's message.
	orphan := f.sqlList("Orphan list", "brand:orphan", "from:Orphan <hello@acme.test>")
	f.wantErr("no row", f.postCampaign(orphan, "Nat at Orphan <hello@acme.test>", nil), http.StatusBadRequest,
		"campaigns.brandFromMismatch", "from", "Nat at Orphan <hello@acme.test>", "expected", "Orphan <hello@acme.test>", "list", "Orphan list")
}

// I3 (Stage 2 F1) -- a campaign `headers` entry keyed From, in any case, is refused on a mapped
// AND on an unmapped campaign, on create and on update; another header passes.
func TestCampaignHeaderFromRefused(t *testing.T) {
	f, tagged, untagged := newPersonaFixture(t)

	for name, list := range map[string]int{"mapped": tagged, "unmapped": untagged} {
		ok := f.campaignOf(name+" control", f.postCampaign(list, "", map[string]any{
			"headers": []map[string]string{{"X-Custom": "1"}}}))
		before := f.count(`SELECT COUNT(*) FROM campaigns`)

		for _, key := range []string{"From", "from", "FROM", "fRoM"} {
			hdrs := map[string]any{"headers": []map[string]string{{"X-Custom": "1"}, {key: "Evil <hello@acme.test>"}}}
			f.wantErr(name+" POST "+key, f.postCampaign(list, "", hdrs), http.StatusBadRequest,
				"campaigns.headerReserved", "header", key)
			f.wantErr(name+" PUT "+key, f.putCampaign(ok.ID, list, "", hdrs), http.StatusBadRequest,
				"campaigns.headerReserved", "header", key)
		}
		// The same entry beside the SES tag header in one object is refused too.
		f.wantErr(name+" same object", f.postCampaign(list, "", map[string]any{
			"headers": []map[string]string{{sesTagHeader: "brand=x", "From": "Evil <hello@acme.test>"}}}),
			http.StatusBadRequest, "campaigns.headerReserved", "header", "From")

		if n := f.count(`SELECT COUNT(*) FROM campaigns`); n != before {
			t.Fatalf("%s: refused creates wrote %d campaign(s)", name, n-before)
		}
		if n := f.count(`SELECT COUNT(*) FROM campaigns WHERE id = $1 AND headers::TEXT ILIKE '%"from"%'`, ok.ID); n != 0 {
			t.Fatalf("%s: a refused update stored a From header", name)
		}
		// Headers that merely contain the word are not the key From.
		f.campaignOf(name+" other headers", f.putCampaign(ok.ID, list, "", map[string]any{
			"headers": []map[string]string{{"X-From": "1"}, {"Reply-To": "r@acme.test"}}}))
	}
}

// I4 -- a test send validates through validateCampaignFields, exactly as TestCampaign calls it
// (NOT the /test route: that needs subscribers and a running manager). A persona From passes; a
// From naming a persona no longer on the row is refused.
func TestCampaignPersonaTestSend(t *testing.T) {
	f, tagged, _ := newPersonaFixture(t)
	f.setPersonas("acme", "Nat at Acme", "Jo at Acme")

	req := func(from string) campReq {
		return campReq{
			Campaign: models.Campaign{
				Name: "C", Subject: "s", FromEmail: from, Body: "hello", Messenger: "email",
				ContentType: models.CampaignContentTypePlain, Type: models.CampaignTypeRegular,
			},
			ListIDs:          []int{tagged},
			SubscriberEmails: pq.StringArray{"someone@example.test"},
		}
	}

	out, err := f.h.app.validateCampaignFields(req(pfNat))
	if err != nil {
		t.Fatalf("a persona From failed test-send validation: %v", err)
	}
	if out.FromEmail != pfNat {
		t.Fatalf("validated From %q", out.FromEmail)
	}

	// The persona is retired (the side door: SQL). The stored From no longer validates.
	f.setPersonas("acme", "Jo at Acme")
	_, err = f.h.app.validateCampaignFields(req(pfNat))
	want := f.h.app.i18n.Ts("campaigns.brandFromNotPersona", "from", pfNat, "list", "Acme list",
		"accepted", pfBrandFrom+", "+pfJo)
	if err == nil || err.Error() != want {
		t.Fatalf("retired persona:\n got  %v\n want %s", err, want)
	}

	// With the whole set gone it is today's exact-match refusal.
	f.setPersonas("acme")
	_, err = f.h.app.validateCampaignFields(req(pfNat))
	want = f.h.app.i18n.Ts("campaigns.brandFromMismatch", "from", pfNat, "expected", pfBrandFrom, "list", "Acme list")
	if err == nil || err.Error() != want {
		t.Fatalf("no personas:\n got  %v\n want %s", err, want)
	}
}

// I7 -- system mail never carries a persona: the opt-in campaign derivation and the empty-From
// derivation both yield the brand From on a brand whose row has personas.
func TestSystemMailUsesBrandFrom(t *testing.T) {
	f, tagged, _ := newPersonaFixture(t)
	f.setPersonas("acme", "Nat at Acme", "Jo at Acme")
	var dbl int
	f.h.db.Get(&dbl, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme double', 'public', 'double', $1) RETURNING id`,
		pq.StringArray{"brand:acme", "from:" + pfBrandFrom})

	// makeOptinCampaignMessage renders the opt-in body from the notification templates, which
	// the harness does not load.
	prev := notifs.Tpls
	notifs.Tpls = template.Must(template.New("optin-campaign").Parse(`{{ range .Lists }}{{ .Name }}{{ end }}`))
	t.Cleanup(func() { notifs.Tpls = prev })

	o, err := f.h.app.makeOptinCampaignMessage(campReq{
		Campaign: models.Campaign{Name: "O", Subject: "s", Type: models.CampaignTypeOptin, Messenger: "email"},
		ListIDs:  []int{dbl},
	})
	if err != nil {
		t.Fatalf("makeOptinCampaignMessage: %v", err)
	}
	if o.FromEmail != pfBrandFrom {
		t.Fatalf("opt-in campaign From %q, want the brand From", o.FromEmail)
	}

	// The empty-From derivation, for the opt-in request and for a regular one.
	for name, r := range map[string]campReq{
		"optin": {Campaign: models.Campaign{Name: "O", Subject: "s", Body: o.Body, Type: models.CampaignTypeOptin,
			Messenger: "email", ContentType: models.CampaignContentTypePlain}, ListIDs: []int{dbl}},
		"regular": {Campaign: models.Campaign{Name: "C", Subject: "s", Body: "b", Type: models.CampaignTypeRegular,
			Messenger: "email", ContentType: models.CampaignContentTypePlain}, ListIDs: []int{tagged}},
		"blank From": {Campaign: models.Campaign{Name: "C", Subject: "s", Body: "b", FromEmail: "  ", Type: models.CampaignTypeRegular,
			Messenger: "email", ContentType: models.CampaignContentTypePlain}, ListIDs: []int{tagged}},
	} {
		out, err := f.h.app.validateCampaignFields(r)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out.FromEmail != pfBrandFrom {
			t.Fatalf("%s: derived From %q, want the brand From", name, out.FromEmail)
		}
	}

	// And through the route: an opt-in campaign created over the API on that list.
	c := f.campaignOf("POST optin", f.admin.json(http.MethodPost, "/api/campaigns", map[string]any{
		"name": "Optin", "subject": "s", "lists": []int{dbl}, "type": "optin", "content_type": "plain", "messenger": "email"}))
	if c.FromEmail != pfBrandFrom {
		t.Fatalf("opt-in campaign over the API stored From %q", c.FromEmail)
	}
}
