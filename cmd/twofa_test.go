package main

// Fork (two-factor) -- integrations PASSKEY-2FA-SPEC D5 and I19: the post-login redirect target is
// always a plain local path, and the two public templates render through the REAL template set
// (the stuffbin glob initHTTPServer parses, the real tplRenderer) with no inline script. Pure
// tests (no database), so they run in CI.

import (
	"bytes"
	"html/template"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/stuffbin"
	"github.com/labstack/echo/v4"
)

func TestTwofaNext(t *testing.T) {
	for in, want := range map[string]string{
		"":                       uriAdmin,
		"/":                      uriAdmin,
		"/admin/campaigns":       "/admin/campaigns",
		"/admin/campaigns?x=1":   "/admin/campaigns",
		"//evil.example/path":    uriAdmin,
		`/\evil.example`:         uriAdmin,
		`\\evil.example`:         uriAdmin,
		`/admin\..\x`:            uriAdmin,
		"../admin":               uriAdmin,
		"https://evil.example/x": "/x",
		"/%09/evil.example":      uriAdmin,
		"/%2509/evil.example":    uriAdmin,
		"/%250A/evil.example":    uriAdmin,
		"/\t/evil.example":       uriAdmin,
	} {
		if got := twofaNext(in); got != want {
			t.Errorf("twofaNext(%q) = %q, want %q", in, got, want)
		}
	}
}

// reInlineScript matches a <script> element with a body (an external <script src></script> has none).
var reInlineScript = regexp.MustCompile(`(?is)<script[^>]*>\s*[^<\s][^<]*</script>`)

func TestTwofaTemplatesRender(t *testing.T) {
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

	const tok = "tok-0123456789"
	for _, c := range []struct {
		name          string
		totp, passkey bool
	}{{"passkey only", false, true}, {"TOTP only", true, false}, {"both", true, true}} {
		html := render("admin-twofa", twofaTpl{Token: tok, NextURI: "/admin", TOTP: c.totp, Passkey: c.passkey})
		if got := strings.Contains(html, `data-webauthn="get"`); got != c.passkey {
			t.Errorf("admin-twofa, %s: passkey block present %v", c.name, got)
		}
		if got := strings.Contains(html, `name="totp_code"`); got != c.totp {
			t.Errorf("admin-twofa, %s: TOTP form present %v", c.name, got)
		}
		if got := strings.Contains(html, `src="/public/static/webauthn.js`); got != c.passkey {
			t.Errorf("admin-twofa, %s: webauthn.js loaded %v", c.name, got)
		}
	}

	html := render("admin-enroll", enrollTpl{Token: tok, NextURI: "/admin", Secret: "JBSWY3DPEHPK3PXP",
		QR: template.URL("data:image/png;base64,iVBORw0KGgo="), Error: "bad code"})
	for _, want := range []string{
		`<img src="data:image/png;base64,iVBORw0KGgo="`,
		`<input type="hidden" name="secret" value="JBSWY3DPEHPK3PXP" />`,
		`<input type="hidden" name="token" value="` + tok + `" />`,
		`data-webauthn="create"`,
		`src="/public/static/webauthn.js?v=v1"`,
		"bad code",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("admin-enroll lacks %s", want)
		}
	}
}
