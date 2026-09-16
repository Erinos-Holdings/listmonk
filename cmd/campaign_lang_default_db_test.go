package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/knadh/listmonk/models"
)

// Fork (SHALA-CUTOVER-SPEC D9) -- the end-to-end half of I5, I8 and I10 against a real
// database. Shares newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go).
//
// Every campaign here is content_type 'plain' on purpose: renderWarnings returns early for
// plain campaigns, so campaignWarningsByID can be exercised without the manager the harness
// does not build. The language half of that function runs either way.

func newLangCampaign(t *testing.T, h *linkHarness, listID int, attribs models.JSON) *models.Campaign {
	t.Helper()
	c, err := h.app.core.CreateCampaign(models.Campaign{
		Type:        models.CampaignTypeRegular,
		Name:        "C",
		Subject:     "s",
		FromEmail:   "hello@acme.test",
		Body:        "body",
		ContentType: models.CampaignContentTypePlain,
		Messenger:   "email",
		Attribs:     attribs,
		Headers:     models.Headers{},
		ArchiveMeta: json.RawMessage("{}"),
	}, []int{listID}, nil)
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	return &c
}

// TestCampaignLangDefault -- I5: a campaign created without attribs.lang is stored with
// "en", through core.CreateCampaign (the point every create path funnels through: the
// editor, the API, and a clone, which the frontend performs as a create carrying the source
// campaign's fields). An explicit language is kept.
func TestCampaignLangDefault(t *testing.T) {
	h := newLinkHarness(t)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'single') RETURNING id`)

	// (a) No attribs at all.
	if got := newLangCampaign(t, h, list, nil).Lang(); got != "en" {
		t.Fatalf("create without attribs: want lang en, got %q", got)
	}

	// (b) Attribs present, no lang key -- the other keys survive.
	c := newLangCampaign(t, h, list, models.JSON{"preheader": "p"})
	if c.Lang() != "en" || c.Preheader() != "p" {
		t.Fatalf("create without lang: want en + preheader kept, got %v", c.Attribs)
	}

	// (c) An explicit language is never overwritten.
	if got := newLangCampaign(t, h, list, models.JSON{"lang": "fr"}).Lang(); got != "fr" {
		t.Fatalf("create with lang fr: got %q", got)
	}

	// (d) A CLONE is a create carrying the source's attribs. A clone of a language-less
	// campaign (one that predates this default, or one whose language was cleared) is
	// therefore born "en"; a clone of an fr campaign stays fr.
	src := newLangCampaign(t, h, list, models.JSON{"lang": "fr"})
	h.db.MustExec(`UPDATE campaigns SET attribs = attribs - 'lang' WHERE id = $1`, src.ID)
	bare, err := h.app.core.GetCampaign(src.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if bare.Lang() != "" {
		t.Fatalf("fixture: the source must be language-less, got %q", bare.Lang())
	}
	if got := newLangCampaign(t, h, list, bare.Attribs).Lang(); got != "en" {
		t.Fatalf("clone of a language-less campaign: want en, got %q", got)
	}
	if got := newLangCampaign(t, h, list, models.JSON{"lang": "de"}).Lang(); got != "de" {
		t.Fatalf("clone of a de campaign: want de, got %q", got)
	}

	// (e) An opt-in campaign is exempt: it is the double opt-in confirmation mail, and a
	// language on it would stop confirmations reaching non-English subscribers.
	optin, err := h.app.core.CreateCampaign(models.Campaign{
		Type: models.CampaignTypeOptin, Name: "O", Subject: "s", FromEmail: "hello@acme.test",
		Body: "b", ContentType: models.CampaignContentTypePlain, Messenger: "email",
		Headers: models.Headers{}, ArchiveMeta: json.RawMessage("{}"),
	}, []int{list}, nil)
	if err != nil {
		t.Fatalf("CreateCampaign optin: %v", err)
	}
	if got := (&optin).Lang(); got != "" {
		t.Fatalf("optin campaign: want no language, got %q", got)
	}
}

// TestCampaignLangClearStoresNoKey -- I10: clearing the language stores NO key, never "".
// A stored "" matches nobody (the send predicate is `attribs->>'lang' IS NULL OR ... = it`),
// so the campaign would finish at 0 sent with nothing saying why. Also proves the create
// default does not sneak back in on update.
func TestCampaignLangClearStoresNoKey(t *testing.T) {
	h := newLinkHarness(t)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'single') RETURNING id`)

	c := newLangCampaign(t, h, list, models.JSON{"lang": "fr", "preheader": "p"})

	// What the form posts for "All": lang "". NormalizeLang deletes the key in place, and
	// that is what the update stores.
	attribs := models.JSON{"lang": "", "preheader": "p"}
	if !models.NormalizeLang(attribs) {
		t.Fatal("empty lang must validate")
	}
	c.Attribs = attribs
	if _, err := h.app.core.UpdateCampaign(c.ID, *c, []int{list}, nil); err != nil {
		t.Fatalf("UpdateCampaign: %v", err)
	}

	var rawText string
	h.db.Get(&rawText, `SELECT attribs::TEXT FROM campaigns WHERE id = $1`, c.ID)
	raw := models.JSON{}
	if err := json.Unmarshal([]byte(rawText), &raw); err != nil {
		t.Fatal(err)
	}
	if _, present := raw["lang"]; present {
		t.Fatalf("cleared language must store NO lang key, got %v", raw)
	}
	if raw["preheader"] != "p" {
		t.Fatalf("clearing the language must not disturb other attribs, got %v", raw)
	}
}

