package main

// Fork (promo series) -- integrations listmonk/PROMO-SERIES-SPEC T1-T7, T9, T10 and T13 against a
// real database, through the REAL statements: promo_email_hash, promo_applicable_tags,
// promo_campaign_tags, promo_matching_orders, promo_feed_stale and promo_purchasers (schema.sql), the
// claim (next-campaigns, through store.NextCampaigns / the claim path), the batches
// (next-campaign-subscribers), get-campaign-lang-audience (core.CampaignLangAudience / FillAudience),
// get-campaign-silent-audience, get-campaign-promo-audience (the GET handler), the feed routes and
// get-campaign-matrix / query-analytics-campaigns. Shares newSilentWorld / newLinkHarness / ssFixture
// (LISTMONK_TEST_PG opt-in). T1 runs the claim and the batches through the statements as they were
// before this change (cmd/testdata/*.pre-promo.sql, verbatim from the previous commit) beside the
// current ones.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// buy writes one feed row the way the upsert does: value_lc = LOWER(value), the buyer as
// promo_email_hash(email), ordered `ago` before now, received now.
func (w *silentWorld) buy(order, kind, value, email, ago string) {
	w.f.t.Helper()
	w.h.db.MustExec(`INSERT INTO promo_purchases (order_id, kind, value, value_lc, email_hash, ordered_at, received_at)
		VALUES ($1, $2, $3, LOWER($3), promo_email_hash($4), NOW() - $5::INTERVAL, NOW())`, order, kind, value, email, ago)
}

// feedAge moves every feed row's received_at to `ago` before now (the staleness clock).
func (w *silentWorld) feedAge(ago string) {
	w.f.t.Helper()
	w.h.db.MustExec(`UPDATE promo_purchases SET received_at = NOW() - $1::INTERVAL`, ago)
}

// purchasers is promo_purchasers(camp, promo_campaign_tags(camp)) as sorted ids.
func (w *silentWorld) purchasers(camp int) []int {
	w.f.t.Helper()
	ids := []int{}
	if err := w.h.db.Select(&ids, `SELECT subscriber_id FROM promo_purchasers($1, promo_campaign_tags($1)) ORDER BY subscriber_id`, camp); err != nil {
		w.f.t.Fatalf("promo_purchasers(%d): %v", camp, err)
	}
	return ids
}

type promoPreview struct {
	Orders  int  `json:"orders"`
	Skipped int  `json:"skipped"`
	Stale   bool `json:"stale"`
}

// promoCall runs GET /api/campaigns/:id/promo-audience as u through the route's permission wrapper
// (handlers.go) with the query (url.Values encoded) -- the status and the body.
func promoCall(t *testing.T, h *linkHarness, u auth.User, camp int, q url.Values) (int, string) {
	t.Helper()
	if h.app.auth == nil {
		h.app.auth = &auth.Auth{}
	}
	e := echo.New()
	path := "/api/campaigns/" + itoa(camp) + "/promo-audience"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, path, nil), rec)
	c.Set("id", camp)
	c.Set(auth.UserHTTPCtxKey, u)
	if err := h.app.auth.Perm(h.app.GetCampaignPromoAudience, "campaigns:manage_all", "campaigns:manage")(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("promo-audience: %v", err)
	}
	return rec.Code, rec.Body.String()
}

