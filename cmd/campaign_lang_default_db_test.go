package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/manager"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
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

	// (f) LIST-GRID-SPEC D11/I8 -- and it STAYS language-less through an update (the keep-the-
	// language rule is for regular campaigns), can start without one, and reaches every
	// language: the unconfirmed en, fr, no-language and unrecognised rows of a double opt-in list.
	optin.Attribs = models.JSON{"preheader": "p"}
	upd, err := h.app.core.UpdateCampaign(optin.ID, optin, []int{list}, nil)
	if err != nil || (&upd).Lang() != "" {
		t.Fatalf("optin update: lang %q, %v", (&upd).Lang(), err)
	}
	var dbl int
	h.db.Get(&dbl, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Double', 'public', 'double') RETURNING id`)
	var want []int
	for i, attribs := range []string{`{"lang":"en"}`, `{"lang":"fr"}`, `{}`, `{"lang":"pt"}`} {
		var id int
		h.db.Get(&id, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), $1, 'S', $2::jsonb) RETURNING id`,
			fmt.Sprintf("optin-%d@x.test", i), attribs)
		h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'unconfirmed')`, id, dbl)
		want = append(want, id)
	}
	oc, err := h.app.core.CreateCampaign(models.Campaign{
		Type: models.CampaignTypeOptin, Name: "O3", Subject: "s", FromEmail: "hello@acme.test",
		Body: "b", ContentType: models.CampaignContentTypePlain, Messenger: "email",
		Headers: models.Headers{}, ArchiveMeta: json.RawMessage("{}"),
	}, []int{dbl}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.app.core.UpdateCampaignStatus(oc.ID, models.CampaignStatusRunning); err != nil {
		t.Fatalf("a language-less OPT-IN campaign must start: %v", err)
	}
	h.db.MustExec(`UPDATE campaigns SET max_subscriber_id = (SELECT MAX(id) FROM subscribers) WHERE id = $1`, oc.ID)
	subs, err := (&store{queries: h.q, core: h.app.core}).NextSubscribers(oc.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, s := range subs {
		got = append(got, s.ID)
	}
	sort.Ints(got)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("opt-in campaign recipients: got %v, want every language %v", got, want)
	}
}

