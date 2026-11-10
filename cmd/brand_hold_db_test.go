package main

// Fork (hold scope) -- integrations listmonk/HOLD-SCOPE-SPEC I1a-I1e, I2, I3, I4, I5 against a real
// database, through the REAL statements: the claim (next-campaigns, through store.NextCampaigns),
// the batches (next-campaign-subscribers, through store.NextSubscribers), the preview
// (get-campaign-lang-audience, core.CampaignLangAudience), the claim-time shop-link snapshot
// (snapshot-campaign-shop-link-audience) and the coverage warning (get-campaign-attrib-coverage,
// core.CampaignAttribCoverage), all reading brand_hold_rows (schema.sql). Shares newLinkHarness
// (LISTMONK_TEST_PG opt-in; plain go test skips). Fixture names are acme.test / beta.test only.

import (
	"fmt"
	"sort"
	"testing"

	"github.com/lib/pq"
)

// bhFixture is the I3 fixture. Holds live on the acme newsletter list (any reason, a final hold
// included), on beta's newsletter list and on an acme TRANSACTIONAL list. The campaigns go to a
// non-exempt acme segment list, an acme transactional list, each of the four re-engagement queues,
// an untagged list, and the segment + transactional pair.
type bhFixture struct {
	ssFixture
	st *store

	news, seg, txn, txnHold, untagged, betaNews int
	queues                                      [4]int

	plain, held, finalHeld, kickHeld, betaHeld, txnHeld int

	cSeg, cTxn, cUntagged, cMix int
	cQueues                     [4]int
}

func (f *bhFixture) mkList(name, optin string, transactional bool, tags ...string) int {
	f.t.Helper()
	if tags == nil {
		tags = []string{}
	}
	var id int
	if err := f.h.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags, transactional) VALUES (gen_random_uuid(), $1, 'private', $2::list_optin, $3, $4) RETURNING id`,
		name, optin, pq.StringArray(tags), transactional); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// hold writes a held row (unsubscribed + meta.hold) on list.
func (f *bhFixture) hold(sub, list int, meta string) {
	f.t.Helper()
	f.h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status, meta) VALUES ($1, $2, 'unsubscribed', $3::JSONB)`, sub, list, meta)
}

const (
	bhNeverEngaged = `{"hold": {"reason": "never-engaged", "rule": "r1", "at": "2026-05-01T00:00:00Z"}}`
	bhFinal        = `{"hold": {"reason": "never-engaged", "rule": "r1", "at": "2026-01-01T00:00:00Z", "final_at": "2026-07-05T00:00:00Z"}}`
	bhKickbox      = `{"hold": {"reason": "kickbox-undeliverable", "at": "2026-05-01T00:00:00Z"}}`
)