func promoPreviewOf(t *testing.T, h *linkHarness, camp int, q url.Values) promoPreview {
	t.Helper()
	code, body := promoCall(t, h, perms("editor", auth.PermCampaignsManageAll), camp, q)
	if code != http.StatusOK {
		t.Fatalf("promo-audience %v: %d %s", q, code, body)
	}
	var r struct {
		Data promoPreview `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	return r.Data
}

func silentAudienceIDs(t *testing.T, h *linkHarness, camp int) int {
	t.Helper()
	a, err := h.app.core.CampaignSilentAudience(camp)
	if err != nil {
		t.Fatalf("silent audience: %v", err)
	}
	return a.Skipped
}

const promoTags = `["deal-3", "bfcm*"]`

func promoAttribs(code, step, tags string, silent bool) string {
	parts := []string{}
	if code != "" {
		parts = append(parts, `"promo_code": `+code)
	}
	if step != "" {
		parts = append(parts, `"promo_step": `+step)
	}
	if tags != "" {
		parts = append(parts, `"promo_tags": `+tags)
	}
	if silent {
		parts = append(parts, `"silent_skip": true`)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// skipKeys is a campaign's skip rows as "sub:reason:journey:due-list" text (the recorded times are
// the claims' own and differ between two claims of identical campaigns only by microseconds).
func skipKeys(t *testing.T, h *linkHarness, camp int) string {
	t.Helper()
	out := []string{}
	for _, r := range skipRowsOf(t, h, camp) {
		k := fmt.Sprintf("%d:", r.Sub)
		if r.Reason != nil {
			k += *r.Reason
		}
		if r.Journey != nil {
			k += fmt.Sprintf(":j%d", *r.Journey)
		}
		if r.DueList != nil {
			k += fmt.Sprintf(":d%d", *r.DueList)
		}
		out = append(out, k)
	}
	return strings.Join(out, " ")
}

// --- T1 (I1) -- the differential over the real claim path ---

func TestPromoDifferential(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	old := claimPath{
		nextCampaigns:   preSilentStmt(t, h.db, "next-campaigns.pre-promo.sql", "next-campaigns"),
		nextSubscribers: preSilentStmt(t, h.db, "next-campaign-subscribers.pre-promo.sql", "next-campaign-subscribers"),
	}
	cur := claimPath{nextCampaigns: h.q.NextCampaigns, nextSubscribers: h.q.NextCampaignSubscribers}

	double := f.list("Acme VIP", "brand:acme")
	h.db.MustExec(`UPDATE lists SET optin = 'double' WHERE id = $1`, double)
	buyerA := w.member("buyer-a@acme.test")
	buyerB := w.member("buyer.b@gmail.com")
	silentBuyer := w.member("silent-buyer@gmail.com", w.three)
	silent := w.member("silent@gmail.com", w.three)
	fr := w.member("fr@gmail.com")
	h.db.MustExec(`UPDATE subscribers SET attribs = '{"lang": "fr"}' WHERE id = $1`, fr)
	plain := w.member("plain@acme.test")
	smart := w.member("smart@gmail.com")
	journeyList := f.list("Acme first order", "brand:acme")
	f.send(f.camp("Acme thanks", true, "regular", "running", `{}`, journeyList), smart, "1 hour")
	heldSub := w.member("held@gmail.com", w.three)
	h.db.MustExec(`UPDATE subscriber_lists SET status = 'unsubscribed', meta = '{"hold": {"reason": "never-engaged"}}' WHERE subscriber_id = $1 AND list_id = $2`, heldSub, w.three)
	h.db.MustExec(`UPDATE settings SET value = 'true' WHERE key = 'app.brand_hold_scope'`)
	dblConfirmed := f.sub("dbl@acme.test", "enabled")
	f.join(dblConfirmed, double, "confirmed")
	dblPending := f.sub("pending@acme.test", "enabled")
	f.join(dblPending, double, "unconfirmed")
	// The feed is fresh and makes buyerA, buyerB (googlemail, dots), silentBuyer, smart, dblConfirmed
	// purchasers under deal-3, bfcm*.
	w.buy("o1", "promotion", "Deal-3", "buyer-a@acme.test", "2 days")
	w.buy("o2", "sku", "BFCM-Box", "buyerb@googlemail.com", "3 days")
	w.buy("o3", "promotion", "DEAL-3", "silent-buyer@gmail.com", "1 day")
	w.buy("o4", "promotion", "deal-3", "smart@gmail.com", "1 day")
	w.buy("o5", "sku", "bfcm-bundle", "dbl@acme.test", "1 day")

	cases := []string{
		`{}`,
		promoAttribs(`"BFCM-D3"`, "", "", false),
		promoAttribs(`"BFCM-D3"`, "1", promoTags, false),
		promoAttribs(`"BFCM-D3"`, "2", "", false),
		promoAttribs("", "2", promoTags, false), // step 2 + tags with NO Code, written by SQL
	}
	for _, c := range cases[:5] {
		var a map[string]any
		json.Unmarshal([]byte(c), &a)
		a["silent_skip"] = true
		b, _ := json.Marshal(a)
		cases = append(cases, string(b))
	}

	// The fixture: a step-2 campaign with the same tags WOULD skip the purchasers.
	probe := f.camp("Probe", false, "regular", "draft", promoAttribs(`"BFCM-D3"`, "2", promoTags, false), w.target, double)
	if p := w.purchasers(probe); !has(p, buyerA) || !has(p, buyerB) || !has(p, silentBuyer) || !has(p, dblConfirmed) {
		t.Fatalf("fixture: purchasers %v lack %d %d %d %d", p, buyerA, buyerB, silentBuyer, dblConfirmed)
	}
	h.db.MustExec(`UPDATE campaigns SET status = 'cancelled' WHERE id = $1`, probe)

	for _, attribs := range cases {
		a := f.camp("Old "+attribs, false, "regular", "running", attribs, w.target, double)
		b := f.camp("New "+attribs, false, "regular", "running", attribs, w.target, double)
		var all []int
		h.db.Select(&all, `SELECT id FROM campaigns WHERE id NOT IN ($1, $2)`, a, b)
		gotOld := old.drain(t, h, a, append(append([]int{}, all...), b))
		gotNew := cur.drain(t, h, b, append(append([]int{}, all...), a))
		if fmt.Sprint(gotOld) != fmt.Sprint(gotNew) {
			t.Fatalf("%s: recipients differ\n old %v\n new %v", attribs, gotOld, gotNew)
		}
		// The purchasers are sent; the Smart Sending skip, the held and the pending are not.
		if !has(gotNew, buyerA) || !has(gotNew, buyerB) || !has(gotNew, plain) || !has(gotNew, dblConfirmed) ||
			has(gotNew, smart) || has(gotNew, heldSub) || has(gotNew, dblPending) {
			t.Fatalf("%s: recipients %v", attribs, gotNew)
		}
		silentOn := strings.Contains(attribs, "silent_skip")
		if has(gotNew, silent) == silentOn || has(gotNew, silentBuyer) == silentOn {
			t.Fatalf("%s: silent members %d %d sent %v %v", attribs, silent, silentBuyer, has(gotNew, silent), has(gotNew, silentBuyer))
		}
		var toOld, toNew int
		h.db.Get(&toOld, `SELECT to_send FROM campaigns WHERE id = $1`, a)
		h.db.Get(&toNew, `SELECT to_send FROM campaigns WHERE id = $1`, b)
		if toOld != toNew || toNew != len(gotNew) {
			t.Fatalf("%s: to_send old %d new %d, recipients %d", attribs, toOld, toNew, len(gotNew))
		}
		if o, n := skipKeys(t, h, a), skipKeys(t, h, b); o != n {
			t.Fatalf("%s: skip rows differ\n old %s\n new %s", attribs, o, n)
		} else if strings.Contains(n, "purchased") {
			t.Fatalf("%s: a purchased row was written: %s", attribs, n)
		}
		h.db.MustExec(`UPDATE campaigns SET status = 'cancelled' WHERE id IN ($1, $2)`, a, b)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM campaign_smart_skips WHERE reason = 'purchased'`)
	if n != 0 {
		t.Fatalf("%d purchased rows exist", n)
	}
}

// --- T2 (I2, I16) -- applicability ---

