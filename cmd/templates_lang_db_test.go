package main

// Fork (list grid) -- integrations LIST-GRID-SPEC D12/I9 against a real database. Shares
// newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go).

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

func templateReq(t *testing.T, h *linkHarness, method string, id int, body string) (int, string) {
	t.Helper()
	ensureManager(h)
	e := echo.New()
	req := httptest.NewRequest(method, "/api/templates", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	handler := h.app.CreateTemplate
	if method == http.MethodPut {
		c.Set("id", id)
		handler = h.app.UpdateTemplate
	}
	if err := handler(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("%s template: %v", method, err)
	}
	return rec.Code, rec.Body.String()
}

// TestTemplateLang -- I9. A template created without lang is stored en; an explicit one is kept;
// an invalid one is a 400 and writes nothing; an update without lang keeps the stored one and
// one with a valid lang changes it; and the API returns it -- from the LIST query too, which
// names its columns and hands its rows straight to the edit form and the clone.
func TestTemplateLang(t *testing.T) {
	h := newLinkHarness(t)
	const body = `<p>{{ template \"content\" . }}</p>`
	tpl := func(name, lang string) string {
		l := ""
		if lang != "-" {
			l = fmt.Sprintf(`, "lang": %q`, lang)
		}
		return fmt.Sprintf(`{"name": %q, "type": "campaign", "body": "%s"%s}`, name, body, l)
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

	code, resp := templateReq(t, h, http.MethodPost, 0, tpl("no lang", "-"))
	if code != http.StatusOK || created(resp).Lang != "en" {
		t.Fatalf("create without lang: %d %s", code, resp)
	}
	bare := created(resp)

	if code, resp = templateReq(t, h, http.MethodPost, 0, tpl("empty lang", "")); code != http.StatusOK || created(resp).Lang != "en" {
		t.Fatalf("create with lang \"\": %d %s", code, resp)
	}
	code, resp = templateReq(t, h, http.MethodPost, 0, tpl("french", "fr"))
	if code != http.StatusOK || created(resp).Lang != "fr" {
		t.Fatalf("create with lang fr: %d %s", code, resp)
	}
	fr := created(resp)

	n := count()
	for _, bad := range []string{"pt", "FR", "fr-CA", "english", " fr"} {
		if code, resp := templateReq(t, h, http.MethodPost, 0, tpl("bad", bad)); code != http.StatusBadRequest {
			t.Fatalf("create with lang %q: want 400, got %d %s", bad, code, resp)
		}
		if code, resp := templateReq(t, h, http.MethodPut, fr.ID, tpl("bad", bad)); code != http.StatusBadRequest {
			t.Fatalf("update with lang %q: want 400, got %d %s", bad, code, resp)
		}
	}
	if count() != n {
		t.Fatal("a refused create wrote a template")
	}

	stored := func(id int) string {
		var l string
		h.db.Get(&l, `SELECT lang FROM templates WHERE id = $1`, id)
		return l
	}
	// An update that does not mention lang keeps it (a client that predates the field).
	if code, resp := templateReq(t, h, http.MethodPut, fr.ID, tpl("french renamed", "-")); code != http.StatusOK || stored(fr.ID) != "fr" {
		t.Fatalf("update without lang: %d %s, stored %q", code, resp, stored(fr.ID))
	}
	if code, resp := templateReq(t, h, http.MethodPut, bare.ID, tpl("now german", "de")); code != http.StatusOK || stored(bare.ID) != "de" {
		t.Fatalf("update with lang de: %d %s, stored %q", code, resp, stored(bare.ID))
	}

	// The API returns it: the single fetch, and the no-body LIST.
	one, err := h.app.core.GetTemplate(fr.ID, false)
	if err != nil || one.Lang != "fr" {
		t.Fatalf("GetTemplate: %q, %v", one.Lang, err)
	}
	all, err := h.app.core.GetTemplates("", true)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]string{}
	for _, tp := range all {
		seen[tp.ID] = tp.Lang
	}
	if seen[fr.ID] != "fr" || seen[bare.ID] != "de" {
		t.Fatalf("GetTemplates langs: %v", seen)
	}
	for id, l := range seen {
		if l == "" {
			t.Fatalf("template %d has no lang in the list (schema default en expected)", id)
		}
	}
	b, _ := json.Marshal(one)
	if !strings.Contains(string(b), `"lang":"fr"`) {
		t.Fatalf("template JSON: %s", b)
	}
}
