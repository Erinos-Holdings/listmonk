package main

// Fork (join link) -- integrations listmonk/JOIN-LINK-SPEC D2/D10: the real public template set
// renders the join pages (confirm / already / cannot, and the result) with <html lang>, the D10
// copy, one POST form only on Confirm and no inline script; a page without a language keeps
// today's bare <html>. No database.

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/internal/joinpage"
	"github.com/knadh/stuffbin"
	"github.com/labstack/echo/v4"
)

func TestJoinTemplatesRender(t *testing.T) {
	static, err := filepath.Abs(filepath.Join("..", "static", "public"))
	if err != nil {
		t.Fatal(err)
	}
	fs, err := stuffbin.NewLocalFS("/", static+":/public")
	if err != nil {
		t.Fatalf("stuffbin: %v", err)
	}
	b, err := os.ReadFile(filepath.Join("..", "i18n", "en.json"))
	if err != nil {
		t.Fatal(err)
	}
	i, err := i18n.New(b)
	if err != nil {
		t.Fatal(err)
	}
	u := &UrlConfig{RootURL: "https://lm.example.test"}
	tpls, err := stuffbin.ParseTemplatesGlob(initTplFuncs(i, u), fs, "/public/templates/*.html")
	if err != nil {
		t.Fatalf("the public template set does not parse: %v", err)
	}
	r := &tplRenderer{templates: tpls, SiteName: "listmonk test", RootURL: u.RootURL, AssetVersion: "v1"}

	render := func(name string, data any) string {
		t.Helper()
		c := echo.New().NewContext(httptest.NewRequest("GET", "/", nil), httptest.NewRecorder())
		c.Set("app", &App{i18n: i})
		var out bytes.Buffer
		if err := r.Render(&out, name, data, c); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		html := out.String()
		if m := reInlineScript.FindString(html); m != "" {
			t.Fatalf("%s carries an inline script: %s", name, m)
		}
		return html
	}

	for _, lang := range []string{"fr", "en"} {
		cp := joinpage.For(lang, "Shala", "a@b.test")
		page := func(state string) joinTpl {
			d := joinTpl{State: state, Copy: cp}
			d.Lang = lang
			d.Title = cp.Heading
			return d
		}

		html := render("join", page(joinpage.StateConfirm))
		for _, want := range []string{`<html lang="` + lang + `">`, `method="post"`, cp.Button} {
			if !strings.Contains(html, want) {
				t.Fatalf("%s confirm page lacks %q:\n%s", lang, want, html)
			}
		}
		// html/template escapes the apostrophes, so compare the escaped heading and body.
		for _, s := range []string{cp.Heading, cp.Body} {
			esc := strings.ReplaceAll(s, "'", "&#39;")
			if !strings.Contains(html, esc) {
				t.Fatalf("%s confirm page lacks %q", lang, s)
			}
		}

		for state, want := range map[string]string{joinpage.StateAlready: cp.Already, joinpage.StateCannot: cp.Cannot} {
			html := render("join", page(state))
			if !strings.Contains(html, strings.ReplaceAll(want, "'", "&#39;")) {
				t.Fatalf("%s %s page lacks %q:\n%s", lang, state, want, html)
			}
			if strings.Contains(html, "<form") {
				t.Fatalf("%s %s page carries a form", lang, state)
			}
		}

		html = render("joined", page(joinpage.StateJoined))
		if !strings.Contains(html, `<html lang="`+lang+`">`) || !strings.Contains(html, strings.ReplaceAll(cp.DoneBody, "'", "&#39;")) {
			t.Fatalf("%s result page:\n%s", lang, html)
		}
	}

	// Every other page keeps the bare <html> (an empty Lang).
	html := render("unsubscribed", unsubbedTpl{publicTpl: publicTpl{Title: "t"}, Email: "a@b.test"})
	if !strings.Contains(html, "<html>") || strings.Contains(html, "<html lang") {
		t.Fatalf("a page without a language changed its <html>:\n%s", html)
	}
}