func TestPromoApplicability(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	buyer := w.member("buyer@acme.test")
	w.member("other@acme.test")
	w.buy("o1", "promotion", "Deal-3", "buyer@acme.test", "1 day")

	skips := map[string]int{
		"step 2 + tags": f.camp("S2", false, "regular", "draft", promoAttribs(`"C"`, "2", promoTags, false), w.target),
		"step 3 + tags": f.camp("S3", false, "regular", "draft", promoAttribs(`"C"`, "3", promoTags, false), w.target),
	}
	none := map[string]int{
		"step 1 + tags":           f.camp("S1", false, "regular", "draft", promoAttribs(`"C"`, "1", promoTags, false), w.target),
		"blank step + tags":       f.camp("SB", false, "regular", "draft", promoAttribs(`"C"`, "", promoTags, false), w.target),
		"step 2, tags absent":     f.camp("SA", false, "regular", "draft", promoAttribs(`"C"`, "2", "", false), w.target),
		"step 2, tags []":         f.camp("SE", false, "regular", "draft", promoAttribs(`"C"`, "2", `[]`, false), w.target),
		"step 2 + tags, no Code":  f.camp("SN", false, "regular", "draft", promoAttribs("", "2", promoTags, false), w.target),
		"step 2 + tags, Code ''":  f.camp("SZ", false, "regular", "draft", promoAttribs(`" "`, "2", promoTags, false), w.target),
		"evergreen step 2 + tags": f.camp("EV", true, "regular", "draft", promoAttribs(`"C"`, "2", promoTags, false), w.target),
		"opt-in step 2 + tags":    f.camp("OP", false, "optin", "draft", promoAttribs(`"C"`, "2", promoTags, false), w.target),
		"no promo attribs":        f.camp("NO", false, "regular", "draft", `{}`, w.target),
	}
	for name, id := range skips {
		if p := w.purchasers(id); len(p) != 1 || p[0] != buyer {
			t.Errorf("%s: purchasers %v, want [%d]", name, p, buyer)
		}
		a, err := h.app.core.CampaignLangAudience(id)
		if err != nil || a.PurchasedSkipped != 1 || a.PromoStale {
			t.Errorf("%s: lang audience %+v %v, want purchased 1", name, a, err)
		}
	}
	for name, id := range none {
		if p := w.purchasers(id); len(p) != 0 {
			t.Errorf("%s: purchasers %v, want none", name, p)
		}
		a, err := h.app.core.CampaignLangAudience(id)
		if err != nil || a.PurchasedSkipped != 0 || a.PromoStale {
			t.Errorf("%s: lang audience %+v %v, want purchased 0", name, a, err)
		}
	}
	// The claim agrees: the step-2 broadcast records the purchaser, the step-1 one does not.
	h.db.MustExec(`UPDATE campaigns SET status = 'cancelled' WHERE status = 'draft'`)
	s2 := f.camp("Run S2", false, "regular", "running", promoAttribs(`"C"`, "2", promoTags, false), w.target)
	s1 := f.camp("Run S1", false, "regular", "running", promoAttribs(`"C"`, "1", promoTags, false), w.target)
	st := &store{queries: h.q, core: h.app.core}
	if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if r := skipRowsOf(t, h, s2); len(r) != 1 || r[0].Sub != buyer || r[0].Reason == nil || *r[0].Reason != "purchased" ||
		r[0].Journey != nil || r[0].DueList != nil || r[0].Sent.Valid || r[0].DueAt.Valid {
		t.Fatalf("step 2 rows %+v, want one purchased row for %d with the pair columns NULL", r, buyer)
	}
	if r := skipRowsOf(t, h, s1); len(r) != 0 {
		t.Fatalf("step 1 rows %+v", r)
	}
}

// --- T3 (I3) -- the rule, one member per flipped condition ---

func TestPromoRule(t *testing.T) {
	w := newSilentWorld(t)
	f := w.f
	type row struct{ sub, kind, value, buyer, ago string }
	skipped := map[string]row{
		"promotion equal, case differs":  {"p1@acme.test", "promotion", "BFCM-DEAL-3", "p1@acme.test", "1 day"},
		"line-item SKU, a * prefix":      {"p2@acme.test", "sku", "BFCM-Box-1", "p2@acme.test", "1 day"},
		"bundle SKU, a * prefix":         {"p3@acme.test", "sku", "bfcm-bundle-xl", "p3@acme.test", "1 day"},
		"an order 59 days old":           {"p4@acme.test", "promotion", "bfcm-deal-3", "p4@acme.test", "59 days"},
		"Gmail address with dots":        {"jane.doe@gmail.com", "promotion", "bfcm-deal-3", "JaneDoe@gmail.com", "1 day"},
		"a googlemail.com buyer":         {"bob@gmail.com", "promotion", "bfcm-deal-3", "b.o.b@googlemail.com", "1 day"},
		"a % _ tag, literal match":       {"pct@acme.test", "promotion", "50%_off", "pct@acme.test", "1 day"},
		"a _ % prefix, literal match":    {"pre@acme.test", "sku", "ab_%123", "pre@acme.test", "1 day"},
		"an order 10 days old":           {"cx@acme.test", "promotion", "bfcm-deal-3", "cx@acme.test", "10 days"},
	}
	sent := map[string]row{
		"a promotion that only starts with the tag": {"s1@acme.test", "promotion", "bfcm-deal-3x", "s1@acme.test", "1 day"},
		"a SKU equal to a non-* tag":                {"s2@acme.test", "sku", "50%_off", "s2@acme.test", "1 day"},
		"a promotion matching a * tag's prefix":     {"s3@acme.test", "promotion", "bfcm-special", "s3@acme.test", "1 day"},
		"an order 61 days old":                      {"s4@acme.test", "promotion", "bfcm-deal-3", "s4@acme.test", "61 days"},
		"a +tag variant":                            {"carl+x@gmail.com", "promotion", "bfcm-deal-3", "carl@gmail.com", "1 day"},
		"% and _ are not patterns (promotion)":      {"s5@acme.test", "promotion", "50XYoff", "s5@acme.test", "1 day"},
		"% and _ are not patterns (SKU prefix)":     {"s6@acme.test", "sku", "abXY123", "s6@acme.test", "1 day"},
		"no order at all":                           {"s7@acme.test", "", "", "", ""},
	}
	ids := map[string]int{}
	i := 0
	for _, set := range []map[string]row{skipped, sent} {
		for name, r := range set {
			i++
			ids[name] = w.member(r.sub)
			if r.kind != "" {
				w.buy(fmt.Sprintf("o%d", i), r.kind, r.value, r.buyer, r.ago)
			}
		}
	}
	camp := f.camp("Mid", false, "regular", "draft", promoAttribs(`"C"`, "2", `["bfcm-deal-3", "bfcm*", "50%_off", "ab_%*"]`, false), w.target)
	got := w.purchasers(camp)
	for name := range skipped {
		if !has(got, ids[name]) {
			t.Errorf("%s (%d): sent, want skipped (%v)", name, ids[name], got)
		}
	}
	for name := range sent {
		if has(got, ids[name]) {
			t.Errorf("%s (%d): skipped, want sent", name, ids[name])
		}
	}
	if len(got) != len(skipped) {
		t.Errorf("purchasers %v, want exactly the %d skipped members", got, len(skipped))
	}
	// Any subscription status (the claim's audience decides who is mailable).
	unsub := w.member("unsub@acme.test")
	w.h.db.MustExec(`UPDATE subscriber_lists SET status = 'unsubscribed' WHERE subscriber_id = $1`, unsub)
	w.buy("ou", "promotion", "bfcm-deal-3", "unsub@acme.test", "1 day")
	if !has(w.purchasers(camp), unsub) {
		t.Errorf("an unsubscribed member who bought is not returned")
	}
	// promo_matching_orders: distinct orders; no rows for an empty array.
	var n int
	w.h.db.Get(&n, `SELECT COUNT(*) FROM promo_matching_orders(ARRAY[]::TEXT[])`)
	if n != 0 {
		t.Fatalf("empty tags: %d rows", n)
	}
	w.h.db.Get(&n, `SELECT COUNT(DISTINCT order_id) FROM promo_matching_orders(ARRAY['bfcm-deal-3'])`)
	// p1 (BFCM-DEAL-3), p4, jane, bob, cx, carl (the +tag buyer) and ou -- not s4 (61 days) or s1.
	if n != 7 {
		t.Fatalf("matching orders for bfcm-deal-3: %d, want 7", n)
	}
}

