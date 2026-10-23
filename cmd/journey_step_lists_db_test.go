package main

// Fork (journey form UX) -- integrations JOURNEY-UX-SPEC IA1, IA2 and IA6 (Amendment A1.1/A1.1b),
// against a real database through the real router (bpFixture, brands_db_test.go):
//   - IA1: a SESSION write that would add a membership row to a no_purchase step list is refused
//     on every named path -- create, PUT, PATCH, the ids and by-query manage endpoints (add only)
//     and both import entries in any mode -- while an API token passes, and a round-trip, a
//     removal, an unsubscribe and a hold stay allowed to a session; A1.1b: type public is refused
//     on a no_purchase list from either side of the pair.
//   - IA2: a refused write leaves every subscriber and membership row as it was.
//   - IA6: the step list is private and absent from GET /api/public/lists.
// LISTMONK_TEST_PG opt-in. Fixture names are acme.test / example.test only.

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/subimporter"
	"github.com/knadh/listmonk/models"
)

type stepFixture struct {
	*bpFixture
	step, plain int
	token       *twofaClient // an API user with the Super Admin role
}

func newStepFixture(t *testing.T) *stepFixture {
	t.Helper()
	f, _, _ := newPersonaFixture(t)
	s := &stepFixture{bpFixture: f}
	s.step = f.sqlList("Acme step", "brand:acme", "from:"+pfBrandFrom)
	f.juTrigger(s.step, juNoPurchase)
	s.plain = f.sqlList("Acme news", "brand:acme", "from:"+pfBrandFrom)

	u, err := f.h.app.core.CreateUser(auth.User{Type: auth.UserTypeAPI, Username: "sweepbot", Name: "sweepbot",
		UserRoleID: auth.SuperAdminRoleID, Status: auth.UserStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cacheUsers(f.h.app.core, f.h.app.auth); err != nil {
		t.Fatal(err)
	}
	s.token = f.h.client()
	s.token.hdr["Authorization"] = "token sweepbot:" + u.Password.String
	return s
}

// sub inserts a subscriber directly, confirmed on the given lists.
func (s *stepFixture) sub(email string, lists ...int) int {
	s.h.t.Helper()
	var id int
	if err := s.h.db.Get(&id, `INSERT INTO subscribers (uuid, email, name, status) VALUES (gen_random_uuid(), $1, 'Orig', 'enabled') RETURNING id`, email); err != nil {
		s.h.t.Fatal(err)
	}
	for _, l := range lists {
		s.h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, id, l)
	}
	return id
}

// state is every subscriber and membership row, for the refused-writes-nothing check (IA2).
func (s *stepFixture) state() string {
	s.h.t.Helper()
	var rows []string
	if err := s.h.db.Select(&rows, `SELECT id || ':' || email || ':' || name || ':' || status FROM subscribers ORDER BY id`); err != nil {
		s.h.t.Fatal(err)
	}
	var subs []string
	if err := s.h.db.Select(&subs, `SELECT subscriber_id || '/' || list_id || '/' || status || '/' || meta::TEXT FROM subscriber_lists ORDER BY subscriber_id, list_id`); err != nil {
		s.h.t.Fatal(err)
	}
	return strings.Join(rows, ",") + "|" + strings.Join(subs, ",")
}

func (s *stepFixture) refused(what string, rec *httptest.ResponseRecorder) {
	s.h.t.Helper()
	s.wantErr(what, rec, http.StatusBadRequest, "lists.noPurchaseManualAdd", "name", "Acme step")
}

func (s *stepFixture) member(sub, list int) string {
	s.h.t.Helper()
	var st []string
	if err := s.h.db.Select(&st, `SELECT status::TEXT FROM subscriber_lists WHERE subscriber_id = $1 AND list_id = $2`, sub, list); err != nil {
		s.h.t.Fatal(err)
	}
	if len(st) == 0 {
		return "<none>"
	}
	return st[0]
}

// importForm is a stock import request: params, and a CSV file unless file is "".
func importForm(cl *twofaClient, params map[string]any, file string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	p, _ := json.Marshal(params)
	w.WriteField("params", string(p))
	if file != "" {
		fw, _ := w.CreateFormFile("file", "subs.csv")
		fw.Write([]byte(file))
	}
	w.Close()
	return cl.do(http.MethodPost, "/api/import/subscribers", w.FormDataContentType(), buf.String())
}

// presetForm is the preset confirm request: the file under name, and its content hash.
func presetForm(cl *twofaClient, key, name string, data []byte) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", name)
	fw.Write(data)
	w.WriteField("content_hash", subimporter.ContentHash(data))
	w.Close()
	return cl.do(http.MethodPost, "/api/import/subscribers/preset/"+key, w.FormDataContentType(), buf.String())
}

