package main

// Fork (approved brand logos) -- integrations LOGO-APPROVAL-SPEC I1, the route half (no database):
// POST /api/brands/:slug/logos is registered under brands:logos ALONE (read off handlers.go -- a
// registration list with no test seam), and that permission check refuses a user holding only
// brands:manage, an API user without brands:logos and a user with no permission at all -- the
// handler never runs, so nothing can change. The database half (403 through the real router with the
// row unchanged, and the lists' invariants) is brands_logos_db_test.go.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
)

func TestBrandLogosRoute(t *testing.T) {
	b, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	re := `g\.POST\("/api/brands/:slug/logos", pm\(a\.PostBrandLogos, "brands:logos"\)\)`
	if !regexp.MustCompile(re).MatchString(src) {
		t.Fatalf("handlers.go does not register %s", re)
	}
	// Registered once, and under no other permission (never brands:manage).
	if n := strings.Count(src, "/api/brands/:slug/logos"); n != 1 {
		t.Fatalf("the logos route is registered %d times, want 1", n)
	}
	if auth.PermBrandsLogos != "brands:logos" || auth.PermBrandsLogos == auth.PermBrandsManage {
		t.Fatalf("PermBrandsLogos = %q", auth.PermBrandsLogos)
	}
	// The permission is in permissions.json (the role editor's list and the install grant).
	pj, err := os.ReadFile("../permissions.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pj), `"brands:logos"`) {
		t.Fatal("permissions.json lacks brands:logos")
	}
}

func TestBrandLogosPermission(t *testing.T) {
	perms := func(p ...string) map[string]struct{} {
		m := map[string]struct{}{}
		for _, x := range p {
			m[x] = struct{}{}
		}
		return m
	}
	user := func(typ string, roleID int, p ...string) auth.User {
		u := auth.User{Type: typ, PermissionsMap: perms(p...)}
		u.UserRole.ID = roleID
		return u
	}

	call := func(u auth.User) (int, bool) {
		ran := false
		h := (&auth.Auth{}).Perm(func(c echo.Context) error { ran = true; return c.NoContent(http.StatusOK) }, "brands:logos")
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/api/brands/acme/logos", strings.NewReader(`{"url":"https://x.test/a.png","action":"remove"}`))
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(auth.UserHTTPCtxKey, u)
		err := h(c)
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, ran
		}
		return rec.Code, ran
	}

	for name, u := range map[string]auth.User{
		"brands:manage only":               user(auth.UserTypeUser, 50, "brands:manage", "brands:get"),
		"every other brands permission":    user(auth.UserTypeUser, 50, "brands:get", "brands:manage", "brands:decide", "lists:manage_all"),
		"an API user without brands:logos": user(auth.UserTypeAPI, 15, "brands:get", "campaigns:review"),
		"a user with no permission":        user(auth.UserTypeUser, 50),
	} {
		code, ran := call(u)
		if code != http.StatusForbidden || ran {
			t.Errorf("%s: status %d, handler ran %v; want 403 and no run", name, code, ran)
		}
	}

	// The holder is let through.
	if code, ran := call(user(auth.UserTypeUser, 2, "brands:logos")); code != http.StatusOK || !ran {
		t.Fatalf("brands:logos holder: status %d, ran %v", code, ran)
	}
}