// --- T4 (I4) -- a missing or stale feed skips nobody ---

func TestPromoStaleFeed(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	buyer := w.member("buyer@gmail.com", w.three) // a purchaser AND silent
	w.member("plain@acme.test")
	attribs := promoAttribs(`"C"`, "2", promoTags, true)
	camp := f.camp("Mid", false, "regular", "draft", attribs, w.target)
	tagsQ := url.Values{"code": {"C"}, "step": {"2"}, "tags": {"deal-3,bfcm*"}}

	check := func(label string, wantSkip bool, wantOrders int) {
		t.Helper()
		if p := w.purchasers(camp); has(p, buyer) != wantSkip {
			t.Fatalf("%s: purchasers %v, want skip %v", label, p, wantSkip)
		}
		a, err := h.app.core.CampaignLangAudience(camp)
		if err != nil {
			t.Fatal(err)
		}
		wantP, wantS := 0, 1
		if wantSkip {
			wantP, wantS = 1, 0
		}
		if a.PurchasedSkipped != wantP || a.SilentSkipped != wantS || a.PromoStale == wantSkip {
			t.Fatalf("%s: purchased_skipped %d silent_skipped %d promo_stale %v, want %d / %d / %v", label, a.PurchasedSkipped, a.SilentSkipped, a.PromoStale, wantP, wantS, !wantSkip)
		}
		if n := silentAudienceIDs(t, h, camp); n != wantS {
			t.Fatalf("%s: silent-audience %d, want %d (a purchaser excluded only when the feed is current)", label, n, wantS)
		}
		pv := promoPreviewOf(t, h, camp, tagsQ)
		if pv.Stale == wantSkip || (pv.Skipped > 0) != wantSkip || pv.Orders != wantOrders {
			t.Fatalf("%s: preview %+v, want stale %v skipped>0 %v orders %d", label, pv, !wantSkip, wantSkip, wantOrders)
		}
	}
	check("empty feed", false, 0)
	w.buy("o1", "promotion", "deal-3", "buyer@gmail.com", "2 days")
	w.feedAge("25 hours")
	check("newest write 25 h old", false, 1)

	// The claim over the stale feed: no purchased row (the buyer is recorded silent instead).
	h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE id = $1`, camp)
	st := &store{queries: h.q, core: h.app.core}
	if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
		t.Fatal(err)
	}
	if r := skipRowsOf(t, h, camp); len(r) != 1 || r[0].Sub != buyer || *r[0].Reason != "silent" {
		t.Fatalf("claim over a stale feed: rows %+v, want one silent row", r)
	}
	h.db.MustExec(`UPDATE campaigns SET status = 'cancelled' WHERE id = $1`, camp)

	camp = f.camp("Mid 2", false, "regular", "draft", attribs, w.target)
	w.feedAge("23 hours")
	check("newest write 23 h old", true, 1)
	h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE id = $1`, camp)
	if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
		t.Fatal(err)
	}
	if r := skipRowsOf(t, h, camp); len(r) != 1 || r[0].Sub != buyer || *r[0].Reason != "purchased" {
		t.Fatalf("claim over a current feed: rows %+v, want one purchased row", r)
	}
}

// --- T5 (I5) -- sent == to_send, a recorded skip never batched, no row at or below the checkpoint ---

