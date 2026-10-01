package main

// Fork (step-up for user administration, roles and settings) -- integrations STEPUP-ADMIN-SPEC
// J2 to J9 and J15 against a real database, through the real router (twofaHarness, built by
// initHTTPHandlers), so the pm wrapper, the session middleware and the permission checks are the
// production ones. LISTMONK_TEST_PG opt-in, as the other two-factor tests.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
	"gopkg.in/volatiletech/null.v6"
)

// suFixture is a Super Admin cookie user plus one target for each of the 14 gated routes.
type suFixture struct {
	h        *twofaHarness
	boss     int
	victim   int // edited
	doomed   int // deleted by id
	doomed2  int // deleted in bulk
	factored int // factors reset
	listID   int
	listRole int // edited
	delRole  int // deleted
}

func newSUFixture(t *testing.T) *suFixture {
	t.Helper()
	h := newTwofaHarness(t)
	ensureManager(h.linkHarness)
	h.app.cfg.Permissions = map[string]struct{}{}
	for _, p := range permissionKeys(t) {
		h.app.cfg.Permissions[p] = struct{}{}
	}

	// The schema seed's SMTP blocks carry no uuid, so a masked password could not be matched back
	// to its stored secret on a full settings save; give them one, as the settings UI's first save would.
	h.db.MustExec(`UPDATE settings SET value = (SELECT jsonb_agg(e || jsonb_build_object('uuid', gen_random_uuid()::TEXT)) FROM jsonb_array_elements(value) e) WHERE key = 'smtp'`)

	f := &suFixture{h: h}
	f.boss = h.user("boss", auth.SuperAdminRoleID)
	f.victim = h.user("victim", h.writerRole)
	f.doomed = h.user("doomed", h.writerRole)
	f.doomed2 = h.user("doomed2", h.writerRole)
	f.factored = h.user("factored", h.writerRole)
	h.setTOTP(f.factored)
	h.db.Get(&f.listID, `INSERT INTO lists (uuid, name, type) VALUES (gen_random_uuid(), 'Brand list', 'private') RETURNING id`)
	lr, err := h.app.core.CreateListRole(auth.ListRole{Name: null.StringFrom("Brand role"), Lists: []auth.ListPermission{{ID: f.listID, Permissions: []string{auth.PermListGet}}}})
	if err != nil {
		t.Fatal(err)
	}
	f.listRole = lr.ID
	h.db.Get(&f.delRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Doomed role', '{campaigns:get}') RETURNING id`)
	if _, err := cacheUsers(h.app.core, h.app.auth); err != nil {
		t.Fatal(err)
	}
	return f
}

// adminState is every user row, role row and setting, as one string.
func (f *suFixture) adminState() string {
	var s string
	f.h.db.Get(&s, `SELECT
		(SELECT COALESCE(string_agg(u::TEXT, '|' ORDER BY id), '') FROM users u) || '#' ||
		(SELECT COALESCE(string_agg(r::TEXT, '|' ORDER BY id), '') FROM roles r) || '#' ||
		(SELECT string_agg(key || '=' || value::TEXT, '|' ORDER BY key) FROM settings)`)
	return s
}

func (f *suFixture) setting(key string) string {
	var v string
	f.h.db.Get(&v, `SELECT value::TEXT FROM settings WHERE key = $1`, key)
	return v
}

func (f *suFixture) count(q string, args ...any) int {
	var n int
	if err := f.h.db.Get(&n, q, args...); err != nil {
		f.h.t.Fatalf("%s: %v", q, err)
	}
	return n
}

// drainReload empties the harness's reload channel (settings saves signal it 500 ms later).
func (f *suFixture) drainReload() {
	for {
		select {
		case <-f.h.app.chReload:
		default:
			return
		}
	}
}

func (f *suFixture) reloaded(wait time.Duration) bool {
	select {
	case <-f.h.app.chReload:
		return true
	case <-time.After(wait):
		return false
	}
}

// suCase is one of the 14 routes of D1: its registered method and path, a request on it that
// would succeed, and a check that the request took effect.
type suCase struct {
	method, route string
	target        string
	ctype, body   string
	effect        func() bool
}

func (c suCase) key() string { return c.method + " " + c.route }

// cases returns the 14 gated routes. The settings bodies are built from a GET of the stored
// settings by cl (a GET is not gated).
func (f *suFixture) cases(t *testing.T, cl *twofaClient) []suCase {
	t.Helper()
	h := f.h
	id := strconv.Itoa
	rec := cl.do(http.MethodGet, "/api/settings", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/settings: %d %s", rec.Code, rec.Body.String())
	}
	// Flip app.check_updates in the full PUT (stored true by the schema seed).
	settings := strings.Replace(string(dataOf(t, rec)), `"app.check_updates":true`, `"app.check_updates":false`, 1)
	if !strings.Contains(settings, `"app.check_updates":false`) {
		t.Fatalf("fixture: app.check_updates not true in %s", settings)
	}
	user := `{"username": "%s", "name": "%s", "email": "%s@example.test", "type": "user", "user_role_id": %d, "status": "enabled", "password_login": true, "password": "%s"}`
	return []suCase{
		{http.MethodPut, "/api/settings", "/api/settings", echo.MIMEApplicationJSON, settings,
			func() bool { return f.setting("app.check_updates") == "false" }},
		{http.MethodPut, "/api/settings/:key", "/api/settings/app.check_updates", echo.MIMEApplicationJSON, "true",
			func() bool { return f.setting("app.check_updates") == "true" }},
		{http.MethodPost, "/api/settings/smtp/test", "/api/settings/smtp/test", echo.MIMEApplicationJSON,
			// No "email": the handler answers 400 before building a pool or rendering the test
			// mail (the harness has no notification templates). Any answer but 403 shows the gate
			// let it through.
			`{"enabled": true, "host": "127.0.0.1", "port": 1, "auth_protocol": "none", "tls_type": "none", "max_conns": 1}`,
			nil},
		{http.MethodPost, "/api/admin/reload", "/api/admin/reload", "", "", nil},
		{http.MethodPost, "/api/users", "/api/users", echo.MIMEApplicationJSON,
			fmt.Sprintf(user, "newbie", "newbie", "newbie", h.writerRole, ppGoodPassword),
			func() bool { return f.count(`SELECT COUNT(*) FROM users WHERE username = 'newbie'`) == 1 }},
		{http.MethodPut, "/api/users/:id", "/api/users/" + id(f.victim), echo.MIMEApplicationJSON,
			fmt.Sprintf(user, "victim", "Victim Renamed", "victim", h.writerRole, ""),
			func() bool {
				return f.count(`SELECT COUNT(*) FROM users WHERE id = $1 AND name = 'Victim Renamed'`, f.victim) == 1
			}},
		{http.MethodDelete, "/api/users", "/api/users?id=" + id(f.doomed2), "", "",
			func() bool { return f.count(`SELECT COUNT(*) FROM users WHERE id = $1`, f.doomed2) == 0 }},
		{http.MethodDelete, "/api/users/:id", "/api/users/" + id(f.doomed), "", "",
			func() bool { return f.count(`SELECT COUNT(*) FROM users WHERE id = $1`, f.doomed) == 0 }},
		{http.MethodDelete, "/api/users/:id/factors", "/api/users/" + id(f.factored) + "/factors", "", "",
			func() bool { typ, _ := h.twofaType(f.factored); return typ == "none" }},
		{http.MethodPost, "/api/roles/users", "/api/roles/users", echo.MIMEApplicationJSON,
			`{"name": "New role", "permissions": ["campaigns:get"]}`,
			func() bool { return f.count(`SELECT COUNT(*) FROM roles WHERE name = 'New role'`) == 1 }},
		{http.MethodPost, "/api/roles/lists", "/api/roles/lists", echo.MIMEApplicationJSON,
			fmt.Sprintf(`{"name": "New list role", "lists": [{"id": %d, "permissions": ["list:get"]}]}`, f.listID),
			func() bool { return f.count(`SELECT COUNT(*) FROM roles WHERE name = 'New list role'`) == 1 }},
		{http.MethodPut, "/api/roles/users/:id", "/api/roles/users/" + id(h.readerRole), echo.MIMEApplicationJSON,
			`{"name": "Reader renamed", "permissions": ["campaigns:get"]}`,
			func() bool {
				return f.count(`SELECT COUNT(*) FROM roles WHERE id = $1 AND name = 'Reader renamed'`, h.readerRole) == 1
			}},
		{http.MethodPut, "/api/roles/lists/:id", "/api/roles/lists/" + id(f.listRole), echo.MIMEApplicationJSON,
			fmt.Sprintf(`{"name": "Brand role renamed", "lists": [{"id": %d, "permissions": ["list:get", "list:manage"]}]}`, f.listID),
			func() bool {
				return f.count(`SELECT COUNT(*) FROM roles WHERE id = $1 AND name = 'Brand role renamed'`, f.listRole) == 1
			}},
		{http.MethodDelete, "/api/roles/:id", "/api/roles/" + id(f.delRole), "", "",
			func() bool { return f.count(`SELECT COUNT(*) FROM roles WHERE id = $1`, f.delRole) == 0 }},
	}
}

// gatedRoutes enumerates the router's non-GET routes under /api/settings, /api/users and
// /api/roles, plus /api/admin/reload.
func gatedRoutes(e *echo.Echo) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range e.Routes() {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == echo.RouteNotFound {
			continue
		}
		p := r.Path
		if p == "/api/admin/reload" || p == "/api/settings" || p == "/api/users" || p == "/api/roles" ||
			strings.HasPrefix(p, "/api/settings/") || strings.HasPrefix(p, "/api/users/") || strings.HasPrefix(p, "/api/roles/") {
			k := r.Method + " " + p
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

// J2 -- a Super Admin cookie session with no stamp is refused on every non-GET route under
// /api/settings, /api/users and /api/roles and on /api/admin/reload (16 at the base: the 14 of D1
// plus PUT and DELETE /api/users/:id/twofa, which their own in-handler requireStepUp refuses), and
// nothing changes.
func TestAdminWritesNeedStepUp(t *testing.T) {
	f := newSUFixture(t)
	h := f.h
	cl := h.sessionClient(f.boss)
	cases := f.cases(t, cl)
	byKey := map[string]suCase{}
	for _, c := range cases {
		byKey[c.key()] = c
	}

	routes := gatedRoutes(h.e)
	have := map[string]bool{}
	for _, r := range routes {
		have[r] = true
	}
	for _, c := range cases {
		if !have[c.key()] {
			t.Fatalf("%s is not registered (routes: %v)", c.key(), routes)
		}
	}
	// 16 at the base. A different count means a route was added or removed under these prefixes:
	// it is still checked below, but classify it (runbook upgrade table) and update the number.
	if len(routes) != 16 {
		t.Errorf("%d non-GET routes under the gated prefixes, want 16: %v", len(routes), routes)
	}

	before := f.adminState()
	f.drainReload()
	for _, r := range routes {
		c, ok := byKey[r]
		if !ok {
			// A route outside the 14 (the factor routes, or one added later): any body, own id.
			method, route, _ := strings.Cut(r, " ")
			target := strings.NewReplacer(":id", strconv.Itoa(f.boss), ":key", "app.check_updates").Replace(route)
			c = suCase{method: method, route: route, target: target, ctype: echo.MIMEApplicationJSON, body: "{}"}
		}
		rec := cl.do(c.method, c.target, c.ctype, c.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s (%s) without a stamp: want 403, got %d %s", r, c.target, rec.Code, rec.Body.String())
		}
	}
	if f.reloaded(time.Second) {
		t.Error("a refused request reloaded the app")
	}
	if after := f.adminState(); after != before {
		t.Errorf("refused requests changed users, roles or settings:\nbefore %s\nafter  %s", before, after)
	}
}

// J3 -- with a fresh stamp each of the 14 routes answers other than 403 and each write takes
// effect.
func TestAdminWritesPassWithStamp(t *testing.T) {
	f := newSUFixture(t)
	h := f.h
	cl := h.sessionClient(f.boss)
	if rec := cl.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d %s", rec.Code, rec.Body.String())
	}
	cases := f.cases(t, cl)

	// The reload first, before any settings save queues its own reload signal.
	sort.SliceStable(cases, func(i, j int) bool {
		return cases[i].route == "/api/admin/reload" && cases[j].route != "/api/admin/reload"
	})
	f.drainReload()
	for _, c := range cases {
		rec := cl.do(c.method, c.target, c.ctype, c.body)
		if rec.Code == http.StatusForbidden {
			t.Errorf("%s with a stamp: 403 %s", c.key(), rec.Body.String())
			continue
		}
		switch {
		case c.route == "/api/admin/reload":
			if rec.Code != http.StatusOK || !f.reloaded(3*time.Second) {
				t.Errorf("%s with a stamp: %d, no reload signal", c.key(), rec.Code)
			}
		case c.effect == nil:
			// smtp/test: any answer other than 403 (no SMTP server is contacted in the test).
		case rec.Code != http.StatusOK || !c.effect():
			t.Errorf("%s with a stamp did not take effect: %d %s", c.key(), rec.Code, rec.Body.String())
		}
	}
}

// J4 -- a stamp older than StepUpTTL is refused on one route of each permission.
func TestAdminWritesStampExpires(t *testing.T) {
	f := newSUFixture(t)
	h := f.h
	cl := h.sessionClient(f.boss)
	if rec := cl.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d", rec.Code)
	}
	h.db.MustExec(`UPDATE sessions SET data = data || jsonb_build_object('stepup_at', EXTRACT(EPOCH FROM NOW())::BIGINT - $2::BIGINT) WHERE id = $1`,
		cl.jar["session"], int64(auth.StepUpTTL/time.Second)+1)

	byKey := map[string]suCase{}
	for _, c := range f.cases(t, cl) {
		byKey[c.key()] = c
	}
	before := f.adminState()
	for _, k := range []string{"PUT /api/settings/:key", "POST /api/users", "DELETE /api/roles/:id"} {
		c := byKey[k]
		rec := cl.do(c.method, c.target, c.ctype, c.body)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), h.app.i18n.T("users.stepUpRequired")) {
			t.Errorf("%s with an expired stamp: want 403 step-up required, got %d %s", k, rec.Code, rec.Body.String())
		}
	}
	if f.adminState() != before {
		t.Error("a request with an expired stamp changed users, roles or settings")
	}
}

