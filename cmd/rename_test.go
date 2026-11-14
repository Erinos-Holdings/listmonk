package main

// Fork (naming standard) -- integrations listmonk/NAMING-STANDARD-SPEC I1-I4, through the handlers
// against a real database (newLinkHarness, LISTMONK_TEST_PG opt-in; a plain `go test` skips them):
//
//	I1 the rename endpoints write the name only (a list also campaign_lists.list_name), in every
//	   campaign status incl. running/finished/cancelled, and never bump updated_at;
//	I2 the former-name rule: the first rename records the exact old name, later renames keep it,
//	   a rename back deletes the row, and a former name over 30 / with whitespace is accepted back
//	   verbatim;
//	I3 the endpoints refuse without campaigns:review_structure, honor the journey lock, refuse
//	   CATALOG/Official_ renames and renames onto locked/official names;
//	I4 (the general-update half) an unchanged long name passes a general update, a changed one is
//	   judged by the rule and records its former name; create applies the rule.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// renamer holds every permission the three rename endpoints need (route + handler).
var renamer = perms("admin", auth.PermCampaignsManageAll, auth.PermCampaignsGetAll, auth.PermCampaignsReviewStructure,
	auth.PermListManageAll, auth.PermListGetAll, auth.PermListJourneys, auth.PermTemplatesManage)

func nameCall(t *testing.T, h *linkHarness, fn echo.HandlerFunc, method string, id int, body string, user auth.User) (int, string) {
	t.Helper()
	ensureManager(h)
	e := echo.New()
	req := httptest.NewRequest(method, "/api/x/"+itoa(id), strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if id > 0 {
		c.Set("id", id)
	}
	c.Set(auth.UserHTTPCtxKey, user)
	if err := fn(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("handler: %v", err)
	}
	return rec.Code, rec.Body.String()
}

func renameBody(name string) string {
	b, _ := json.Marshal(map[string]string{"name": name})
	return string(b)
}

// rowExcept is the record's whole row as JSON minus the name (what a rename must leave alone).
func rowExcept(t *testing.T, h *linkHarness, table string, id int) string {
	t.Helper()
	var s string
	if err := h.db.Get(&s, `SELECT (to_jsonb(r) - 'name')::TEXT FROM `+table+` r WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	return s
}

func storedName(t *testing.T, h *linkHarness, table string, id int) string {
	t.Helper()
	var s string
	if err := h.db.Get(&s, `SELECT name FROM `+table+` WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	return s
}

func formerRow(t *testing.T, h *linkHarness, kind string, id int) (string, bool) {
	t.Helper()
	var s []string
	if err := h.db.Select(&s, `SELECT name FROM former_names WHERE kind = $1 AND id = $2`, kind, id); err != nil {
		t.Fatal(err)
	}
	if len(s) == 0 {
		return "", false
	}
	return s[0], true
}

func sqlCampaign(t *testing.T, h *linkHarness, name, status string, listID int) int {
	t.Helper()
	var id int
	if err := h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id, status, attribs)
		VALUES (gen_random_uuid(), $1, 's', 'Acme <hello@acme.test>', 'b', 'email', (SELECT id FROM templates WHERE type = 'campaign' LIMIT 1), $2, '{"lang": "en"}')
		RETURNING id`, name, status); err != nil {
		t.Fatal(err)
	}
	if listID > 0 {
		h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, (SELECT name FROM lists WHERE id = $2))`, id, listID)
	}
	// An updated_at in the past, so a bump would show.
	h.db.MustExec(`UPDATE campaigns SET updated_at = '2026-01-01T00:00:00Z' WHERE id = $1`, id)
	return id
}