func TestPromoClaim(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	o1 := w.member("o1@acme.test") // the lowest id: the first batch's recipient
	buyers := []int{w.member("b1@acme.test"), w.member("b2@acme.test")}
	sends := []int{o1, w.member("o2@acme.test"), w.member("o3@acme.test")}
	w.buy("x1", "promotion", "deal-3", "b1@acme.test", "1 day")
	w.buy("x2", "sku", "BFCM-1", "b2@acme.test", "1 day")
	b := f.camp("Mid", false, "regular", "running", promoAttribs(`"C"`, "3", promoTags, false), w.target)
	st := &store{queries: h.q, core: h.app.core}
	if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
		t.Fatal(err)
	}
	rows := skipRowsOf(t, h, b)
	if len(rows) != 2 || rows[0].Sub != buyers[0] || rows[1].Sub != buyers[1] || *rows[0].Reason != "purchased" || *rows[1].Reason != "purchased" {
		t.Fatalf("skip rows %+v, want purchased %v", rows, buyers)
	}
	var toSend int
	h.db.Get(&toSend, `SELECT to_send FROM campaigns WHERE id = $1`, b)
	var got []int
	for i := 0; i < 10; i++ {
		out, err := st.NextSubscribers(b, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) == 0 {
			break
		}
		for _, s := range out {
			got = append(got, s.ID)
		}
		if i == 0 {
			if len(out) != 1 || out[0].ID != o1 {
				t.Fatalf("first batch %v, want [%d]", got, o1)
			}
			// Pause after one batch, o1 buys (below the checkpoint), re-claim: no row for o1.
			w.buy("x3", "promotion", "deal-3", "o1@acme.test", "1 minute")
			if !has(w.purchasers(b), o1) {
				t.Fatal("fixture: o1 is not a purchaser now")
			}
			h.db.MustExec(`UPDATE campaigns SET status = 'paused' WHERE id = $1`, b)
			h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE id = $1`, b)
			if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
				t.Fatalf("re-claim: %v", err)
			}
			if after := skipRowsOf(t, h, b); len(after) != 2 || after[0].Sub != buyers[0] || after[1].Sub != buyers[1] {
				t.Fatalf("after the re-claim rows %+v, want only %v", after, buyers)
			}
			var again int
			h.db.Get(&again, `SELECT to_send FROM campaigns WHERE id = $1`, b)
			if again != toSend {
				t.Fatalf("re-claim to_send %d, want %d", again, toSend)
			}
		}
	}
	sort.Ints(got)
	for _, s := range buyers {
		if has(got, s) {
			t.Fatalf("a recorded skip %d was batched: %v", s, got)
		}
	}
	for _, s := range sends {
		if !has(got, s) {
			t.Fatalf("%d not sent: %v", s, got)
		}
	}
	if len(got) != toSend {
		t.Fatalf("sent %d (%v) != to_send %d", len(got), got, toSend)
	}
}

// --- T6 (I6) -- precedence, and the counts before Start agree with the claim ---

func TestPromoPrecedence(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	look := w.member("look@acme.test") // a look-back skip who bought
	journeyList := f.list("Acme first order", "brand:acme")
	j := f.camp("Acme thanks", true, "regular", "running", `{}`, journeyList)
	f.send(j, look, "1 hour")
	silentBuyer := w.member("sb@gmail.com", w.three) // silent and bought
	buyer := w.member("buyer@acme.test")             // bought only
	pureSilent := w.member("ps@gmail.com", w.three)  // silent only
	w.member("plain@acme.test")
	for i, e := range []string{"look@acme.test", "sb@gmail.com", "buyer@acme.test"} {
		w.buy(fmt.Sprintf("o%d", i), "promotion", "deal-3", e, "1 day")
	}
	on := f.camp("On", false, "regular", "draft", promoAttribs(`"C"`, "2", promoTags, true), w.target)
	off := f.camp("Off", false, "regular", "draft", promoAttribs("", "", "", true), w.target)

	aOn, err := h.app.core.CampaignLangAudience(on)
	if err != nil {
		t.Fatal(err)
	}
	aOff, _ := h.app.core.CampaignLangAudience(off)
	if aOn.Skipped != aOff.Skipped || aOn.Skipped != 1 {
		t.Fatalf("Smart Sending counts on %d off %d, want 1 / 1", aOn.Skipped, aOff.Skipped)
	}
	if aOn.PurchasedSkipped != 2 || aOn.SilentSkipped != 1 || aOff.PurchasedSkipped != 0 || aOff.SilentSkipped != 2 {
		t.Fatalf("on %+v off %+v, want purchased 2 / silent 1 and purchased 0 / silent 2", aOn, aOff)
	}
	if n := silentAudienceIDs(t, h, on); n != 1 {
		t.Fatalf("silent-audience on %d, want 1 (the silent purchaser excluded)", n)
	}
	if n := silentAudienceIDs(t, h, off); n != 2 {
		t.Fatalf("silent-audience off %d, want 2", n)
	}
	cm, _ := h.app.core.GetCampaign(on, "", "")
	h.app.core.FillAudience(&cm)
	total := aOn.Skipped + aOn.PurchasedSkipped + aOn.SilentSkipped
	if cm.AudienceSkipped.Int != total || cm.AudiencePurchasedSkipped.Int != 2 || cm.AudienceSilentSkipped.Int != 1 {
		t.Fatalf("served total %v purchased %v silent %v, want %d / 2 / 1", cm.AudienceSkipped, cm.AudiencePurchasedSkipped, cm.AudienceSilentSkipped, total)
	}

	h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE id = ANY($1)`, pq.Array([]int{on, off}))
	st := &store{queries: h.q, core: h.app.core}
	if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
		t.Fatal(err)
	}
	reasons := func(camp int) map[int]string {
		out := map[int]string{}
		for _, r := range skipRowsOf(t, h, camp) {
			out[r.Sub] = *r.Reason
			if r.Sub == look && (r.Journey == nil || *r.Journey != j || !r.Sent.Valid) {
				t.Fatalf("the look-back row %+v lost its pair", r)
			}
		}
		return out
	}
	gotOn := reasons(on)
	if len(gotOn) != total || gotOn[look] != "lookback" || gotOn[silentBuyer] != "purchased" || gotOn[buyer] != "purchased" || gotOn[pureSilent] != "silent" {
		t.Fatalf("on rows %v, want %d: look lookback, sb purchased, buyer purchased, ps silent", gotOn, total)
	}
	gotOff := reasons(off)
	if len(gotOff) != 3 || gotOff[look] != "lookback" || gotOff[silentBuyer] != "silent" || gotOff[pureSilent] != "silent" {
		t.Fatalf("off rows %v", gotOff)
	}
}

// --- T7 (I7) -- hostile data: nothing raises, the string "2" is off ---

