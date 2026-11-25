package main

// Fork (SKU journey live count) -- integrations SKU-LIVE-COUNT-SPEC I6, I9, I9a (no database): the
// POST /api/lists/sku-count handler against an httptest Lambda with static AWS credentials (the
// sms_test.go fake) and in-memory list/brand lookups.
//   I6  -- the typed re-marshal drops an injected address field.
//   I9  -- list_id > 0 needs manage on THAT list (403 otherwise) and forwards the LIST's brand, never
//          the body's; list_id absent/0 needs lists:manage_all and an existing brand; HasListPerm is
//          never called with 0 (a user with no permission at all is refused for list_id 0).
//   I9a -- unconfigured -> 503; Lambda 400 -> 400 with its text cut at 200; Lambda 500 and a
//          transport error -> 502 with no cause in the body.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// skuCountReply is a well-formed Lambda answer.
const skuCountReply = `{"as_of":"2026-11-25T14:00:00.000Z","next_run_at":"2026-11-25T15:10:00.000Z",` +
	`"automations_running":0,"assumed_all_languages":true,` +
	`"skus":[{"sku":"joybelle-brushonglue","brand_sku":true,"buyers":1425,"suggestions":[]},` +
	`{"sku":"jb-11009-01","brand_sku":false,"buyers":null,"suggestions":[{"sku":"JoyBelle-BrushOnGlue","brand_sku":"JB-11009-01","title":"Brush-On Nail Glue"}]}],` +
	`"window":{"buyers":1425,"eligible":168,"skips":{"consent":1200,"outside-window":0},"consent":{"not-subscribed":1100}},` +
	`"due":{"days":90,"joins":5,"by_age":[{"from_days":90,"to_days":120,"joins":5}],"by_lang":{"en":5},"skips":{"not-due":160},"consent":{}}}`

type fakeSkuLookups struct {
	lists  map[int]models.List
	brands map[string]bool
	asked  []int
}

func (f *fakeSkuLookups) GetList(id int) (models.List, error) {
	f.asked = append(f.asked, id)
	l, ok := f.lists[id]
	if !ok {
		return models.List{}, echo.NewHTTPError(http.StatusBadRequest, "list not found")
	}
	return l, nil
}

func (f *fakeSkuLookups) BrandExists(slug string) (bool, error) { return f.brands[slug], nil }

func newFakeSkuLookups() *fakeSkuLookups {
	return &fakeSkuLookups{
		lists:  map[int]models.List{106: {Base: models.Base{ID: 106}, Brand: "joybelle"}, 107: {Base: models.Base{ID: 107}}},
		brands: map[string]bool{"joybelle": true, "ruze": true},
	}
}

func skuTestApp(t *testing.T, f *smsFake) *App {
	t.Helper()
	a, _ := smsTestApp(t, nil, nil)
	if f != nil {
		s := newSkuCountProxy(f.srv.URL+"/", smsTestRegion, smsTestCreds)
		s.now = func() time.Time { return smsTestNow }
		a.skuCountProxy = s
	}
	return a
}

var (
	skuManager   = auth.User{Username: "jb", PermissionsMap: map[string]struct{}{}, ListPermissionsMap: map[int]map[string]struct{}{106: {auth.PermListGet: {}, auth.PermListManage: {}}}}
	skuGetter    = auth.User{Username: "viewer", PermissionsMap: map[string]struct{}{}, ListPermissionsMap: map[int]map[string]struct{}{106: {auth.PermListGet: {}}}}
	skuAllLists  = auth.User{Username: "boss", PermissionsMap: map[string]struct{}{auth.PermListManageAll: {}}}
	skuNobody    = auth.User{Username: "nobody", PermissionsMap: map[string]struct{}{}}
	skuOtherList = auth.User{Username: "ruze", PermissionsMap: map[string]struct{}{}, ListPermissionsMap: map[int]map[string]struct{}{0: {auth.PermListManage: {}}, 50: {auth.PermListManage: {}}}}
)