// apiClient creates an enabled API user with the role and returns a client carrying its token.
func (f *suFixture) apiClient(t *testing.T, username string, roleID int) *twofaClient {
	t.Helper()
	u, err := f.h.app.core.CreateUser(auth.User{Type: auth.UserTypeAPI, Username: username, Name: username, UserRoleID: roleID, Status: auth.UserStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cacheUsers(f.h.app.core, f.h.app.auth); err != nil {
		t.Fatal(err)
	}
	cl := f.h.client()
	cl.hdr["Authorization"] = "token " + username + ":" + u.Password.String
	return cl
}

// J5 -- an API token whose role holds the three permissions, and a Super Admin token, get 403 with
// the no-token text on each of the 14 routes and nothing changes; a cookie session without a
// stamp -- a Super Admin's, and a non-Super-Admin role holder's -- gets the step-up-required text.
func TestAdminWritesRefuseTokens(t *testing.T) {
	f := newSUFixture(t)
	h := f.h
	var adminRole int
	h.db.Get(&adminRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Admins', '{users:get,users:manage,roles:get,roles:manage,settings:get,settings:manage}') RETURNING id`)
	clients := []struct {
		name string
		cl   *twofaClient
		msg  string
	}{
		{"token with the three permissions", f.apiClient(t, "adminbot", adminRole), h.app.i18n.T("users.stepUpNoToken")},
		{"Super Admin token", f.apiClient(t, "superbot", auth.SuperAdminRoleID), h.app.i18n.T("users.stepUpNoToken")},
		{"cookie session without a stamp", h.sessionClient(f.boss), h.app.i18n.T("users.stepUpRequired")},
		// Not a Super Admin: auth.Perm takes its permission-map branch for this one.
		{"cookie session of a role holder without a stamp", h.sessionClient(h.user("deputy", adminRole)), h.app.i18n.T("users.stepUpRequired")},
	}
	cases := f.cases(t, clients[0].cl)
	before := f.adminState()
	f.drainReload()
	for _, cc := range clients {
		for _, c := range cases {
			rec := cc.cl.do(c.method, c.target, c.ctype, c.body)
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), cc.msg) {
				t.Errorf("%s, %s: want 403 %q, got %d %s", cc.name, c.key(), cc.msg, rec.Code, rec.Body.String())
			}
		}
	}
	if f.reloaded(time.Second) {
		t.Error("a refused request reloaded the app")
	}
	if f.adminState() != before {
		t.Error("a refused token or session request changed users, roles or settings")
	}

	// The same role holder, stamped, gets through (the gate is not Super Admin only either way).
	deputy := clients[3].cl
	if rec := deputy.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d %s", rec.Code, rec.Body.String())
	}
	if rec := deputy.do(http.MethodPost, "/api/roles/users", echo.MIMEApplicationJSON, `{"name": "Deputy role", "permissions": ["campaigns:get"]}`); rec.Code != http.StatusOK ||
		f.count(`SELECT COUNT(*) FROM roles WHERE name = 'Deputy role'`) != 1 {
		t.Errorf("stamped role holder, POST /api/roles/users: %d %s", rec.Code, rec.Body.String())
	}
}

