package main

// Fork (brand roles) -- integrations listmonk/BRAND-ROLES-SPEC I1, I3, I4, I5, I6, I8 and I11 against
// a real database through the real router (bpFixture: initHTTPHandlers, the session middleware, pm,
// cacheUsers), so the resolution, the permission checks and the route wiring are the production
// ones. LISTMONK_TEST_PG opt-in (newLinkHarness skips without it). Fixture names are acme.test /
// example.test only; the brand slugs are the fixture's own.
//
//   I1  a manage grant gives get+manage on every list of the brand -- one created, one re-branded
//       onto it after the grant included; a list moved off it drops out (core.GetUser and the
//       session's next GET /api/profile).
//   I3  manage beats analytics per list; All brands covers every branded list and no untagged
//       list, and a brand added after the grant.
//   I4  every analytics site includes analytics-level lists (one case per site, each Dashboard site
//       with and without list_id); the picker and matrix hold a draft reached only through
//       analytics lists back until it is started.
//   I5  every non-analytics site omits or refuses them (one case per checkCampaignPerm caller class).
//   I6  D5's refusals and the atomic grant replacement.
//   I8  child rows are never read; get-user, get-users and get-list-roles agree.
//   I11 GET /api/brands/launch.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/lib/pq"
)

const rbFrom = "hello@acme.test"

type rbFixture struct {
	*bpFixture
	acme                                 int // the persona fixture's brand:acme list
	shala, shala2, ruze, ruze2, untagged int
	shalaCamp, shalaDraft                int
	ruzeSent, ruzeDraft, ruzeJourney     int
	shalaSub, ruzeSub                    int
}

func newRBFixture(t *testing.T) *rbFixture {
	t.Helper()
	f, acme, _ := newPersonaFixture(t)
	r := &rbFixture{bpFixture: f, acme: acme}
	for _, b := range []string{"shala", "ruze"} {
		f.brandRow(b, rbFrom, "")
	}
	r.shala = f.sqlList("Shala news", "brand:shala", "from:"+rbFrom)
	r.shala2 = f.sqlList("Shala two", "brand:shala", "from:"+rbFrom)
	r.ruze = f.sqlList("Ruze news", "brand:ruze", "from:"+rbFrom)
	r.ruze2 = f.sqlList("Ruze two", "brand:ruze", "from:"+rbFrom)
	r.untagged = f.sqlList("Seedless", "seed")
	f.h.db.MustExec(`UPDATE lists SET type = 'public' WHERE id = ANY($1)`, pq.Array([]int{r.shala, r.ruze}))

	r.shalaCamp = r.camp("Shala sent", "finished", false, 40, r.shala)
	r.shalaDraft = r.camp("Shala draft", "draft", false, 0, r.shala)
	r.ruzeSent = r.camp("Ruze sent", "finished", false, 50, r.ruze)
	r.ruzeDraft = r.camp("Ruze draft", "draft", false, 0, r.ruze)
	// An automation with send hours in the window but no started_at: the one shape that makes the
	// matrix's started-only rule observable (a draft broadcast never has activity).
	r.ruzeJourney = r.camp("Ruze journey", "draft", true, 0, r.ruze)
	for _, c := range []int{r.shalaCamp, r.ruzeSent, r.ruzeJourney} {
		f.h.db.MustExec(`INSERT INTO campaign_send_hours (campaign_id, hour, n) VALUES ($1, DATE_TRUNC('hour', NOW()), 5)`, c)
	}

	r.shalaSub = r.sub("shala@example.test", r.shala)
	r.ruzeSub = r.sub("ruze@example.test", r.ruze)
	// Views and a click on the Ruze campaign so every Dashboard panel has something of Ruze's.
	f.h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id, country, client) VALUES ($1, $2, 'US', 'apple-mail')`, r.ruzeSent, r.ruzeSub)
	var link int
	f.h.db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://shop.acme.test/') RETURNING id`)
	f.h.db.MustExec(`INSERT INTO link_clicks (campaign_id, link_id, subscriber_id, country, client) VALUES ($1, $2, $3, 'US', 'apple-mail')`, r.ruzeSent, link, r.ruzeSub)
	return r
}

// camp inserts a campaign on one list; any status but draft/scheduled is started.
func (r *rbFixture) camp(name, status string, evergreen bool, sent int, list int) int {
	r.h.t.Helper()
	var id int
	if err := r.h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id, status, evergreen, sent, started_at)
		VALUES (gen_random_uuid(), $1, 's', $2, 'b', 'email', (SELECT id FROM templates WHERE type = 'campaign' ORDER BY id LIMIT 1), $3::campaign_status, $4, $5,
		CASE WHEN $3 IN ('draft', 'scheduled') THEN NULL ELSE NOW() - INTERVAL '1 hour' END) RETURNING id`, name, rbFrom, status, evergreen, sent); err != nil {
		r.h.t.Fatal(err)
	}
	r.h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'x')`, id, list)
	return id
}