func newBrandHoldFixture(t *testing.T) *bhFixture {
	h := newLinkHarness(t)
	f := &bhFixture{ssFixture: ssFixture{t: t, h: h}, st: &store{queries: h.q, core: h.app.core}}

	f.news = f.mkList("Acme newsletter", "single", false, "brand:acme")
	f.seg = f.mkList("Acme segment", "single", false, "brand:acme")
	f.txn = f.mkList("Acme receipts", "single", true, "brand:acme")
	f.txnHold = f.mkList("Acme order updates", "single", true, "brand:acme")
	f.untagged = f.mkList("Scratch", "single", false)
	f.betaNews = f.mkList("Beta newsletter", "single", false, "brand:beta")
	for i := range f.queues {
		f.queues[i] = f.mkList(fmt.Sprintf("Acme re-engagement %d", i+1), "single", false,
			"brand:acme", fmt.Sprintf("reengage:%d:%d", f.news, i+1))
	}

	f.plain = f.sub("plain@acme.test", "enabled")
	f.held = f.sub("held@acme.test", "enabled")
	f.finalHeld = f.sub("final@acme.test", "enabled")
	f.kickHeld = f.sub("kickbox@acme.test", "enabled")
	f.betaHeld = f.sub("betaheld@beta.test", "enabled")
	f.txnHeld = f.sub("txnheld@acme.test", "enabled")

	f.hold(f.held, f.news, bhNeverEngaged)
	f.hold(f.finalHeld, f.news, bhFinal)
	f.hold(f.kickHeld, f.news, bhKickbox)
	f.hold(f.betaHeld, f.betaNews, bhNeverEngaged)
	f.hold(f.txnHeld, f.txnHold, bhNeverEngaged)

	exempt := append([]int{f.txn, f.untagged}, f.queues[:]...)
	for _, s := range []int{f.plain, f.held} {
		f.join(s, f.seg, "confirmed")
		for _, l := range exempt {
			f.join(s, l, "confirmed")
		}
	}
	for _, s := range []int{f.finalHeld, f.kickHeld, f.betaHeld, f.txnHeld} {
		f.join(s, f.seg, "confirmed")
	}

	f.cSeg = f.camp("Acme segment broadcast", false, "regular", "draft", `{}`, f.seg)
	f.cTxn = f.camp("Acme receipts broadcast", false, "regular", "draft", `{}`, f.txn)
	f.cUntagged = f.camp("Scratch broadcast", false, "regular", "draft", `{}`, f.untagged)
	f.cMix = f.camp("Acme segment + receipts", false, "regular", "draft", `{}`, f.seg, f.txn)
	for i, q := range f.queues {
		f.cQueues[i] = f.camp(fmt.Sprintf("Acme win-back %d", i+1), false, "regular", "draft", `{}`, q)
	}
	return f
}

func (f *bhFixture) campaigns() []int {
	return append([]int{f.cSeg, f.cTxn, f.cUntagged, f.cMix}, f.cQueues[:]...)
}

