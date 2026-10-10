package main

// Fork (brand picker) -- integrations BRAND-PICKER-SPEC I5, I6 (wiring), I7 and I19 against a real
// database through the real router (twofaHarness, built by initHTTPHandlers), so the pm wrapper,
// the session middleware and the per-list permission checks are the production ones. Shared by
// lists_brand_db_test.go and lists_lock_db_test.go. LISTMONK_TEST_PG opt-in. The harness
// configures one SMTP from address, hello@acme.test (newLinkHarness). Fixture names are
// example.test / acme.test only.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/paginator"
	"github.com/lib/pq"
	"gopkg.in/volatiletech/null.v6"
)

type bpFixture struct {
	h     *twofaHarness
	boss  int
	admin *twofaClient // a Super Admin cookie session
}

func newBPFixture(t *testing.T) *bpFixture {
	t.Helper()
	h := newTwofaHarness(t)
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})
	// The harness seeds the Super Admin role with no permissions; the router's pm() passes it by
	// role id, but the handlers' list checks read the permission map, as on a real install whose
	// Super Admin role carries every permission.
	h.db.MustExec(`UPDATE roles SET permissions = $2 WHERE id = $1`, auth.SuperAdminRoleID,
		pq.StringArray{auth.PermListGetAll, auth.PermListManageAll, "brands:get", "brands:manage"})
	f := &bpFixture{h: h}
	f.boss = h.user("boss", auth.SuperAdminRoleID)
	if _, err := cacheUsers(h.app.core, h.app.auth); err != nil {
		t.Fatal(err)
	}
	f.admin = h.sessionClient(f.boss)
	return f
}

// brandRow inserts a brands row directly (site "" = NULL).
func (f *bpFixture) brandRow(slug, from, site string) {
	f.h.t.Helper()
	f.h.db.MustExec(`INSERT INTO brands (slug, from_email, site) VALUES ($1, $2, NULLIF($3, ''))`, slug, from, site)
}

// sqlList inserts a list directly with the given tags.
func (f *bpFixture) sqlList(name string, tags ...string) int {
	f.h.t.Helper()
	var id int
	if tags == nil {
		tags = []string{}
	}
	if err := f.h.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), $1, 'private', 'single', $2) RETURNING id`,
		name, pq.StringArray(tags)); err != nil {
		f.h.t.Fatal(err)
	}
	return id
}

// tags is a list's stored tags, sorted and joined with |.
func (f *bpFixture) tags(id int) string {
	f.h.t.Helper()
	var out []string
	if err := f.h.db.Select(&out, `SELECT UNNEST(tags) FROM lists WHERE id = $1`, id); err != nil {
		f.h.t.Fatal(err)
	}
	sort.Strings(out)
	return strings.Join(out, "|")
}

func (f *bpFixture) count(q string, args ...any) int {
	f.h.t.Helper()
	var n int
	if err := f.h.db.Get(&n, q, args...); err != nil {
		f.h.t.Fatalf("%s: %v", q, err)
	}
	return n
}

// perListUser is a user whose user role carries userPerms and whose list role grants listPerms
// on the given lists only.
func (f *bpFixture) perListUser(name string, userPerms []string, listPerms []string, lists ...int) *twofaClient {
	f.h.t.Helper()
	var ur int
	f.h.db.Get(&ur, `INSERT INTO roles (type, name, permissions) VALUES ('user', $1, $2) RETURNING id`, name+" user role", pq.StringArray(userPerms))
	perms := make([]auth.ListPermission, 0, len(lists))
	for _, l := range lists {
		perms = append(perms, auth.ListPermission{ID: l, Permissions: listPerms})
	}
	lr, err := f.h.app.core.CreateListRole(auth.ListRole{Name: null.StringFrom(name + " list role"), Lists: perms})
	if err != nil {
		f.h.t.Fatal(err)
	}
	id := f.h.user(name, ur)
	f.h.db.MustExec(`UPDATE users SET list_role_id = $2 WHERE id = $1`, id, lr.ID)
	if _, err := cacheUsers(f.h.app.core, f.h.app.auth); err != nil {
		f.h.t.Fatal(err)
	}
	return f.h.sessionClient(id)
}

// message is the error body's message.
func message(rec *httptest.ResponseRecorder) string {
	var out struct {
		Message string `json:"message"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Message
}

// wantErr asserts a status and that the message is the translation of key with args.
func (f *bpFixture) wantErr(what string, rec *httptest.ResponseRecorder, code int, key string, args ...string) {
	f.h.t.Helper()
	if rec.Code != code {
		f.h.t.Fatalf("%s: status %d want %d: %s", what, rec.Code, code, rec.Body.String())
	}
	if key != "" {
		if want := f.h.app.i18n.Ts(key, args...); message(rec) != want {
			f.h.t.Fatalf("%s: message %q want %q (%s)", what, message(rec), want, key)
		}
	}
}

func (f *bpFixture) ok(what string, rec *httptest.ResponseRecorder) json.RawMessage {
	f.h.t.Helper()
	if rec.Code != http.StatusOK {
		f.h.t.Fatalf("%s: status %d: %s", what, rec.Code, rec.Body.String())
	}
	return dataOf(f.h.t, rec)
}

