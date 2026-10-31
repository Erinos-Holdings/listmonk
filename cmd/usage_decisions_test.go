package main

// Fork (usage decisions) -- integrations USAGE-DECISIONS-SPEC I4/I5/I6/I8 (the queries half)/I9.
// Handler tests with the store faked (no database): canDecide's table, every POST refusal writing
// nothing, the GET and the POST going through the same gate, the Slack notice's text and failure
// tolerance, and the queries files holding no UPDATE/DELETE/TRUNCATE on usage_decisions. The
// trigger half of I8 is internal/migrations/v6_2_29_db_test.go (LISTMONK_TEST_PG).

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/decisions"
	"github.com/labstack/echo/v4"
)

// fakeDecisionStore records appends.
type fakeDecisionStore struct {
	mu      sync.Mutex
	rows    []decisions.Row
	inserts int
}

func (f *fakeDecisionStore) UsageDecisions() ([]decisions.Row, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]decisions.Row(nil), f.rows...), nil
}

func (f *fakeDecisionStore) InsertUsageDecision(r decisions.Row) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inserts++
	r.ID = int64(len(f.rows) + 1)
	f.rows = append(f.rows, r)
	return nil
}

// decisionUser is a user as core.setupUserFields leaves it: the role in UserRole.ID, UserRoleID 0.
func decisionUser(typ string, roleID int, perms ...string) auth.User {
	u := auth.User{Type: typ, Name: "Kaylinn Brophy", Username: "kbrophy"}
	u.ID = 4
	u.UserRole.ID = roleID
	u.PermissionsMap = map[string]struct{}{}
	for _, p := range perms {
		u.PermissionsMap[p] = struct{}{}
	}
	return u
}

var (
	approver   = decisionUser(auth.UserTypeUser, 7, "brands:get", auth.PermBrandsDecide)
	superAdmin = decisionUser(auth.UserTypeUser, auth.SuperAdminRoleID, "brands:get", auth.PermBrandsDecide)
	apiDecider = decisionUser(auth.UserTypeAPI, 7, "brands:get", auth.PermBrandsDecide)
	noPerm     = decisionUser(auth.UserTypeUser, 7, "brands:get", "brands:manage")
)

func decisionsApp(t *testing.T) (*App, *fakeDecisionStore) {
	t.Helper()
	a := testSendApp(t)
	d, err := decisions.Lifecycle()
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeDecisionStore{}
	a.decisionDoc = d
	a.usageDecisions = store
	a.log = log.New(io.Discard, "", 0)
	return a, store
}

// callDecisions runs a handler with the user in the context; an echo.HTTPError is returned as
// its code (the error handler's job in the real router).
func callDecisions(t *testing.T, a *App, u auth.User, method, key, body string) (int, string) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, "/api/usage/decisions/"+key, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.UserHTTPCtxKey, u)
	h := a.GetUsageDecisions
	if method == http.MethodPost {
		c.SetParamNames("key")
		c.SetParamValues(key)
		h = a.PostUsageDecision
	}
	if err := h(c); err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he.Code, he.Message.(string)
		}
		t.Fatalf("handler error: %v", err)
	}
	return rec.Code, rec.Body.String()
}