// IA1 + IA2 -- every session path that would add a step-list row is refused and writes nothing.
func TestJourneyStepListSessionAddRefused(t *testing.T) {
	s := newStepFixture(t)
	other := s.sub("other@example.test", s.plain)
	onStep := s.sub("onstep@example.test", s.plain, s.step)
	before := s.state()
	check := func(what string, rec *httptest.ResponseRecorder) {
		t.Helper()
		s.refused(what, rec)
		if got := s.state(); got != before {
			t.Fatalf("%s: a refused write changed the rows:\n%s\n->\n%s", what, before, got)
		}
	}

	check("POST subscriber on the step list", s.admin.json(http.MethodPost, "/api/subscribers",
		map[string]any{"email": "new@example.test", "name": "New", "lists": []int{s.plain, s.step}}))

	check("PUT adding the step list", s.admin.json(http.MethodPut, "/api/subscribers/"+itoa(other),
		map[string]any{"email": "other@example.test", "name": "Changed", "status": "enabled", "lists": []int{s.plain, s.step}}))

	check("PATCH adding the step list", s.admin.json(http.MethodPatch, "/api/subscribers/"+itoa(other),
		map[string]any{"name": "Changed", "lists": []int{s.plain, s.step}}))

	check("manage add", s.admin.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{other}, "action": "add", "target_list_ids": []int{s.step}, "status": "confirmed"}))
	// One subscriber already on the list does not cover the one who is not.
	check("manage add, mixed", s.admin.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{onStep, other}, "action": "add", "target_list_ids": []int{s.plain, s.step}, "status": "confirmed"}))
	check("manage add by id route", s.admin.json(http.MethodPut, "/api/subscribers/lists/"+itoa(other),
		map[string]any{"ids": []int{other}, "action": "add", "target_list_ids": []int{s.step}, "status": "confirmed"}))

	check("by-query add", s.admin.json(http.MethodPut, "/api/subscribers/query/lists",
		map[string]any{"query": "", "action": "add", "target_list_ids": []int{s.step}, "status": "confirmed"}))

	// Both import entries, any mode; the stock entry refuses before it reads the file.
	for _, mode := range []string{subimporter.ModeSubscribe, subimporter.ModeBlocklist} {
		check("import "+mode, importForm(s.admin, map[string]any{"mode": mode, "lists": []int{s.step}, "delim": ","},
			"email,name\nimp@example.test,Imp\n"))
	}

	ps, err := subimporter.ParsePresets([]byte(`[{"key":"k","name":"N","columns":{"email":"e"},`+
		`"list_name_pattern":"^(?P<d>.+)\\.csv$","list_name_template":"{d}","merge":"fill"}]`), models.CampaignLangs)
	if err != nil {
		t.Fatal(err)
	}
	s.h.app.importPresets = ps
	s.h.app.db = s.h.db
	check("preset import onto the step list", presetForm(s.admin, "k", "Acme step.csv", []byte("e\npre@example.test\n")))
	if st := s.h.app.importer.GetStats().Status; st == subimporter.StatusImporting {
		t.Fatal("a refused preset import started a session")
	}
	if n := s.count(`SELECT COUNT(*) FROM lists WHERE name = 'Acme step'`); n != 1 {
		t.Fatalf("%d lists named Acme step", n)
	}
}

