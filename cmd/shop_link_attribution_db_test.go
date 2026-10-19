package main

// Fork (shop-link attribution) -- integrations SHOP-LINK-ATTRIBUTION-SPEC I1-I5 against a real
// database, through the handlers: get-campaign-shop-link-counts counts only clicks on a link whose
// stored URL carries shop_links (I1), buckets each by the clicking subscriber's stored kind for the
// brand named in the expression (I2/I3), honors the link counts' counted expression in both
// preparations (I4), and type=shop_links is refused exactly where type=links is (I5). Shares
// newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go) and the brand-analytics
// users (brand_analytics_db_test.go).

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/models"
)

// shopLinkRows decodes a type=shop_links response body into "kind=count" pairs in response order.
func shopLinkRows(t *testing.T, body string) string {
	t.Helper()
	var resp struct {
		Data []models.CampaignAnalyticsShopLink `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("body %s: %v", body, err)
	}
	out := make([]string, 0, len(resp.Data))
	for _, r := range resp.Data {
		out = append(out, fmt.Sprintf("%s=%d", r.Kind, r.Count))
	}
	return strings.Join(out, " ")
}

// TestShopLinkAttribution is I1, I2, I3 and I4: one campaign with a shop link in each grammar, a
// thirstygirl shop link and a static link, through GET /api/campaigns/analytics/shop_links in both
// preparations of the query.
func TestShopLinkAttribution(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db

	var list, camp, otherCamp int
	db.Get(&list, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Ruze', 'public', 'single', '{brand:ruze}') RETURNING id`)
	newCamp := func(name string) int {
		var id int
		if err := db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id) VALUES (gen_random_uuid(), $1, 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id`, name); err != nil {
			t.Fatal(err)
		}
		db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'Ruze')`, id, list)
		return id
	}
	camp, otherCamp = newCamp("Shop"), newCamp("Other")

	newLink := func(u string) int {
		var id int
		if err := db.Get(&id, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), $1) RETURNING id`, u); err != nil {
			t.Fatal(err)
		}
		return id
	}
	var (
		ruzeDot   = newLink(`{{ .Subscriber.Attribs.shop_links.ruze.url }}`)
		ruzeIndex = newLink(`{{ index .Subscriber.Attribs.shop_links "ruze" }}`)
		tgDot     = newLink(`{{ .Subscriber.Attribs.shop_links.thirstygirl.url }}`)
		static    = newLink(`https://curatedfor.you/p?x=1`)
	)

	newSub := func(email, attribs string) int {
		var id int
		if err := db.Get(&id, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), $1, 'S', $2::JSONB) RETURNING id`, email, attribs); err != nil {
			t.Fatal(err)
		}
		return id
	}
	entry := func(brand, kind string) string {
		return fmt.Sprintf(`{"shop_links":{%q:{"url":"https://x.test/%s","kind":%q}}}`, brand, kind, kind)
	}
	var (
		sSelf    = newSub("self@x", entry("ruze", "self"))
		sCreator = newSub("creator@x", entry("ruze", "creator"))
		// I2: holds a ruze founder entry AND a thirstygirl corporate entry.
		sFounder = newSub("founder@x", `{"shop_links":{"ruze":{"url":"https://x.test/f","kind":"founder"},"thirstygirl":{"url":"https://x.test/c","kind":"corporate"}}}`)
		sCorp    = newSub("corp@x", entry("ruze", "corporate"))
		sNone    = newSub("none@x", `{}`)
		sNoKind  = newSub("nokind@x", `{"shop_links":{"ruze":{"url":"https://x.test/n"}}}`)
		// D3 (Stage 4 F4): an empty-string kind is none too (no live row has the shape).
		sEmptyKind = newSub("emptykind@x", `{"shop_links":{"ruze":{"url":"https://x.test/e","kind":""}}}`)
		sOdd       = newSub("odd@x", entry("ruze", "partner"))
		// I1: clicks only the static link; its kind would surface as a bucket if static clicks counted.
		sStatic = newSub("static@x", entry("ruze", "staticonly"))
	)

	// sub 0 = NULL subscriber (individual tracking off, or a deleted subscriber).
	click := func(c, sub, link, times int) {
		for i := 0; i < times; i++ {
			db.MustExec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id) VALUES ($1, NULLIF($2, 0), $3)`, c, sub, link)
		}
	}
	click(camp, sSelf, ruzeDot, 1)
	click(camp, sSelf, ruzeIndex, 1)
	click(camp, sCreator, ruzeIndex, 1)
	click(camp, sFounder, ruzeDot, 3) // I4: three clicks, one subscriber
	click(camp, sFounder, tgDot, 1)   // I2: buckets by the thirstygirl entry (corporate)
	click(camp, sCorp, ruzeDot, 1)
	click(camp, sNone, ruzeDot, 1)
	click(camp, sNoKind, ruzeIndex, 1)
	click(camp, sEmptyKind, ruzeDot, 1)
	click(camp, sOdd, ruzeDot, 1)
	click(camp, 0, ruzeDot, 2)
	// I1: static-link clicks never appear (a subscriber of their own, a known one, and anonymous).
	click(camp, sStatic, static, 4)
	click(camp, sSelf, static, 5)
	click(camp, 0, static, 3)
	// Outside the campaign filter: a second creator-kind subscriber on the other campaign.
	click(otherCamp, newSub("creator2@x", entry("ruze", "creator")), ruzeDot, 7)

	member := baUser([]string{auth.PermCampaignsGetAnalytics}, list)

	// Unique (individual tracking ON -- the harness's preparation): each subscriber once per
	// bucket; NULL-subscriber rows count zero under COUNT(DISTINCT subscriber_id), as in Links.
	code, body := analyticsReq(t, h, member, "shop_links", camp)
	if code != http.StatusOK {
		t.Fatalf("shop_links: %d %s", code, body)
	}
	if got, want := shopLinkRows(t, body), "none=3 corporate=2 creator=1 founder=1 partner=1 self=1 unknown=0"; got != want {
		t.Fatalf("unique buckets\n got  %s\n want %s", got, want)
	}

	// Total (individual tracking OFF): a fresh preparation behind the same handler.
	ko.Set("privacy.individual_tracking", false)
	qTotal := prepareQueries(loadQueryMap(t), db, ko)
	ko.Set("privacy.individual_tracking", true)
	unique := h.app.core
	h.app.core = core.New(&core.Opt{Queries: qTotal, DB: db, I18n: h.app.i18n, Log: log.New(os.Stderr, "", 0)}, &core.Hooks{})
	code, body = analyticsReq(t, h, member, "shop_links", camp)
	h.app.core = unique
	if code != http.StatusOK {
		t.Fatalf("shop_links total: %d %s", code, body)
	}
	if got, want := shopLinkRows(t, body), "founder=3 none=3 corporate=2 self=2 unknown=2 creator=1 partner=1"; got != want {
		t.Fatalf("total buckets\n got  %s\n want %s", got, want)
	}

	// Several campaign ids aggregate (the other campaign's creator clicks join the creator bucket).
	code, body = analyticsMultiReq(t, h, member, "shop_links", camp, otherCamp)
	if code != http.StatusOK {
		t.Fatalf("shop_links two campaigns: %d %s", code, body)
	}
	if got, want := shopLinkRows(t, body), "none=3 corporate=2 creator=2 founder=1 partner=1 self=1 unknown=0"; got != want {
		t.Fatalf("two campaigns\n got  %s\n want %s", got, want)
	}
}