func sqlList(t *testing.T, h *linkHarness, name, typ string, tags []string) int {
	t.Helper()
	var id int
	if err := h.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags, updated_at) VALUES (gen_random_uuid(), $1, $2, 'single', $3, '2026-01-01T00:00:00Z') RETURNING id`,
		name, typ, "{"+strings.Join(tags, ",")+"}"); err != nil {
		t.Fatal(err)
	}
	return id
}

func sqlTemplate(t *testing.T, h *linkHarness, name string) int {
	t.Helper()
	var id int
	if err := h.db.Get(&id, `INSERT INTO templates (name, type, subject, body, updated_at) VALUES ($1, 'campaign', '', '{{ template "content" . }}', '2026-01-01T00:00:00Z') RETURNING id`, name); err != nil {
		t.Fatal(err)
	}
	return id
}

// I1.
func TestRenameWritesNameOnly(t *testing.T) {
	h := newLinkHarness(t)
	list := sqlList(t, h, "ACME Old list", "private", []string{"brand:acme", `"from:Acme <hello@acme.test>"`})

	for _, status := range []string{"draft", "scheduled", "running", "paused", "finished", "cancelled"} {
		id := sqlCampaign(t, h, "Old "+status+" name that is far longer than thirty", status, list)
		before := rowExcept(t, h, "campaigns", id)
		if code, msg := nameCall(t, h, h.app.RenameCampaign, http.MethodPut, id, renameBody("New "+status), renamer); code != http.StatusOK {
			t.Fatalf("%s: %d %s", status, code, msg)
		}
		if got := storedName(t, h, "campaigns", id); got != "New "+status {
			t.Fatalf("%s: name %q", status, got)
		}
		if after := rowExcept(t, h, "campaigns", id); after != before {
			t.Fatalf("%s: the rename changed more than the name\n before %s\n after  %s", status, before, after)
		}
	}

	camp := sqlCampaign(t, h, "On the list", "finished", list)
	before := rowExcept(t, h, "lists", list)
	if code, msg := nameCall(t, h, h.app.RenameList, http.MethodPut, list, renameBody("ACME Renamed"), renamer); code != http.StatusOK {
		t.Fatalf("list: %d %s", code, msg)
	}
	if after := rowExcept(t, h, "lists", list); after != before {
		t.Fatalf("list: the rename changed more than the name\n before %s\n after  %s", before, after)
	}
	var ln string
	h.db.Get(&ln, `SELECT list_name FROM campaign_lists WHERE campaign_id = $1`, camp)
	if ln != "ACME Renamed" {
		t.Fatalf("campaign_lists.list_name = %q", ln)
	}

	tpl := sqlTemplate(t, h, "Old template name")
	before = rowExcept(t, h, "templates", tpl)
	if code, msg := nameCall(t, h, h.app.RenameTemplate, http.MethodPut, tpl, renameBody("Newsletter"), renamer); code != http.StatusOK {
		t.Fatalf("template: %d %s", code, msg)
	}
	if after := rowExcept(t, h, "templates", tpl); after != before {
		t.Fatalf("template: the rename changed more than the name\n before %s\n after  %s", before, after)
	}

	// Missing records are 404.
	for name, fn := range map[string]echo.HandlerFunc{"campaign": h.app.RenameCampaign, "list": h.app.RenameList, "template": h.app.RenameTemplate} {
		if code, _ := nameCall(t, h, fn, http.MethodPut, 999999, renameBody("X"), renamer); code != http.StatusNotFound {
			t.Fatalf("%s: missing record -> %d, want 404", name, code)
		}
	}
}

// I2 -- a 41-rune, space-led former name.
func TestRenameFormerNameRule(t *testing.T) {
	h := newLinkHarness(t)
	old := " Thirsty Girl Welcome Email WE1 — final 2"
	if n := len([]rune(old)); n != 41 {
		t.Fatalf("fixture length %d", n)
	}
	id := sqlCampaign(t, h, old, "finished", 0)

	rename := func(name string) (int, string) {
		return nameCall(t, h, h.app.RenameCampaign, http.MethodPut, id, renameBody(name), renamer)
	}
	if code, msg := rename("  Welcome  "); code != http.StatusOK {
		t.Fatalf("first rename: %d %s", code, msg)
	}
	if got := storedName(t, h, "campaigns", id); got != "Welcome" {
		t.Fatalf("the new name is stored trimmed: %q", got)
	}
	if f, ok := formerRow(t, h, "campaign", id); !ok || f != old {
		t.Fatalf("former name %q %v, want exactly %q", f, ok, old)
	}
	cm, _ := h.app.core.GetCampaign(id, "", "")
	if !cm.FormerName.Valid || cm.FormerName.String != old {
		t.Fatalf("GET former_name = %v", cm.FormerName)
	}

	// A later rename keeps the ORIGINAL name.
	if code, msg := rename("Welcome D0"); code != http.StatusOK {
		t.Fatalf("second rename: %d %s", code, msg)
	}
	if f, _ := formerRow(t, h, "campaign", id); f != old {
		t.Fatalf("second rename replaced the former name: %q", f)
	}

	// A name over 30 that is NOT the former name is refused; the trimmed former name is not it.
	if code, _ := rename(strings.Repeat("x", 31)); code != http.StatusBadRequest {
		t.Fatalf("over 30 accepted: %d", code)
	}
	if code, _ := rename(strings.TrimSpace(old)); code != http.StatusBadRequest {
		t.Fatalf("the trimmed (41-rune) former name accepted: %d", code)
	}

	// The exact former name is accepted verbatim (no trim, no length rule) and deletes the row.
	if code, msg := rename(old); code != http.StatusOK {
		t.Fatalf("rename back: %d %s", code, msg)
	}
	if got := storedName(t, h, "campaigns", id); got != old {
		t.Fatalf("rename back stored %q", got)
	}
	if _, ok := formerRow(t, h, "campaign", id); ok {
		t.Fatal("a rename back left the former_names row")
	}
	cm, _ = h.app.core.GetCampaign(id, "", "")
	if cm.FormerName.Valid {
		t.Fatalf("former_name after the reverse = %v", cm.FormerName)
	}

	// The same rule for a list and a template.
	list := sqlList(t, h, " Liyorá  ", "public", []string{"brand:liyora", `"from:LIYORÁ <hello@liyora.test>"`})
	h.db.MustExec(`INSERT INTO brands (slug, from_email) VALUES ('liyora', 'LIYORÁ <hello@liyora.test>')`)
	if code, msg := nameCall(t, h, h.app.RenameList, http.MethodPut, list, renameBody("LIYORÁ Emails"), renamer); code != http.StatusOK {
		t.Fatalf("list rename: %d %s", code, msg)
	}
	if f, _ := formerRow(t, h, "list", list); f != " Liyorá  " {
		t.Fatalf("list former name %q", f)
	}
	if code, msg := nameCall(t, h, h.app.RenameList, http.MethodPut, list, renameBody(" Liyorá  "), renamer); code != http.StatusOK {
		t.Fatalf("list reverse: %d %s", code, msg)
	}
	if _, ok := formerRow(t, h, "list", list); ok || storedName(t, h, "lists", list) != " Liyorá  " {
		t.Fatal("list reverse did not restore verbatim")
	}
	tpl := sqlTemplate(t, h, "Template with a long name of forty-one ch")
	if code, msg := nameCall(t, h, h.app.RenameTemplate, http.MethodPut, tpl, renameBody("Short"), renamer); code != http.StatusOK {
		t.Fatalf("template rename: %d %s", code, msg)
	}
	if f, _ := formerRow(t, h, "template", tpl); f != "Template with a long name of forty-one ch" {
		t.Fatalf("template former name %q", f)
	}
}

// I3.
func TestRenameGuards(t *testing.T) {
	h := newLinkHarness(t)
	camp := sqlCampaign(t, h, "Plain", "draft", 0)
	list := sqlList(t, h, "Plain list", "private", nil)
	tpl := sqlTemplate(t, h, "Plain template")

	// No campaigns:review_structure -> 403 on all three, nothing written.
	noReview := perms("marketing", auth.PermCampaignsManageAll, auth.PermListManageAll, auth.PermTemplatesManage, auth.PermListJourneys)
	for name, c := range map[string]struct {
		fn    echo.HandlerFunc
		id    int
		table string
	}{"campaign": {h.app.RenameCampaign, camp, "campaigns"}, "list": {h.app.RenameList, list, "lists"}, "template": {h.app.RenameTemplate, tpl, "templates"}} {
		before := storedName(t, h, c.table, c.id)
		if code, _ := nameCall(t, h, c.fn, http.MethodPut, c.id, renameBody("Renamed"), noReview); code != http.StatusForbidden {
			t.Fatalf("%s without review_structure: %d", name, code)
		}
		if storedName(t, h, c.table, c.id) != before {
			t.Fatalf("%s: a refused rename wrote", name)
		}
	}

	// The journey lock: a journey-locked list needs lists:journeys too.
	locked := sqlList(t, h, "RUZE Re-engage D180", "private", []string{"reengage:1:1"})
	noJourneys := perms("admin", auth.PermCampaignsReviewStructure, auth.PermListManageAll)
	if code, _ := nameCall(t, h, h.app.RenameList, http.MethodPut, locked, renameBody("RUZE X"), noJourneys); code != http.StatusForbidden {
		t.Fatalf("journey-locked list without lists:journeys: %d", code)
	}
	if code, msg := nameCall(t, h, h.app.RenameList, http.MethodPut, locked, renameBody("RUZE X"), renamer); code != http.StatusOK {
		t.Fatalf("journey-locked list with lists:journeys: %d %s", code, msg)
	}

	// The locked list (by name) is never renamed, and no list is renamed onto its name.
	catalogList := sqlList(t, h, models.LockedListNames[0], "private", nil)
	if code, _ := nameCall(t, h, h.app.RenameList, http.MethodPut, catalogList, renameBody("Other"), renamer); code != http.StatusConflict {
		t.Fatalf("locked list rename: %d", code)
	}
	if code, _ := nameCall(t, h, h.app.RenameList, http.MethodPut, list, renameBody(models.LockedListNames[0]), renamer); code != http.StatusConflict {
		t.Fatalf("rename onto a locked list name: %d", code)
	}

	// Code-named records: a CATALOG campaign and an Official_ template are never renamed; nothing
	// is renamed into a CATALOG/Official_ name.
	cat := sqlCampaign(t, h, "CATALOG 01-button", "draft", 0)
	if code, _ := nameCall(t, h, h.app.RenameCampaign, http.MethodPut, cat, renameBody("Button"), renamer); code != http.StatusBadRequest {
		t.Fatalf("CATALOG rename: %d", code)
	}
	official := sqlTemplate(t, h, "Official_footer_corporate_en")
	if code, _ := nameCall(t, h, h.app.RenameTemplate, http.MethodPut, official, renameBody("Footer"), renamer); code != http.StatusBadRequest {
		t.Fatalf("Official_ rename: %d", code)
	}
	for name, c := range map[string]struct {
		fn echo.HandlerFunc
		id int
	}{"campaign": {h.app.RenameCampaign, camp}, "list": {h.app.RenameList, list}, "template": {h.app.RenameTemplate, tpl}} {
		for _, into := range []string{"CATALOG x", "Official_x"} {
			if code, _ := nameCall(t, h, c.fn, http.MethodPut, c.id, renameBody(into), renamer); code != http.StatusBadRequest {
				t.Fatalf("%s renamed into %q: %d", name, into, code)
			}
		}
	}
	if code, _ := nameCall(t, h, h.app.RenameTemplate, http.MethodPut, tpl, renameBody("Official_footer_corporate_en"), renamer); code != http.StatusBadRequest {
		t.Fatalf("rename onto an existing Official_ name: %d", code)
	}
	if _, ok := formerRow(t, h, "template", official); ok {
		t.Fatal("a refused rename recorded a former name")
	}
}

// I4 (the general-update half) and the create rule.
func TestGeneralUpdateNameRule(t *testing.T) {
	h := newLinkHarness(t)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'single') RETURNING id`)
	lists := fmt.Sprintf(`"lists": [%d]`, list)
	c := newLangCampaign(t, h, list, models.JSON{"lang": "en"})
	long := "A legacy name that is far longer than thirty"
	h.db.MustExec(`UPDATE campaigns SET name = $1 WHERE id = $2`, long, c.ID)

	// Unchanged long name: a general update passes and records nothing.
	if code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "name": "`+long+`", "subject": "s2"}`); code != http.StatusOK {
		t.Fatalf("unchanged long name: %d %s", code, msg)
	}
	if _, ok := formerRow(t, h, "campaign", c.ID); ok {
		t.Fatal("an unchanged name recorded a former name")
	}
	// A changed name over 30 is refused; a valid one is stored trimmed and records the former name.
	if code, _ := putCampaign(t, h, c.ID, `{`+lists+`, "name": "`+strings.Repeat("y", 31)+`"}`); code != http.StatusBadRequest {
		t.Fatalf("a changed name over 30: %d", code)
	}
	if code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "name": " Short handle "}`); code != http.StatusOK {
		t.Fatalf("a changed valid name: %d %s", code, msg)
	}
	if got := storedName(t, h, "campaigns", c.ID); got != "Short handle" {
		t.Fatalf("stored %q", got)
	}
	if f, _ := formerRow(t, h, "campaign", c.ID); f != long {
		t.Fatalf("form rename former name %q", f)
	}

	// Lists: an unchanged long name passes; a changed one is judged on the part after the prefix.
	h.db.MustExec(`INSERT INTO brands (slug, from_email) VALUES ('acme', 'Acme Shop <hello@acme.test>')`)
	lid := sqlList(t, h, "A legacy list name far longer than thirty", "private", nil)
	lc := &triggerHarness{t: t, h: h}
	admin := perms("admin", auth.PermListManageAll, auth.PermListJourneys)
	if code, _ := lc.put(lid, `{"name": "A legacy list name far longer than thirty"}`, admin); code != http.StatusOK {
		t.Fatalf("unchanged list name: %d", code)
	}
	if code, _ := lc.put(lid, `{"name": "ACME `+strings.Repeat("z", 30)+`", "brand": "acme"}`, admin); code != http.StatusOK {
		t.Fatalf("a branded private name, 30 after the prefix: %d", code)
	}
	if code, _ := lc.put(lid, `{"name": "ACME `+strings.Repeat("z", 31)+`"}`, admin); code != http.StatusBadRequest {
		t.Fatalf("31 after the prefix: %d", code)
	}
	if f, _ := formerRow(t, h, "list", lid); f != "A legacy list name far longer than thirty" {
		t.Fatalf("list former name %q", f)
	}

	// Create: the rule applies (a clone is a create).
	if code, _ := lc.call(h.app.CreateList, http.MethodPost, "/api/lists", 0, `{"name": "`+strings.Repeat("q", 31)+`", "type": "private", "optin": "single"}`, admin); code != http.StatusBadRequest {
		t.Fatalf("create list over 30: %d", code)
	}
	if code, _ := templateReq(t, h, http.MethodPost, 0, `{"name": "`+strings.Repeat("q", 31)+`", "type": "campaign", "body": "{{ template \"content\" . }}"}`); code != http.StatusBadRequest {
		t.Fatalf("create template over 30: %d", code)
	}
	if code, msg := templateReq(t, h, http.MethodPost, 0, `{"name": "Official_footer_with_a_very_long_name_en", "type": "campaign_visual", "body": "<p>x</p>", "body_source": "{}"}`); code != http.StatusOK {
		t.Fatalf("create Official_ template (exempt): %d %s", code, msg)
	}
	if code, _ := nameCall(t, h, h.app.CreateCampaign, http.MethodPost, 0, `{"name": "`+strings.Repeat("q", 31)+`", "subject": "s", "from_email": "hello@acme.test", `+lists+`, "content_type": "plain", "body": "b", "messenger": "email"}`,
		perms("m", auth.PermCampaignsManageAll, auth.PermListGetAll, auth.PermListManageAll)); code != http.StatusBadRequest {
		t.Fatalf("create campaign over 30: %d", code)
	}
}
