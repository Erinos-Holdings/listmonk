package main

// Fork (two-factor) -- integrations PASSKEY-2FA-SPEC D5 and I19: the post-login redirect target is
// always a plain local path, and the two public templates render through the REAL template set
// (the stuffbin glob initHTTPServer parses, the real tplRenderer) with no inline script. Pure
// tests (no database), so they run in CI.

import (
	"bytes"
	"encoding/json"
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

// permissionKeys reads every permission key from the repo's permissions.json (the file the app
// loads at boot, initConstConfig).
func permissionKeys(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "permissions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var groups []struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.Unmarshal(b, &groups); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, g := range groups {
		out = append(out, g.Permissions...)
	}
	return out
}

// Fork (integrations STEPUP-ADMIN-SPEC J1) -- step-up is keyed on exactly users:manage,
// roles:manage and settings:manage, all three exist in permissions.json (an upstream rename would
// otherwise drop the gate silently), and any one of several listed permissions is enough.
func TestStepUpPermSet(t *testing.T) {
	want := map[string]bool{"users:manage": true, "roles:manage": true, "settings:manage": true}
	seen := map[string]bool{}
	for _, p := range permissionKeys(t) {
		if got := needsStepUp(p); got != want[p] {
			t.Errorf("needsStepUp(%q) = %v, want %v", p, got, want[p])
		}
		seen[p] = true
	}
	for p := range want {
		if !seen[p] {
			t.Errorf("%s is not in permissions.json", p)
		}
	}
	if len(stepUpPerms) != len(want) {
		t.Errorf("stepUpPerms has %d entries, want %d", len(stepUpPerms), len(want))
	}
	if !needsStepUp("users:get", "settings:manage") || !needsStepUp("roles:manage", "campaigns:get") {
		t.Error("needsStepUp is false for a list holding one step-up permission")
	}
	if needsStepUp() || needsStepUp("users:get", "roles:get", "settings:get", "settings:maintain") {
		t.Error("needsStepUp is true for a list holding no step-up permission")
	}
}

// Fork (integrations STEPUP-ADMIN-SPEC J14) -- cmd/handlers.go uses the auth middleware's Perm in
// exactly one place, the pm wrapper that adds the step-up gate, so no route can be registered
// around the gate by calling it directly.
func TestPermWrapperIsOnlyPermUse(t *testing.T) {
	b, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if n := len(regexp.MustCompile(`\.Perm\b`).FindAllString(src, -1)); n != 1 {
		t.Fatalf("cmd/handlers.go refers to .Perm %d times, want 1 (inside the pm wrapper)", n)
	}
	if !strings.Contains(src, "return a.auth.Perm(next, perms...)") {
		t.Fatal("the one .Perm reference in cmd/handlers.go is not the pm wrapper's return")
	}
	if !regexp.MustCompile(`if needsStepUp\(perms\.\.\.\) \{\s*next = a\.stepUpGate\(next\)\s*\}\s*return a\.auth\.Perm\(next, perms\.\.\.\)`).MatchString(src) {
		t.Fatal("the pm wrapper in cmd/handlers.go no longer wraps step-up permissions with stepUpGate")
	}
}