// TestShopLinkAnalyticsPermission is I5: type=shop_links answers with the status type=links answers
// for the same user and ids, through the same a.auth.Perm wrapper handlers.go registers, and 400s on
// the same bad dates.
func TestShopLinkAnalyticsPermission(t *testing.T) {
	h := newLinkHarness(t)
	h.app.auth = &auth.Auth{}
	f := newBAFixture(h)

	var (
		memberA  = baUser([]string{auth.PermCampaignsGetAnalytics}, f.listA)
		memberB  = baUser([]string{auth.PermCampaignsGetAnalytics}, f.listB)
		noPerm   = baUser(nil, f.listA, f.listB)
		bothList = baUser([]string{auth.PermCampaignsGetAnalytics}, f.listA, f.listB)
	)
	for _, tc := range []struct {
		name string
		u    auth.User
		ids  []int
		want int
	}{
		{"member A on A", memberA, []int{f.campA}, http.StatusOK},
		{"member A on A+AB", memberA, []int{f.campA, f.campAB}, http.StatusOK},
		{"member B on A", memberB, []int{f.campA}, http.StatusForbidden},
		{"member A on A+B (one unpermitted)", memberA, []int{f.campA, f.campB}, http.StatusForbidden},
		{"no analytics permission", noPerm, []int{f.campA}, http.StatusForbidden},
		{"both lists on A+B", bothList, []int{f.campA, f.campB}, http.StatusOK},
	} {
		lc, lb := analyticsMultiReq(t, h, tc.u, "links", tc.ids...)
		sc, sb := analyticsMultiReq(t, h, tc.u, "shop_links", tc.ids...)
		if lc != tc.want || sc != lc {
			t.Fatalf("%s: links %d (%s), shop_links %d (%s), want both %d", tc.name, lc, lb, sc, sb, tc.want)
		}
	}

	// Bad dates: the same 400 for both types (and, for a refused user, the same 403 first).
	for _, tc := range []struct {
		name     string
		u        auth.User
		from, to string
		want     int
	}{
		{"short from", memberA, "2026", time.Now().UTC().Format(time.RFC3339), http.StatusBadRequest},
		{"missing to", memberA, time.Now().UTC().Format(time.RFC3339), "", http.StatusBadRequest},
		{"overlong", memberA, strings.Repeat("2", 31), strings.Repeat("2", 31), http.StatusBadRequest},
		{"refused user, bad dates", memberB, "x", "y", http.StatusForbidden},
	} {
		req := func(typ string) (int, string) {
			q := url.Values{}
			q.Set("id", fmt.Sprint(f.campA))
			q.Set("from", tc.from)
			q.Set("to", tc.to)
			return baCall(t, tc.u, "/api/campaigns/analytics/"+typ+"?"+q.Encode(),
				h.app.auth.Perm(h.app.GetCampaignViewAnalytics, "campaigns:get_analytics"), "type", typ)
		}
		lc, lb := req("links")
		sc, sb := req("shop_links")
		if lc != tc.want || sc != lc || sb != lb {
			t.Fatalf("%s: links %d (%s), shop_links %d (%s), want both %d with the same message", tc.name, lc, lb, sc, sb, tc.want)
		}
	}
}