func skuCall(t *testing.T, a *App, lk skuCountLookups, user auth.User, body string) (int, string) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/lists/sku-count", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.UserHTTPCtxKey, user)
	if err := a.skuCount(c, lk); err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("handler: %v", err)
	}
	return rec.Code, rec.Body.String()
}

const skuBody106 = `{"list_id":106,"brand":"ruze","kind":"replenish","skus":[" JoyBelle-BrushOnGlue ","joybelle-brushonglue"],"days":90,"extra":"x"}`

func TestSkuCountRoute(t *testing.T) {
	b, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`g\.POST\("/api/lists/sku-count", a\.PostListSkuCount\)`).Match(b) {
		t.Fatal("handlers.go does not register POST /api/lists/sku-count")
	}
}

// I9 -- list_id > 0: manage on that list, the LIST's brand forwarded (the body's ignored), the body
// typed (unknown keys dropped, SKUs normalized, days kept).
func TestSkuCountListPermission(t *testing.T) {
	f := newSMSFake(t)
	f.reply = skuCountReply
	a := skuTestApp(t, f)
	lk := newFakeSkuLookups()

	if code, _ := skuCall(t, a, lk, skuGetter, skuBody106); code != http.StatusForbidden {
		t.Fatalf("get-only user: want 403, got %d", code)
	}
	if code, _ := skuCall(t, a, lk, skuNobody, skuBody106); code != http.StatusForbidden {
		t.Fatalf("no permission: want 403, got %d", code)
	}
	if f.count() != 0 {
		t.Fatal("a refused request reached the Lambda")
	}

	code, out := skuCall(t, a, lk, skuManager, skuBody106)
	if code != http.StatusOK {
		t.Fatalf("manager: want 200, got %d %s", code, out)
	}
	var sent map[string]any
	if err := json.Unmarshal(f.last().body, &sent); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"brand": "joybelle", "list_id": float64(106), "kind": "replenish", "skus": []any{"joybelle-brushonglue"}, "days": float64(90)}
	if fmt.Sprint(sent) != fmt.Sprint(want) {
		t.Fatalf("forwarded %v, want %v", sent, want)
	}
	if f.last().method != http.MethodPost || f.last().path != "/" || !strings.HasPrefix(f.last().header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
		t.Fatalf("not a signed POST to the root: %s %s", f.last().method, f.last().path)
	}

	// manage_all reaches any list; a list with no brand is 400; a missing list is the lookup's error.
	if code, _ := skuCall(t, a, lk, skuAllLists, `{"list_id":107,"kind":"replenish","skus":["a"]}`); code != http.StatusBadRequest {
		t.Fatalf("brandless list: want 400, got %d", code)
	}
	if code, _ := skuCall(t, a, lk, skuAllLists, `{"list_id":999,"kind":"replenish","skus":["a"]}`); code != http.StatusBadRequest {
		t.Fatalf("missing list: want 400, got %d", code)
	}
}

// I9 -- list_id absent or 0: lists:manage_all and an existing brand; HasListPerm is never called with 0.
func TestSkuCountNewListPermission(t *testing.T) {
	f := newSMSFake(t)
	f.reply = skuCountReply
	a := skuTestApp(t, f)
	lk := newFakeSkuLookups()

	for _, body := range []string{
		`{"list_id":0,"brand":"joybelle","kind":"first_sku","skus":["a"]}`,
		`{"brand":"joybelle","kind":"first_sku","skus":["a"]}`,
	} {
		// HasListPerm(.., 0) would pass vacuously for anyone: these users must be refused.
		for _, u := range []auth.User{skuNobody, skuManager, skuOtherList} {
			if code, _ := skuCall(t, a, lk, u, body); code != http.StatusForbidden {
				t.Fatalf("%s, %s: want 403, got %d", u.Username, body, code)
			}
		}
	}
	if code, _ := skuCall(t, a, lk, skuAllLists, `{"list_id":0,"brand":"nope","kind":"first_sku","skus":["a"]}`); code != http.StatusBadRequest {
		t.Fatalf("unknown brand: want 400, got %d", code)
	}
	if code, _ := skuCall(t, a, lk, skuAllLists, `{"list_id":0,"kind":"first_sku","skus":["a"]}`); code != http.StatusBadRequest {
		t.Fatalf("no brand: want 400, got %d", code)
	}
	if f.count() != 0 {
		t.Fatal("a refused request reached the Lambda")
	}
	code, out := skuCall(t, a, lk, skuAllLists, `{"list_id":0,"brand":"joybelle","kind":"first_sku","skus":["a"],"days":null}`)
	if code != http.StatusOK {
		t.Fatalf("manage_all: want 200, got %d %s", code, out)
	}
	if got := string(f.last().body); got != `{"brand":"joybelle","list_id":0,"kind":"first_sku","skus":["a"],"days":null}` {
		t.Fatalf("forwarded %s", got)
	}
	if len(lk.asked) != 0 {
		t.Fatalf("a new-list count read list %v", lk.asked)
	}
}

