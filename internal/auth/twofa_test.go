package auth

// Fork (two-factor) -- integrations PASSKEY-2FA-SPEC I1, I2 and I27 (the derivation half): who is a
// read-only user, who is enforced, and the relying party derived from app.root_url. Pure tests.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func permissionsJSON(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "permissions.json"))
	if err != nil {
		t.Fatalf("permissions.json: %v", err)
	}
	var groups []struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &groups); err != nil {
		t.Fatalf("permissions.json: %v", err)
	}
	var out []string
	for _, g := range groups {
		out = append(out, g.Permissions...)
	}
	sort.Strings(out)
	return out
}

// I1 -- every key in permissions.json is classified, and the read set is exactly the 13
// get/get_all/get_analytics keys; subscribers:sql_query and an unknown action are not read.
func TestIsReadPermClassifiesEveryPermission(t *testing.T) {
	want := map[string]bool{
		"lists:get_all": true, "brands:get": true,
		"subscribers:get": true, "subscribers:get_all": true,
		"campaigns:get": true, "campaigns:get_all": true, "campaigns:get_analytics": true,
		"bounces:get": true, "media:get": true, "templates:get": true,
		"users:get": true, "roles:get": true, "settings:get": true,
	}
	if len(want) != 13 {
		t.Fatalf("fixture: %d read keys, want 13", len(want))
	}

	var reads []string
	for _, p := range permissionsJSON(t) {
		got := IsReadPerm(p)
		if got != want[p] {
			t.Errorf("IsReadPerm(%q) = %v, want %v", p, got, want[p])
		}
		if got {
			reads = append(reads, p)
		}
	}
	if len(reads) != 13 {
		t.Fatalf("permissions.json read set = %v (%d), want exactly the 13 get/get_all/get_analytics keys -- a new permission must be classified here", reads, len(reads))
	}

	for _, p := range []string{"subscribers:sql_query", "subscribers:get_some", "foo:bar", "nocolon", "", "list:manage", "campaigns:review_structure"} {
		if IsReadPerm(p) {
			t.Errorf("IsReadPerm(%q) = true, want false (more than read)", p)
		}
	}
	if !IsReadPerm(PermListGet) {
		t.Error("IsReadPerm(list:get) = false")
	}
}

func listRole(perms ...string) *ListRolePermissions {
	return &ListRolePermissions{ID: 7, Lists: []ListPermission{{ID: 1, Permissions: perms}}}
}

// I2 -- TwofaEnforced. The role id is read from UserRole.ID, the field setupUserFields fills (it
// zeroes UserRoleID).
func TestTwofaEnforced(t *testing.T) {
	user := func(roleID int, perms []string, lr *ListRolePermissions) User {
		u := User{Type: UserTypeUser, PasswordLogin: true, ListRole: lr}
		u.UserRole.ID = roleID
		u.UserRole.Permissions = perms
		return u
	}

	cases := []struct {
		name string
		u    User
		want bool
	}{
		{"super admin", user(SuperAdminRoleID, []string{"campaigns:get"}, nil), true},
		{"super admin, empty stored permission list", user(SuperAdminRoleID, nil, nil), true},
		{"role with one non-read permission", user(5, []string{"campaigns:get", "campaigns:manage"}, nil), true},
		{"role with sql_query", user(5, []string{"subscribers:get", "subscribers:sql_query"}, nil), true},
		{"read-only role with a list:manage list role", user(5, []string{"campaigns:get"}, listRole(PermListGet, PermListManage)), true},
		{"read-only role with list:get lists", user(5, []string{"campaigns:get", "campaigns:get_analytics", "lists:get_all"}, listRole(PermListGet)), false},
		{"read-only role, no list role", user(5, []string{"settings:get"}, nil), false},
	}
	for _, tc := range cases {
		if got := tc.u.IsTwofaEnforced(); got != tc.want {
			t.Errorf("%s: IsTwofaEnforced = %v, want %v", tc.name, got, tc.want)
		}
	}

	// The trap: a row whose role id sits only in UserRoleID (a login-user row) is NOT read as
	// Super Admin -- callers must load the user (core.GetUser) first.
	raw := User{Type: UserTypeUser, PasswordLogin: true, UserRoleID: SuperAdminRoleID}
	if raw.IsTwofaEnforced() {
		t.Error("UserRoleID alone made the user enforced; the rule must read UserRole.ID")
	}

	// An API user and a non-password (OIDC-only) user are never enforced.
	api := user(SuperAdminRoleID, []string{"campaigns:manage"}, nil)
	api.Type = UserTypeAPI
	if api.IsTwofaEnforced() {
		t.Error("an API user is enforced")
	}
	oidc := user(SuperAdminRoleID, []string{"campaigns:manage"}, nil)
	oidc.PasswordLogin = false
	if oidc.IsTwofaEnforced() {
		t.Error("a user without password_login is enforced")
	}

	// twofa_required = enforced AND the switch: the JSON carries both, as setupUserFields fills
	// them (core.setupUserFields -- the DB tests read them through GET /api/users).
	for _, sw := range []bool{false, true} {
		for _, tc := range cases {
			u := tc.u
			u.TwofaEnforced = u.IsTwofaEnforced()
			u.TwofaRequired = u.TwofaEnforced && sw
			b, _ := json.Marshal(u)
			var m map[string]any
			json.Unmarshal(b, &m)
			if m["twofa_enforced"] != tc.want || m["twofa_required"] != (tc.want && sw) {
				t.Errorf("%s, switch %v: json twofa_enforced=%v twofa_required=%v", tc.name, sw, m["twofa_enforced"], m["twofa_required"])
			}
		}
	}

	// HasTwofaFactor: TOTP on, or a passkey.
	if (&User{TwofaType: "none"}).HasTwofaFactor() || !(&User{TwofaType: "totp"}).HasTwofaFactor() || !(&User{TwofaType: "none", PasskeyCount: 1}).HasTwofaFactor() {
		t.Error("HasTwofaFactor misclassified")
	}
}

// I27 (derivation) -- one pure function: https host, host with port, localhost, unparseable.
func TestRelyingPartyFromRootURL(t *testing.T) {
	ok := []struct{ in, id, origin string }{
		{"https://Mail.Example.test", "mail.example.test", "https://mail.example.test"},
		{"https://mail.example.test/", "mail.example.test", "https://mail.example.test"},
		{"https://mail.example.test:8443/sub", "mail.example.test", "https://mail.example.test:8443"},
		{"http://localhost:9000", "localhost", "http://localhost:9000"},
		{" http://localhost ", "localhost", "http://localhost"},
	}
	for _, c := range ok {
		id, origin, err := RelyingPartyFromRootURL(c.in)
		if err != nil || id != c.id || origin != c.origin {
			t.Errorf("RelyingPartyFromRootURL(%q) = %q, %q, %v; want %q, %q", c.in, id, origin, err, c.id, c.origin)
		}
	}
	for _, in := range []string{"", "not a url", "mail.example.test", "ftp://mail.example.test", "https://", "://bad", "http://[::1"} {
		if id, origin, err := RelyingPartyFromRootURL(in); err == nil {
			t.Errorf("RelyingPartyFromRootURL(%q) = %q, %q, nil; want an error", in, id, origin)
		}
	}
}
