package main

// Fork (shop-link rate) -- integrations SHOP-LINK-RATE-SPEC I1/I2 against a real database, driving
// the manager's store directly: store.NextCampaigns claims the running broadcasts and snapshots each
// one's audience by shop-link kind per brand its body names (the claim's predicate, one count per
// subscriber), exactly once; an evergreen and a body without a shop-link expression run no snapshot
// query; a snapshot failure is logged and swallowed. Shares newLinkHarness (LISTMONK_TEST_PG opt-in;
// see link_redirect_db_test.go).

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/linkresolve"
)

// audienceRows reads a campaign's snapshot as sorted "brand/kind=n" pairs.
func audienceRows(t *testing.T, h *linkHarness, camp int) string {
	t.Helper()
	var rows []struct {
		Brand    string `db:"brand"`
		Kind     string `db:"kind"`
		Audience int    `db:"audience"`
	}
	if err := h.db.Select(&rows, `SELECT brand, kind, audience FROM campaign_shop_link_audience WHERE campaign_id = $1`, camp); err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s/%s=%d", r.Brand, r.Kind, r.Audience))
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

func trackLinkBody(exprs ...string) string {
	var b strings.Builder
	b.WriteString("<p>Hello</p>")
	for _, e := range exprs {
		fmt.Fprintf(&b, `<a href="{{ TrackLink "%s" . }}">shop</a>`, e)
	}
	return b.String()
}

// TestShopLinkAudienceSnapshot is I1 and I2.
func TestShopLinkAudienceSnapshot(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db

	var buf bytes.Buffer
	st := newManagerStore(h.q, h.app.core, nil, log.New(&buf, "", 0))

	newList := func(name string) int {
		var id int
		if err := db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), $1, 'public', 'single', '{brand:ruze}') RETURNING id`, name); err != nil {
			t.Fatal(err)
		}
		return id
	}
	l1, l2 := newList("Ruze A"), newList("Ruze B")

	newSub := func(email, status, attribs string, lists map[int]string) int {
		var id int
		if err := db.Get(&id, `INSERT INTO subscribers (uuid, email, name, status, attribs) VALUES (gen_random_uuid(), $1, 'S', $2::subscriber_status, $3::JSONB) RETURNING id`, email, status, attribs); err != nil {
			t.Fatal(err)
		}
		for l, s := range lists {
			db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, $3::subscription_status)`, id, l, s)
		}
		return id
	}
	entry := func(brand, kind string) string {
		return fmt.Sprintf(`{"shop_links":{%q:{"url":"https://x.test/%s","kind":%q}}}`, brand, kind, kind)
	}
	on1 := map[int]string{l1: "confirmed"}
	newSub("self@x", "enabled", entry("ruze", "self"), on1)
	newSub("creator@x", "enabled", entry("ruze", "creator"), on1)
	newSub("founder@x", "enabled", `{"shop_links":{"ruze":{"url":"https://x.test/f","kind":"founder"},"thirstygirl":{"url":"https://x.test/c","kind":"corporate"}}}`, on1)
	newSub("corp@x", "enabled", entry("ruze", "corporate"), on1)
	newSub("none@x", "enabled", `{}`, on1)
	newSub("emptykind@x", "enabled", `{"shop_links":{"ruze":{"url":"https://x.test/e","kind":""}}}`, on1)
	// Outside the claim predicate: blocklisted, and unsubscribed from the list. Both hold self, so a
	// leak would read self=2 or more.
	newSub("blocked@x", "blocklisted", entry("ruze", "self"), on1)
	newSub("unsub@x", "enabled", entry("ruze", "self"), map[int]string{l1: "unsubscribed"})
	// F2: on both of campaign A's lists, counted once.
	newSub("both@x", "enabled", entry("ruze", "founder"), map[int]string{l1: "confirmed", l2: "unconfirmed"})

	newCamp := func(name, body string, evergreen bool, lists ...int) int {
		var id int
		if err := db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, content_type, messenger, status, evergreen, template_id)
			VALUES (gen_random_uuid(), $1, 's', 'f@x', $2, 'richtext', 'email', 'running', $3, (SELECT id FROM templates LIMIT 1)) RETURNING id`, name, body, evergreen); err != nil {
			t.Fatal(err)
		}
		for _, l := range lists {
			db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'L')`, id, l)
		}
		return id
	}
	ruze := `{{ .Subscriber.Attribs.shop_links.ruze.url }}`
	tg := `{{ .Subscriber.Attribs.shop_links.thirstygirl.url }}`
	campA := newCamp("A", trackLinkBody(ruze, `https://curatedfor.you/static`), false, l1, l2)
	campB := newCamp("B", trackLinkBody(tg, ruze), false, l1)                                     // I2: two brands
	campC := newCamp("C", trackLinkBody(`{{ .Subscriber.Attribs.rewards_site_url }}`), false, l1) // I2: no shop link
	campE := newCamp("E", trackLinkBody(ruze), true, l1)                                          // I1/F1: evergreen

	// Stage 4 F1 -- the claim predicate's other two arms. (a) A double-opt-in list: only the
	// confirmed member is in the audience. (b) A French campaign: only the fr subscriber is.
	var lDouble int
	if err := db.Get(&lDouble, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Ruze double', 'public', 'double', '{brand:ruze}') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	newSub("dconf@x", "enabled", entry("ruze", "self"), map[int]string{lDouble: "confirmed"})
	newSub("dunconf@x", "enabled", entry("ruze", "founder"), map[int]string{lDouble: "unconfirmed"})
	campD := newCamp("D", trackLinkBody(ruze), false, lDouble)

	lLang := newList("Ruze lang")
	newSub("fr@x", "enabled", `{"lang":"fr","shop_links":{"ruze":{"url":"https://x.test/c","kind":"creator"}}}`, map[int]string{lLang: "confirmed"})
	newSub("en@x", "enabled", `{"lang":"en","shop_links":{"ruze":{"url":"https://x.test/s","kind":"self"}}}`, map[int]string{lLang: "confirmed"})
	campF := newCamp("F", trackLinkBody(ruze), false, lLang)
	db.MustExec(`UPDATE campaigns SET attribs = '{"lang":"fr"}' WHERE id = $1`, campF)

	// paritySum asserts a campaign's per-brand snapshot sum equals the claim's to_send.
	paritySum := func(camp int, brand string) {
		t.Helper()
		var toSend, sum int
		db.Get(&toSend, `SELECT to_send FROM campaigns WHERE id = $1`, camp)
		db.Get(&sum, `SELECT COALESCE(SUM(audience), 0) FROM campaign_shop_link_audience WHERE campaign_id = $1 AND brand = $2`, camp, brand)
		if toSend != sum {
			t.Fatalf("campaign %d: to_send %d, %s snapshot sum %d (the claim and the snapshot predicates drifted)", camp, toSend, brand, sum)
		}
	}

	returned := func(ids []int) map[int]bool {
		out, err := st.NextCampaigns([]int64{}, []int64{})
		if err != nil {
			t.Fatalf("NextCampaigns: %v", err)
		}
		got := map[int]bool{}
		for _, c := range out {
			got[c.ID] = true
		}
		for _, id := range ids {
			if !got[id] {
				t.Fatalf("NextCampaigns did not return campaign %d (got %v)", id, got)
			}
		}
		return got
	}

	// First claim.
	returned([]int{campA, campB, campC, campE})
	if got, want := audienceRows(t, h, campA), "ruze/corporate=1 ruze/creator=1 ruze/founder=2 ruze/none=2 ruze/self=1"; got != want {
		t.Fatalf("campaign A snapshot\n got  %s\n want %s", got, want)
	}
	// The per-brand sum equals the claim's to_send (F2).
	var toSend int
	db.Get(&toSend, `SELECT to_send FROM campaigns WHERE id = $1`, campA)
	if toSend != 7 {
		t.Fatalf("campaign A to_send = %d, want 7 (the snapshot's per-brand sum)", toSend)
	}
	if got, want := audienceRows(t, h, campB), "ruze/corporate=1 ruze/creator=1 ruze/founder=2 ruze/none=2 ruze/self=1 thirstygirl/corporate=1 thirstygirl/none=6"; got != want {
		t.Fatalf("campaign B snapshot (two brands)\n got  %s\n want %s", got, want)
	}
	paritySum(campB, "ruze")
	paritySum(campB, "thirstygirl")
	if got, want := audienceRows(t, h, campD), "ruze/self=1"; got != want {
		t.Fatalf("campaign D snapshot (double opt-in)\n got  %s\n want %s", got, want)
	}
	paritySum(campD, "ruze")
	if got, want := audienceRows(t, h, campF), "ruze/creator=1"; got != want {
		t.Fatalf("campaign F snapshot (lang fr)\n got  %s\n want %s", got, want)
	}
	paritySum(campF, "ruze")
	if got := audienceRows(t, h, campC); got != "" {
		t.Fatalf("campaign C (no shop link) has a snapshot: %s", got)
	}
	if got := audienceRows(t, h, campE); got != "" {
		t.Fatalf("evergreen campaign E has a snapshot: %s", got)
	}
	if buf.Len() != 0 {
		t.Fatalf("first claim logged: %s", buf.String())
	}

	var takenA time.Time
	db.Get(&takenA, `SELECT MIN(taken_at) FROM campaign_shop_link_audience WHERE campaign_id = $1`, campA)

	// A re-claim (the campaigns excluded from current again) after the audience changed: the
	// original rows and taken_at stay, and no error line was swallowed (F6).
	newSub("late@x", "enabled", entry("ruze", "self"), on1)
	returned([]int{campA, campB, campC, campE})
	if got, want := audienceRows(t, h, campA), "ruze/corporate=1 ruze/creator=1 ruze/founder=2 ruze/none=2 ruze/self=1"; got != want {
		t.Fatalf("re-claim changed campaign A's snapshot\n got  %s\n want %s", got, want)
	}
	var takenA2 time.Time
	var nA int
	db.Get(&takenA2, `SELECT MAX(taken_at) FROM campaign_shop_link_audience WHERE campaign_id = $1`, campA)
	db.Get(&nA, `SELECT COUNT(*) FROM campaign_shop_link_audience WHERE campaign_id = $1 AND taken_at <> $2`, campA, takenA)
	if !takenA2.Equal(takenA) || nA != 0 {
		t.Fatalf("re-claim moved taken_at: first %v, now %v (%d rows differ)", takenA, takenA2, nA)
	}
	if buf.Len() != 0 {
		t.Fatalf("re-claim logged a snapshot error: %s", buf.String())
	}

	// I2: a snapshot failure (the table dropped) is logged and NextCampaigns still returns the
	// campaigns. Only the four shop-link broadcasts log; the evergreen and the no-shop-link body
	// run no snapshot query at all (a query would fail and log here).
	db.MustExec(`DROP TABLE campaign_shop_link_audience`)
	returned([]int{campA, campB, campC, campE})
	logged := buf.String()
	for _, id := range []int{campA, campB, campD, campF} {
		if !strings.Contains(logged, fmt.Sprintf("campaign %d ", id)) {
			t.Fatalf("snapshot failure for campaign %d not logged: %q", id, logged)
		}
	}
	for _, id := range []int{campC, campE} {
		if strings.Contains(logged, fmt.Sprintf("campaign %d ", id)) {
			t.Fatalf("campaign %d ran a snapshot query: %q", id, logged)
		}
	}
	if n := strings.Count(logged, "error snapshotting shop-link audience"); n != 4 {
		t.Fatalf("%d snapshot error lines, want 4: %q", n, logged)
	}
}