// switchTo sets app.brand_hold_scope: "absent" deletes the row, anything else is the JSONB text.
func (f *bhFixture) switchTo(v string) {
	f.t.Helper()
	if v == "absent" {
		f.h.db.MustExec(`DELETE FROM settings WHERE key = 'app.brand_hold_scope'`)
		return
	}
	f.h.db.MustExec(`INSERT INTO settings (key, value) VALUES ('app.brand_hold_scope', $1::JSONB)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, v)
}

func ids(xs ...int) string {
	s := append([]int{}, xs...)
	sort.Ints(s)
	return fmt.Sprint(s)
}

// on is the I3 expectation with the rule ON; off is everyone the lists' opt-in rules admit.
func (f *bhFixture) on() map[int]string {
	m := map[int]string{
		f.cSeg:      ids(f.plain, f.betaHeld, f.txnHeld),
		f.cTxn:      ids(f.plain, f.held),
		f.cUntagged: ids(f.plain, f.held),
		f.cMix:      ids(f.plain, f.held, f.betaHeld, f.txnHeld),
	}
	for _, c := range f.cQueues {
		m[c] = ids(f.plain, f.held)
	}
	return m
}

func (f *bhFixture) off() map[int]string {
	m := f.on()
	m[f.cSeg] = ids(f.plain, f.held, f.finalHeld, f.kickHeld, f.betaHeld, f.txnHeld)
	m[f.cMix] = m[f.cSeg]
	return m
}

// parseIDs reads back an ids() string.
func parseIDs(set string) []int {
	var xs []int
	cur, in := 0, false
	for _, r := range set {
		if r >= '0' && r <= '9' {
			cur, in = cur*10+int(r-'0'), true
			continue
		}
		if in {
			xs = append(xs, cur)
		}
		cur, in = 0, false
	}
	return xs
}

func countOf(set string) int { return len(parseIDs(set)) }

func containsID(set string, id int) bool {
	for _, x := range parseIDs(set) {
		if x == id {
			return true
		}
	}
	return false
}

// claim resets every fixture campaign to a fresh running state and runs one claim; to_send per
// campaign.
func (f *bhFixture) claim() map[int]int {
	f.t.Helper()
	f.h.db.MustExec(`UPDATE campaigns SET status = 'running', last_subscriber_id = 0, to_send = 0, sent = 0 WHERE id = ANY($1)`, pq.Array(f.campaigns()))
	if _, err := f.st.NextCampaigns([]int64{}, []int64{}); err != nil {
		f.t.Fatalf("next-campaigns: %v", err)
	}
	out := map[int]int{}
	for _, c := range f.campaigns() {
		var n int
		if err := f.h.db.Get(&n, `SELECT to_send FROM campaigns WHERE id = $1`, c); err != nil {
			f.t.Fatal(err)
		}
		out[c] = n
	}
	return out
}

// batches drives next-campaign-subscribers to exhaustion (batches of 2) after a claim.
func (f *bhFixture) batches(camp int) []int {
	f.t.Helper()
	var got []int
	for i := 0; i < 20; i++ {
		out, err := f.st.NextSubscribers(camp, 2)
		if err != nil {
			f.t.Fatalf("next-campaign-subscribers: %v", err)
		}
		if len(out) == 0 {
			break
		}
		for _, s := range out {
			got = append(got, s.ID)
		}
	}
	return got
}

func (f *bhFixture) preview(camp int) int {
	f.t.Helper()
	a, err := f.h.app.core.CampaignLangAudience(camp)
	if err != nil {
		f.t.Fatalf("get-campaign-lang-audience: %v", err)
	}
	return a.Total
}

func (f *bhFixture) coverage(camp int) int {
	f.t.Helper()
	_, total, err := f.h.app.core.CampaignAttribCoverage(camp, "k")
	if err != nil {
		f.t.Fatalf("get-campaign-attrib-coverage: %v", err)
	}
	return total
}

// snapshot runs the claim-time shop-link snapshot for brand acme on a clean table; the recipients.
func (f *bhFixture) snapshot(camp int) string {
	f.t.Helper()
	f.h.db.MustExec(`DELETE FROM campaign_shop_link_recipients WHERE campaign_id = $1`, camp)
	f.h.db.MustExec(`DELETE FROM campaign_shop_link_audience WHERE campaign_id = $1`, camp)
	if _, err := f.h.q.SnapshotCampaignShopLinkAudience.Exec(camp, pq.StringArray{"acme"}); err != nil {
		f.t.Fatalf("snapshot-campaign-shop-link-audience: %v", err)
	}
	var got []int
	if err := f.h.db.Select(&got, `SELECT subscriber_id FROM campaign_shop_link_recipients WHERE campaign_id = $1`, camp); err != nil {
		f.t.Fatal(err)
	}
	return ids(got...)
}

// readAll is every reader's answer for every fixture campaign under the current switch state.
func (f *bhFixture) readAll() map[int]string {
	f.t.Helper()
	out := map[int]string{}
	for _, c := range f.campaigns() {
		out[c] = fmt.Sprintf("preview=%d coverage=%d snapshot=%s", f.preview(c), f.coverage(c), f.snapshot(c))
	}
	toSend := f.claim()
	for _, c := range f.campaigns() {
		out[c] += fmt.Sprintf(" claim=%d batches=%s", toSend[c], ids(f.batches(c)...))
	}
	return out
}

// I1a -- the claim counts no subscriber held on a non-transactional list of the brand, for a
// non-exempt campaign list (I3's exemptions: each campaign's count is its on-set's size).
func TestBrandHoldClaim(t *testing.T) {
	f := newBrandHoldFixture(t)
	f.switchTo("true")
	got := f.claim()
	for c, want := range f.on() {
		if got[c] != countOf(want) {
			t.Errorf("campaign %d to_send %d, want %d (%s)", c, got[c], countOf(want), want)
		}
	}
	// max_subscriber_id is read from the same audience.
	var maxSeg int
	f.h.db.Get(&maxSeg, `SELECT max_subscriber_id FROM campaigns WHERE id = $1`, f.cSeg)
	if maxSeg != f.txnHeld {
		t.Fatalf("segment max_subscriber_id %d, want %d", maxSeg, f.txnHeld)
	}
}

// I1b -- the batches return no such subscriber.
func TestBrandHoldBatches(t *testing.T) {
	f := newBrandHoldFixture(t)
	f.switchTo("true")
	f.claim()
	for c, want := range f.on() {
		if got := ids(f.batches(c)...); got != want {
			t.Errorf("campaign %d batches %s, want %s", c, got, want)
		}
	}
}

// I1c -- the preview total excludes them.
func TestBrandHoldPreview(t *testing.T) {
	f := newBrandHoldFixture(t)
	f.switchTo("true")
	for c, want := range f.on() {
		if got := f.preview(c); got != countOf(want) {
			t.Errorf("campaign %d preview %d, want %d (%s)", c, got, countOf(want), want)
		}
	}
}

// I1d -- the claim-time shop-link snapshot writes no recipient row for them.
func TestBrandHoldSnapshot(t *testing.T) {
	f := newBrandHoldFixture(t)
	f.switchTo("true")
	for c, want := range f.on() {
		if got := f.snapshot(c); got != want {
			t.Errorf("campaign %d snapshot recipients %s, want %s", c, got, want)
		}
	}
}

// I1e -- the coverage warning counts none of them.
func TestBrandHoldCoverage(t *testing.T) {
	f := newBrandHoldFixture(t)
	f.switchTo("true")
	for c, want := range f.on() {
		if got := f.coverage(c); got != countOf(want) {
			t.Errorf("campaign %d coverage total %d, want %d (%s)", c, got, countOf(want), want)
		}
	}
}

// I2 -- dark: with the row absent, false, or a non-boolean (the string "true" included), all five
// readers return exactly what they return with brand_hold_rows replaced by a function returning no
// rows (the readers "without the function"), and that is the off-set; ON differs.
func TestBrandHoldDark(t *testing.T) {
	f := newBrandHoldFixture(t)

	dark := map[string]map[int]string{}
	for _, v := range []string{"absent", "false", `"true"`, "1", `{"on": true}`} {
		f.switchTo(v)
		dark[v] = f.readAll()
	}
	f.switchTo("true")
	onRead := f.readAll()

	// Without the function: a stand-in returning no rows, whatever the switch.
	f.h.db.MustExec(`CREATE OR REPLACE FUNCTION brand_hold_rows(p_campaign_id INT)
		RETURNS TABLE (list_id INT, subscriber_id INT) AS $$ SELECT NULL::INT, NULL::INT WHERE FALSE; $$ LANGUAGE sql STABLE`)
	without := f.readAll()

	for v, got := range dark {
		for c, w := range without {
			if got[c] != w {
				t.Errorf("switch %s, campaign %d:\n got     %s\n without %s", v, c, got[c], w)
			}
		}
	}
	for c, want := range f.off() {
		n := countOf(want)
		if exp := fmt.Sprintf("preview=%d coverage=%d snapshot=%s claim=%d batches=%s", n, n, want, n, want); without[c] != exp {
			t.Errorf("campaign %d without the function:\n got  %s\n want %s", c, without[c], exp)
		}
	}
	if onRead[f.cSeg] == without[f.cSeg] || onRead[f.cMix] == without[f.cMix] {
		t.Fatalf("the rule ON changed nothing -- the fixture does not exercise it")
	}
}

// I4 -- driving the claim then the batches to exhaustion with held-elsewhere subscribers, the sent
// ids number to_send (hazard 56) on every campaign, and no campaign_smart_skips row exists for a
// subscriber the rule excluded from that campaign -- though each of them has a recent same-brand
// journey send that would have made them a Smart Sending skip.
func TestBrandHoldSentEqualsToSend(t *testing.T) {
	f := newBrandHoldFixture(t)
	welcome := f.mkList("Acme welcome list", "single", false, "brand:acme")
	j := f.camp("Acme welcome", true, "regular", "running", `{}`, welcome)
	for _, s := range []int{f.plain, f.held, f.finalHeld, f.kickHeld} {
		f.send(j, s, "1 hour")
	}
	f.switchTo("true")

	toSend := f.claim()
	off := f.off()
	on := f.on()
	for _, c := range f.campaigns() {
		sent := f.batches(c)
		if len(sent) != toSend[c] {
			t.Errorf("campaign %d sent %d (%v), to_send %d", c, len(sent), sent, toSend[c])
		}
		var excluded []int
		for _, s := range []int{f.plain, f.held, f.finalHeld, f.kickHeld, f.betaHeld, f.txnHeld} {
			if containsID(off[c], s) && !containsID(on[c], s) {
				excluded = append(excluded, s)
			}
		}
		var n int
		f.h.db.Get(&n, `SELECT COUNT(*) FROM campaign_smart_skips WHERE campaign_id = $1 AND subscriber_id = ANY($2)`, c, pq.Array(excluded))
		if n != 0 {
			t.Errorf("campaign %d: %d smart-skip rows for excluded %v", c, n, excluded)
		}
	}
	// The fixture exercises Smart Sending at all: plain (in every audience) is skipped on the
	// segment broadcast, so its to_send is the on-set minus one.
	if toSend[f.cSeg] != countOf(on[f.cSeg])-1 {
		t.Fatalf("segment to_send %d, want %d (plain skipped)", toSend[f.cSeg], countOf(on[f.cSeg])-1)
	}
}

// I5 -- evergreen and opt-in campaigns are untouched: the evergreen candidate query
// (next-evergreen-subscribers) and an opt-in campaign's claim and batches return the same rows
// with the switch on and off, held-elsewhere subscribers included.
func TestBrandHoldOtherTypesUntouched(t *testing.T) {
	f := newBrandHoldFixture(t)
	dbl := f.mkList("Acme double", "double", false, "brand:acme")
	f.join(f.plain, dbl, "unconfirmed")
	f.join(f.held, dbl, "unconfirmed")
	opt := f.camp("Acme opt-in", false, "optin", "running", `{}`, dbl)

	ev := f.camp("Acme segment welcome", true, "regular", "running", `{}`, f.seg)
	f.h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - INTERVAL '1 day', send_delay_secs = 0 WHERE id = $1`, ev)
	f.h.db.MustExec(`UPDATE subscriber_lists SET confirmed_at = NOW() - INTERVAL '1 hour' WHERE list_id = $1`, f.seg)

	read := func() string {
		f.h.db.MustExec(`DELETE FROM campaign_sends WHERE campaign_id = $1`, ev)
		evOut, err := f.st.NextEvergreenSubscribers(ev, 100)
		if err != nil {
			t.Fatalf("next-evergreen-subscribers: %v", err)
		}
		var evIDs []int
		for _, s := range evOut {
			evIDs = append(evIDs, s.ID)
		}
		f.h.db.MustExec(`UPDATE campaigns SET status = 'running', last_subscriber_id = 0, to_send = 0 WHERE id = $1`, opt)
		if _, err := f.st.NextCampaigns([]int64{}, []int64{}); err != nil {
			t.Fatalf("next-campaigns: %v", err)
		}
		var toSend int
		f.h.db.Get(&toSend, `SELECT to_send FROM campaigns WHERE id = $1`, opt)
		return fmt.Sprintf("evergreen=%s optin to_send=%d batches=%s", ids(evIDs...), toSend, ids(f.batches(opt)...))
	}
	f.switchTo("false")
	offRead := read()
	f.switchTo("true")
	onRead := read()
	want := fmt.Sprintf("evergreen=%s optin to_send=2 batches=%s",
		ids(f.plain, f.held, f.finalHeld, f.kickHeld, f.betaHeld, f.txnHeld), ids(f.plain, f.held))
	if offRead != want || onRead != want {
		t.Fatalf("\n off %s\n on  %s\nwant %s", offRead, onRead, want)
	}
}