// The shape checks (hazard 110): kind, SKUs, days; a list_id below 0.
func TestSkuCountShape(t *testing.T) {
	f := newSMSFake(t)
	f.reply = skuCountReply
	a := skuTestApp(t, f)
	lk := newFakeSkuLookups()
	for name, body := range map[string]string{
		"not json":     `nope`,
		"bad kind":     `{"list_id":106,"kind":"skus","skus":["a"]}`,
		"no skus":      `{"list_id":106,"kind":"replenish"}`,
		"empty skus":   `{"list_id":106,"kind":"replenish","skus":[" "]}`,
		"sku number":   `{"list_id":106,"kind":"replenish","skus":[3]}`,
		"days 0":       `{"list_id":106,"kind":"replenish","skus":["a"],"days":0}`,
		"days 366":     `{"list_id":106,"kind":"replenish","skus":["a"],"days":366}`,
		"days string":  `{"list_id":106,"kind":"replenish","skus":["a"],"days":"9"}`,
		"days float":   `{"list_id":106,"kind":"replenish","skus":["a"],"days":9.5}`,
		"negative id":  `{"list_id":-1,"kind":"replenish","skus":["a"]}`,
		"sku too long": `{"list_id":106,"kind":"replenish","skus":["` + strings.Repeat("s", 101) + `"]}`,
	} {
		if code, _ := skuCall(t, a, lk, skuManager, body); code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d", name, code)
		}
	}
	if f.count() != 0 {
		t.Fatal("a refused shape reached the Lambda")
	}
	// 365 is the fork's limit (the Lambda answers its own 400 for days at or over its reach-back).
	if code, _ := skuCall(t, a, lk, skuManager, `{"list_id":106,"kind":"replenish","skus":["a"],"days":365}`); code != http.StatusOK {
		t.Fatalf("days 365: want 200, got %d", code)
	}
}