// TestShopLinkAudienceBrandPatternSQL runs ShopLinkBrandPattern through Postgres' SUBSTRING (the
// click query's extraction) and Go's ShopLinkBrands over the same expressions: the two agree.
func TestShopLinkAudienceBrandPatternSQL(t *testing.T) {
	h := newLinkHarness(t)
	for _, e := range []string{
		`{{ .Subscriber.Attribs.shop_links.ruze.url }}`,
		`{{ index .Subscriber.Attribs.shop_links "ruze" }}`,
		`{{ .Subscriber.Attribs.shop_links.thirstygirl.url }}`,
		`{{ or .Subscriber.Attribs.shop_links.thirstygirl.url "https://thirstygirl.test" }}`,
		`https://ruze.test/?ref={{ .Subscriber.Attribs.shop_links.ruze.slug }}`,
		`{{ (index .Subscriber.Attribs.shop_links "brand-2_x").url }}`,
		`{{ .Subscriber.Attribs.rewards_site_url }}`,
	} {
		var sqlBrand *string
		if err := h.db.Get(&sqlBrand, `SELECT SUBSTRING($1::TEXT FROM $2::TEXT)`, e, linkresolve.ShopLinkBrandPattern); err != nil {
			t.Fatal(err)
		}
		goBrands := linkresolve.ShopLinkBrands([]string{e})
		got := ""
		if sqlBrand != nil {
			got = *sqlBrand
		}
		want := ""
		if len(goBrands) == 1 {
			want = goBrands[0]
		}
		if got != want {
			t.Fatalf("%q: SQL brand %q, Go brand %q", e, got, want)
		}
	}
}