func TestPromoHostileData(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	buyer := w.member("buyer@acme.test")
	w.member("no-at-sign")
	w.member("@leading.test")
	w.buy("o1", "promotion", "deal-3", "buyer@acme.test", "1 day")
	w.buy(strings.Repeat("o", 128), "promotion", strings.Repeat("x", 128), "buyer@acme.test", "1 day")
	w.buy("o3", "sku", strings.Repeat("y", 128), "no-at-sign", "1 day")
	h.db.MustExec(`INSERT INTO promo_purchases (order_id, kind, value, value_lc, email_hash, ordered_at) VALUES ('o4', 'weird', '%', '%', '', NOW())`)

	for attribs, wantSkip := range map[string]bool{
		promoAttribs(`"C"`, `"2"`, promoTags, false):                 false, // a string is not step 2
		promoAttribs(`"C"`, `true`, promoTags, false):                false,
		promoAttribs(`"C"`, `{}`, promoTags, false):                  false,
		promoAttribs(`"C"`, `null`, promoTags, false):                false,
		promoAttribs(`"C"`, `2.0`, promoTags, false):                 true, // the JSONB number 2 (JSONB equality)
		promoAttribs(`"C"`, `2`, `"deal-3"`, false):                  false,
		promoAttribs(`"C"`, `2`, `{"a": "deal-3"}`, false):           false,
		promoAttribs(`"C"`, `2`, `[1, null, {}, "deal-3"]`, false):   true, // string elements only
		promoAttribs(`5`, `2`, promoTags, false):                     false,
		promoAttribs(`{"x": 1}`, `2`, promoTags, false):              false,
		promoAttribs(`"C"`, `2`, `["*", "%", "_*", ""]`, false):       false, // no * tag below the prefix floor matches
		promoAttribs(`"C"`, `2`, `["`+strings.Repeat("x", 128)+`"]`, false): true,
	} {
		id := f.camp("Odd", false, "regular", "draft", attribs, w.target)
		if got := has(w.purchasers(id), buyer); got != wantSkip {
			t.Errorf("%s: buyer skipped %v, want %v", attribs, got, wantSkip)
		}
		if _, err := h.app.core.CampaignLangAudience(id); err != nil {
			t.Errorf("%s: lang audience raised %v", attribs, err)
		}
		if _, err := h.app.core.CampaignSilentAudience(id); err != nil {
			t.Errorf("%s: silent audience raised %v", attribs, err)
		}
		promoPreviewOf(t, h, id, nil)
	}
	// Attribs that are not an object at all (written by SQL).
	for _, a := range []string{`[1, 2]`, `"x"`, `7`, `null`} {
		id := f.camp("Shape", false, "regular", "draft", `{}`, w.target)
		h.db.MustExec(`UPDATE campaigns SET attribs = $2::JSONB WHERE id = $1`, id, a)
		if p := w.purchasers(id); len(p) != 0 {
			t.Errorf("attribs %s: purchasers %v", a, p)
		}
		var n int
		if err := h.db.Get(&n, `SELECT purchased_skipped FROM (SELECT 0 AS x) z, LATERAL (SELECT COUNT(*) AS purchased_skipped FROM promo_purchasers($1, promo_campaign_tags($1))) y`, id); err != nil {
			t.Errorf("attribs %s: %v", a, err)
		}
		// Go cannot scan a non-object attribs into a campaign (unrelated to promo): out of the claim below.
		h.db.MustExec(`UPDATE campaigns SET status = 'cancelled' WHERE id = $1`, id)
	}
	var s string
	for _, q := range []string{
		`SELECT promo_email_hash(NULL)`, `SELECT promo_email_hash('@')`, `SELECT promo_email_hash('a@')`,
		`SELECT promo_applicable_tags(NULL, NULL, NULL, NULL, NULL)::TEXT`,
		`SELECT promo_applicable_tags('regular', FALSE, '"C"', '2', 'null')::TEXT`,
		`SELECT promo_campaign_tags(-1)::TEXT`, `SELECT COUNT(*)::TEXT FROM promo_purchasers(-1, ARRAY['x'])`,
		`SELECT COUNT(*)::TEXT FROM promo_purchasers(1, NULL)`, `SELECT COUNT(*)::TEXT FROM promo_matching_orders(NULL)`,
		`SELECT COUNT(*)::TEXT FROM promo_matching_orders(ARRAY[NULL, '', '*', '%'])`,
	} {
		if err := h.db.Get(&s, q); err != nil {
			t.Errorf("%s raised %v", q, err)
		}
	}
	// And the claim over all of it.
	h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE status = 'draft'`)
	st := &store{queries: h.q, core: h.app.core}
	if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
		t.Fatalf("claim over hostile data: %v", err)
	}
}

// --- T9 (I9) -- the canonical-email hash over the shared case file ---

func TestPromoEmailHashCaseFile(t *testing.T) {
	h := newLinkHarness(t)
	b, err := os.ReadFile(filepath.Join("..", "models", "testdata", "email-canonical.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Cases []struct {
			Note      string `json:"note"`
			Email     string `json:"email"`
			Canonical string `json:"canonical"`
			Hash      string `json:"hash"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	if len(fx.Cases) < 9 {
		t.Fatalf("the case file has %d rows, want the 9 required", len(fx.Cases))
	}
	for _, c := range fx.Cases {
		var got string
		if err := h.db.Get(&got, `SELECT promo_email_hash($1)`, c.Email); err != nil {
			t.Fatal(err)
		}
		var want string
		h.db.Get(&want, `SELECT ENCODE(SHA256(CONVERT_TO($1, 'UTF8')), 'hex')`, c.Canonical)
		if got != c.Hash || want != c.Hash {
			t.Errorf("%s (%q): promo_email_hash %s, sha256(canonical) %s, file %s", c.Note, c.Email, got, want, c.Hash)
		}
	}
}

// --- T10 (I10, I2) -- the feed routes and the preview route ---