// I6 -- the typed re-marshal: an address injected anywhere in the Lambda's answer is dropped.
func TestSkuCountReMarshal(t *testing.T) {
	f := newSMSFake(t)
	f.reply = strings.NewReplacer(
		`"as_of":`, `"email":"leak@example.test","subscribers":[{"id":7,"email":"leak@example.test"}],"as_of":`,
		`"buyers":1425,"suggestions"`, `"buyers":1425,"emails":["leak@example.test"],"suggestions"`,
		`"skips":{"consent":1200,`, `"skips":{"leak@example.test":1,"consent":1200,`,
		`"by_lang":{"en":5}`, `"by_lang":{"en":5,"Leak@Example.test":2},"ids":[7]`,
	).Replace(skuCountReply)
	a := skuTestApp(t, f)
	code, out := skuCall(t, a, newFakeSkuLookups(), skuManager, skuBody106)
	if code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", code, out)
	}
	if strings.Contains(out, "@") || strings.Contains(out, "leak") || strings.Contains(out, `"ids"`) || strings.Contains(out, `"subscribers"`) {
		t.Fatalf("an address passed through: %s", out)
	}
	var got struct {
		Data skuCountResp `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	d := got.Data
	if d.Due == nil || d.Due.Joins != 5 || d.Window == nil || d.Window.Eligible != 168 || d.Window.Skips["consent"] != 1200 ||
		len(d.Skus) != 2 || d.Skus[1].Buyers != nil || d.Skus[1].Suggestions[0].BrandSku != "JB-11009-01" || !d.AssumedAllLanguages {
		t.Fatalf("counts lost in the re-marshal: %+v", d)
	}
	// The whole key set of the answer is D1's.
	var m map[string]any
	json.Unmarshal([]byte(out), &m)
	keys := []string{}
	for k := range m["data"].(map[string]any) {
		keys = append(keys, k)
	}
	if len(keys) != 7 {
		t.Fatalf("response keys %v", keys)
	}
}

// I9a -- status mapping.
func TestSkuCountStatusMapping(t *testing.T) {
	lk := newFakeSkuLookups()

	// Unconfigured: 503 (after the permission check).
	if code, _ := skuCall(t, skuTestApp(t, nil), lk, skuManager, skuBody106); code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured: want 503, got %d", code)
	}

	f := newSMSFake(t)
	a := skuTestApp(t, f)

	f.status, f.reply = http.StatusBadRequest, `{"error":"`+strings.Repeat("d", 250)+`"}`
	code, msg := skuCall(t, a, lk, skuManager, skuBody106)
	if code != http.StatusBadRequest || msg != strings.Repeat("d", 200) {
		t.Fatalf("Lambda 400: got %d %q", code, msg)
	}
	f.status, f.reply = http.StatusBadRequest, `{"error":"brand has no newsletter list"}`
	if code, msg := skuCall(t, a, lk, skuManager, skuBody106); code != http.StatusBadRequest || msg != "brand has no newsletter list" {
		t.Fatalf("Lambda 400 text: got %d %q", code, msg)
	}

	f.status, f.reply = http.StatusInternalServerError, `{"error":"secret cause at db-host-1"}`
	code, msg = skuCall(t, a, lk, skuManager, skuBody106)
	if code != http.StatusBadGateway || strings.Contains(msg, "secret") || msg != a.i18n.T("lists.skuCountUnavailable") {
		t.Fatalf("Lambda 500: got %d %q", code, msg)
	}
	f.status, f.reply = http.StatusOK, `not json`
	if code, _ := skuCall(t, a, lk, skuManager, skuBody106); code != http.StatusBadGateway {
		t.Fatalf("undecodable: want 502, got %d", code)
	}

	// A transport error (the server is gone).
	f.srv.Close()
	code, msg = skuCall(t, a, lk, skuManager, skuBody106)
	if code != http.StatusBadGateway || strings.Contains(msg, "127.0.0.1") || msg != a.i18n.T("lists.skuCountUnavailable") {
		t.Fatalf("transport: got %d %q", code, msg)
	}

	if skuCountTimeout != 25*time.Second || newSkuCountProxy("x", "r", nil).client.Timeout != 25*time.Second {
		t.Fatal("the count timeout is 25 s")
	}
}

// D5 -- the URL shape and the boot line (the SMS Admin shape, its own key).
func TestSkuCountURL(t *testing.T) {
	if _, reason := parseLambdaURL("lists.sku_count_url", ""); reason != "lists.sku_count_url is empty" {
		t.Fatalf("empty: %q", reason)
	}
	if region, reason := parseLambdaURL("lists.sku_count_url", "https://abc123.lambda-url.us-west-2.on.aws/"); region != "us-west-2" || reason != "" {
		t.Fatalf("accepted: %q %q", region, reason)
	}
	if _, reason := parseLambdaURL("lists.sku_count_url", "https://example.test/"); !strings.HasPrefix(reason, "lists.sku_count_url is ill-shaped") {
		t.Fatalf("ill-shaped: %q", reason)
	}
}