// J6 -- the stamp grants nothing: a stamped cookie session without the permission, and an API
// token without it, get the permission refusal (not the no-token text) on a route of each permission.
func TestStampGrantsNoPermission(t *testing.T) {
	f := newSUFixture(t)
	h := f.h
	writer := h.user("writer", h.writerRole)
	cl := h.sessionClient(writer)
	if rec := cl.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d", rec.Code)
	}
	tok := f.apiClient(t, "writerbot", h.writerRole)
	byKey := map[string]suCase{}
	for _, c := range f.cases(t, h.sessionClient(f.boss)) {
		byKey[c.key()] = c
	}
	before := f.adminState()
	for _, k := range []struct{ route, perm string }{
		{"PUT /api/settings/:key", "settings:manage"},
		{"POST /api/users", "users:manage"},
		{"DELETE /api/roles/:id", "roles:manage"},
	} {
		c := byKey[k.route]
		for name, client := range map[string]*twofaClient{"stamped session": cl, "token": tok} {
			rec := client.do(c.method, c.target, c.ctype, c.body)
			body := rec.Body.String()
			if rec.Code != http.StatusForbidden || !strings.Contains(body, "permission denied: "+k.perm) ||
				strings.Contains(body, h.app.i18n.T("users.stepUpNoToken")) {
				t.Errorf("%s without %s, %s: want 403 permission denied, got %d %s", name, k.perm, k.route, rec.Code, body)
			}
		}
	}
	if f.adminState() != before {
		t.Error("a caller without the permission changed users, roles or settings")
	}
}

