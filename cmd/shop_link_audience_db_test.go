package main

// Fork (shop-link rate) -- integrations SHOP-LINK-RATE-SPEC I1/I2 against a real database, driving
// the manager's store directly: store.NextCampaigns claims the running broadcasts and snapshots each
// one's audience by shop-link kind per brand its body names (the claim's predicate, one count per
// subscriber), exactly once; an evergreen and a body without a shop-link expression run no snapshot
// query; a snapshot failure is logged and swallowed. Shares newLinkHarness (LISTMONK_TEST_PG opt-in;
// see link_redirect_db_test.go).
//
// Fork (shop-link frozen) -- integrations SHOP-LINK-FROZEN-SPEC I1/I6/I7/I8. The same claim also
// writes one campaign_shop_link_recipients row per audience member per brand (one statement, so the
// summary is the recipients' GROUP BY); the snapshot now runs in a goroutine, so every assertion
// follows st.snapshots.Wait() after NextCampaigns; and the goroutine's recover keeps a panic in the
// snapshot from taking the process down.

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

// recipientRows reads a campaign's per-recipient snapshot as sorted "email/brand/kind" triples.
// Fork (shop-link frozen, integrations SHOP-LINK-FROZEN-SPEC I1/I6).
func recipientRows(t *testing.T, h *linkHarness, camp int) string {
	t.Helper()
	var rows []struct {
		Email string `db:"email"`
		Brand string `db:"brand"`
		Kind  string `db:"kind"`
	}
	if err := h.db.Select(&rows, `SELECT COALESCE(s.email, '#' || r.subscriber_id::TEXT) AS email, r.brand, r.kind
		FROM campaign_shop_link_recipients r LEFT JOIN subscribers s ON s.id = r.subscriber_id
		WHERE r.campaign_id = $1`, camp); err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s/%s/%s", r.Email, r.Brand, r.Kind))
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// assertSummaryIsRecipientsGroupBy is I7: for every campaign with a snapshot, the summary rows
// equal SELECT brand, kind, COUNT(*) FROM recipients GROUP BY brand, kind, in both directions.
func assertSummaryIsRecipientsGroupBy(t *testing.T, h *linkHarness) {
	t.Helper()
	var diff []string
	if err := h.db.Select(&diff, `
		WITH g AS (
			SELECT campaign_id, brand, kind, COUNT(*)::INT AS audience FROM campaign_shop_link_recipients
				GROUP BY campaign_id, brand, kind
		),
		a AS (SELECT campaign_id, brand, kind, audience FROM campaign_shop_link_audience)
		SELECT COALESCE(a.campaign_id, g.campaign_id)::TEXT || '/' || COALESCE(a.brand, g.brand) || '/' ||
			COALESCE(a.kind, g.kind) || ' summary=' || COALESCE(a.audience::TEXT, '-') || ' recipients=' || COALESCE(g.audience::TEXT, '-')
		FROM a FULL OUTER JOIN g ON (a.campaign_id = g.campaign_id AND a.brand = g.brand AND a.kind = g.kind)
		WHERE a.audience IS DISTINCT FROM g.audience`); err != nil {
		t.Fatal(err)
	}
	if len(diff) != 0 {
		t.Fatalf("summary is not the recipients' GROUP BY: %v", diff)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM campaign_shop_link_audience`)
	if n == 0 {
		t.Fatal("I7 vacuous: no summary rows in the fixture")
	}
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
	st := newManagerStore(h.q, h.app.core, nil, db, log.New(&buf, "", 0))

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
		// Fork (shop-link frozen, SHOP-LINK-FROZEN-SPEC D3) -- the snapshots run in goroutines.
		st.snapshots.Wait()
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

	// Fork (shop-link frozen, SHOP-LINK-FROZEN-SPEC I1) -- the same claim wrote one recipient row per
	// audience member with the kind held at that moment (both@x once despite two lists; the
	// blocklisted and the unsubscribed have none).
	wantA := "both@x/ruze/founder corp@x/ruze/corporate creator@x/ruze/creator emptykind@x/ruze/none founder@x/ruze/founder none@x/ruze/none self@x/ruze/self"
	if got := recipientRows(t, h, campA); got != wantA {
		t.Fatalf("campaign A recipients\n got  %s\n want %s", got, wantA)
	}
	// I6: two brands in a body -> two recipient rows per recipient.
	wantB := "both@x/ruze/founder both@x/thirstygirl/none corp@x/ruze/corporate corp@x/thirstygirl/none " +
		"creator@x/ruze/creator creator@x/thirstygirl/none emptykind@x/ruze/none emptykind@x/thirstygirl/none " +
		"founder@x/ruze/founder founder@x/thirstygirl/corporate none@x/ruze/none none@x/thirstygirl/none " +
		"self@x/ruze/self self@x/thirstygirl/none"
	if got := recipientRows(t, h, campB); got != wantB {
		t.Fatalf("campaign B recipients (two brands)\n got  %s\n want %s", got, wantB)
	}
	if got, want := recipientRows(t, h, campD), "dconf@x/ruze/self"; got != want {
		t.Fatalf("campaign D recipients (double opt-in)\n got  %s\n want %s", got, want)
	}
	if got, want := recipientRows(t, h, campF), "fr@x/ruze/creator"; got != want {
		t.Fatalf("campaign F recipients (lang fr)\n got  %s\n want %s", got, want)
	}
	if got := recipientRows(t, h, campC); got != "" {
		t.Fatalf("campaign C (no shop link) has recipient rows: %s", got)
	}
	if got := recipientRows(t, h, campE); got != "" {
		t.Fatalf("evergreen campaign E has recipient rows: %s", got)
	}
	// I7: the summary is the recipients' GROUP BY for every snapshotted campaign.
	assertSummaryIsRecipientsGroupBy(t, h)

	var takenA time.Time
	db.Get(&takenA, `SELECT MIN(taken_at) FROM campaign_shop_link_audience WHERE campaign_id = $1`, campA)

	// A re-claim (the campaigns excluded from current again) after the audience changed: the
	// original rows and taken_at stay, and no error line was swallowed (F6).
	newSub("late@x", "enabled", entry("ruze", "self"), on1)
	// Fork (shop-link frozen, SHOP-LINK-FROZEN-SPEC I1) -- and another recipient's kind changes.
	db.MustExec(`UPDATE subscribers SET attribs = $1::JSONB WHERE email = 'self@x'`, entry("ruze", "corporate"))
	returned([]int{campA, campB, campC, campE})
	if got, want := audienceRows(t, h, campA), "ruze/corporate=1 ruze/creator=1 ruze/founder=2 ruze/none=2 ruze/self=1"; got != want {
		t.Fatalf("re-claim changed campaign A's snapshot\n got  %s\n want %s", got, want)
	}
	if got := recipientRows(t, h, campA); got != wantA {
		t.Fatalf("re-claim changed campaign A's recipients\n got  %s\n want %s", got, wantA)
	}
	if got := recipientRows(t, h, campB); got != wantB {
		t.Fatalf("re-claim changed campaign B's recipients\n got  %s\n want %s", got, wantB)
	}
	if got := recipientRows(t, h, campE); got != "" {
		t.Fatalf("re-claim gave evergreen campaign E recipient rows: %s", got)
	}
	assertSummaryIsRecipientsGroupBy(t, h)
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
	// Stage 4 F2 -- the failed statement rolled back whole: campaign A's recipient rows are unchanged.
	if got := recipientRows(t, h, campA); got != wantA {
		t.Fatalf("failed re-claim changed campaign A's recipients\n got  %s\n want %s", got, wantA)
	}
}

// TestShopLinkAudiencePanicRecovered is SHOP-LINK-FROZEN-SPEC I8: a panic inside the snapshot
// goroutine (the snapshot statement nil-ed on a COPY of the queries struct, so the transaction's
// Stmtx dereferences nil) is recovered and logged with the campaign; NextCampaigns returns the
// campaign, snapshots.Wait() returns, and this test is still running to assert it.
func TestShopLinkAudiencePanicRecovered(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db

	q := *h.q
	q.SnapshotCampaignShopLinkAudience = nil
	var buf bytes.Buffer
	st := newManagerStore(&q, h.app.core, nil, db, log.New(&buf, "", 0))

	var list, sub, camp int
	if err := db.Get(&list, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Ruze', 'public', 'single', '{brand:ruze}') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&sub, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 'p@x', 'S', '{"shop_links":{"ruze":{"url":"https://x.test/s","kind":"self"}}}') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, sub, list)
	if err := db.Get(&camp, `INSERT INTO campaigns (uuid, name, subject, from_email, body, content_type, messenger, status, template_id)
		VALUES (gen_random_uuid(), 'Panicky', 's', 'f@x', $1, 'richtext', 'email', 'running', (SELECT id FROM templates LIMIT 1)) RETURNING id`,
		trackLinkBody(`{{ .Subscriber.Attribs.shop_links.ruze.url }}`)); err != nil {
		t.Fatal(err)
	}
	db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'L')`, camp, list)

	out, err := st.NextCampaigns([]int64{}, []int64{})
	if err != nil {
		t.Fatalf("NextCampaigns: %v", err)
	}
	found := false
	for _, c := range out {
		found = found || c.ID == camp
	}
	if !found {
		t.Fatalf("NextCampaigns did not return campaign %d", camp)
	}
	st.snapshots.Wait()

	logged := buf.String()
	if want := fmt.Sprintf("panic snapshotting shop-link audience for campaign %d (Panicky)", camp); !strings.Contains(logged, want) {
		t.Fatalf("recover did not log %q: %q", want, logged)
	}
	if !strings.Contains(logged, "goroutine ") {
		t.Fatalf("recover log carries no stack: %q", logged)
	}
	var n int
	db.Get(&n, `SELECT (SELECT COUNT(*) FROM campaign_shop_link_audience) + (SELECT COUNT(*) FROM campaign_shop_link_recipients)`)
	if n != 0 {
		t.Fatalf("%d snapshot rows written after a panic, want 0", n)
	}
}

// TestShopLinkSnapshotNoCampaignRace is SHOP-LINK-FROZEN Stage 4 F1: the manager goes on to
// newPipe(c) -> LoadInlineImages, which assigns c.Body on the *models.Campaign NextCampaigns
// returned, while the snapshot goroutine runs. The goroutine must hold no pointer into the campaign,
// so writing the returned campaign's Body and ContentType before Wait() is race-free (run with -race).
func TestShopLinkSnapshotNoCampaignRace(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db
	st := newManagerStore(h.q, h.app.core, nil, db, log.New(&bytes.Buffer{}, "", 0))

	var list, sub, camp int
	if err := db.Get(&list, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Ruze', 'public', 'single', '{brand:ruze}') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&sub, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 'r@x', 'S', '{"shop_links":{"ruze":{"url":"https://x.test/s","kind":"self"}}}') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, sub, list)
	if err := db.Get(&camp, `INSERT INTO campaigns (uuid, name, subject, from_email, body, content_type, messenger, status, template_id)
		VALUES (gen_random_uuid(), 'Racy', 's', 'f@x', $1, 'richtext', 'email', 'running', (SELECT id FROM templates LIMIT 1)) RETURNING id`,
		trackLinkBody(`{{ .Subscriber.Attribs.shop_links.ruze.url }}`)); err != nil {
		t.Fatal(err)
	}
	db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'L')`, camp, list)

	out, err := st.NextCampaigns([]int64{}, []int64{})
	if err != nil {
		t.Fatalf("NextCampaigns: %v", err)
	}
	// What LoadInlineImages does on the same pointer, before the snapshot has finished.
	for _, c := range out {
		c.Body = c.Body + " "
		c.ContentType = "html"
	}
	st.snapshots.Wait()

	if got := recipientRows(t, h, camp); got != "r@x/ruze/self" {
		t.Fatalf("recipients = %q, want r@x/ruze/self", got)
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