func TestPromoFeedRoutes(t *testing.T) {
	h := newLinkHarness(t)
	h.app.auth = &auth.Auth{}
	writer := perms("listmonk_sync", auth.PermSubscribersManage)
	reader := perms("listmonk_agent", auth.PermSubscribersGetAll)
	put := func(u auth.User, body string) (int, string) {
		return convCall(t, u, http.MethodPut, "/api/promo/purchases", body, h.app.auth.Perm(h.app.PutPromoPurchases, "subscribers:manage"))
	}
	get := func(u auth.User, path string) (int, string) {
		return convCall(t, u, http.MethodGet, path, "", h.app.auth.Perm(h.app.GetPromoPurchases, "subscribers:get_all"))
	}
	rows := func() string {
		t.Helper()
		var out []string
		if err := h.db.Select(&out, `SELECT order_id || '/' || kind || '/' || value || '/' || value_lc || '/' || LEFT(email_hash, 4) || '/' || TO_CHAR(ordered_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS')
			FROM promo_purchases ORDER BY order_id, kind, value`); err != nil {
			t.Fatal(err)
		}
		return strings.Join(out, " ")
	}
	hash := strings.Repeat("ab", 32)
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	order := func(id string, promos, skus string) string {
		return fmt.Sprintf(`{"order_id": %q, "email_hash": %q, "ordered_at": %q, "promotions": %s, "skus": %s}`, id, hash, at.Format(time.RFC3339), promos, skus)
	}
	body := `{"orders": [` + order("A1", `["BFCM-Deal-2", " komigobox ", "BFCM-Deal-2"]`, `["Exltd-SculptPeptides"]`) + `, ` + order("A2", `[]`, `[]`) + `]}`
	code, resp := put(writer, body)
	if code != http.StatusOK || !strings.Contains(resp, `"rows":3`) {
		t.Fatalf("put: %d %s", code, resp)
	}
	ts := at.Format("2006-01-02T15:04:05")
	want := "A1/promotion/BFCM-Deal-2/bfcm-deal-2/abab/" + ts + " A1/promotion/komigobox/komigobox/abab/" + ts + " A1/sku/Exltd-SculptPeptides/exltd-sculptpeptides/abab/" + ts
	if got := rows(); got != want {
		t.Fatalf("rows\n got %s\nwant %s", got, want)
	}
	var first time.Time
	h.db.Get(&first, `SELECT MAX(received_at) FROM promo_purchases`)
	time.Sleep(20 * time.Millisecond)
	if code, resp := put(writer, body); code != http.StatusOK {
		t.Fatalf("put again: %d %s", code, resp)
	}
	var second time.Time
	h.db.Get(&second, `SELECT MIN(received_at) FROM promo_purchases`)
	if got := rows(); got != want || !second.After(first) {
		t.Fatalf("upsert twice: rows %s, received %v -> %v (want same rows, newer received_at)", got, first, second)
	}

	for name, b := range map[string]string{
		"not JSON":           `orders`,
		"no orders":          `{}`,
		"short hash":         `{"orders": [{"order_id": "B1", "email_hash": "abc", "ordered_at": "` + at.Format(time.RFC3339) + `", "promotions": ["x"]}]}`,
		"upper-case hash":    `{"orders": [{"order_id": "B1", "email_hash": "` + strings.ToUpper(hash) + `", "ordered_at": "` + at.Format(time.RFC3339) + `", "promotions": ["x"]}]}`,
		"bad time":           `{"orders": [{"order_id": "B1", "email_hash": "` + hash + `", "ordered_at": "yesterday", "promotions": ["x"]}]}`,
		"2 days ahead":       `{"orders": [{"order_id": "B1", "email_hash": "` + hash + `", "ordered_at": "` + time.Now().Add(49*time.Hour).Format(time.RFC3339) + `", "promotions": ["x"]}]}`,
		"empty order id":     `{"orders": [` + order("", `["x"]`, `[]`) + `]}`,
		"129-char order id":  `{"orders": [` + order(strings.Repeat("o", 129), `["x"]`, `[]`) + `]}`,
		"129-char value":     `{"orders": [` + order("B1", `["`+strings.Repeat("v", 129)+`"]`, `[]`) + `]}`,
		"blank value":        `{"orders": [` + order("B1", `["  "]`, `[]`) + `]}`,
		"control value":      `{"orders": [` + order("B1", `["a\u0007b"]`, `[]`) + `]}`,
		"non-string value":   `{"orders": [` + order("B1", `[5]`, `[]`) + `]}`,
		"201 values":         `{"orders": [` + order("B1", `["`+strings.Repeat(`x", "`, 200)+`x"]`, `[]`) + `]}`,
		"one bad order of 2": `{"orders": [` + order("B1", `["good"]`, `[]`) + `, ` + order("", `["x"]`, `[]`) + `]}`,
		"1001 orders":        `{"orders": [` + strings.TrimSuffix(strings.Repeat(order("B1", `["x"]`, `[]`)+",", 1001), ",") + `]}`,
	} {
		if code, resp := put(writer, b); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s, want 400", name, code, resp)
		}
		if got := rows(); got != want {
			t.Fatalf("%s wrote\n got %s\nwant %s", name, got, want)
		}
	}

	// Permissions: the reader cannot write, the writer cannot read.
	if code, _ := put(reader, body); code != http.StatusForbidden {
		t.Fatalf("reader PUT: %d, want 403", code)
	}
	if code, _ := get(writer, "/api/promo/purchases"); code != http.StatusForbidden {
		t.Fatalf("writer GET: %d, want 403", code)
	}

	// GET: the summary, and one order's rows.
	code, resp = get(reader, "/api/promo/purchases")
	var sum struct {
		Data struct {
			Rows   int  `json:"rows"`
			Orders int  `json:"orders"`
			Stale  bool `json:"stale"`
			Last   *string `json:"last_received_at"`
			Oldest *string `json:"oldest_ordered_at"`
		} `json:"data"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(resp), &sum) != nil || sum.Data.Rows != 3 || sum.Data.Orders != 1 || sum.Data.Stale ||
		sum.Data.Last == nil || sum.Data.Oldest == nil || strings.Contains(resp, "@") {
		t.Fatalf("summary %d %s", code, resp)
	}
	code, resp = get(reader, "/api/promo/purchases?order_id=A1")
	var one struct {
		Data []struct {
			Kind      string `json:"kind"`
			Value     string `json:"value"`
			EmailHash string `json:"email_hash"`
		} `json:"data"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(resp), &one) != nil || len(one.Data) != 3 || one.Data[0].EmailHash != hash {
		t.Fatalf("order read %d %s", code, resp)
	}
	if code, resp := get(reader, "/api/promo/purchases?order_id=none"); code != http.StatusOK || !strings.Contains(resp, `"data":[]`) {
		t.Fatalf("unknown order %d %s", code, resp)
	}
	// An empty feed reads stale.
	h.db.MustExec(`DELETE FROM promo_purchases`)
	if _, resp := get(reader, "/api/promo/purchases"); !strings.Contains(resp, `"rows":0`) || !strings.Contains(resp, `"stale":true`) {
		t.Fatalf("empty feed summary %s", resp)
	}
}