func TestCanDecide(t *testing.T) {
	both := decisionUser(auth.UserTypeUser, auth.SuperAdminRoleID)
	both.UserRoleID = auth.SuperAdminRoleID
	legacyField := decisionUser(auth.UserTypeUser, 0)
	legacyField.UserRoleID = auth.SuperAdminRoleID
	superNoStored := decisionUser(auth.UserTypeUser, auth.SuperAdminRoleID)
	superAPI := decisionUser(auth.UserTypeAPI, auth.SuperAdminRoleID, auth.PermBrandsDecide)
	cases := []struct {
		name string
		u    auth.User
		want bool
	}{
		{"user role holding brands:decide", approver, true},
		{"API token holding brands:decide", apiDecider, false},
		{"Super Admin (UserRole.ID) holding brands:decide", superAdmin, true},
		{"Super Admin with no stored permission list", superNoStored, true},
		{"Super Admin on both fields", both, true},
		{"Super Admin on UserRoleID only", legacyField, true},
		{"API token on the Super Admin role", superAPI, false},
		{"user role without brands:decide", noPerm, false},
		{"brands:manage is not brands:decide", decisionUser(auth.UserTypeUser, 7, "brands:manage"), false},
	}
	for _, c := range cases {
		if got := canDecide(c.u); got != c.want {
			t.Errorf("%s: canDecide = %v, want %v", c.name, got, c.want)
		}
	}

	// I6: the GET's can_decide and the POST's gate are the same function -- swap it and both
	// paths call it, and the GET reports its answer.
	a, store := decisionsApp(t)
	orig := decideGate
	defer func() { decideGate = orig }()
	var calls []string
	decideGate = func(u auth.User) bool { calls = append(calls, u.Username); return false }
	_, body := callDecisions(t, a, approver, http.MethodGet, "", "")
	if !strings.Contains(body, `"can_decide":false`) {
		t.Fatalf("GET did not report the gate's answer: %s", body)
	}
	code, _ := callDecisions(t, a, approver, http.MethodPost, "np-entry", `{"hash":"x","decision":"accept"}`)
	if code != http.StatusForbidden || len(calls) != 2 || store.inserts != 0 {
		t.Fatalf("gate calls %v, POST code %d, inserts %d", calls, code, store.inserts)
	}
	decideGate = func(u auth.User) bool { return true }
	if _, body := callDecisions(t, a, noPerm, http.MethodGet, "", ""); !strings.Contains(body, `"can_decide":true`) {
		t.Fatalf("GET did not report the gate's answer: %s", body)
	}
	decideGate = orig
	if _, body := callDecisions(t, a, approver, http.MethodGet, "", ""); !strings.Contains(body, `"can_decide":true`) {
		t.Fatalf("approver GET: %s", body)
	}
	if _, body := callDecisions(t, a, superAdmin, http.MethodGet, "", ""); !strings.Contains(body, `"can_decide":true`) {
		t.Fatalf("Super Admin GET: %s", body)
	}
}