func (r *rbFixture) sub(email string, list int) int {
	r.h.t.Helper()
	var id int
	if err := r.h.db.Get(&id, `INSERT INTO subscribers (uuid, email, name, status) VALUES (gen_random_uuid(), $1, 'S', 'enabled') RETURNING id`, email); err != nil {
		r.h.t.Fatal(err)
	}
	r.h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, id, list)
	return id
}

// resolved is the user's resolved list entries as "id:perm|perm", sorted by id (core.GetUser).
func (r *rbFixture) resolved(userID int) string {
	r.h.t.Helper()
	u, err := r.h.app.core.GetUser(userID, "", "")
	if err != nil {
		r.h.t.Fatal(err)
	}
	return entriesKey(u.ListRole)
}

func entriesKey(lr *auth.ListRolePermissions) string {
	if lr == nil {
		return "<nil>"
	}
	var out []string
	for _, l := range lr.Lists {
		out = append(out, fmt.Sprintf("%d:%s", l.ID, strings.Join(l.Permissions, "|")))
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func mg(id int) string { return fmt.Sprintf("%d:list:get|list:manage", id) }
func an(id int) string { return fmt.Sprintf("%d:list:analytics", id) }

func keyOf(entries ...string) string {
	sort.Strings(entries)
	return strings.Join(entries, ",")
}

// profileKey is the session's GET /api/profile list_role entries, keyed as entriesKey.
func profileKey(t *testing.T, cl *twofaClient) string {
	t.Helper()
	rec := cl.json(http.MethodGet, "/api/profile", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/profile: %d %s", rec.Code, rec.Body.String())
	}
	var p struct {
		ListRole *auth.ListRolePermissions `json:"list_role"`
	}
	if err := json.Unmarshal(dataOf(t, rec), &p); err != nil {
		t.Fatal(err)
	}
	return entriesKey(p.ListRole)
}

var managerPerms = []string{
	auth.PermCampaignsGet, auth.PermCampaignsManage, auth.PermCampaignsGetAnalytics, auth.PermCampaignsSend,
	auth.PermSubscribersGet, auth.PermSubscribersManage, auth.PermSubscribersImport, auth.PermBouncesGet,
	auth.PermBrandsGet,
}

// I1 and I3.
func TestBrandRoleResolution(t *testing.T) {
	r := newRBFixture(t)

	samCl, sam := r.brandRoleUser("sam", managerPerms, grant("shala", auth.BrandLevelManage))
	_, megan := r.brandRoleUser("megan", managerPerms, grant("shala", auth.BrandLevelManage), grant("", auth.BrandLevelAnalytics))
	_, eliz := r.brandRoleUser("eliz", []string{auth.PermCampaignsGetAnalytics}, grant("ruze", auth.BrandLevelAnalytics))

	// I1 -- manage on every Shala list (and nothing else), analytics on every Ruze list.
	if got, want := r.resolved(sam), keyOf(mg(r.shala), mg(r.shala2)); got != want {
		t.Fatalf("sam: %s, want %s", got, want)
	}
	if got, want := r.resolved(eliz), keyOf(an(r.ruze), an(r.ruze2)); got != want {
		t.Fatalf("eliz: %s, want %s", got, want)
	}
	// I3 -- manage beats All brands analytics per list; the untagged list is never reached.
	if got, want := r.resolved(megan), keyOf(mg(r.shala), mg(r.shala2), an(r.ruze), an(r.ruze2), an(r.acme)); got != want {
		t.Fatalf("megan: %s, want %s", got, want)
	}
	if got := profileKey(t, samCl); got != keyOf(mg(r.shala), mg(r.shala2)) {
		t.Fatalf("sam's profile: %s", got)
	}

	// I1 -- a list created on Shala after the grant, and a Ruze list re-branded onto Shala, are
	// covered on the session's NEXT request (no cache, no role write); an archived list too.
	created := r.sqlList("Shala new", "brand:shala", "from:"+rbFrom)
	r.h.db.MustExec(`UPDATE lists SET status = 'archived' WHERE id = $1`, created)
	r.h.db.MustExec(`UPDATE lists SET tags = '{brand:shala}' WHERE id = $1`, r.ruze2)
	want := keyOf(mg(r.shala), mg(r.shala2), mg(created), mg(r.ruze2))
	if got := profileKey(t, samCl); got != want {
		t.Fatalf("sam after a create and a re-brand: %s, want %s", got, want)
	}
	if got, want := r.resolved(eliz), keyOf(an(r.ruze)); got != want {
		t.Fatalf("eliz after Ruze two moved off Ruze: %s, want %s", got, want)
	}
	// A list moved off the brand drops out.
	r.h.db.MustExec(`UPDATE lists SET tags = '{seed}' WHERE id = $1`, r.shala2)
	if got, want := profileKey(t, samCl), keyOf(mg(r.shala), mg(created), mg(r.ruze2)); got != want {
		t.Fatalf("sam after Shala two lost its brand: %s, want %s", got, want)
	}

	// I3 -- All brands covers a brand added after the grant; never the untagged lists.
	r.brandRow("newco", rbFrom, "")
	nc := r.sqlList("Newco news", "brand:newco", "from:"+rbFrom)
	if got, want := r.resolved(megan), keyOf(mg(r.shala), mg(created), mg(r.ruze2), an(r.ruze), an(nc), an(r.acme)); got != want {
		t.Fatalf("megan after newco: %s, want %s", got, want)
	}
}

// rbGet fires a GET and returns the status and body.
func rbGet(cl *twofaClient, target string) (int, string) {
	rec := cl.json(http.MethodGet, target, nil)
	return rec.Code, rec.Body.String()
}

func idsOfRows(t *testing.T, body string) []int {
	t.Helper()
	var resp struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("%s: %v", body, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		// The matrix is {rows: [...]} or similar; accept either shape.
		var obj map[string]json.RawMessage
		if err2 := json.Unmarshal(resp.Data, &obj); err2 != nil {
			t.Fatalf("%s: %v", body, err)
		}
		for _, k := range []string{"rows", "campaigns", "results"} {
			if raw, ok := obj[k]; ok {
				json.Unmarshal(raw, &rows)
				break
			}
		}
	}
	out := []int{}
	for _, row := range rows {
		if id, ok := row["id"].(float64); ok {
			out = append(out, int(id))
		}
	}
	sort.Ints(out)
	return out
}

func rbHas(ids []int, id int) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// I4 -- the analytics sites.
func TestBrandRoleAnalyticsSites(t *testing.T) {
	r := newRBFixture(t)

	meganCl, _ := r.brandRoleUser("megan", managerPerms, grant("shala", auth.BrandLevelManage), grant("", auth.BrandLevelAnalytics))
	elizCl, _ := r.brandRoleUser("eliz", []string{auth.PermCampaignsGetAnalytics, auth.PermBrandsGet}, grant("ruze", auth.BrandLevelAnalytics))
	// The comparators differ from Eliz only in their grants: Ruze at manage, and nothing.
	ruzeMgr, _ := r.brandRoleUser("ruzemgr", []string{auth.PermCampaignsGetAnalytics, auth.PermBrandsGet}, grant("ruze", auth.BrandLevelManage))
	nobody, _ := r.brandRoleUser("nobody", []string{auth.PermCampaignsGetAnalytics, auth.PermBrandsGet})

	// The picker: a started Ruze campaign is offered, a Ruze draft (reached only through analytics
	// lists) is not, a Shala draft is (Megan holds Shala at manage and is not analytics-only).
	code, body := rbGet(meganCl, "/api/analytics/campaigns?per_page=50")
	if code != http.StatusOK {
		t.Fatalf("picker: %d %s", code, body)
	}
	ids := idsOfRows(t, body)
	if !rbHas(ids, r.ruzeSent) || rbHas(ids, r.ruzeDraft) || rbHas(ids, r.ruzeJourney) || !rbHas(ids, r.shalaDraft) {
		t.Fatalf("megan's picker %v: want Ruze sent %d and Shala draft %d, never Ruze draft %d / journey %d",
			ids, r.ruzeSent, r.shalaDraft, r.ruzeDraft, r.ruzeJourney)
	}
	if _, body := rbGet(meganCl, fmt.Sprintf("/api/analytics/campaigns?id=%d&id=%d", r.ruzeDraft, r.ruzeSent)); !reflect.DeepEqual(idsOfRows(t, body), []int{r.ruzeSent}) {
		t.Fatalf("megan's picker ?id= prefill: %s", body)
	}
	// The grouped picker (Campaign mode) holds the draft back too.
	if code, body := rbGet(meganCl, "/api/analytics/campaigns?grouped=true&query=Ruze"); code != http.StatusOK || strings.Contains(body, fmt.Sprintf("\"id\":%d,", r.ruzeDraft)) {
		t.Fatalf("megan's grouped picker: %d %s", code, body)
	}
	// Eliz (analytics-only) sees the started Ruze campaign and nothing of Shala.
	if _, body := rbGet(elizCl, "/api/analytics/campaigns?per_page=50"); !reflect.DeepEqual(idsOfRows(t, body), []int{r.ruzeSent}) {
		t.Fatalf("eliz's picker: %s", body)
	}

	// The matrix: the Ruze journey with activity but no started_at is held back, then shown once
	// started; the started Ruze broadcast is in.
	matrixIDs := func(cl *twofaClient) []int {
		code, body := rbGet(cl, "/api/analytics/campaigns/matrix")
		if code != http.StatusOK {
			t.Fatalf("matrix: %d %s", code, body)
		}
		return idsOfRows(t, body)
	}
	if m := matrixIDs(meganCl); !rbHas(m, r.ruzeSent) || rbHas(m, r.ruzeJourney) || !rbHas(m, r.shalaCamp) {
		t.Fatalf("megan's matrix %v", m)
	}
	r.h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - INTERVAL '2 hours' WHERE id = ANY($1)`, pq.Array([]int{r.ruzeJourney, r.ruzeDraft}))
	if m := matrixIDs(meganCl); !rbHas(m, r.ruzeJourney) {
		t.Fatalf("megan's matrix after the journey started: %v", m)
	}
	if _, body := rbGet(meganCl, "/api/analytics/campaigns?per_page=50"); !rbHas(idsOfRows(t, body), r.ruzeDraft) {
		t.Fatalf("megan's picker after the Ruze draft started: %s", body)
	}
	if m := matrixIDs(elizCl); !rbHas(m, r.ruzeSent) || rbHas(m, r.shalaCamp) {
		t.Fatalf("eliz's matrix %v", m)
	}

	// Every per-campaign analytics read allows a campaign on an analytics-level list, and refuses one
	// whose lists are all outside the scope exactly as today (403).
	from, to := "2000-01-01T00:00:00Z", "2100-01-01T00:00:00Z"
	for _, typ := range []string{"views", "clicks", "bounces", "links", "countries", "clients", "shop_links", "gmail_gap", "conversions", "conversion_orders"} {
		q := func(id int) string {
			return fmt.Sprintf("/api/campaigns/analytics/%s?id=%d&from=%s&to=%s", typ, id, from, to)
		}
		if code, body := rbGet(elizCl, q(r.ruzeSent)); code != http.StatusOK {
			t.Fatalf("eliz %s on the Ruze campaign: %d %s", typ, code, body)
		}
		if code, body := rbGet(meganCl, q(r.ruzeSent)); code != http.StatusOK {
			t.Fatalf("megan %s on the Ruze campaign: %d %s", typ, code, body)
		}
		if code, _ := rbGet(elizCl, q(r.shalaCamp)); code != http.StatusForbidden {
			t.Fatalf("eliz %s on the Shala campaign: %d, want 403", typ, code)
		}
		if code, _ := rbGet(nobody, q(r.ruzeSent)); code != http.StatusForbidden {
			t.Fatalf("no-grant %s on the Ruze campaign: %d, want 403", typ, code)
		}
	}
	if code, body := rbGet(elizCl, "/api/campaigns/conversions"); code != http.StatusOK {
		t.Fatalf("eliz conversions meta: %d %s", code, body)
	}

	// The Dashboard: each site, with list_id = an analytics-level list and without, reads exactly
	// what the same user holding Ruze at MANAGE reads, and not what a user with no grant reads.
	for _, site := range []string{"charts", "counts", "clients", "countries", "brand-matrix"} {
		for _, qs := range []string{"", fmt.Sprintf("?list_id=%d", r.ruze)} {
			target := "/api/dashboard/" + site + qs
			code, got := rbGet(elizCl, target)
			if code != http.StatusOK {
				t.Fatalf("eliz %s: %d %s", target, code, got)
			}
			_, want := rbGet(ruzeMgr, target)
			_, none := rbGet(nobody, target)
			if got != want {
				t.Fatalf("eliz %s differs from the Ruze manager's\n eliz %s\n mgr  %s", target, got, want)
			}
			if got == none {
				t.Fatalf("eliz %s equals the no-grant user's (analytics lists not reached): %s", target, got)
			}
		}
	}
	// Counts under list_id = Ruze: the Ruze campaigns and the Ruze subscriber.
	_, body = rbGet(elizCl, fmt.Sprintf("/api/dashboard/counts?list_id=%d", r.ruze))
	var counts struct {
		Data struct {
			Campaigns   struct{ Total int } `json:"campaigns"`
			Subscribers struct{ Total int } `json:"subscribers"`
		} `json:"data"`
	}
	json.Unmarshal([]byte(body), &counts)
	if counts.Data.Campaigns.Total != 3 || counts.Data.Subscribers.Total != 1 {
		t.Fatalf("eliz counts on Ruze: %s", body)
	}
}

// I5 -- the non-analytics sites, as a Brand Manager holding Shala at manage and All brands at
// analytics, on the Ruze campaign / list / subscriber (analytics-level for her).
func TestBrandRoleNonAnalyticsSites(t *testing.T) {
	r := newRBFixture(t)
	meganCl, _ := r.brandRoleUser("megan", managerPerms, grant("shala", auth.BrandLevelManage), grant("", auth.BrandLevelAnalytics))
	// The review routes need a step-up-free cookie session; campaigns:send is held.
	camp := itoa(r.ruzeDraft)
	before := r.campaignRow(r.ruzeDraft)

	for _, c := range []struct {
		what, method, target string
		body                 any
	}{
		{"get", http.MethodGet, "/api/campaigns/" + camp, nil},
		{"silent-audience", http.MethodGet, "/api/campaigns/" + camp + "/silent-audience", nil},
		{"preview", http.MethodGet, "/api/campaigns/" + camp + "/preview", nil},
		{"preview post", http.MethodPost, "/api/campaigns/" + camp + "/preview", map[string]any{"body": "x", "content_type": "plain"}},
		{"archive preview", http.MethodPost, "/api/campaigns/" + camp + "/preview/archive", map[string]any{}},
		{"update", http.MethodPut, "/api/campaigns/" + camp, pfBody(r.ruze, rbFrom, map[string]any{"name": "Hijacked"})},
		{"status", http.MethodPut, "/api/campaigns/" + camp + "/status", map[string]any{"status": "running"}},
		// (POST /api/campaigns/:id/content is no checkCampaignPerm caller: it converts the posted body
		// and reads no campaign -- spec §6 Implementation notes.)
		{"archive", http.MethodPut, "/api/campaigns/" + camp + "/archive", map[string]any{"archive": true}},
		{"test", http.MethodPost, "/api/campaigns/" + camp + "/test", pfBody(r.ruze, rbFrom, map[string]any{"subscribers": []string{"ruze@example.test"}})},
		{"review start", http.MethodPost, "/api/campaigns/" + camp + "/reviews", map[string]any{}},
		{"review latest", http.MethodGet, "/api/campaigns/" + camp + "/reviews/latest", nil},
		{"promo-audience", http.MethodGet, "/api/campaigns/" + camp + "/promo-audience", nil},
		{"delete by id", http.MethodDelete, "/api/campaigns/" + camp, nil},
		{"list GET /api/lists/:id", http.MethodGet, "/api/lists/" + itoa(r.ruze), nil},
	} {
		rec := meganCl.json(c.method, c.target, c.body)
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
			t.Fatalf("%s on the Ruze campaign: %d %s, want a refusal", c.what, rec.Code, rec.Body.String())
		}
	}
	// Bulk delete by id and by query leave the Ruze campaign in place.
	meganCl.json(http.MethodDelete, "/api/campaigns?id="+camp, nil)
	meganCl.do(http.MethodDelete, "/api/campaigns?query=Ruze", "", "")
	meganCl.do(http.MethodDelete, "/api/campaigns?all=true", "", "")
	if got := r.campaignRow(r.ruzeDraft); got != before {
		t.Fatalf("the Ruze campaign changed:\n %s\n %s", before, got)
	}
	if n := r.count(`SELECT COUNT(*) FROM campaigns WHERE id = $1`, r.ruzeSent); n != 1 {
		t.Fatal("a bulk delete removed a Ruze campaign")
	}

	// The campaigns list holds no Ruze campaign.
	_, body := rbGet(meganCl, "/api/campaigns?per_page=all")
	if strings.Contains(body, "Ruze") {
		t.Fatalf("campaigns list shows a Ruze campaign: %s", body)
	}
	// Campaign create on a Ruze list is refused and creates nothing.
	n := r.count(`SELECT COUNT(*) FROM campaigns`)
	if rec := meganCl.json(http.MethodPost, "/api/campaigns", pfBody(r.ruze, rbFrom, map[string]any{"name": "On Ruze"})); rec.Code == http.StatusOK {
		t.Fatalf("create on a Ruze list: %d %s", rec.Code, rec.Body.String())
	}
	if r.count(`SELECT COUNT(*) FROM campaigns`) != n {
		t.Fatal("a refused create wrote a campaign")
	}
	// Subscribers by list_id = Ruze: never the Ruze subscriber.
	_, body = rbGet(meganCl, fmt.Sprintf("/api/subscribers?list_id=%d&per_page=all", r.ruze))
	if strings.Contains(body, "ruze@example.test") {
		t.Fatalf("subscribers by Ruze list_id: %s", body)
	}
	if code, body := rbGet(meganCl, fmt.Sprintf("/api/subscribers/%d", r.ruzeSub)); code == http.StatusOK {
		t.Fatalf("GET the Ruze subscriber: %d %s", code, body)
	}
	// Lists, minimal and full: no Ruze list.
	for _, q := range []string{"/api/lists?minimal=true&per_page=all", "/api/lists?per_page=all"} {
		_, body := rbGet(meganCl, q)
		if strings.Contains(body, "Ruze") || !strings.Contains(body, "Shala news") {
			t.Fatalf("%s: %s", q, body)
		}
	}
	// Import into a Ruze list: refused before reading any file.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("params", fmt.Sprintf(`{"mode":"subscribe","delim":",","lists":[%d]}`, r.ruze))
	mw.Close()
	if rec := meganCl.do(http.MethodPost, "/api/import/subscribers", mw.FormDataContentType(), buf.String()); rec.Code != http.StatusForbidden {
		t.Fatalf("import into Ruze: %d %s, want 403", rec.Code, rec.Body.String())
	}
	// GET /api/brands/health: the Ruze row's lists[] stays get-only (empty for her).
	r.h.db.MustExec(`INSERT INTO brand_health (brand, day, status, doc) VALUES ('ruze', CURRENT_DATE, 'ok', '{"brand":"ruze","status":"ok"}'),
		('shala', CURRENT_DATE, 'ok', '{"brand":"shala","status":"ok"}')`)
	raw := r.ok("brands/health", meganCl.json(http.MethodGet, "/api/brands/health", nil))
	var docs []struct {
		Brand string `json:"brand"`
		Lists []struct {
			ID int `json:"id"`
		} `json:"lists"`
	}
	json.Unmarshal(raw, &docs)
	for _, d := range docs {
		if d.Brand == "ruze" && len(d.Lists) != 0 {
			t.Fatalf("brands/health ruze lists[] = %+v, want none", d.Lists)
		}
		if d.Brand == "shala" && len(d.Lists) != 2 {
			t.Fatalf("brands/health shala lists[] = %+v, want the two Shala lists", d.Lists)
		}
	}
}

func (r *rbFixture) campaignRow(id int) string {
	r.h.t.Helper()
	var s string
	r.h.db.Get(&s, `SELECT c::TEXT FROM campaigns c WHERE id = $1`, id)
	return s
}

// grantsOf is a role's role_brands rows as "brand:level" ("*" = All brands), sorted.
func (r *rbFixture) grantsOf(role int) string {
	r.h.t.Helper()
	var rows []string
	r.h.db.Select(&rows, `SELECT COALESCE(brand, '*') || ':' || level FROM role_brands WHERE role_id = $1 ORDER BY 1`, role)
	return strings.Join(rows, ",")
}

// I6 -- D5's refusals, the non-list role id, and the atomic replacement.
func TestBrandRoleWrites(t *testing.T) {
	r := newRBFixture(t)
	if rec := r.admin.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatalf("step-up: %d %s", rec.Code, rec.Body.String())
	}

	created := r.ok("create", r.admin.json(http.MethodPost, "/api/roles/lists",
		map[string]any{"name": "Brand role", "brands": []map[string]any{{"brand": "shala", "level": "manage"}, {"brand": nil, "level": "analytics"}}}))
	var role struct {
		ID int `json:"id"`
	}
	json.Unmarshal(created, &role)
	if got := r.grantsOf(role.ID); got != "*:analytics,shala:manage" {
		t.Fatalf("created grants %q", got)
	}

	bodies := []struct {
		what string
		body string
	}{
		{"a non-empty lists field", fmt.Sprintf(`{"name":"x","brands":[],"lists":[{"id":%d,"permissions":["list:get"]}]}`, r.ruze)},
		{"an unknown brand", `{"name":"x","brands":[{"brand":"nope","level":"manage"}]}`},
		{"All brands at manage", `{"name":"x","brands":[{"brand":null,"level":"manage"}]}`},
		{"one brand at both levels", `{"name":"x","brands":[{"brand":"ruze","level":"manage"},{"brand":"ruze","level":"analytics"}]}`},
		{"a duplicate entry", `{"name":"x","brands":[{"brand":"ruze","level":"analytics"},{"brand":"ruze","level":"analytics"}]}`},
		{"a duplicate All brands", `{"name":"x","brands":[{"brand":null,"level":"analytics"},{"brand":null,"level":"analytics"}]}`},
		{"an unknown level", `{"name":"x","brands":[{"brand":"ruze","level":"owner"}]}`},
		{"an empty brand", `{"name":"x","brands":[{"brand":"","level":"analytics"}]}`},
		{"no name", `{"name":"","brands":[]}`},
	}
	state := func() string {
		var s string
		r.h.db.Get(&s, `SELECT COALESCE(string_agg(x::TEXT, '|' ORDER BY x::TEXT), '') FROM (SELECT r.id, r.name, r.type, r.parent_id FROM roles r) x`)
		var g string
		r.h.db.Get(&g, `SELECT COALESCE(string_agg(b::TEXT, '|' ORDER BY b::TEXT), '') FROM role_brands b`)
		return s + "#" + g
	}
	before := state()
	for _, b := range bodies {
		for _, w := range []struct{ method, target string }{
			{http.MethodPost, "/api/roles/lists"},
			{http.MethodPut, "/api/roles/lists/" + itoa(role.ID)},
		} {
			rec := r.admin.do(w.method, w.target, "application/json", b.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s %s with %s: %d %s, want 400", w.method, w.target, b.what, rec.Code, rec.Body.String())
			}
		}
	}
	// A PUT whose id is not a list role (a user role): 400, and that role untouched.
	var userRole int
	r.h.db.Get(&userRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Plain', '{campaigns:get}') RETURNING id`)
	before = state()
	if rec := r.admin.json(http.MethodPut, "/api/roles/lists/"+itoa(userRole), map[string]any{"name": "Plain", "brands": []map[string]any{{"brand": "ruze", "level": "analytics"}}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT a user role as a list role: %d %s", rec.Code, rec.Body.String())
	}
	if state() != before {
		t.Fatal("refused writes changed the roles or grants")
	}
	var perms pq.StringArray
	r.h.db.Get(&perms, `SELECT permissions FROM roles WHERE id = $1`, userRole)
	if !reflect.DeepEqual([]string(perms), []string{"campaigns:get"}) {
		t.Fatalf("the user role's permissions changed: %v", perms)
	}

	// A write replaces the grants: the old set gone, the new one exactly.
	r.ok("update", r.admin.json(http.MethodPut, "/api/roles/lists/"+itoa(role.ID),
		map[string]any{"name": "Brand role 2", "brands": []map[string]any{{"brand": "ruze", "level": "analytics"}}}))
	if got := r.grantsOf(role.ID); got != "ruze:analytics" {
		t.Fatalf("after update grants %q", got)
	}
	// Atomic: a write that fails at the database (a brand deleted between validation and the
	// insert is simulated with a trigger) leaves the name and the grants as they were.
	r.h.db.MustExec(`CREATE FUNCTION rb_fail() RETURNS TRIGGER AS $$ BEGIN RAISE EXCEPTION 'rb boom'; END; $$ LANGUAGE plpgsql`)
	r.h.db.MustExec(`CREATE TRIGGER rb_fail BEFORE INSERT ON role_brands FOR EACH ROW EXECUTE FUNCTION rb_fail()`)
	if rec := r.admin.json(http.MethodPut, "/api/roles/lists/"+itoa(role.ID),
		map[string]any{"name": "Renamed", "brands": []map[string]any{{"brand": "shala", "level": "manage"}}}); rec.Code != http.StatusInternalServerError {
		t.Fatalf("failing write: %d %s", rec.Code, rec.Body.String())
	}
	r.h.db.MustExec(`DROP TRIGGER rb_fail ON role_brands`)
	if got := r.grantsOf(role.ID); got != "ruze:analytics" {
		t.Fatalf("a failed write changed the grants: %q", got)
	}
	if n := r.count(`SELECT COUNT(*) FROM roles WHERE id = $1 AND name = 'Brand role 2'`, role.ID); n != 1 {
		t.Fatal("a failed write changed the name")
	}

	// GET /api/roles/lists returns brands and the resolved lists.
	raw := r.ok("list roles", r.admin.json(http.MethodGet, "/api/roles/lists", nil))
	var roles []auth.ListRole
	json.Unmarshal(raw, &roles)
	found := false
	for _, x := range roles {
		if x.ID == role.ID {
			found = true
			if len(x.Brands) != 1 || *x.Brands[0].Brand != "ruze" || len(x.Lists) != 2 {
				t.Fatalf("listing: %+v", x)
			}
		}
	}
	if !found {
		t.Fatal("role missing from the listing")
	}
}

// I8 -- child rows are never read; the three statements agree.
func TestBrandRoleChildRowsNeverRead(t *testing.T) {
	r := newRBFixture(t)
	cl, uid := r.brandRoleUser("sam", managerPerms, grant("shala", auth.BrandLevelManage))
	var roleID int
	r.h.db.Get(&roleID, `SELECT list_role_id FROM users WHERE id = $1`, uid)

	// A per-list child row (the pre-v6.2.42 shape) on a Ruze list -- no Ruze grant: no access.
	r.h.db.MustExec(`INSERT INTO roles (type, parent_id, list_id, permissions) VALUES ('list', $1, $2, '{list:get,list:manage}')`, roleID, r.ruze)
	want := keyOf(mg(r.shala), mg(r.shala2))
	if got := r.resolved(uid); got != want {
		t.Fatalf("with a Ruze child row: %s, want %s", got, want)
	}
	if code, _ := rbGet(cl, "/api/lists/"+itoa(r.ruze)); code != http.StatusForbidden {
		t.Fatalf("GET the Ruze list through a child row: %d", code)
	}

	// A role write leaves the child row untouched.
	if rec := r.admin.stepUpPassword(); rec.Code != http.StatusOK {
		t.Fatal("step-up")
	}
	r.ok("update", r.admin.json(http.MethodPut, "/api/roles/lists/"+itoa(roleID),
		map[string]any{"name": "sam list role", "brands": []map[string]any{{"brand": "shala", "level": "manage"}, {"brand": nil, "level": "analytics"}}}))
	if n := r.count(`SELECT COUNT(*) FROM roles WHERE parent_id = $1 AND list_id = $2`, roleID, r.ruze); n != 1 {
		t.Fatalf("a role write touched the child row (%d)", n)
	}

	// get-user, get-users and get-list-roles return the same resolved set for the role.
	one := r.resolved(uid)
	users, err := r.h.app.core.GetUsers()
	if err != nil {
		t.Fatal(err)
	}
	var many string
	for _, u := range users {
		if u.ID == uid {
			many = entriesKey(u.ListRole)
		}
	}
	roles, err := r.h.app.core.GetListRoles()
	if err != nil {
		t.Fatal(err)
	}
	var listed string
	for _, x := range roles {
		if x.ID == roleID {
			listed = entriesKey(&auth.ListRolePermissions{Lists: x.Lists})
		}
	}
	if want := keyOf(mg(r.shala), mg(r.shala2), an(r.ruze), an(r.ruze2), an(r.acme)); one != want || many != want || listed != want {
		t.Fatalf("get-user %s\nget-users %s\nget-list-roles %s\nwant %s", one, many, listed, want)
	}
}

// launchRows decodes GET /api/brands/launch and asserts the body carries brand and unlaunched only.
func launchRows(t *testing.T, cl *twofaClient) map[string]bool {
	t.Helper()
	rec := cl.json(http.MethodGet, "/api/brands/launch", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/brands/launch: %d %s", rec.Code, rec.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(dataOf(t, rec), &rows); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, row := range rows {
		if len(row) != 2 {
			t.Fatalf("a launch row carries more than brand and unlaunched: %v", row)
		}
		b, ok1 := row["brand"].(string)
		u, ok2 := row["unlaunched"].(bool)
		if !ok1 || !ok2 {
			t.Fatalf("a launch row's shape: %v", row)
		}
		out[b] = u
	}
	for _, s := range []string{"ok", "warn", "issues", "unknown"} {
		if strings.Contains(rec.Body.String(), `"`+s+`"`) {
			t.Fatalf("the launch body carries a status value %q: %s", s, rec.Body.String())
		}
	}
	return out
}

// launchRuleCases is the shared agreement table (models/testdata/launch-rule.json), also read by
// the frontend's health-sort test against isUnlaunchedBrandDoc.
func launchRuleCases(t *testing.T) []struct {
	Status     string `json:"status"`
	Unlaunched bool   `json:"unlaunched"`
} {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "models", "testdata", "launch-rule.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Status     string `json:"status"`
		Unlaunched bool   `json:"unlaunched"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

// I11 -- GET /api/brands/launch.
func TestBrandsLaunch(t *testing.T) {
	r := newRBFixture(t)
	elizCl, _ := r.brandRoleUser("eliz", []string{auth.PermCampaignsGetAnalytics}, grant("ruze", auth.BrandLevelAnalytics))
	noneCl, _ := r.brandRoleUser("bare", nil)

	// The agreement table: one brand per status, its latest row carrying it (an older row of
	// another status underneath, so "latest" is exercised).
	cases := launchRuleCases(t)
	want := map[string]bool{}
	for i, c := range cases {
		slug := fmt.Sprintf("st%d", i)
		r.brandRow(slug, rbFrom, "")
		r.h.db.MustExec(`INSERT INTO brand_health (brand, day, status, doc) VALUES ($1, CURRENT_DATE - 3, $2, '{}'), ($1, CURRENT_DATE, $3, '{}')`,
			slug, map[bool]string{true: "ok", false: "unknown"}[c.Unlaunched], c.Status)
		want[slug] = c.Unlaunched
	}
	// No row is launched; the fixture's own brands have none.
	for _, b := range []string{"acme", "shala", "ruze"} {
		want[b] = false
	}

	// Every brands row for Super Admin (session), and for a token holding lists:get_all.
	if got := launchRows(t, r.admin); !reflect.DeepEqual(got, want) {
		t.Fatalf("admin: %v\nwant %v", got, want)
	}
	var tokRole int
	r.h.db.Get(&tokRole, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Reader API', '{lists:get_all}') RETURNING id`)
	u, err := r.h.app.core.CreateUser(auth.User{Type: auth.UserTypeAPI, Username: "reader", Name: "reader", UserRoleID: tokRole, Status: auth.UserStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cacheUsers(r.h.app.core, r.h.app.auth); err != nil {
		t.Fatal(err)
	}
	tok := r.h.client()
	tok.hdr["Authorization"] = "token reader:" + u.Password.String
	if got := launchRows(t, tok); !reflect.DeepEqual(got, want) {
		t.Fatalf("lists:get_all token: %v\nwant %v", got, want)
	}

	// The roster only, for a user with no permission at all beyond a brand role: Eliz gets Ruze.
	if got := launchRows(t, elizCl); !reflect.DeepEqual(got, map[string]bool{"ruze": false}) {
		t.Fatalf("eliz: %v", got)
	}
	r.h.db.MustExec(`INSERT INTO brand_health (brand, day, status, doc) VALUES ('ruze', CURRENT_DATE, 'unknown', '{}')`)
	if got := launchRows(t, elizCl); !reflect.DeepEqual(got, map[string]bool{"ruze": true}) {
		t.Fatalf("eliz after Ruze went unknown: %v", got)
	}
	// A token whose user holds the brand role reads the same roster.
	var elizRole int
	r.h.db.Get(&elizRole, `SELECT list_role_id FROM users WHERE username = 'eliz'`)
	tu, err := r.h.app.core.CreateUser(auth.User{Type: auth.UserTypeAPI, Username: "elizbot", Name: "elizbot", UserRoleID: tokRole, ListRoleID: &elizRole, Status: auth.UserStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}
	r.h.db.MustExec(`UPDATE roles SET permissions = '{}' WHERE id = $1`, tokRole)
	if _, err := cacheUsers(r.h.app.core, r.h.app.auth); err != nil {
		t.Fatal(err)
	}
	tok2 := r.h.client()
	tok2.hdr["Authorization"] = "token elizbot:" + tu.Password.String
	if got := launchRows(t, tok2); !reflect.DeepEqual(got, map[string]bool{"ruze": true}) {
		t.Fatalf("eliz's token: %v", got)
	}
	// No grant at all: an empty list, still 200 (no pm permission).
	if got := launchRows(t, noneCl); len(got) != 0 {
		t.Fatalf("no grants: %v", got)
	}
}