func TestPromoAudienceRoute(t *testing.T) {
	w := newSilentWorld(t)
	f, h := w.f, w.h
	buyer := w.member("buyer@acme.test")
	w.member("plain@acme.test")
	w.buy("o1", "promotion", "BFCM-Deal-2", "buyer@acme.test", "1 day")
	w.buy("o2", "promotion", "BFCM-BMSM-2", "other@acme.test", "1 day")
	camp := f.camp("Draft", false, "regular", "draft", promoAttribs(`"C"`, "1", `["BFCM-Deal-2"]`, false), w.target)
	other := f.list("Other", "brand:acme")
	tags := "BFCM-Deal-2, BFCM-BMSM-2"

	// Permissions: campaigns:manage plus the campaign's list.
	q := url.Values{"code": {"C"}, "step": {"2"}, "tags": {tags}}
	if code, _ := promoCall(t, h, baUser([]string{auth.PermCampaignsGet}, w.target), camp, q); code != http.StatusForbidden {
		t.Fatalf("campaigns:get only: %d, want 403", code)
	}
	if code, _ := promoCall(t, h, baUser([]string{auth.PermCampaignsManage}, other), camp, q); code != http.StatusForbidden {
		t.Fatalf("campaigns:manage without the campaign's list: %d, want 403", code)
	}
	gc, _ := baCall(t, baUser([]string{auth.PermCampaignsGet, auth.PermCampaignsManage}, other), "/", func(c echo.Context) error {
		c.Set("id", camp)
		return h.app.GetCampaign(c)
	})
	if gc != http.StatusForbidden {
		t.Fatalf("fixture: the campaign GET lets the outsider in (%d)", gc)
	}
	code, body := promoCall(t, h, baUser([]string{auth.PermCampaignsManage}, w.target), camp, q)
	if code != http.StatusOK || !strings.Contains(body, `"skipped":1`) || !strings.Contains(body, `"orders":2`) {
		t.Fatalf("a manager of the list: %d %s", code, body)
	}

	// The stand-in cases over the seeded feed.
	for name, tc := range map[string]struct {
		q       url.Values
		orders  int
		skipped int
	}{
		"stored (step 1)":   {nil, 1, 0},
		"step 1":            {url.Values{"code": {"C"}, "step": {"1"}, "tags": {tags}}, 2, 0},
		"blank step":        {url.Values{"code": {"C"}, "step": {""}, "tags": {tags}}, 2, 0},
		"no code":           {url.Values{"code": {""}, "step": {"2"}, "tags": {tags}}, 2, 0},
		"step 2 with tags":  {url.Values{"code": {"C"}, "step": {"2"}, "tags": {tags}}, 2, 1},
		"step 3, case":      {url.Values{"code": {"C"}, "step": {"3"}, "tags": {"bfcm-deal-2"}}, 1, 1},
		"step 2, no tags":   {url.Values{"code": {"C"}, "step": {"2"}, "tags": {""}}, 0, 0},
		"stored tags, step": {url.Values{"step": {"2"}}, 1, 1},
	} {
		pv := promoPreviewOf(t, h, camp, tc.q)
		if pv.Orders != tc.orders || pv.Skipped != tc.skipped || pv.Stale {
			t.Errorf("%s: %+v, want orders %d skipped %d", name, pv, tc.orders, tc.skipped)
		}
	}
	for name, q := range map[string]url.Values{
		"malformed tag": {"tags": {"ab*"}},
		"bad step":      {"step": {"4"}},
		"long code":     {"code": {strings.Repeat("x", 31)}},
	} {
		if code, _ := promoCall(t, h, perms("editor", auth.PermCampaignsManageAll), camp, q); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
	// A stale feed: skipped 0, orders still counted.
	w.feedAge("25 hours")
	pv := promoPreviewOf(t, h, camp, url.Values{"code": {"C"}, "step": {"2"}, "tags": {tags}})
	if !pv.Stale || pv.Skipped != 0 || pv.Orders != 2 {
		t.Fatalf("stale: %+v, want stale, skipped 0, orders 2", pv)
	}
	// Nothing is stored by the preview.
	cm, _ := h.app.core.GetCampaign(camp, "", "")
	if cm.Attribs["promo_step"] != 1.0 {
		t.Fatalf("the preview wrote %v", cm.Attribs)
	}
	_ = buyer
}

// --- T13 (I13) -- the four reason counts sum to skipped wherever served; Code and Step served ---

func TestPromoReasonCounts(t *testing.T) {
	h := newLinkHarness(t)
	f := ssFixture{t: t, h: h}
	acme := f.list("Acme newsletter", "brand:acme")
	b := f.camp("Acme mid", false, "regular", "finished", promoAttribs(`"BFCM-D3"`, "2", promoTags, false), acme)
	h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - INTERVAL '1 day', sent = 10 WHERE id = $1`, b)
	ins := func(sub int, journey any, due any, reason any) {
		h.db.MustExec(`INSERT INTO campaign_smart_skips (campaign_id, subscriber_id, journey_campaign_id, journey_sent_at, due_list_id, due_at, reason)
			VALUES ($1, $2, $3, CASE WHEN $3::INT IS NULL THEN NULL ELSE NOW() END, $4, CASE WHEN $4::INT IS NULL THEN NULL ELSE NOW() END, $5)`,
			b, sub, journey, due, reason)
	}
	ins(1, 9, nil, "lookback")
	ins(2, nil, 9, "lookahead")
	ins(3, nil, nil, "silent")
	ins(4, nil, nil, "purchased")
	ins(5, nil, nil, "purchased")
	ins(6, 9, nil, nil) // NULL reason, look-back half -- never purchased
	want := [5]int{6, 2, 1, 1, 2}

	raw, err := h.app.core.GetCampaignMatrix(true, nil, false, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var mx struct {
		Rows []struct {
			ID        int     `json:"id"`
			Skipped   *int    `json:"skipped"`
			Lookback  *int    `json:"skipped_lookback"`
			Lookahead *int    `json:"skipped_lookahead"`
			Silent    *int    `json:"skipped_silent"`
			Purchased *int    `json:"skipped_purchased"`
			Code      *string `json:"promo_code"`
			Step      *int    `json:"promo_step"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &mx); err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, r := range mx.Rows {
		if r.ID != b {
			continue
		}
		seen = true
		if r.Skipped == nil || r.Lookback == nil || r.Lookahead == nil || r.Silent == nil || r.Purchased == nil {
			t.Fatalf("matrix row %+v carries a null", r)
		}
		got := [5]int{*r.Skipped, *r.Lookback, *r.Lookahead, *r.Silent, *r.Purchased}
		if got != want || got[1]+got[2]+got[3]+got[4] != got[0] {
			t.Fatalf("matrix %v, want %v (summing)", got, want)
		}
		if r.Code == nil || *r.Code != "BFCM-D3" || r.Step == nil || *r.Step != 2 {
			t.Fatalf("matrix code %v step %v", r.Code, r.Step)
		}
	}
	if !seen {
		t.Fatalf("matrix rows %s", raw)
	}
	ac, err := h.app.core.QueryAnalyticsCampaigns([]int{b}, "", true, nil, false, 10, nil)
	if err != nil || len(ac) != 1 {
		t.Fatalf("analytics campaigns %v %v", ac, err)
	}
	a := ac[0]
	got := [5]int{a.Skipped.Int, a.SkippedLookback.Int, a.SkippedLookahead.Int, a.SkippedSilent.Int, a.SkippedPurchased.Int}
	if got != want || a.PromoCode.String != "BFCM-D3" || a.PromoStep.Int != 2 {
		t.Fatalf("analytics %v code %v step %v, want %v", got, a.PromoCode, a.PromoStep, want)
	}
	// A journey's counts are null; a campaign without promo fields serves null Code / Step; a Step
	// written as a string is not served.
	j := f.camp("Acme welcome", true, "regular", "running", `{}`, acme)
	odd := f.camp("Odd", false, "regular", "draft", `{"promo_code": 5, "promo_step": "2"}`, acme)
	ac, _ = h.app.core.QueryAnalyticsCampaigns([]int{j, odd}, "", true, nil, false, 10, nil)
	if len(ac) != 2 {
		t.Fatalf("analytics %+v", ac)
	}
	for _, x := range ac {
		if x.ID == j && (x.SkippedPurchased.Valid || x.PromoCode.Valid || x.PromoStep.Valid) {
			t.Fatalf("journey %+v", x)
		}
		if x.ID == odd && (x.PromoCode.Valid || x.PromoStep.Valid || !x.SkippedPurchased.Valid || x.SkippedPurchased.Int != 0) {
			t.Fatalf("odd attribs %+v", x)
		}
	}
}