// TestCampaignLangClearStoresNoKey -- Shala I10, which LIST-GRID-SPEC D11 keeps for OPT-IN
// campaigns only (a regular campaign's language can no longer be cleared, see
// TestCampaignLangCannotBeCleared): clearing a hand-set language stores NO key, never "". A
// stored "" matches nobody (the send predicate is `attribs->>'lang' IS NULL OR ... = it`), so
// the confirmation mail would finish at 0 sent with nothing saying why.
func TestCampaignLangClearStoresNoKey(t *testing.T) {
	h := newLinkHarness(t)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'double') RETURNING id`)

	c, err := h.app.core.CreateCampaign(models.Campaign{
		Type: models.CampaignTypeOptin, Name: "O", Subject: "s", FromEmail: "hello@acme.test",
		Body: "b", ContentType: models.CampaignContentTypePlain, Messenger: "email",
		Attribs: models.JSON{"lang": "fr", "preheader": "p"},
		Headers: models.Headers{}, ArchiveMeta: json.RawMessage("{}"),
	}, []int{list}, nil)
	if err != nil || (&c).Lang() != "fr" {
		t.Fatalf("optin with a hand-set language: %q, %v", (&c).Lang(), err)
	}

	// What the form posts for "All": lang "". NormalizeLang deletes the key in place, and
	// that is what the update stores.
	attribs := models.JSON{"lang": "", "preheader": "p"}
	if !models.NormalizeLang(attribs) {
		t.Fatal("empty lang must validate")
	}
	c.Attribs = attribs
	if _, err := h.app.core.UpdateCampaign(c.ID, c, []int{list}, nil); err != nil {
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

// stubMessenger lets validateCampaignFields find the "email" messenger. Nothing is ever pushed.
type stubMessenger struct{}

func (stubMessenger) Name() string              { return "email" }
func (stubMessenger) Push(models.Message) error { return nil }
func (stubMessenger) Flush() error              { return nil }
func (stubMessenger) Close() error              { return nil }

// ensureManager gives the app a manager that is constructed but never Run(): the campaign and
// template handlers ask it for the messenger roster and the template funcs, nothing more.
func ensureManager(h *linkHarness) {
	if h.app.manager == nil {
		h.app.manager = manager.New(manager.Config{}, &store{queries: h.q, core: h.app.core}, h.app.i18n, h.app.log)
		h.app.manager.AddMessenger(stubMessenger{})
	}
}

// putCampaign runs the UpdateCampaign handler as a campaign manager and returns the status.
func putCampaign(t *testing.T, h *linkHarness, id int, body string) (int, string) {
	t.Helper()
	ensureManager(h)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/campaigns/"+itoa(id), strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("id", id) // what the hasID middleware stores for getID
	c.Set(auth.UserHTTPCtxKey, auth.User{PermissionsMap: map[string]struct{}{
		auth.PermCampaignsManageAll: {}, auth.PermListGetAll: {}, auth.PermListManageAll: {},
	}})
	if err := h.app.UpdateCampaign(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("UpdateCampaign: %v", err)
	}
	return rec.Code, rec.Body.String()
}

func storedLang(t *testing.T, h *linkHarness, id int) string {
	t.Helper()
	c, err := h.app.core.GetCampaign(id, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return (&c).Lang()
}

// TestCampaignLangCannotBeCleared -- LIST-GRID-SPEC D11/I8. An update that drops the language
// of a REGULAR campaign keeps the stored one: attribs without the key (what the form posts),
// lang "" (the old "All"), and no attribs at all. On a STARTED campaign the same save must not
// 400 as a lang-lock violation (review M1) -- while a real change of language still does.
func TestCampaignLangCannotBeCleared(t *testing.T) {
	h := newLinkHarness(t)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'single') RETURNING id`)
	lists := fmt.Sprintf(`"lists": [%d]`, list)

	c := newLangCampaign(t, h, list, models.JSON{"lang": "fr", "preheader": "p"})

	for name, body := range map[string]string{
		"attribs without lang": `{` + lists + `, "attribs": {"preheader": "q"}}`,
		"lang empty":           `{` + lists + `, "attribs": {"preheader": "q", "lang": ""}}`,
		"no attribs":           `{` + lists + `, "name": "renamed"}`,
	} {
		if code, msg := putCampaign(t, h, c.ID, body); code != http.StatusOK {
			t.Fatalf("%s: %d %s", name, code, msg)
		}
		if got := storedLang(t, h, c.ID); got != "fr" {
			t.Fatalf("%s: the language was cleared or changed, got %q", name, got)
		}
	}
	// The other attribs of the request ARE what is stored.
	if got, _ := h.app.core.GetCampaign(c.ID, "", ""); (&got).Preheader() != "q" {
		t.Fatalf("preheader: got %q", (&got).Preheader())
	}
	// A draft may still CHANGE its language.
	if code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "attribs": {"lang": "de"}}`); code != http.StatusOK || storedLang(t, h, c.ID) != "de" {
		t.Fatalf("draft language change: %d %s, stored %q", code, msg, storedLang(t, h, c.ID))
	}

	// STARTED. started_at is what the lang lock reads. The status stays draft only because a
	// paused/scheduled edit also runs the footer guard, which needs the manager this harness
	// does not build. The lock itself does not look at the status.
	h.db.MustExec(`UPDATE campaigns SET started_at = NOW() WHERE id = $1`, c.ID)
	if code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "attribs": {"preheader": "after start"}}`); code != http.StatusOK {
		t.Fatalf("PUT {preheader} on a started campaign must not be a lang-lock violation: %d %s", code, msg)
	}
	if code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "attribs": {"preheader": "x", "lang": ""}}`); code != http.StatusOK {
		t.Fatalf("PUT lang \"\" on a started campaign: %d %s", code, msg)
	}
	if got := storedLang(t, h, c.ID); got != "de" {
		t.Fatalf("started campaign: language %q, want de", got)
	}
	if code, _ := putCampaign(t, h, c.ID, `{`+lists+`, "attribs": {"lang": "it"}}`); code != http.StatusBadRequest {
		t.Fatalf("a real language change on a started campaign must still 400, got %d", code)
	}

	// Every other caller goes through core.UpdateCampaign, which applies the same rule.
	cm, _ := h.app.core.GetCampaign(c.ID, "", "")
	cm.Attribs = models.JSON{"preheader": "core"}
	if out, err := h.app.core.UpdateCampaign(c.ID, cm, []int{list}, nil); err != nil || (&out).Lang() != "de" {
		t.Fatalf("core.UpdateCampaign dropped the language: %q, %v", (&out).Lang(), err)
	}
}

// langlessDraft is a regular draft with no language, which only SQL can produce now.
func langlessDraft(t *testing.T, h *linkHarness, list int) int {
	t.Helper()
	c := newLangCampaign(t, h, list, nil)
	h.db.MustExec(`UPDATE campaigns SET attribs = attribs - 'lang', send_at = NOW() + INTERVAL '1 day' WHERE id = $1`, c.ID)
	return c.ID
}

func putCampaignStatus(t *testing.T, h *linkHarness, id int, status string) (int, string) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/campaigns/"+itoa(id)+"/status", strings.NewReader(`{"status": "`+status+`"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("id", id) // what the hasID middleware stores for getID
	c.Set(auth.UserHTTPCtxKey, auth.User{PermissionsMap: map[string]struct{}{auth.PermCampaignsManageAll: {}}})
	if err := h.app.UpdateCampaignStatus(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("UpdateCampaignStatus: %v", err)
	}
	return rec.Code, rec.Body.String()
}

