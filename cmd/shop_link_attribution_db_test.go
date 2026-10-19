package main

// Fork (shop-link attribution) -- integrations SHOP-LINK-ATTRIBUTION-SPEC I1-I5 against a real
// database, through the handlers: get-campaign-shop-link-counts counts only clicks on a link whose
// stored URL carries shop_links (I1), buckets each by the clicking subscriber's stored kind for the
// brand named in the expression (I2/I3), honors the link counts' counted expression in both
// preparations (I4), and type=shop_links is refused exactly where type=links is (I5). Shares
// newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go) and the brand-analytics
// users (brand_analytics_db_test.go).
//
// Fork (shop-link frozen) -- integrations SHOP-LINK-FROZEN-SPEC I2/I3: a click buckets by its
// subscriber's campaign_shop_link_recipients kind for the link's brand when the row exists, else by
// the kind held today; a NULL subscriber stays unknown.

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
	"github.com/lib/pq"
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

// shopLinkRateRows decodes a type=shop_links response body into "kind=count/audience" pairs in
// response order (audience "null" when absent) plus the snapshots value, which must be the same on
// every row. Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D5/I5).
func shopLinkRateRows(t *testing.T, body string) (string, int) {
	t.Helper()
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("body %s: %v", body, err)
	}
	out := make([]string, 0, len(resp.Data))
	snaps := -1
	for _, r := range resp.Data {
		a, ok := r["audience"]
		if !ok {
			t.Fatalf("row without an audience field: %s", body)
		}
		aud := "null"
		if a != nil {
			aud = fmt.Sprint(a)
		}
		s, ok := r["snapshots"].(float64)
		if !ok {
			t.Fatalf("row without a numeric snapshots field: %s", body)
		}
		if snaps >= 0 && int(s) != snaps {
			t.Fatalf("snapshots differs across rows: %s", body)
		}
		snaps = int(s)
		out = append(out, fmt.Sprintf("%s=%v/%s", r["kind"], r["count"], aud))
	}
	return strings.Join(out, " "), snaps
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

	// Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC I5) -- with no snapshot anywhere,
	// every audience is null and snapshots is 0.
	if got, n := shopLinkRateRows(t, body); n != 0 || strings.Contains(got, "/0") || strings.Count(got, "/null") != 7 {
		t.Fatalf("no snapshot: %s (snapshots %d), want every audience null and snapshots 0", got, n)
	}

	// Snapshot rows for both campaigns (two brands on camp, so audiences sum across brands), and a
	// third campaign with clicks but no snapshot.
	snap := func(c int, brand, kind string, n int) {
		db.MustExec(`INSERT INTO campaign_shop_link_audience (campaign_id, brand, kind, audience) VALUES ($1, $2, $3, $4)`, c, brand, kind, n)
	}
	snap(camp, "ruze", "self", 10)
	snap(camp, "ruze", "creator", 20)
	snap(camp, "ruze", "founder", 30)
	snap(camp, "ruze", "none", 5)
	snap(camp, "thirstygirl", "corporate", 4)
	snap(camp, "thirstygirl", "none", 40)
	snap(otherCamp, "ruze", "creator", 7)
	snap(otherCamp, "ruze", "self", 1)
	noSnap := newCamp("No snapshot")
	click(noSnap, newSub("creator3@x", entry("ruze", "creator")), ruzeDot, 1)

	for _, tc := range []struct {
		name  string
		ids   []int
		want  string
		snaps int
	}{
		// One snapshot: summed across brands; partner (absent from the snapshot) reads 0; unknown null.
		{"one campaign", []int{camp}, "none=3/45 corporate=2/4 creator=1/20 founder=1/30 partner=1/0 self=1/10 unknown=0/null", 1},
		// A kind with audience but no clicks is a count-0 row.
		{"other campaign", []int{otherCamp}, "creator=1/7 self=0/1", 1},
		// Full selection: summed across the campaigns.
		{"both snapshotted", []int{camp, otherCamp}, "none=3/45 corporate=2/4 creator=2/27 founder=1/30 partner=1/0 self=1/11 unknown=0/null", 2},
		// Partial: the campaign without a snapshot adds clicks, no audience.
		{"partial", []int{camp, noSnap}, "none=3/45 corporate=2/4 creator=2/20 founder=1/30 partner=1/0 self=1/10 unknown=0/null", 1},
		// None: null audiences.
		{"none snapshotted", []int{noSnap}, "creator=1/null", 0},
	} {
		code, body := analyticsMultiReq(t, h, member, "shop_links", tc.ids...)
		if code != http.StatusOK {
			t.Fatalf("%s: %d %s", tc.name, code, body)
		}
		got, n := shopLinkRateRows(t, body)
		if got != tc.want || n != tc.snaps {
			t.Fatalf("%s\n got  %s (snapshots %d)\n want %s (snapshots %d)", tc.name, got, n, tc.want, tc.snaps)
		}
	}
	// (Every case above is also I3's fallback for a campaign with a summary but no recipient rows,
	// a broadcast sent before the recipient table existed, and for noSnap, a campaign with none.)

	// Fork (shop-link frozen, integrations SHOP-LINK-FROZEN-SPEC I2) -- clicks bucket by the kind
	// held at send. A campaign snapshotted through the real statement, then one recipient's kind
	// changes: their click counts under the SNAPSHOT kind.
	frozen := newCamp("Frozen")
	member0 := func(email, attribs string) int {
		id := newSub(email, attribs)
		db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, id, list)
		return id
	}
	fzSelf := member0("fz-self@x", entry("ruze", "self"))
	fzFounder := member0("fz-founder@x", entry("ruze", "founder"))
	if _, err := h.q.SnapshotCampaignShopLinkAudience.Exec(frozen, pq.StringArray{"ruze"}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	db.MustExec(`UPDATE subscribers SET attribs = $1::JSONB WHERE id = $2`, entry("ruze", "corporate"), fzSelf)
	click(frozen, fzSelf, ruzeDot, 1)
	click(frozen, fzFounder, ruzeIndex, 1)
	// I3: a subscriber who joined after the snapshot (no recipient row on a campaign that has
	// them) buckets by today's kind, and a NULL subscriber is unknown.
	click(frozen, member0("fz-late@x", entry("ruze", "creator")), ruzeDot, 1)
	click(frozen, 0, ruzeDot, 1)
	code, body = analyticsMultiReq(t, h, member, "shop_links", frozen)
	if code != http.StatusOK {
		t.Fatalf("frozen: %d %s", code, body)
	}
	if got, n := shopLinkRateRows(t, body); got != "creator=1/0 founder=1/1 self=1/1 unknown=0/null" || n != 1 {
		t.Fatalf("frozen campaign\n got  %s (snapshots %d)\n want creator=1/0 founder=1/1 self=1/1 unknown=0/null (snapshots 1)", got, n)
	}

	// I2 (the brand is the expression's): a recipient row for camp's founder@x under thirstygirl
	// moves only their thirstygirl click (corporate today -> creator at send); their ruze clicks have
	// no ruze row and stay founder by today's kind (I3).
	db.MustExec(`INSERT INTO campaign_shop_link_recipients (campaign_id, subscriber_id, brand, kind) VALUES ($1, $2, 'thirstygirl', 'creator')`, camp, sFounder)
	code, body = analyticsMultiReq(t, h, member, "shop_links", camp)
	if code != http.StatusOK {
		t.Fatalf("camp with a recipient row: %d %s", code, body)
	}
	if got, want := shopLinkRows(t, body), "none=3 creator=2 corporate=1 founder=1 partner=1 self=1 unknown=0"; got != want {
		t.Fatalf("camp with a thirstygirl recipient row\n got  %s\n want %s", got, want)
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