// I5 -- PUT /api/brands/:slug re-projects onto every list of the brand atomically. Success: both
// acme lists take the new From and lose the site, keeping their free tags; the other brand's list
// and an untagged list are untouched. Failure (a BEFORE UPDATE trigger raising for the second
// acme list): 500, and the row AND the first list are as they were.
func TestBrandUpdateReprojects(t *testing.T) {
	f := newBPFixture(t)
	f.brandRow("acme", "Acme <hello@acme.test>", "https://shop.acme.test")
	f.brandRow("beta", "Beta <hello@beta.test>", "")
	a1 := f.sqlList("Acme one", "holiday", "brand:acme", "from:Acme <hello@acme.test>", "site:https://shop.acme.test")
	a2 := f.sqlList("Acme two", "brand:acme", "from:Acme <hello@acme.test>", "site:https://shop.acme.test", "repermission:9")
	b1 := f.sqlList("Beta", "brand:beta", "from:Beta <hello@beta.test>")
	u1 := f.sqlList("Untagged", "seed")
	beforeB, beforeU := f.tags(b1), f.tags(u1)

	f.ok("PUT acme", f.admin.json(http.MethodPut, "/api/brands/acme", map[string]any{"from_email": "Acme Shop <hello@acme.test>"}))
	if got := f.tags(a1); got != "brand:acme|from:Acme Shop <hello@acme.test>|holiday" {
		t.Fatalf("list one after re-projection: %q", got)
	}
	if got := f.tags(a2); got != "brand:acme|from:Acme Shop <hello@acme.test>|repermission:9" {
		t.Fatalf("list two after re-projection: %q", got)
	}
	if f.tags(b1) != beforeB || f.tags(u1) != beforeU {
		t.Fatalf("other lists changed: %q %q", f.tags(b1), f.tags(u1))
	}
	if f.count(`SELECT COUNT(*) FROM brands WHERE slug = 'acme' AND from_email = 'Acme Shop <hello@acme.test>' AND site IS NULL`) != 1 {
		t.Fatal("row not updated")
	}

	// The failure: the second list's UPDATE raises; the transaction rolls back whole.
	f.h.db.MustExec(`CREATE FUNCTION bp_fail() RETURNS TRIGGER AS $$ BEGIN RAISE EXCEPTION 'injected'; END; $$ LANGUAGE plpgsql`)
	f.h.db.MustExec(`CREATE TRIGGER bp_fail BEFORE UPDATE ON lists FOR EACH ROW WHEN (OLD.id = ` + itoa(a2) + `) EXECUTE FUNCTION bp_fail()`)
	before1 := f.tags(a1)
	rec := f.admin.json(http.MethodPut, "/api/brands/acme", map[string]any{"from_email": "Acme Again <hello@acme.test>", "site": "https://acme.test/s"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("injected failure: %d %s", rec.Code, rec.Body.String())
	}
	if f.tags(a1) != before1 {
		t.Fatalf("first list changed by a failed re-projection: %q", f.tags(a1))
	}
	if f.count(`SELECT COUNT(*) FROM brands WHERE slug = 'acme' AND from_email = 'Acme Shop <hello@acme.test>' AND site IS NULL`) != 1 {
		t.Fatal("row changed by a failed re-projection")
	}
	f.h.db.MustExec(`DROP TRIGGER bp_fail ON lists`)

	// An unknown slug is a 404.
	f.wantErr("PUT unknown", f.admin.json(http.MethodPut, "/api/brands/nope", map[string]any{"from_email": "hello@acme.test"}), http.StatusNotFound, "")
}

// I6 (wiring) -- the brands handlers pass the configured from_addresses lookup and the importer's
// sanitizer into BrandProblem: an unconfigured address and an unsanitizable bare From are refused
// on POST and on PUT, and so are the new length and reserved-slug rules.
func TestBrandCreateRefusesUnconfiguredFrom(t *testing.T) {
	f := newBPFixture(t)
	post := func(body map[string]any) *httptest.ResponseRecorder {
		return f.admin.json(http.MethodPost, "/api/brands", body)
	}

	f.wantErr("unconfigured", post(map[string]any{"slug": "zed", "from_email": "Zed <hello@zed.test>"}),
		http.StatusBadRequest, "lists.brandFromTagUnknownAddress", "from", "Zed <hello@zed.test>")
	f.wantErr("unsanitizable bare", post(map[string]any{"slug": "zed", "from_email": "not-an-address"}),
		http.StatusBadRequest, "lists.brandFromTagInvalid", "from", "not-an-address")
	f.wantErr("reserved slug", post(map[string]any{"slug": "health", "from_email": "hello@acme.test"}),
		http.StatusBadRequest, "lists.brandSlugReserved", "brand", "health")
	f.wantErr("bad slug", post(map[string]any{"slug": "Thirsty Girl", "from_email": "hello@acme.test"}),
		http.StatusBadRequest, "lists.brandTagInvalidSlug", "brand", "Thirsty Girl")
	long := strings.Repeat("A", 90) + " <hello@acme.test>"
	f.wantErr("too long", post(map[string]any{"slug": "zed", "from_email": long}),
		http.StatusBadRequest, "lists.brandFromTagTooLong", "from", long)
	f.wantErr("relative site", post(map[string]any{"slug": "zed", "from_email": "hello@acme.test", "site": "/store"}),
		http.StatusBadRequest, "lists.siteTagInvalid")
	if n := f.count(`SELECT COUNT(*) FROM brands`); n != 0 {
		t.Fatalf("%d rows written by refused POSTs", n)
	}

	var b struct {
		Slug        string  `json:"slug"`
		FromEmail   string  `json:"from_email"`
		Site        *string `json:"site"`
		DisplayName string  `json:"display_name"`
	}
	json.Unmarshal(f.ok("valid", post(map[string]any{"slug": "acme", "from_email": " Acme <hello@acme.test> ", "site": "https://shop.acme.test"})), &b)
	if b.Slug != "acme" || b.FromEmail != "Acme <hello@acme.test>" || b.Site == nil || *b.Site != "https://shop.acme.test" || b.DisplayName != "Acme" {
		t.Fatalf("created %+v", b)
	}

	f.wantErr("PUT unconfigured", f.admin.json(http.MethodPut, "/api/brands/acme", map[string]any{"from_email": "Acme <hello@zed.test>"}),
		http.StatusBadRequest, "lists.brandFromTagUnknownAddress", "from", "Acme <hello@zed.test>")
	f.wantErr("PUT unsanitizable", f.admin.json(http.MethodPut, "/api/brands/acme", map[string]any{"from_email": "nope"}),
		http.StatusBadRequest, "lists.brandFromTagInvalid", "from", "nope")
}

// I7 -- a POST whose slug matches an existing row exactly or only by case is refused (409).
func TestBrandSlugCaseUnique(t *testing.T) {
	f := newBPFixture(t)
	f.ok("create", f.admin.json(http.MethodPost, "/api/brands", map[string]any{"slug": "acme", "from_email": "Acme <hello@acme.test>"}))
	for _, slug := range []string{"acme", "Acme", "ACME"} {
		f.wantErr("duplicate "+slug, f.admin.json(http.MethodPost, "/api/brands", map[string]any{"slug": slug, "from_email": "hello@acme.test"}),
			http.StatusConflict, "brands.exists", "brand", slug)
	}
	if n := f.count(`SELECT COUNT(*) FROM brands`); n != 1 {
		t.Fatalf("%d rows, want 1", n)
	}
}

// I19 -- GET /api/brands is readable by a user holding only a per-list list:manage (no
// lists:get_all) -- who can then save their list's form with a brand -- and by a brands:get-only
// user. Writes still need lists:manage_all.
func TestBrandsReadableWithoutListsGetAll(t *testing.T) {
	f := newBPFixture(t)
	f.brandRow("acme", "Acme <hello@acme.test>", "")
	f.brandRow("beta", "hello@beta.test", "")
	mine := f.sqlList("Mine")

	manager := f.perListUser("manager", []string{"campaigns:get"}, []string{auth.PermListGet, auth.PermListManage}, mine)
	analyst := f.perListUser("analyst", []string{"brands:get"}, []string{auth.PermListGet}, mine)

	for name, cl := range map[string]*twofaClient{"per-list list:manage": manager, "brands:get only": analyst} {
		var rows []struct {
			Slug        string `json:"slug"`
			FromEmail   string `json:"from_email"`
			DisplayName string `json:"display_name"`
		}
		if err := json.Unmarshal(f.ok(name+" GET /api/brands", cl.json(http.MethodGet, "/api/brands", nil)), &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0].Slug != "acme" || rows[0].DisplayName != "Acme" || rows[1].Slug != "beta" || rows[1].DisplayName != "hello@beta.test" {
			t.Fatalf("%s: rows %+v", name, rows)
		}
	}

	// The per-list manager saves their list's form with a brand.
	f.ok("manager saves", manager.json(http.MethodPut, "/api/lists/"+itoa(mine), map[string]any{"name": "Mine", "brand": "acme", "tags": []string{"x"}}))
	if got := f.tags(mine); got != "brand:acme|from:Acme <hello@acme.test>|x" {
		t.Fatalf("manager's save: %q", got)
	}

	// Neither may write a brand row.
	for name, cl := range map[string]*twofaClient{"manager": manager, "analyst": analyst} {
		if rec := cl.json(http.MethodPost, "/api/brands", map[string]any{"slug": "zed", "from_email": "hello@acme.test"}); rec.Code != http.StatusForbidden {
			t.Fatalf("%s POST /api/brands: %d", name, rec.Code)
		}
		if rec := cl.json(http.MethodPut, "/api/brands/acme", map[string]any{"from_email": "hello@acme.test"}); rec.Code != http.StatusForbidden {
			t.Fatalf("%s PUT /api/brands/acme: %d", name, rec.Code)
		}
	}
}