func testLanglessRefused(t *testing.T, status string) {
	h := newLinkHarness(t)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'single') RETURNING id`)
	id := langlessDraft(t, h, list)

	// The handler refuses, naming the field, BEFORE the footer guard (which would need the
	// manager) -- and core refuses for every other caller.
	code, msg := putCampaignStatus(t, h, id, status)
	if code != http.StatusBadRequest || !strings.Contains(strings.ToLower(msg), "language") {
		t.Fatalf("language-less draft -> %s: want 400 naming the language, got %d %s", status, code, msg)
	}
	if _, err := h.app.core.UpdateCampaignStatus(id, status); err == nil {
		t.Fatalf("core: language-less draft -> %s must be refused", status)
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("core: want 400, got %v", err)
	}
	var got string
	h.db.Get(&got, `SELECT status FROM campaigns WHERE id = $1`, id)
	if got != models.CampaignStatusDraft {
		t.Fatalf("a refused transition must leave the campaign a draft, got %s", got)
	}

	// Give it a language and the same transition goes through (core -- the handler's positive
	// path renders the footer guard).
	h.db.MustExec(`UPDATE campaigns SET attribs = attribs || '{"lang": "en"}' WHERE id = $1`, id)
	if _, err := h.app.core.UpdateCampaignStatus(id, status); err != nil {
		t.Fatalf("with a language -> %s: %v", status, err)
	}
}

// TestLanglessDraftCannotStart -- LIST-GRID-SPEC D11/I8.
func TestLanglessDraftCannotStart(t *testing.T) {
	testLanglessRefused(t, models.CampaignStatusRunning)
}

// TestLanglessDraftCannotSchedule -- LIST-GRID-SPEC D11/I8 (review H5). Scheduling is its own
// transition and the scheduler starts the campaign with no further handler.
func TestLanglessDraftCannotSchedule(t *testing.T) {
	testLanglessRefused(t, models.CampaignStatusScheduled)
}

// TestCampaignUnreachableLangWarning -- LIST-GRID-SPEC D13/I11. A campaign over a list holding
// one ACTIVE subscriber with an unrecognised language (pt, written by SQL -- the API and the
// importer cannot store one any more) warns with N=1, and is silent at zero. Reads the cached
// list stats view. It replaces the Shala-I8 language-less warning (TestCampaignLangLessWarning,
// deleted with it: a regular campaign always has a language now).
func TestCampaignUnreachableLangWarning(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db

	mkList := func(name, optin string) int {
		var id int
		db.Get(&id, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), $1, 'public', $2::list_optin) RETURNING id`, name, optin)
		return id
	}
	addSub := func(listID int, email, attribs, status string) int {
		var id int
		db.Get(&id, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), $1, 'S', $2::jsonb) RETURNING id`, email, attribs)
		db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, $3::subscription_status)`, id, listID, status)
		return id
	}

	mixed := mkList("Mixed", "single")
	addSub(mixed, "pt@x.test", `{"lang":"pt"}`, "confirmed")
	addSub(mixed, "fr@x.test", `{"lang":"fr"}`, "confirmed")
	addSub(mixed, "none@x.test", `{}`, "confirmed")
	// Not ACTIVE, so they would not receive the campaign anyway and are not counted.
	addSub(mixed, "pt-unsub@x.test", `{"lang":"pt"}`, "unsubscribed")
	blocked := addSub(mixed, "pt-blocked@x.test", `{"lang":"pt"}`, "confirmed")
	db.MustExec(`UPDATE subscribers SET status = 'blocklisted' WHERE id = $1`, blocked)

	clean := mkList("Clean", "single")
	addSub(clean, "en@x.test", `{"lang":"en"}`, "confirmed")
	addSub(clean, "none2@x.test", `{}`, "confirmed")

	// (a) One unrecognised ACTIVE row -> one warning naming 1.
	w := h.app.campaignWarningsByID(newLangCampaign(t, h, mixed, models.JSON{"lang": "en"}).ID)
	if len(w) != 1 || !strings.Contains(w[0], "1 subscriber") || !strings.Contains(w[0], "unrecognised language") {
		t.Fatalf("mixed list: want one warning naming 1 subscriber, got %q", w)
	}
	// ... whatever the campaign's language.
	if w := h.app.campaignWarningsByID(newLangCampaign(t, h, mixed, models.JSON{"lang": "fr"}).ID); len(w) != 1 {
		t.Fatalf("fr campaign on the mixed list: got %q", w)
	}

	// (b) Silent at zero.
	if w := h.app.campaignWarningsByID(newLangCampaign(t, h, clean, nil).ID); len(w) != 0 {
		t.Fatalf("clean list: want no warnings, got %q", w)
	}

	// (c) A legacy language-less campaign and an opt-in campaign reach every language, the
	// unrecognised rows included, so neither warns.
	legacy := newLangCampaign(t, h, mixed, nil)
	db.MustExec(`UPDATE campaigns SET attribs = attribs - 'lang' WHERE id = $1`, legacy.ID)
	if w := h.app.campaignWarningsByID(legacy.ID); len(w) != 0 {
		t.Fatalf("language-less campaign: want no warnings, got %q", w)
	}
	dbl := mkList("Double", "double")
	addSub(dbl, "pt-unconf@x.test", `{"lang":"pt"}`, "unconfirmed")
	optin, err := h.app.core.CreateCampaign(models.Campaign{
		Type: models.CampaignTypeOptin, Name: "O2", Subject: "s", FromEmail: "hello@acme.test",
		Body: "b", ContentType: models.CampaignContentTypePlain, Messenger: "email",
		Headers: models.Headers{}, ArchiveMeta: json.RawMessage("{}"),
	}, []int{dbl}, nil)
	if err != nil {
		t.Fatalf("CreateCampaign optin: %v", err)
	}
	if w := h.app.campaignWarningsByID(optin.ID); len(w) != 0 {
		t.Fatalf("opt-in campaign: want no warnings, got %q", w)
	}
}