// IA1 -- what stays open: a session's round-trip of an existing step membership, a removal, an
// unsubscribe and a hold; an add whose every row exists; and every add through an API token.
func TestJourneyStepListAllowed(t *testing.T) {
	s := newStepFixture(t)
	m := s.sub("member@example.test", s.plain, s.step)
	other := s.sub("other@example.test", s.plain)

	// Round-trip with an unrelated edit: the name changes, the step row is untouched.
	s.ok("PUT round-trip", s.admin.json(http.MethodPut, "/api/subscribers/"+itoa(m),
		map[string]any{"email": "member@example.test", "name": "Renamed", "status": "enabled", "lists": []int{s.plain, s.step}}))
	if s.member(m, s.step) != "confirmed" || s.count(`SELECT COUNT(*) FROM subscribers WHERE id = $1 AND name = 'Renamed'`, m) != 1 {
		t.Fatal("round-trip did not keep the step row or apply the name")
	}
	s.ok("PATCH round-trip", s.admin.json(http.MethodPatch, "/api/subscribers/"+itoa(m),
		map[string]any{"lists": []int{s.plain, s.step}}))
	s.ok("manage add, every row exists", s.admin.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{m}, "action": "add", "target_list_ids": []int{s.step}, "status": "confirmed"}))

	// Cleanup: unsubscribe, hold, remove (manage), and removal through PUT.
	s.ok("manage unsubscribe", s.admin.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{m}, "action": "unsubscribe", "target_list_ids": []int{s.step}}))
	if got := s.member(m, s.step); got != "unsubscribed" {
		t.Fatalf("unsubscribe left %s", got)
	}
	h := s.sub("held@example.test", s.step)
	s.ok("manage hold", s.admin.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{h}, "action": "hold", "target_list_ids": []int{s.step}, "hold": map[string]any{"reason": "manual", "source": "test"}}))
	s.ok("by-query unsubscribe", s.admin.json(http.MethodPut, "/api/subscribers/query/lists",
		map[string]any{"query": "", "action": "unsubscribe", "target_list_ids": []int{s.step}}))
	s.ok("manage remove", s.admin.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{h}, "action": "remove", "target_list_ids": []int{s.step}}))
	if got := s.member(h, s.step); got != "<none>" {
		t.Fatalf("remove left %s", got)
	}
	s.ok("PUT removing the step list", s.admin.json(http.MethodPut, "/api/subscribers/"+itoa(m),
		map[string]any{"email": "member@example.test", "name": "Renamed", "status": "enabled", "lists": []int{s.plain}}))
	if got := s.member(m, s.step); got != "<none>" {
		t.Fatalf("PUT removal left %s", got)
	}
	// A session add to a trigger-less list is untouched by the rule.
	s.ok("manage add to a plain list", s.admin.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{m}, "action": "add", "target_list_ids": []int{s.plain}, "status": "confirmed"}))

	// The API token adds on every path.
	s.ok("token POST", s.token.json(http.MethodPost, "/api/subscribers",
		map[string]any{"email": "tok@example.test", "name": "Tok", "lists": []int{s.step}, "preconfirm_subscriptions": true}))
	s.ok("token manage add", s.token.json(http.MethodPut, "/api/subscribers/lists",
		map[string]any{"ids": []int{other}, "action": "add", "target_list_ids": []int{s.step}, "status": "confirmed"}))
	if got := s.member(other, s.step); got != "confirmed" {
		t.Fatalf("token add left %s", got)
	}
	s.ok("token PATCH", s.token.json(http.MethodPatch, "/api/subscribers/"+itoa(m),
		map[string]any{"lists": []int{s.plain, s.step}}))
	s.ok("token by-query add", s.token.json(http.MethodPut, "/api/subscribers/query/lists",
		map[string]any{"query": "", "action": "add", "target_list_ids": []int{s.step}, "status": "confirmed"}))
	// The token's import passes the step-list check and reaches the next one (no file sent).
	rec := importForm(s.token, map[string]any{"mode": "subscribe", "lists": []int{s.step}, "delim": ","}, "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(message(rec), "file") || strings.Contains(message(rec), "Acme step") {
		t.Fatalf("token import: want the missing-file refusal, got %d %s", rec.Code, rec.Body.String())
	}
}

// IA1 (A1.1b) + IA6 -- a no_purchase step list is never public, from either side of the pair, and
// is absent from the public lists endpoint.
func TestJourneyStepListNeverPublic(t *testing.T) {
	s := newStepFixture(t)
	listType := func(id int) string {
		var typ string
		if err := s.h.db.Get(&typ, `SELECT type::TEXT FROM lists WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		return typ
	}

	s.wantErr("type public on the step list", s.admin.json(http.MethodPut, "/api/lists/"+itoa(s.step),
		map[string]any{"name": "Acme step", "type": "public"}), http.StatusBadRequest, "lists.noPurchasePublic")
	if listType(s.step) != models.ListTypePrivate {
		t.Fatal("refused write made the step list public")
	}
	s.wantErr("type public + no_purchase together", s.admin.json(http.MethodPut, "/api/lists/"+itoa(s.plain),
		map[string]any{"name": "Acme news", "type": "public", "join_trigger": json.RawMessage(juNoPurchase)}),
		http.StatusBadRequest, "lists.noPurchasePublic")
	if listType(s.plain) != models.ListTypePrivate || s.juStoredTrigger(s.plain) != "<nil>" {
		t.Fatal("refused write changed the plain list")
	}
	// A public list cannot take the trigger either.
	s.h.db.MustExec(`UPDATE lists SET type = 'public' WHERE id = $1`, s.plain)
	s.wantErr("no_purchase on a public list", s.juPutList(s.plain, "Acme news", juNoPurchase),
		http.StatusBadRequest, "lists.noPurchasePublic")
	if s.juStoredTrigger(s.plain) != "<nil>" {
		t.Fatal("refused write stored the trigger")
	}
	// The token is held to it too: the rule is the list's, not the actor's.
	s.wantErr("token: type public on the step list", s.token.json(http.MethodPut, "/api/lists/"+itoa(s.step),
		map[string]any{"name": "Acme step", "type": "public"}), http.StatusBadRequest, "lists.noPurchasePublic")

	// Still allowed: a private save of the step list, a public list with a purchase trigger.
	s.ok("private save of the step list", s.admin.json(http.MethodPut, "/api/lists/"+itoa(s.step),
		map[string]any{"name": "Acme step", "type": "private"}))
	s.ok("first_order on a public list", s.juPutList(s.plain, "Acme news", juFirstOrder))

	// IA6 -- the public lists endpoint carries the public list and never the step list.
	var uuid string
	if err := s.h.db.Get(&uuid, `SELECT uuid::TEXT FROM lists WHERE id = $1`, s.step); err != nil {
		t.Fatal(err)
	}
	rec := s.h.client().do(http.MethodGet, "/api/public/lists", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("public lists: %d %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, uuid) || !strings.Contains(body, "Acme news") {
		t.Fatalf("public lists: %s", body)
	}
}