// TestCampaignLangLessWarning -- I8: campaignWarningsByID warns for a language-less campaign
// whose target lists hold a subscriber reading something other than English, and is silent
// when they do not. COALESCE-EN: a subscriber with no attribs.lang counts as English.
func TestCampaignLangLessWarning(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db

	mkList := func(name string) int {
		var id int
		db.Get(&id, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), $1, 'public', 'single') RETURNING id`, name)
		return id
	}
	addSub := func(listID int, email, attribs string) {
		var id int
		db.Get(&id, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), $1, 'S', $2::jsonb) RETURNING id`, email, attribs)
		db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, id, listID)
	}

	mixed := mkList("Mixed")
	addSub(mixed, "fr@x.test", `{"lang":"fr"}`)
	addSub(mixed, "en@x.test", `{"lang":"en"}`)
	addSub(mixed, "none@x.test", `{}`) // COALESCE-EN — counts as English.

	english := mkList("English only")
	addSub(english, "en2@x.test", `{"lang":"en"}`)
	addSub(english, "none2@x.test", `{}`)

	langLess := func(listID int) int {
		c := newLangCampaign(t, h, listID, nil)
		db.MustExec(`UPDATE campaigns SET attribs = attribs - 'lang' WHERE id = $1`, c.ID)
		return c.ID
	}

	// (a) Language-less over a list holding one fr subscriber → one warning, naming the count.
	id := langLess(mixed)
	w := h.app.campaignWarningsByID(id)
	if len(w) != 1 || !strings.Contains(w[0], "1") {
		t.Fatalf("mixed list: want one warning naming 1 subscriber, got %q", w)
	}

	// (b) An English-only list → silent. Subscribers with no language are not "another
	// language"; warning on them would fire on every campaign and mean nothing.
	if w := h.app.campaignWarningsByID(langLess(english)); len(w) != 0 {
		t.Fatalf("english-only list: want no warnings, got %q", w)
	}

	// (c) The same list, but the campaign HAS a language → silent, whatever the audience:
	// the language predicate already decides who receives it, and the zero-audience warning
	// at start covers the empty case.
	if w := h.app.campaignWarningsByID(newLangCampaign(t, h, mixed, models.JSON{"lang": "en"}).ID); len(w) != 0 {
		t.Fatalf("en campaign on a mixed list: want no warnings, got %q", w)
	}

	// (d) An unsubscribed non-English row is not an audience member, so it cannot trigger
	// the warning.
	db.MustExec(`UPDATE subscriber_lists SET status = 'unsubscribed' WHERE list_id = $1 AND subscriber_id = (SELECT id FROM subscribers WHERE email = 'fr@x.test')`, mixed)
	if w := h.app.campaignWarningsByID(langLess(mixed)); len(w) != 0 {
		t.Fatalf("unsubscribed fr row: want no warnings, got %q", w)
	}

	// (e) An opt-in campaign never carries a language and reaches every unconfirmed row by
	// design, so it is exempt (implementation review F7): a double-opt-in list holding an
	// unconfirmed fr row must not warn.
	var dbl int
	db.Get(&dbl, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Double', 'public', 'double') RETURNING id`)
	var frID int
	db.Get(&frID, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 'fr2@x.test', 'S', '{"lang":"fr"}'::jsonb) RETURNING id`)
	db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'unconfirmed')`, frID, dbl)
	optin, err := h.app.core.CreateCampaign(models.Campaign{
		Type: models.CampaignTypeOptin, Name: "O2", Subject: "s", FromEmail: "hello@acme.test",
		Body: "b", ContentType: models.CampaignContentTypePlain, Messenger: "email",
		Headers: models.Headers{}, ArchiveMeta: json.RawMessage("{}"),
	}, []int{dbl}, nil)
	if err != nil {
		t.Fatalf("CreateCampaign optin: %v", err)
	}
	if w := h.app.campaignWarningsByID(optin.ID); len(w) != 0 {
		t.Fatalf("opt-in campaign on a double-opt-in list with an unconfirmed fr row: want no warnings, got %q", w)
	}
}
