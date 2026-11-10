package main

// Fork (journey lists) -- integrations listmonk/JOURNEY-LISTS-SPEC D6/D7, invariants I7 and I14: the
// standard-journey-list lock answers every list and membership write path as its D7 row says,
// against a real database through the real router (bpFixture). A Marketing session (lists and
// subscribers manage, import, no lists:journeys) is refused 403 lists.journeyLocked and nothing is
// written; suppressions, write-backs and the brand re-projection stay open; a system role holding
// lists:journeys passes. LISTMONK_TEST_PG opt-in (newLinkHarness skips without it). Fixture names
// are acme.test / example.test only.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/subimporter"
	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
)

type lockFixture struct {
	*bpFixture
	fo, step, post, queue, sku, plain int
	mkt                               *twofaClient // a Marketing session: no lists:journeys
	sys                               *twofaClient // an API token whose role holds lists:journeys
}

var marketingPerms = []string{
	auth.PermListGetAll, auth.PermListManageAll, "subscribers:get_all", "subscribers:get", "subscribers:manage",
	"subscribers:import", "bounces:get", "bounces:manage", "settings:maintain",
}

func newLockFixture(t *testing.T) *lockFixture {
	t.Helper()
	f, _, _ := newPersonaFixture(t)
	l := &lockFixture{bpFixture: f}
	from := "from:" + pfBrandFrom
	l.fo = f.sqlList("Acme first order", "brand:acme", from)
	f.juTrigger(l.fo, `{"kind":"first_order"}`)
	l.step = f.sqlList("Acme No purchase 2 days", "brand:acme", from)
	f.juTrigger(l.step, `{"kind":"no_purchase","days":2}`)
	l.post = f.sqlList("Acme Post purchase 14 days", "brand:acme", from)
	f.juTrigger(l.post, `{"kind":"post_purchase","days":14}`)
	l.queue = f.sqlList("Acme re-engagement 1", "brand:acme", from, "reengage:5:1")
	l.sku = f.sqlList("Acme widget buyers", "brand:acme", from)
	f.juTrigger(l.sku, `{"kind":"skus","skus":["widget"]}`)
	l.plain = f.sqlList("Acme news", "brand:acme", from)

	var mktRole, sysRole int
	f.h.db.Get(&mktRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Marketing', $1) RETURNING id`, pq.StringArray(marketingPerms))
	f.h.db.Get(&sysRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'ManagePreferences API', $1) RETURNING id`,
		pq.StringArray(append([]string{auth.PermListJourneys}, marketingPerms...)))
	mkt := f.h.user("marketer", mktRole)
	u, err := f.h.app.core.CreateUser(auth.User{Type: auth.UserTypeAPI, Username: "syncbot", Name: "syncbot", UserRoleID: sysRole, Status: auth.UserStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cacheUsers(f.h.app.core, f.h.app.auth); err != nil {
		t.Fatal(err)
	}
	l.mkt = f.h.sessionClient(mkt)
	l.sys = f.h.client()
	l.sys.hdr["Authorization"] = "token syncbot:" + u.Password.String
	return l
}

func (l *lockFixture) sub(email string, lists ...int) int {
	l.h.t.Helper()
	var id int
	if err := l.h.db.Get(&id, `INSERT INTO subscribers (uuid, email, name, status) VALUES (gen_random_uuid(), $1, 'Orig', 'enabled') RETURNING id`, email); err != nil {
		l.h.t.Fatal(err)
	}
	for _, x := range lists {
		l.h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, id, x)
	}
	return id
}

// state is every list, subscriber and membership row: a refused request writes nothing.
func (l *lockFixture) state() string {
	l.h.t.Helper()
	var rows []string
	if err := l.h.db.Select(&rows, `SELECT id || ':' || name || ':' || type || ':' || optin || ':' || status || ':' || COALESCE(description, '') || ':' ||
		ARRAY_TO_STRING(tags, '|') || ':' || COALESCE(join_trigger::TEXT, '') || ':' || transactional::TEXT || ':' || trigger_active::TEXT FROM lists ORDER BY id`); err != nil {
		l.h.t.Fatal(err)
	}
	var subs []string
	if err := l.h.db.Select(&subs, `SELECT s.id || ':' || s.email || ':' || s.name || ':' || s.status || ':' || COALESCE((SELECT STRING_AGG(sl.list_id || '/' || sl.status, ',' ORDER BY sl.list_id)
		FROM subscriber_lists sl WHERE sl.subscriber_id = s.id), '') FROM subscribers s ORDER BY s.id`); err != nil {
		l.h.t.Fatal(err)
	}
	return joinStrings(rows) + "#" + joinStrings(subs)
}

func joinStrings(s []string) string {
	out := ""
	for _, x := range s {
		out += x + ";"
	}
	return out
}

// refusedName asserts the 403 lists.journeyLocked naming the list, and that nothing changed.
func (l *lockFixture) refusedName(what string, rec *httptest.ResponseRecorder, name, before string) {
	l.h.t.Helper()
	l.wantErr(what, rec, http.StatusForbidden, "lists.journeyLocked", "name", name)
	if got := l.state(); got != before {
		l.h.t.Fatalf("%s: a refused request changed the rows:\n%s\n->\n%s", what, before, got)
	}
}

func (l *lockFixture) member(sub, list int) string {
	l.h.t.Helper()
	var st []string
	l.h.db.Select(&st, `SELECT status::TEXT FROM subscriber_lists WHERE subscriber_id = $1 AND list_id = $2`, sub, list)
	if len(st) == 0 {
		return "<none>"
	}
	return st[0]
}

// L1 -- any change through PUT /api/lists/:id to a locked list is refused, one case per field group,
// on each locked kind (first order, both step kinds, a queue). The check runs on the STORED list.
func TestLockUpdateLockedList(t *testing.T) {
	l := newLockFixture(t)
	l.brandRow("beta", "Beta <hello@beta.test>", "")
	names := map[int]string{l.fo: "Acme first order", l.step: "Acme No purchase 2 days", l.post: "Acme Post purchase 14 days", l.queue: "Acme re-engagement 1"}
	for id, name := range names {
		cases := map[string]map[string]any{
			"name":           {"name": name + " renamed"},
			"brand":          {"name": name, "brand": "beta"},
			"type":           {"name": name, "type": "temporary"},
			"optin":          {"name": name, "optin": "double"},
			"description":    {"name": name, "description": "changed"},
			"tags":           {"name": name, "tags": []string{"holiday"}},
			"status=archive": {"name": name, "status": "archived"},
			"transactional":  {"name": name, "transactional": true},
			"trigger kind":   {"name": name, "join_trigger": json.RawMessage(`{"kind":"skus","skus":["a"]}`)},
			"trigger days":   {"name": name, "join_trigger": json.RawMessage(`{"kind":"no_purchase","days":5}`)},
			"trigger null":   {"name": name, "join_trigger": nil},
			"round-trip":     {"name": name},
		}
		for what, body := range cases {
			before := l.state()
			l.refusedName(name+": "+what, l.mkt.json(http.MethodPut, "/api/lists/"+itoa(id), body), name, before)
		}
	}
	// The same edit by a holder of lists:journeys passes (the system role, and Super Admin).
	l.ok("system role renames a locked list", l.sys.json(http.MethodPut, "/api/lists/"+itoa(l.fo), map[string]any{"name": "Acme first order 2"}))
	l.ok("super admin renames a locked list", l.admin.json(http.MethodPut, "/api/lists/"+itoa(l.fo), map[string]any{"name": "Acme first order"}))
	// An unlocked list stays Marketing's (SKU and plain).
	l.ok("marketing renames a SKU list", l.mkt.json(http.MethodPut, "/api/lists/"+itoa(l.sku), map[string]any{"name": "Acme widget buyers 2"}))
	l.ok("marketing edits a plain list", l.mkt.json(http.MethodPut, "/api/lists/"+itoa(l.plain), map[string]any{"name": "Acme news", "description": "d"}))
}

// L2 -- setting a standard kind on an unlocked list is refused; setting or changing skus is allowed.
func TestLockSetStandardKind(t *testing.T) {
	l := newLockFixture(t)
	for _, jt := range []string{`{"kind":"first_order"}`, `{"kind":"no_purchase","days":2}`, `{"kind":"post_purchase","days":5}`} {
		for _, id := range []int{l.plain, l.sku} {
			before := l.state()
			name := map[int]string{l.plain: "Acme news", l.sku: "Acme widget buyers"}[id]
			l.refusedName("set "+jt, l.mkt.json(http.MethodPut, "/api/lists/"+itoa(id),
				map[string]any{"name": name, "join_trigger": json.RawMessage(jt)}), name, before)
		}
	}
	l.ok("set skus on a plain list", l.mkt.json(http.MethodPut, "/api/lists/"+itoa(l.plain),
		map[string]any{"name": "Acme news", "join_trigger": json.RawMessage(`{"kind":"skus","skus":["b"]}`)}))
	l.ok("change skus", l.mkt.json(http.MethodPut, "/api/lists/"+itoa(l.sku),
		map[string]any{"name": "Acme widget buyers", "join_trigger": json.RawMessage(`{"kind":"skus","skus":["c"]}`)}))
	if got := l.juStoredTrigger(l.sku); got != `{"skus": ["c"], "kind": "skus"}` && got != `{"kind": "skus", "skus": ["c"]}` {
		t.Fatalf("sku change stored %s", got)
	}
	// A holder of lists:journeys may set a standard kind.
	l.ok("system role sets first_order", l.sys.json(http.MethodPut, "/api/lists/"+itoa(l.plain),
		map[string]any{"name": "Acme news", "join_trigger": json.RawMessage(`{"kind":"first_order"}`)}))
}

// L3 -- POST /api/lists with a reengage: tag is refused; any other create is allowed.
func TestLockCreateReengage(t *testing.T) {
	l := newLockFixture(t)
	before := l.state()
	l.refusedName("create a queue", l.mkt.json(http.MethodPost, "/api/lists",
		map[string]any{"name": "Acme re-engagement 2", "type": "private", "optin": "single", "tags": []string{"reengage:5:2"}}), "Acme re-engagement 2", before)
	l.ok("create a plain list", l.mkt.json(http.MethodPost, "/api/lists",
		map[string]any{"name": "Acme spring", "type": "private", "optin": "single", "tags": []string{"spring"}}))
	l.ok("system role creates a queue", l.sys.json(http.MethodPost, "/api/lists",
		map[string]any{"name": "Acme re-engagement 2", "type": "private", "optin": "single", "tags": []string{"reengage:5:2"}}))
}

// L4 -- DELETE /api/lists/:id on a locked list is refused.
func TestLockDeleteList(t *testing.T) {
	l := newLockFixture(t)
	for id, name := range map[int]string{l.fo: "Acme first order", l.step: "Acme No purchase 2 days", l.queue: "Acme re-engagement 1"} {
		before := l.state()
		l.refusedName("delete "+name, l.mkt.json(http.MethodDelete, "/api/lists/"+itoa(id), nil), name, before)
	}
	l.ok("delete the SKU list", l.mkt.json(http.MethodDelete, "/api/lists/"+itoa(l.sku), nil))
	l.ok("system role deletes a locked list", l.sys.json(http.MethodDelete, "/api/lists/"+itoa(l.post), nil))
}

// L5 -- bulk delete by ids and by query: refused WHOLE when any matched list is locked.
func TestLockDeleteListsBulk(t *testing.T) {
	l := newLockFixture(t)
	before := l.state()
	l.refusedName("bulk ids", l.mkt.json(http.MethodDelete, "/api/lists?id="+itoa(l.plain)+"&id="+itoa(l.fo), nil), "Acme first order", before)
	rec := l.mkt.json(http.MethodDelete, "/api/lists?query=acme", nil)
	l.wantErr("bulk query", rec, http.StatusForbidden, "")
	if got := l.state(); got != before {
		t.Fatalf("a refused bulk query delete removed lists:\n%s\n->\n%s", before, got)
	}
	l.ok("bulk ids, unlocked only", l.mkt.json(http.MethodDelete, "/api/lists?id="+itoa(l.plain)+"&id="+itoa(l.sku), nil))
	if l.count(`SELECT COUNT(*) FROM lists WHERE id IN ($1, $2)`, l.plain, l.sku) != 0 {
		t.Fatal("unlocked bulk delete did not delete")
	}
}

// L8 -- the reconciler's write-back is not an edit: allowed on a locked list without lists:journeys.
func TestLockTriggerSyncAllowed(t *testing.T) {
	l := newLockFixture(t)
	l.ok("trigger-sync on a locked list", l.mkt.json(http.MethodPut, "/api/lists/"+itoa(l.fo)+"/trigger-sync",
		map[string]any{"synced_at": "2026-11-10T05:05:12Z", "state": "live", "slug": "acme-first-order"}))
	if l.count(`SELECT COUNT(*) FROM lists WHERE id = $1 AND trigger_sync->>'slug' = 'acme-first-order'`, l.fo) != 1 {
		t.Fatal("write-back not stored")
	}
}

// L9 -- PUT /api/brands/:slug re-projects every brand list, locked ones included.
func TestLockBrandReprojection(t *testing.T) {
	l := newLockFixture(t)
	l.ok("brand re-projection", l.mkt.json(http.MethodPut, "/api/brands/acme", map[string]any{"from_email": "Acme Shop <hello@acme.test>"}))
	for _, id := range []int{l.fo, l.step, l.queue} {
		if l.count(`SELECT COUNT(*) FROM lists WHERE id = $1 AND 'from:Acme Shop <hello@acme.test>' = ANY(tags)`, id) != 1 {
			t.Fatalf("locked list %d was not re-projected: %s", id, l.tags(id))
		}
	}
}

// M1 -- POST /api/subscribers naming a locked list is refused.
func TestLockCreateSubscriber(t *testing.T) {
	l := newLockFixture(t)
	before := l.state()
	l.refusedName("create on the first-order list", l.mkt.json(http.MethodPost, "/api/subscribers",
		map[string]any{"email": "new@example.test", "name": "New", "lists": []int{l.plain, l.fo}}), "Acme first order", before)
	l.refusedName("create on a queue", l.mkt.json(http.MethodPost, "/api/subscribers",
		map[string]any{"email": "new@example.test", "name": "New", "lists": []int{l.queue}}), "Acme re-engagement 1", before)
	l.ok("create on unlocked lists", l.mkt.json(http.MethodPost, "/api/subscribers",
		map[string]any{"email": "new@example.test", "name": "New", "lists": []int{l.plain, l.sku}}))
}

// M2 -- PUT /api/subscribers/:id adding or dropping a locked list is refused; a round-trip, a
// status change to blocklisted and an edit of an unlocked membership are allowed.
func TestLockUpdateSubscriber(t *testing.T) {
	l := newLockFixture(t)
	m := l.sub("member@example.test", l.plain, l.fo)
	other := l.sub("other@example.test", l.plain)
	put := func(id int, email string, lists []int, extra map[string]any) *httptest.ResponseRecorder {
		body := map[string]any{"email": email, "name": "Changed", "status": "enabled", "lists": lists}
		for k, v := range extra {
			body[k] = v
		}
		return l.mkt.json(http.MethodPut, "/api/subscribers/"+itoa(id), body)
	}
	before := l.state()
	l.refusedName("add the first-order list", put(other, "other@example.test", []int{l.plain, l.fo}, nil), "Acme first order", before)
	l.refusedName("drop the first-order list", put(m, "member@example.test", []int{l.plain}, nil), "Acme first order", before)
	l.refusedName("drop everything", put(m, "member@example.test", []int{}, nil), "Acme first order", before)

	l.ok("round-trip", put(m, "member@example.test", []int{l.plain, l.fo}, nil))
	if l.member(m, l.fo) != "confirmed" {
		t.Fatal("round-trip lost the locked membership")
	}
	l.ok("unlocked membership edit", put(m, "member@example.test", []int{l.plain, l.fo, l.sku}, nil))
	if l.member(m, l.sku) != "confirmed" {
		t.Fatal("unlocked add not applied")
	}
	l.ok("blocklist round-trip", put(m, "member@example.test", []int{l.plain, l.fo, l.sku}, map[string]any{"status": "blocklisted"}))
	if l.member(m, l.fo) != "unsubscribed" {
		t.Fatalf("blocklisting did not unsubscribe the locked row: %s", l.member(m, l.fo))
	}
	l.ok("system role drops the locked list", l.sys.json(http.MethodPut, "/api/subscribers/"+itoa(m),
		map[string]any{"email": "member@example.test", "name": "Changed", "status": "enabled", "lists": []int{l.plain}}))
}

// M3 -- PATCH carrying lists that add or drop a locked list is refused; attribs/name-only allowed.
func TestLockPatchSubscriber(t *testing.T) {
	l := newLockFixture(t)
	m := l.sub("member@example.test", l.plain, l.fo)
	other := l.sub("other@example.test", l.plain)
	before := l.state()
	l.refusedName("PATCH drop", l.mkt.json(http.MethodPatch, "/api/subscribers/"+itoa(m), map[string]any{"lists": []int{l.plain}}), "Acme first order", before)
	l.refusedName("PATCH add", l.mkt.json(http.MethodPatch, "/api/subscribers/"+itoa(other), map[string]any{"lists": []int{l.plain, l.fo}}), "Acme first order", before)
	l.ok("PATCH name only", l.mkt.json(http.MethodPatch, "/api/subscribers/"+itoa(m), map[string]any{"name": "Renamed"}))
	l.ok("PATCH attribs only", l.mkt.json(http.MethodPatch, "/api/subscribers/"+itoa(m), map[string]any{"attribs": map[string]any{"x": 1}}))
	l.ok("PATCH lists round-trip", l.mkt.json(http.MethodPatch, "/api/subscribers/"+itoa(m), map[string]any{"lists": []int{l.plain, l.fo}}))
	if l.member(m, l.fo) != "confirmed" {
		t.Fatal("PATCH lost the locked membership")
	}
}

// M4 -- PUT /api/subscribers/lists and /lists/:id: add and remove on a locked target are refused;
// unsubscribe is allowed (hold: TestLockSuppressionAllowed).
func TestLockManageSubscriberLists(t *testing.T) {
	l := newLockFixture(t)
	m := l.sub("member@example.test", l.plain, l.fo, l.queue)
	other := l.sub("other@example.test", l.plain)
	before := l.state()
	l.refusedName("add", l.mkt.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{other}, "action": "add", "target_list_ids": []int{l.plain, l.fo}, "status": "confirmed"}), "Acme first order", before)
	l.refusedName("add by id route", l.mkt.json(http.MethodPut, "/api/subscribers/lists/"+itoa(other),
		map[string]any{"ids": []int{other}, "action": "add", "target_list_ids": []int{l.fo}, "status": "confirmed"}), "Acme first order", before)
	l.refusedName("remove", l.mkt.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{m}, "action": "remove", "target_list_ids": []int{l.queue}}), "Acme re-engagement 1", before)
	l.ok("unsubscribe", l.mkt.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{m}, "action": "unsubscribe", "target_list_ids": []int{l.fo}}))
	if l.member(m, l.fo) != "unsubscribed" {
		t.Fatal("unsubscribe not applied")
	}
	l.ok("add to an unlocked list", l.mkt.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{other}, "action": "add", "target_list_ids": []int{l.sku}, "status": "confirmed"}))
}

// M5 -- PUT /api/subscribers/query/lists: add and remove on a locked target refused; unsubscribe allowed.
func TestLockManageListsByQuery(t *testing.T) {
	l := newLockFixture(t)
	m := l.sub("member@example.test", l.plain, l.fo)
	l.h.db.MustExec(`UPDATE roles SET permissions = permissions || '{subscribers:sql_query}' WHERE name = 'Marketing'`)
	if _, err := cacheUsers(l.h.app.core, l.h.app.auth); err != nil {
		t.Fatal(err)
	}
	before := l.state()
	for _, action := range []string{"add", "remove"} {
		l.refusedName("by-query "+action, l.mkt.json(http.MethodPut, "/api/subscribers/query/lists",
			map[string]any{"query": "subscribers.id = " + itoa(m), "action": action, "target_list_ids": []int{l.fo}, "status": "confirmed"}), "Acme first order", before)
	}
	l.ok("by-query unsubscribe", l.mkt.json(http.MethodPut, "/api/subscribers/query/lists",
		map[string]any{"query": "subscribers.id = " + itoa(m), "action": "unsubscribe", "target_list_ids": []int{l.fo}}))
	if l.member(m, l.fo) != "unsubscribed" {
		t.Fatal("by-query unsubscribe not applied")
	}
}

// M6 -- a subscribe-mode import naming a locked list is refused at start; blocklist mode (which
// targets no list) passes the lock (it then stops on the missing file, past the lock).
func TestLockImport(t *testing.T) {
	l := newLockFixture(t)
	before := l.state()
	l.refusedName("import subscribe into the first-order list", importForm(l.mkt,
		map[string]any{"mode": subimporter.ModeSubscribe, "lists": []int{l.plain, l.fo}, "delim": ","}, "email,name\nimp@example.test,Imp\n"), "Acme first order", before)
	rec := importForm(l.mkt, map[string]any{"mode": subimporter.ModeBlocklist, "lists": []int{l.fo}, "delim": ","}, "")
	if rec.Code == http.StatusForbidden {
		t.Fatalf("blocklist-mode import refused by the lock: %s", rec.Body.String())
	}
	if st := l.h.app.importer.GetStats().Status; st == subimporter.StatusImporting {
		t.Fatal("an import session started")
	}
}

// M7 -- the preset import applies the M6 rule to the preset's target list.
func TestLockImportPreset(t *testing.T) {
	l := newLockFixture(t)
	ps, err := subimporter.ParsePresets([]byte(`[{"key":"k","name":"N","columns":{"email":"e"},`+
		`"list_name_pattern":"^(?P<d>.+)\\.csv$","list_name_template":"{d}","merge":"fill"}]`), models.CampaignLangs)
	if err != nil {
		t.Fatal(err)
	}
	l.h.app.importPresets = ps
	l.h.app.db = l.h.db
	rec := presetForm(l.mkt, "k", "Acme first order.csv", []byte("e\npre@example.test\n"))
	l.wantErr("preset import onto the first-order list", rec, http.StatusForbidden, "lists.journeyLocked", "name", "Acme first order")
	if st := l.h.app.importer.GetStats().Status; st == subimporter.StatusImporting {
		t.Fatal("a refused preset import started a session")
	}
}

// M8/M9/M10, M4 hold -- the lock never blocks a suppression or a person deletion.
func TestLockSuppressionAllowed(t *testing.T) {
	l := newLockFixture(t)
	a := l.sub("a@example.test", l.plain, l.fo)
	b := l.sub("b@example.test", l.step)
	c := l.sub("c@example.test", l.queue)
	d := l.sub("d@example.test", l.post)
	e := l.sub("e@example.test", l.fo)

	l.ok("hold on a locked list", l.mkt.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{a}, "action": "hold", "target_list_ids": []int{l.fo}, "hold": map[string]any{"reason": "manual", "source": "test"}}))
	l.ok("blocklist one", l.mkt.json(http.MethodPut, "/api/subscribers/"+itoa(a)+"/blocklist", nil))
	l.ok("blocklist many", l.mkt.json(http.MethodPut, "/api/subscribers/blocklist", map[string]any{"ids": []int{b}}))
	l.h.db.MustExec(`UPDATE roles SET permissions = permissions || '{subscribers:sql_query}' WHERE name = 'Marketing'`)
	if _, err := cacheUsers(l.h.app.core, l.h.app.auth); err != nil {
		t.Fatal(err)
	}
	l.ok("blocklist by query", l.mkt.json(http.MethodPut, "/api/subscribers/query/blocklist", map[string]any{"query": "subscribers.id = " + itoa(c)}))
	l.ok("bounces blocklist", l.mkt.json(http.MethodPut, "/api/bounces/blocklist", nil))
	l.ok("brand unsubscribe", l.mkt.json(http.MethodPut, "/api/subscribers/brand-unsubscribe", map[string]any{"ids": []int{d}, "brand": "acme"}))
	if l.member(d, l.post) != "unsubscribed" {
		t.Fatalf("brand unsubscribe on a locked list: %s", l.member(d, l.post))
	}
	l.ok("delete one", l.mkt.json(http.MethodDelete, "/api/subscribers/"+itoa(e), nil))
	l.ok("delete many", l.mkt.json(http.MethodDelete, "/api/subscribers?id="+itoa(d), nil))
	l.ok("delete by query", l.mkt.json(http.MethodPost, "/api/subscribers/query/delete", map[string]any{"query": "subscribers.id = " + itoa(c)}))
	l.ok("maintenance delete blocklisted", l.mkt.json(http.MethodDelete, "/api/maintenance/subscribers/blocklisted", nil))
	if l.count(`SELECT COUNT(*) FROM subscribers`) != 0 {
		t.Fatalf("%d subscribers left after the deletions", l.count(`SELECT COUNT(*) FROM subscribers`))
	}
}

// M11 -- the public subscription path already refuses a private list, and every standard list is
// private: pinned for a private locked list.
func TestLockPublicSubscribeRefusesLocked(t *testing.T) {
	l := newLockFixture(t)
	l.h.app.cfg.EnablePublicSubPage = true
	var uu string
	l.h.db.Get(&uu, `SELECT uuid::TEXT FROM lists WHERE id = $1`, l.fo)
	before := l.state()
	rec := l.h.client().json(http.MethodPost, "/api/public/subscription", map[string]any{"email": "pub@example.test", "name": "Pub", "list_uuids": []string{uu}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("public subscribe onto a private locked list: %d %s", rec.Code, rec.Body.String())
	}
	if got := l.state(); got != before {
		t.Fatal("a refused public subscribe wrote rows")
	}
}

// System writers -- M1 and M4 as a user whose role holds lists:journeys answer 200.
func TestLockSystemRoleAllowed(t *testing.T) {
	l := newLockFixture(t)
	other := l.sub("other@example.test", l.plain)
	l.ok("system create on the first-order list", l.sys.json(http.MethodPost, "/api/subscribers",
		map[string]any{"email": "buyer@example.test", "name": "Buyer", "lists": []int{l.fo}, "preconfirm_subscriptions": true}))
	l.ok("system add to a step list", l.sys.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{other}, "action": "add", "target_list_ids": []int{l.step}, "status": "confirmed"}))
	l.ok("system add to a queue", l.sys.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{other}, "action": "add", "target_list_ids": []int{l.queue}, "status": "confirmed"}))
	if l.member(other, l.step) != "confirmed" || l.member(other, l.queue) != "confirmed" {
		t.Fatal("system adds not applied")
	}
}

// I14 -- journey_locked on every list response equals isJourneyLockedList.
func TestJourneyLockedField(t *testing.T) {
	l := newLockFixture(t)
	want := map[int]bool{l.fo: true, l.step: true, l.post: true, l.queue: true, l.sku: false, l.plain: false}
	check := func(what string, raw json.RawMessage) {
		t.Helper()
		var lists []struct {
			ID     int  `json:"id"`
			Locked *bool `json:"journey_locked"`
		}
		var page struct {
			Results json.RawMessage `json:"results"`
		}
		if json.Unmarshal(raw, &page) == nil && len(page.Results) > 0 {
			raw = page.Results
		}
		if err := json.Unmarshal(raw, &lists); err != nil {
			var one struct {
				ID     int   `json:"id"`
				Locked *bool `json:"journey_locked"`
			}
			if err := json.Unmarshal(raw, &one); err != nil {
				t.Fatalf("%s: %v", what, err)
			}
			lists = append(lists, struct {
				ID     int  `json:"id"`
				Locked *bool `json:"journey_locked"`
			}{one.ID, one.Locked})
		}
		seen := 0
		for _, x := range lists {
			w, ok := want[x.ID]
			if !ok {
				continue
			}
			seen++
			if x.Locked == nil || *x.Locked != w {
				t.Fatalf("%s: list %d journey_locked %v, want %v", what, x.ID, x.Locked, w)
			}
			// One definition: the field equals the helper on the stored list.
			stored, err := l.h.app.core.GetList(x.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			if isJourneyLockedList(stored) != w {
				t.Fatalf("%s: isJourneyLockedList(%d) disagrees", what, x.ID)
			}
		}
		if seen == 0 {
			t.Fatalf("%s: no fixture list in the response", what)
		}
	}
	check("GET /api/lists", l.ok("full", l.mkt.json(http.MethodGet, "/api/lists?per_page=all", nil)))
	check("GET /api/lists minimal", l.ok("minimal", l.mkt.json(http.MethodGet, "/api/lists?minimal=true&per_page=all", nil)))
	for id := range want {
		check("GET /api/lists/"+itoa(id), l.ok("one", l.mkt.json(http.MethodGet, "/api/lists/"+itoa(id), nil)))
	}
}