// J7 -- outside the three permissions nothing changes: a settings:maintain token on a maintenance
// route, a token's and a stampless session's GETs of users, user roles and settings.
func TestStepUpLeavesOtherRoutesAlone(t *testing.T) {
	f := newSUFixture(t)
	h := f.h
	var role int
	h.db.Get(&role, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Maintainer', '{settings:maintain,users:get,roles:get,settings:get}') RETURNING id`)
	tok := f.apiClient(t, "maintbot", role)
	if rec := tok.do(http.MethodDelete, "/api/maintenance/subscribers/orphan", "", ""); rec.Code != http.StatusOK {
		t.Errorf("settings:maintain token on /api/maintenance: %d %s", rec.Code, rec.Body.String())
	}
	sess := h.sessionClient(f.boss)
	for name, cl := range map[string]*twofaClient{"token": tok, "stampless session": sess} {
		for _, p := range []string{"/api/users", "/api/roles/users", "/api/settings"} {
			if rec := cl.do(http.MethodGet, p, "", ""); rec.Code != http.StatusOK {
				t.Errorf("%s GET %s: %d %s", name, p, rec.Code, rec.Body.String())
			}
		}
	}
}

// J15 -- GET /api/profile/twofa reports stepup_ttl: 0 with no stamp, with an expired stamp and for
// an API token; between 1 and 300 right after a step-up; never any key material.
func TestStepUpTTLReported(t *testing.T) {
	f := newSUFixture(t)
	h := f.h
	h.setTOTP(f.boss)
	cl := h.sessionClient(f.boss)
	ttl := func(cl *twofaClient) int {
		t.Helper()
		rec := cl.do(http.MethodGet, "/api/profile/twofa", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/profile/twofa: %d %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		for _, bad := range []string{twofaSecret, "twofa_key", "publicKey", "public_key", "credential"} {
			if strings.Contains(body, bad) {
				t.Fatalf("GET /api/profile/twofa leaks %q: %s", bad, body)
			}
		}
		var out struct {
			StepUpTTL *int `json:"stepup_ttl"`
		}
		if err := json.Unmarshal(dataOf(t, rec), &out); err != nil || out.StepUpTTL == nil {
			t.Fatalf("no stepup_ttl in %s", body)
		}
		return *out.StepUpTTL
	}

	if n := ttl(cl); n != 0 {
		t.Errorf("no stamp: stepup_ttl %d, want 0", n)
	}
	if rec := cl.stepUpTOTP(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d %s", rec.Code, rec.Body.String())
	}
	if n := ttl(cl); n < 1 || n > 300 {
		t.Errorf("right after a step-up: stepup_ttl %d, want 1..300", n)
	}
	h.db.MustExec(`UPDATE sessions SET data = data || jsonb_build_object('stepup_at', EXTRACT(EPOCH FROM NOW())::BIGINT - 301) WHERE id = $1`, cl.jar["session"])
	if n := ttl(cl); n != 0 {
		t.Errorf("expired stamp: stepup_ttl %d, want 0", n)
	}
	if n := ttl(f.apiClient(t, "ttlbot", auth.SuperAdminRoleID)); n != 0 {
		t.Errorf("API token: stepup_ttl %d, want 0", n)
	}
}