// USAGE-REENGAGEMENT-SPEC D-I4: the GET carries the stored tables verbatim, and the sunset-policy
// items are ordinary items (numbered, hashed).
func TestGetCarriesTables(t *testing.T) {
	a, _ := decisionsApp(t)
	code, body := callDecisions(t, a, approver, http.MethodGet, "", "")
	if code != 200 {
		t.Fatalf("GET: %d %s", code, body)
	}
	var resp struct {
		Data decisions.View `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	tb, ok := resp.Data.Tables[decisions.SectionSunsetPolicy]
	want := a.decisionDoc.Tables[decisions.SectionSunsetPolicy]
	if !ok || len(tb.Rows) != 3 || tb.Note != want.Note || tb.Columns[0] != want.Columns[0] || tb.Rows[1][0] != want.Rows[1][0] {
		t.Fatalf("GET tables: %+v", resp.Data.Tables)
	}
	n := 0
	for _, it := range resp.Data.Items {
		if it.Section == decisions.SectionSunsetPolicy {
			n++
			if it.N == nil || it.Hash != decisions.Hash(it.Text) {
				t.Errorf("%s: %+v", it.Key, it)
			}
		}
	}
	if n != 8 || len(resp.Data.Items) != 22 {
		t.Fatalf("%d sunset-policy items of %d", n, len(resp.Data.Items))
	}
}

func TestPostRefusals(t *testing.T) {
	a, store := decisionsApp(t)
	it, _ := a.decisionDoc.Find("np-entry")
	hash := decisions.Hash(it.Text)
	body := func(h, decision string, note any) string {
		m := map[string]any{"hash": h, "decision": decision}
		if note != nil {
			m["note"] = note
		}
		b, _ := json.Marshal(m)
		return string(b)
	}
	long := strings.Repeat("é", 2001)
	rec, _ := a.decisionDoc.Find("both-per-brand")
	recHash := decisions.Hash(rec.Text)
	cases := []struct {
		name string
		u    auth.User
		key  string
		body string
		code int
		msg  string
	}{
		{"API token", apiDecider, "np-entry", body(hash, "accept", nil), 403, a.i18n.T("decisions.apiRefused")},
		// USAGE-REENGAGEMENT-SPEC D-I5 (Stage 2 F11): an API token on the Super Admin role is still refused.
		{"API token on the Super Admin role", decisionUser(auth.UserTypeAPI, auth.SuperAdminRoleID, "brands:get", auth.PermBrandsDecide), "np-entry", body(hash, "accept", nil), 403, a.i18n.T("decisions.apiRefused")},
		// D-I4: a table name is not an item -- the unknown-key 404, nothing written.
		{"table name", approver, "sunset-policy", body(hash, "accept", nil), 404, a.i18n.T("decisions.unknown")},
		{"no permission", noPerm, "np-entry", body(hash, "accept", nil), 403, a.i18n.T("decisions.forbidden")},
		{"unknown key", approver, "np-nope", body(hash, "accept", nil), 404, a.i18n.T("decisions.unknown")},
		{"stale hash", approver, "np-entry", body(strings.Repeat("0", 64), "accept", nil), 409, a.i18n.T("decisions.stale")},
		{"bad decision", approver, "np-entry", body(hash, "defer", nil), 400, a.i18n.T("decisions.badDecision")},
		{"empty decision", approver, "np-entry", body(hash, "", nil), 400, a.i18n.T("decisions.badDecision")},
		{"reject without a note", approver, "np-entry", body(hash, "reject", nil), 400, a.i18n.T("decisions.noteRequired")},
		{"reject with a blank note", approver, "np-entry", body(hash, "reject", " \n\t "), 400, a.i18n.T("decisions.noteRequired")},
		{"accept with a note", approver, "np-entry", body(hash, "accept", "fine"), 400, a.i18n.T("decisions.acceptNote")},
		{"note over 2,000 characters", approver, "np-entry", body(hash, "reject", long), 400, a.i18n.T("decisions.noteTooLong")},
		{"malformed body", approver, "np-entry", `{`, 400, ""},
		// 2026-10-31 polish: a decision outside the item's choices (decisions.Choices). np-entry has
		// no "Recommended:" line (Accept | Reject); both-per-brand has one (Current | Recommended | Reject).
		{"current on an item without a Recommended", approver, "np-entry", body(hash, "current", nil), 400, a.i18n.T("decisions.badDecision")},
		{"recommended on an item without a Recommended", approver, "np-entry", body(hash, "recommended", nil), 400, a.i18n.T("decisions.badDecision")},
		{"accept on an item with a Recommended", approver, "both-per-brand", body(recHash, "accept", nil), 400, a.i18n.T("decisions.badDecision")},
		{"recommended with a note", approver, "both-per-brand", body(recHash, "recommended", "yes"), 400, a.i18n.T("decisions.acceptNote")},
		{"current with a note", approver, "both-per-brand", body(recHash, "current", "yes"), 400, a.i18n.T("decisions.acceptNote")},
	}
	for _, c := range cases {
		code, msg := callDecisions(t, a, c.u, http.MethodPost, c.key, c.body)
		if code != c.code || (c.msg != "" && msg != c.msg) {
			t.Errorf("%s: %d %q, want %d %q", c.name, code, msg, c.code, c.msg)
		}
		if store.inserts != 0 {
			t.Fatalf("%s: a refusal wrote %d rows", c.name, store.inserts)
		}
	}
	for _, k := range []string{"decisions.apiRefused", "decisions.forbidden", "decisions.unknown", "decisions.stale", "decisions.badDecision", "decisions.noteRequired", "decisions.acceptNote", "decisions.noteTooLong"} {
		if a.i18n.T(k) == k {
			t.Errorf("%s is missing from en.json", k)
		}
	}

	// The valid paths: exactly 2,000 characters is allowed; an accept with no note (or an explicit
	// empty one); each appends one row and returns the item's new GET shape.
	code, out := callDecisions(t, a, approver, http.MethodPost, "np-entry", body(hash, "reject", "  "+strings.Repeat("é", 2000)+" "))
	if code != 200 || store.inserts != 1 {
		t.Fatalf("2,000-char reject: %d %s (inserts %d)", code, out, store.inserts)
	}
	if n := store.rows[0].Note; n == nil || *n != strings.Repeat("é", 2000) {
		t.Fatal("the stored note is not the trimmed note")
	}
	if store.rows[0].UserID != 4 || store.rows[0].UserName != "Kaylinn Brophy" || store.rows[0].TextHash != hash || store.rows[0].ItemKey != "np-entry" {
		t.Fatalf("stored row: %+v", store.rows[0])
	}
	code, out = callDecisions(t, a, approver, http.MethodPost, "np-entry", body(hash, "accept", ""))
	if code != 200 || store.inserts != 2 {
		t.Fatalf("accept: %d %s", code, out)
	}
	var resp struct {
		Data decisions.ItemView `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Key != "np-entry" || resp.Data.Current == nil || resp.Data.Current.Decision != "accept" || len(resp.Data.History) != 2 || resp.Data.History[1].Decision != "reject" {
		t.Fatalf("POST response: %+v", resp.Data)
	}
	if store.rows[1].Note != nil {
		t.Fatal("an accept stored a note")
	}
	if !slices.Equal(resp.Data.Choices, []string{decisions.Accept, decisions.Reject}) {
		t.Fatalf("np-entry choices %v", resp.Data.Choices)
	}

	// Current and Recommended on an item with a "Recommended:" line: each appends one row, no note.
	for i, d := range []string{decisions.Current, decisions.Recommended} {
		code, out = callDecisions(t, a, approver, http.MethodPost, "both-per-brand", body(recHash, d, nil))
		if code != 200 || store.inserts != 3+i || store.rows[2+i].Decision != d || store.rows[2+i].Note != nil {
			t.Fatalf("%s: %d %s (inserts %d)", d, code, out, store.inserts)
		}
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(resp.Data.Choices, []string{decisions.Current, decisions.Recommended, decisions.Reject}) || resp.Data.Current.Decision != decisions.Recommended {
		t.Fatalf("both-per-brand response: %+v", resp.Data)
	}
}

// 2026-10-31 polish: the notice's verb for the two new decisions; neither carries a Note line.
func TestSlackNoticeChoiceVerbs(t *testing.T) {
	n := 13
	it := decisions.Item{Key: "both-per-brand", N: &n, Text: "Frequency Risk.\nRecommended: x"}
	for d, verb := range map[string]string{decisions.Current: "kept current on", decisions.Recommended: "chose recommended on"} {
		got := decisionNoticeText("K", d, it, "", "https://email.example")
		want := `Lifecycle decision: K ` + verb + ` #13 "Frequency Risk."` + "\nhttps://email.example/admin/brands"
		if got != want {
			t.Errorf("%s\n got %q\nwant %q", d, got, want)
		}
	}
}

func TestSlackNotice(t *testing.T) {
	var (
		mu     sync.Mutex
		bodies []string
		status = http.StatusOK
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		code := status
		mu.Unlock()
		w.WriteHeader(code)
	}))
	defer srv.Close()

	var logs strings.Builder
	lo := log.New(&logs, "", 0)
	n := newDecisionNotifier(srv.URL, "https://email.example/", lo)
	if n.url == "" {
		t.Fatal("an https URL was refused")
	}
	n.client = srv.Client()

	d, _ := decisions.Lifecycle()
	three, _ := d.Find("np-same-day-double")
	both, _ := d.Find("both-language")
	text := func(i int) string {
		var m map[string]string
		if err := json.Unmarshal([]byte(bodies[i]), &m); err != nil {
			t.Fatalf("body %q: %v", bodies[i], err)
		}
		return m["text"]
	}

	// A reject: the number, the first line cut at 120 runes, the escaped note, the Brands link.
	n.Notify("K <b>&", decisions.Reject, three, "see <!channel> & <https://x|y>")
	n.wg.Wait()
	first := strings.SplitN(three.Text, "\n", 2)[0]
	want := `Lifecycle decision: K &lt;b&gt;&amp; rejected #3 "` + slackEscape(string([]rune(first)[:120])) + `"` +
		"\nNote: see &lt;!channel&gt; &amp; &lt;https://x|y&gt;\nhttps://email.example/admin/brands"
	if got := text(0); got != want {
		t.Fatalf("reject text\n got %q\nwant %q", got, want)
	}

	// An accept on a Both series item (numbered 14 since 2026-10-30): no Note line.
	n.Notify("K", decisions.Accept, both, "")
	n.wg.Wait()
	if got := text(1); got != `Lifecycle decision: K accepted #14 "`+cutRunes(both.Text, 120)+`"`+"\nhttps://email.example/admin/brands" {
		t.Fatalf("accept text %q", got)
	}

	// The cut is by rune: a multibyte first line over 120 runes keeps whole characters.
	long := decisions.Item{Key: "x", N: intPtr(1), Text: strings.Repeat("é", 130) + "\nsecond"}
	if got := decisionNoticeText("K", decisions.Accept, long, "", ""); !strings.Contains(got, `"`+strings.Repeat("é", 120)+`"`) {
		t.Fatalf("rune cut: %q", got)
	}

	// A 500 is logged, never fatal -- and it does not fail the write through the handler.
	mu.Lock()
	status = http.StatusInternalServerError
	mu.Unlock()
	a, store := decisionsApp(t)
	a.decisionNotice = n
	it, _ := a.decisionDoc.Find("np-entry")
	code, _ := callDecisions(t, a, approver, http.MethodPost, "np-entry", `{"hash":"`+decisions.Hash(it.Text)+`","decision":"accept"}`)
	n.wg.Wait()
	if code != 200 || store.inserts != 1 {
		t.Fatalf("a Slack 500 failed the write: %d, inserts %d", code, store.inserts)
	}
	if !strings.Contains(logs.String(), "decisions: slack notice failed") || strings.Contains(logs.String(), srv.URL) {
		t.Fatalf("log %q", logs.String())
	}

	// A transport error (*url.Error, which names the URL) logs only its cause. A webhook's secret
	// is its path (hooks.slack.com/services/<secret>), so the path must never appear; the cause's
	// dial address (the public host) may.
	dead := httptest.NewTLSServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	var dl strings.Builder
	dn := newDecisionNotifier(deadURL+"/services/WEBHOOKSECRET", "https://email.example", log.New(&dl, "", 0))
	dn.Notify("K", decisions.Accept, both, "")
	dn.wg.Wait()
	if !strings.Contains(dl.String(), "decisions: slack notice failed") || strings.Contains(dl.String(), "WEBHOOKSECRET") {
		t.Fatalf("transport-error log leaks the URL or is missing: %q", dl.String())
	}

	// Unset or non-https: no request at all.
	mu.Lock()
	before := len(bodies)
	mu.Unlock()
	for _, raw := range []string{"", "http://" + strings.TrimPrefix(srv.URL, "https://")} {
		var l strings.Builder
		off := newDecisionNotifier(raw, "https://email.example", log.New(&l, "", 0))
		if off.url != "" || !strings.Contains(l.String(), "usage-decision notices are off") {
			t.Fatalf("%q: url %q log %q", raw, off.url, l.String())
		}
		off.client = srv.Client()
		off.Notify("K", decisions.Accept, both, "")
		off.wg.Wait()
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != before {
		t.Fatalf("a disabled notifier posted (%d -> %d)", before, len(bodies))
	}
}

func intPtr(n int) *int { return &n }

// TestNoMutatingQueries -- I8's queries half: the append-only table is touched only by an INSERT
// and a SELECT in queries/*.sql.
func TestNoMutatingQueries(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "queries", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("queries: %v (%d files)", err, len(files))
	}
	comment := regexp.MustCompile(`(?m)--.*$`)
	mutating := regexp.MustCompile(`(?i)\b(UPDATE|DELETE|TRUNCATE|MERGE|DROP|ALTER)\b`)
	seen := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range strings.Split(string(b), "-- name:") {
			sql := comment.ReplaceAllString(q, "")
			if !strings.Contains(sql, "usage_decisions") {
				continue
			}
			seen++
			if m := mutating.FindString(sql); m != "" {
				t.Errorf("%s: a query touching usage_decisions contains %s", filepath.Base(f), m)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("%d queries touch usage_decisions, want 2 (the INSERT and the SELECT)", seen)
	}
}
