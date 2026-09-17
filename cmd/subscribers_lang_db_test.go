package main

// Fork (list grid) -- integrations LIST-GRID-SPEC D13/I10 against a real database: an incoming
// subscriber attribs.lang is normalized or refused at the admin/API edge, judged against the
// stored value; a legacy row stays editable; the public paths are untouched; a bulk import drops
// the key and counts the row. Shares newLinkHarness (LISTMONK_TEST_PG opt-in).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/subimporter"
	"github.com/labstack/echo/v4"
)

var subAdmin = auth.User{PermissionsMap: map[string]struct{}{
	auth.PermListGetAll: {}, auth.PermListManageAll: {}, auth.PermSubscribersGetAll: {}, auth.PermSubscribersManage: {},
}}

// subReq runs a subscriber write handler as an admin. id 0 = create.
func subReq(t *testing.T, h *linkHarness, handler echo.HandlerFunc, method string, id int, body string) (int, string) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, "/api/subscribers", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.UserHTTPCtxKey, subAdmin)
	if id > 0 {
		c.Set("id", id)
	}
	if err := handler(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("%s: %v", method, err)
	}
	return rec.Code, rec.Body.String()
}

func subAttribs(t *testing.T, h *linkHarness, email string) (int, map[string]any) {
	t.Helper()
	var (
		id  int
		raw string
	)
	if err := h.db.QueryRow(`SELECT id, attribs::TEXT FROM subscribers WHERE email = $1`, email).Scan(&id, &raw); err != nil {
		t.Fatalf("subscriber %s: %v", email, err)
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return id, out
}

// TestSubscriberLangAPI -- I10, the API half.
func TestSubscriberLangAPI(t *testing.T) {
	h := newLinkHarness(t)
	var list int
	var listUUID string
	h.db.QueryRow(`INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'single') RETURNING id, uuid`).Scan(&list, &listUUID)
	mk := func(email, attribs string) string {
		return fmt.Sprintf(`{"email": %q, "name": "N", "status": "enabled", "lists": [%d], "attribs": %s}`, email, list, attribs)
	}
	count := func() int {
		var n int
		h.db.Get(&n, `SELECT COUNT(*) FROM subscribers`)
		return n
	}

	// CREATE: an invalid language is a 400 naming it and NOTHING is written; a valid one is
	// stored normalized; none is none.
	n := count()
	for _, bad := range []string{`"pt"`, `"estonian"`, `"eng"`, `5`, `["fr"]`} {
		code, msg := subReq(t, h, h.app.CreateSubscriber, http.MethodPost, 0, mk("bad@x.test", `{"lang": `+bad+`}`))
		if code != http.StatusBadRequest {
			t.Fatalf("create with lang %s: want 400, got %d %s", bad, code, msg)
		}
	}
	if _, msg := subReq(t, h, h.app.CreateSubscriber, http.MethodPost, 0, mk("bad@x.test", `{"lang": "pt"}`)); !strings.Contains(msg, "pt") {
		t.Fatalf("the 400 must name the value, got %q", msg)
	}
	if count() != n {
		t.Fatal("a refused create wrote a subscriber")
	}
	for email, c := range map[string]struct{ in, want string }{
		"ca@x.test":    {`{"lang": "fr-CA", "city": "x"}`, "fr"},
		"upper@x.test": {`{"lang": "FR"}`, "fr"},
		"empty@x.test": {`{"lang": "", "city": "x"}`, ""},
		"none@x.test":  {`{"city": "x"}`, ""},
	} {
		if code, msg := subReq(t, h, h.app.CreateSubscriber, http.MethodPost, 0, mk(email, c.in)); code != http.StatusOK {
			t.Fatalf("create %s: %d %s", email, code, msg)
		}
		_, at := subAttribs(t, h, email)
		if got, present := at["lang"]; (c.want == "" && present) || (c.want != "" && got != c.want) {
			t.Fatalf("create %s: stored attribs %v, want lang %q", email, at, c.want)
		}
	}

	// A LEGACY row: pt, written past the API (a pre-rule import).
	var legacy int
	var legacyUUID string
	h.db.QueryRow(`INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 'legacy@x.test', 'Old Name', '{"lang": "pt", "city": "lisboa"}') RETURNING id, uuid`).Scan(&legacy, &legacyUUID)
	h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, legacy, list)

	// PATCH name alone -> 200, pt untouched (review H3: the merged attribs hold the STORED pt).
	if code, msg := subReq(t, h, h.app.PatchSubscriber, http.MethodPatch, legacy, `{"name": "New Name"}`); code != http.StatusOK {
		t.Fatalf("PATCH name on a legacy pt row: want 200, got %d %s", code, msg)
	}
	// PATCH another attrib (what HandleListmonkSync's merge does) -> 200.
	if code, msg := subReq(t, h, h.app.PatchSubscriber, http.MethodPatch, legacy, `{"attribs": {"site": "s"}}`); code != http.StatusOK {
		t.Fatalf("PATCH an unrelated attrib on a legacy pt row: want 200, got %d %s", code, msg)
	}
	// PATCH resending the SAME stored value -> 200 (unchanged is never judged).
	if code, msg := subReq(t, h, h.app.PatchSubscriber, http.MethodPatch, legacy, `{"attribs": {"lang": "pt"}}`); code != http.StatusOK {
		t.Fatalf("PATCH resending the stored pt: want 200, got %d %s", code, msg)
	}
	// PUT of the whole object, as the edit form sends it -> 200.
	put := fmt.Sprintf(`{"email": "legacy@x.test", "name": "Put Name", "status": "enabled", "lists": [%d], "attribs": {"lang": "pt", "city": "porto"}}`, list)
	if code, msg := subReq(t, h, h.app.UpdateSubscriber, http.MethodPut, legacy, put); code != http.StatusOK {
		t.Fatalf("PUT resending the stored pt: want 200, got %d %s", code, msg)
	}
	if _, at := subAttribs(t, h, "legacy@x.test"); at["lang"] != "pt" || at["city"] != "porto" {
		t.Fatalf("legacy row after the unrelated writes: %v", at)
	}

	// A CHANGED invalid value -> 400 and nothing written, on PATCH and on PUT.
	if code, _ := subReq(t, h, h.app.PatchSubscriber, http.MethodPatch, legacy, `{"name": "X", "attribs": {"lang": "nl"}}`); code != http.StatusBadRequest {
		t.Fatalf("PATCH a new invalid lang: want 400, got %d", code)
	}
	if code, _ := subReq(t, h, h.app.UpdateSubscriber, http.MethodPut, legacy, strings.Replace(put, `"pt"`, `"nl"`, 1)); code != http.StatusBadRequest {
		t.Fatalf("PUT a new invalid lang: want 400, got %d", code)
	}
	var name string
	h.db.Get(&name, `SELECT name FROM subscribers WHERE id = $1`, legacy)
	if _, at := subAttribs(t, h, "legacy@x.test"); at["lang"] != "pt" || name != "Put Name" {
		t.Fatalf("a refused write changed the row: name %q attribs %v", name, at)
	}

	// The PUBLIC paths on that row are unaffected: manage-preferences (resends stored attribs
	// through core.UpdateSubscriber) and the campaign unsubscribe.
	h.app.cfg.Privacy.AllowPreferences = true
	var campUUID string
	h.db.Get(&campUUID, `SELECT uuid FROM campaigns WHERE id = $1`, newLangCampaign(t, h, list, nil).ID)
	public := func(form url.Values) int {
		e := echo.New()
		e.Renderer = stubRenderer{}
		req := httptest.NewRequest(http.MethodPost, "/subscription/"+campUUID+"/"+legacyUUID, strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("campUUID", "subUUID")
		c.SetParamValues(campUUID, legacyUUID)
		if err := h.app.SubscriptionPrefs(c); err != nil {
			t.Fatalf("SubscriptionPrefs: %v", err)
		}
		return rec.Code
	}
	if code := public(url.Values{"manage": {"true"}, "name": {"Public Name"}, "l": {listUUID}}); code != http.StatusOK {
		t.Fatalf("public manage-preferences on a legacy pt row: %d", code)
	}
	h.db.Get(&name, `SELECT name FROM subscribers WHERE id = $1`, legacy)
	if _, at := subAttribs(t, h, "legacy@x.test"); name != "Public Name" || at["lang"] != "pt" {
		t.Fatalf("public manage-preferences: name %q attribs %v", name, at)
	}
	if code := public(url.Values{}); code != http.StatusOK {
		t.Fatalf("public unsubscribe on a legacy pt row: %d", code)
	}
	var status string
	h.db.Get(&status, `SELECT status FROM subscriber_lists WHERE subscriber_id = $1 AND list_id = $2`, legacy, list)
	if status != "unsubscribed" {
		t.Fatalf("public unsubscribe: status %s", status)
	}

	// A legacy row CAN be repaired: a valid new value is accepted and normalized.
	if code, msg := subReq(t, h, h.app.PatchSubscriber, http.MethodPatch, legacy, `{"attribs": {"lang": "ES-mx"}}`); code != http.StatusOK {
		t.Fatalf("PATCH repair: %d %s", code, msg)
	}
	if _, at := subAttribs(t, h, "legacy@x.test"); at["lang"] != "es" {
		t.Fatalf("PATCH repair: %v", at)
	}
}

// TestSubscriberLangImport -- I10, the import half. A CSV row with pt lands WITHOUT a lang key
// and is counted; a normalizable one is normalized; nothing is rejected.
func TestSubscriberLangImport(t *testing.T) {
	h := newLinkHarness(t)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'single') RETURNING id`)

	csv := "email,name,attributes\n" +
		`pt@imp.test,P,"{""lang"": ""pt"", ""city"": ""lisboa""}"` + "\n" +
		`num@imp.test,N,"{""lang"": 7}"` + "\n" +
		`ca@imp.test,C,"{""lang"": ""fr-CA""}"` + "\n" +
		`en@imp.test,E,"{""lang"": ""en""}"` + "\n" +
		`none@imp.test,Z,"{""city"": ""x""}"` + "\n" +
		"bare@imp.test,B,\n"
	path := filepath.Join(t.TempDir(), "subs.csv")
	if err := os.WriteFile(path, []byte(csv), 0o600); err != nil {
		t.Fatal(err)
	}

	sess, err := h.app.importer.NewSession(subimporter.SessionOpt{
		Filename: "subs.csv", Mode: subimporter.ModeSubscribe, SubStatus: "confirmed",
		Overwrite: true, OverwriteUserInfo: true, Delim: ",", ListIDs: []int{list},
	})
	if err != nil {
		t.Fatal(err)
	}
	go sess.Start()
	if err := sess.LoadCSV(path, ','); err != nil {
		t.Fatalf("LoadCSV: %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for h.app.importer.GetStats().Status == subimporter.StatusImporting {
		if time.Now().After(deadline) {
			t.Fatal("import did not finish")
		}
		time.Sleep(50 * time.Millisecond)
	}

	st := h.app.importer.GetStats()
	if st.Status != subimporter.StatusFinished || st.Imported != 6 {
		t.Fatalf("import: %+v\n%s", st, h.app.importer.GetLogs())
	}
	if st.LangDropped != 2 {
		t.Fatalf("lang_dropped = %d, want 2 (pt and the non-string)", st.LangDropped)
	}
	b, _ := json.Marshal(st)
	if !strings.Contains(string(b), `"lang_dropped":2`) {
		t.Fatalf("status JSON: %s", b)
	}
	if logs := string(h.app.importer.GetLogs()); !strings.Contains(logs, "pt@imp.test") || !strings.Contains(logs, "num@imp.test") {
		t.Fatalf("the log must name the offenders:\n%s", logs)
	}

	for email, want := range map[string]string{
		"pt@imp.test": "", "num@imp.test": "", "ca@imp.test": "fr", "en@imp.test": "en", "none@imp.test": "", "bare@imp.test": "",
	} {
		_, at := subAttribs(t, h, email)
		if got, present := at["lang"]; (want == "" && present) || (want != "" && got != want) {
			t.Fatalf("%s: attribs %v, want lang %q", email, at, want)
		}
	}
	if _, at := subAttribs(t, h, "pt@imp.test"); at["city"] != "lisboa" {
		t.Fatalf("dropping the language must keep the other attribs: %v", at)
	}
}
