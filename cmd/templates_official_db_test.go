package main

// Fork (official footer) -- integrations OFFICIAL-FOOTER-SPEC I11 against a real database, through
// the handlers. Shares newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go) and
// templateReq (templates_lang_db_test.go).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

func deleteTemplateReq(t *testing.T, h *linkHarness, id int) (int, string) {
	t.Helper()
	ensureManager(h)
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/templates", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("id", id)
	if err := h.app.DeleteTemplate(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("delete template: %v", err)
	}
	return rec.Code, rec.Body.String()
}

func TestOfficialTemplateRefusals(t *testing.T) {
	h := newLinkHarness(t)
	const plain = `{\"root\":{\"type\":\"EmailLayout\",\"data\":{\"childrenIds\":[]}}}`
	const nested = `{\"root\":{\"type\":\"EmailLayout\",\"data\":{\"childrenIds\":[\"a\"]}},\"a\":{\"type\":\"OfficialFooter\",\"data\":{\"props\":{\"kind\":\"corporate\"}}}}`
	tpl := func(name, source string) string {
		return fmt.Sprintf(`{"name": %q, "type": "campaign_visual", "body": "<p>x</p>", "body_source": "%s", "lang": "en"}`, name, source)
	}
	created := func(resp string) models.Template {
		var out struct {
			Data models.Template `json:"data"`
		}
		if err := json.Unmarshal([]byte(resp), &out); err != nil {
			t.Fatalf("%v: %s", err, resp)
		}
		return out.Data
	}
	count := func() int {
		var n int
		h.db.Get(&n, `SELECT COUNT(*) FROM templates`)
		return n
	}
	name := func(id int) string {
		var s string
		h.db.Get(&s, `SELECT name FROM templates WHERE id = $1`, id)
		return s
	}

	code, resp := templateReq(t, h, http.MethodPost, 0, tpl("Official_Footer_EN", plain))
	if code != http.StatusOK {
		t.Fatalf("create official: %d %s", code, resp)
	}
	official := created(resp)
	code, resp = templateReq(t, h, http.MethodPost, 0, tpl("Draft with footer", nested))
	if code != http.StatusOK {
		t.Fatalf("a non-official template may hold OfficialFooter blocks: %d %s", code, resp)
	}
	draft := created(resp)

	n := count()
	if code, resp := templateReq(t, h, http.MethodPost, 0, tpl("Official_Footer_EN", plain)); code != http.StatusBadRequest || !strings.Contains(resp, "already exists") {
		t.Fatalf("duplicate official create: want 400, got %d %s", code, resp)
	}
	if code, resp := templateReq(t, h, http.MethodPost, 0, tpl("Official_Footer_DE", nested)); code != http.StatusBadRequest {
		t.Fatalf("official create holding an OfficialFooter block: want 400, got %d %s", code, resp)
	}
	if count() != n {
		t.Fatal("a refused create wrote a template")
	}

	if code, resp := templateReq(t, h, http.MethodPut, official.ID, tpl("Footer EN", plain)); code != http.StatusBadRequest {
		t.Fatalf("rename off the prefix: want 400, got %d %s", code, resp)
	}
	if code, resp := templateReq(t, h, http.MethodPut, official.ID, tpl("Official_Footer_EN", nested)); code != http.StatusBadRequest {
		t.Fatalf("official update holding an OfficialFooter block: want 400, got %d %s", code, resp)
	}
	if code, resp := templateReq(t, h, http.MethodPut, draft.ID, tpl("Official_Footer_EN", plain)); code != http.StatusBadRequest {
		t.Fatalf("rename onto an existing official name: want 400, got %d %s", code, resp)
	}
	if name(official.ID) != "Official_Footer_EN" || name(draft.ID) != "Draft with footer" {
		t.Fatal("a refused update wrote")
	}

	// Other updates pass.
	if code, resp := templateReq(t, h, http.MethodPut, official.ID, tpl("Official_Footer_EN", plain)); code != http.StatusOK {
		t.Fatalf("official same-name update: %d %s", code, resp)
	}
	if code, resp := templateReq(t, h, http.MethodPut, draft.ID, tpl("Draft renamed", nested)); code != http.StatusOK {
		t.Fatalf("non-official update: %d %s", code, resp)
	}

	if code, resp := deleteTemplateReq(t, h, official.ID); code != http.StatusBadRequest || !strings.Contains(resp, "seed file") {
		t.Fatalf("delete official: want 400, got %d %s", code, resp)
	}
	if name(official.ID) == "" {
		t.Fatal("the official template was deleted")
	}
	if code, resp := deleteTemplateReq(t, h, draft.ID); code != http.StatusOK {
		t.Fatalf("delete non-official: %d %s", code, resp)
	}
}
