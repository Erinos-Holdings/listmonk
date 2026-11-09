package main

// Fork (smart sending) -- integrations listmonk/SMART-SENDING-SPEC I1-I5 against a real database,
// through the REAL statements: brand_journey_sends_24h (schema.sql), the claim (next-campaigns,
// through store.NextCampaigns), the batches (next-campaign-subscribers, through store.NextSubscribers),
// get-campaign-lang-audience (core.FillAudience), get-campaign-stats and get-campaign-matrix. Shares
// newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go). Fixture names are acme.test /
// beta.test only.

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
)

type ssFixture struct {
	t *testing.T
	h *linkHarness
}

func (f ssFixture) list(name string, tags ...string) int {
	f.t.Helper()
	if tags == nil {
		tags = []string{}
	}
	var id int
	if err := f.h.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), $1, 'private', 'single', $2) RETURNING id`,
		name, pq.StringArray(tags)); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f ssFixture) sub(email, status string) int {
	f.t.Helper()
	var id int
	if err := f.h.db.Get(&id, `INSERT INTO subscribers (uuid, email, name, status) VALUES (gen_random_uuid(), $1, 'S', $2) RETURNING id`, email, status); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f ssFixture) join(sub, list int, status string) {
	f.t.Helper()
	f.h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, $3)`, sub, list, status)
}

// camp inserts a campaign of the type and status, with the attribs (JSON text) and its lists.
func (f ssFixture) camp(name string, evergreen bool, typ, status, attribs string, lists ...int) int {
	f.t.Helper()
	var id int
	if err := f.h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, content_type, messenger, evergreen, type, status, attribs)
		VALUES (gen_random_uuid(), $1, 's', 'hello@acme.test', 'b', 'plain', 'email', $2, $3::campaign_type, $4::campaign_status, $5::JSONB) RETURNING id`,
		name, evergreen, typ, status, attribs); err != nil {
		f.t.Fatal(err)
	}
	for _, l := range lists {
		f.h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'l')`, id, l)
	}
	return id
}

// send records a journey send for sub on camp, sent `ago` before now (a Postgres interval); ago ""
// is a claim with no send (sent_at NULL).
func (f ssFixture) send(camp, sub int, ago string) {
	f.t.Helper()
	if ago == "" {
		f.h.db.MustExec(`INSERT INTO campaign_sends (campaign_id, subscriber_id) VALUES ($1, $2)`, camp, sub)
		return
	}
	f.h.db.MustExec(`INSERT INTO campaign_sends (campaign_id, subscriber_id, claimed_at, sent_at)
		VALUES ($1, $2, NOW() - $3::INTERVAL, NOW() - $3::INTERVAL)`, camp, sub, ago)
}

// fn is brand_journey_sends_24h(camp) as "sub:journey" pairs, sorted.
func (f ssFixture) fn(camp int) string {
	f.t.Helper()
	var rows []struct {
		Sub     int `db:"subscriber_id"`
		Journey int `db:"journey_campaign_id"`
	}
	if err := f.h.db.Select(&rows, `SELECT subscriber_id, journey_campaign_id FROM brand_journey_sends_24h($1)`, camp); err != nil {
		f.t.Fatalf("brand_journey_sends_24h(%d): %v", camp, err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%d:%d", r.Sub, r.Journey))
	}
	sort.Strings(out)
	return fmt.Sprint(out)
}

func pairs(p ...[2]int) string {
	out := make([]string, 0, len(p))
	for _, x := range p {
		out = append(out, fmt.Sprintf("%d:%d", x[0], x[1]))
	}
	sort.Strings(out)
	return fmt.Sprint(out)
}

// I1 / I2 -- the function: rows for every subscriber with a same-brand evergreen send in the window
// (one row each, the newest journey); none for an evergreen, an opt-in campaign, smart_sending false
// or a campaign with no tagged list; a stored string value is "on" and raises nothing; a tagged list
// beside an untagged one resolves the brand; a claim with no send, a send at 24 h + 1 min and another
// brand's journey never count.
func TestSmartSendingFunction(t *testing.T) {
	f := ssFixture{t: t, h: newLinkHarness(t)}
	acme := f.list("Acme newsletter", "brand:acme")
	acmeJourney := f.list("Acme first order", "brand:acme")
	beta := f.list("Beta newsletter", "brand:beta")
	scratch := f.list("Scratch")

	j1 := f.camp("Acme welcome", true, "regular", "running", `{}`, acme)
	j2 := f.camp("Acme thanks", true, "regular", "running", `{}`, acmeJourney)
	k := f.camp("Beta welcome", true, "regular", "running", `{}`, beta)

	s1, s2, s3 := f.sub("s1@acme.test", "enabled"), f.sub("s2@acme.test", "enabled"), f.sub("s3@acme.test", "enabled")
	s4, s5 := f.sub("s4@acme.test", "enabled"), f.sub("s5@acme.test", "enabled")
	f.send(j1, s1, "1 hour")
	f.send(j1, s2, "2 hours")
	f.send(j2, s2, "30 minutes") // the newest of s2's two: one row, j2's
	f.send(k, s3, "1 hour")      // I2: another brand's journey
	f.send(j1, s4, "")           // a claim with no send
	f.send(j1, s5, "24 hours 1 minute")

	want := pairs([2]int{s1, j1}, [2]int{s2, j2})
	on := f.camp("Broadcast", false, "regular", "draft", `{}`, acme)
	if got := f.fn(on); got != want {
		t.Fatalf("broadcast: %s, want %s", got, want)
	}
	if got := f.fn(f.camp("Broadcast true", false, "regular", "draft", `{"smart_sending": true}`, acme)); got != want {
		t.Fatalf("smart_sending true: %s, want %s", got, want)
	}
	// S3: a string is "on" (the API refuses it; the SQL never raises on it).
	if got := f.fn(f.camp("Broadcast string", false, "regular", "draft", `{"smart_sending": "n/a"}`, acme)); got != want {
		t.Fatalf("smart_sending string: %s, want %s", got, want)
	}
	// S4: a tagged list beside an untagged scratch list resolves the brand.
	if got := f.fn(f.camp("Broadcast mixed", false, "regular", "draft", `{}`, acme, scratch)); got != want {
		t.Fatalf("tagged + untagged lists: %s, want %s", got, want)
	}
	for name, id := range map[string]int{
		"evergreen":           f.camp("Journey", true, "regular", "draft", `{}`, acme),
		"opt-in":              f.camp("Optin", false, "optin", "draft", `{}`, acme),
		"smart_sending false": f.camp("Launch", false, "regular", "draft", `{"smart_sending": false}`, acme),
		"no tagged list":      f.camp("Untagged", false, "regular", "draft", `{}`, scratch),
		"no lists":            f.camp("Listless", false, "regular", "draft", `{}`),
	} {
		if got := f.fn(id); got != "[]" {
			t.Errorf("%s: %s, want no rows", name, got)
		}
	}
	// I2: the beta broadcast sees only beta's journey.
	if got, want := f.fn(f.camp("Beta broadcast", false, "regular", "draft", `{}`, beta)), pairs([2]int{s3, k}); got != want {
		t.Fatalf("beta broadcast: %s, want %s", got, want)
	}
}

// I3 / I4 / I5 -- the claim records every skip and to_send = audience - skips; the batches return
// exactly the audience minus the recorded set; a recent journey send for someone outside the audience
// (blocklisted, unsubscribed) records nothing; the before-Start count equals the claim's row count and
// audience_skipped is null exactly when audience is; a re-claim after a pause records no skip for a
// subscriber at or below the checkpoint and leaves to_send unchanged; the stats and the matrix carry
// the count, and the matrix marks a switched-off broadcast.
func TestSmartSendingClaim(t *testing.T) {
	h := newLinkHarness(t)
	f := ssFixture{t: t, h: h}
	acme := f.list("Acme newsletter", "brand:acme")
	welcome := f.list("Acme welcome list", "brand:acme")
	j := f.camp("Acme welcome", true, "regular", "running", `{}`, welcome)

	var subs []int
	for i := 1; i <= 5; i++ {
		s := f.sub(fmt.Sprintf("a%d@acme.test", i), "enabled")
		f.join(s, acme, "unconfirmed")
		subs = append(subs, s)
	}
	blocked := f.sub("blocked@acme.test", "blocklisted")
	f.join(blocked, acme, "unconfirmed")
	gone := f.sub("gone@acme.test", "enabled")
	f.join(gone, acme, "unsubscribed")
	// Recent journey sends: two in the audience, two outside it.
	for _, s := range []int{subs[1], subs[3], blocked, gone} {
		f.send(j, s, "2 hours")
	}

	b := f.camp("Acme broadcast", false, "regular", "draft", `{}`, acme)
	off := f.camp("Acme launch", false, "regular", "draft", `{"smart_sending": false}`, acme)

	// I5 (before Start): FillAudience fills audience and audience_skipped together for a draft.
	cm, err := h.app.core.GetCampaign(b, "", "")
	if err != nil {
		t.Fatal(err)
	}
	h.app.core.FillAudience(&cm)
	if !cm.Audience.Valid || cm.Audience.Int != 5 || !cm.AudienceSkipped.Valid || cm.AudienceSkipped.Int != 2 || !cm.SmartSendingOn {
		t.Fatalf("draft audience %v skipped %v on %v, want 5 / 2 / true", cm.Audience, cm.AudienceSkipped, cm.SmartSendingOn)
	}
	cmOff, _ := h.app.core.GetCampaign(off, "", "")
	h.app.core.FillAudience(&cmOff)
	if cmOff.AudienceSkipped.Int != 0 || cmOff.SmartSendingOn {
		t.Fatalf("switched-off draft skipped %v on %v, want 0 / false", cmOff.AudienceSkipped, cmOff.SmartSendingOn)
	}
	before := cm.AudienceSkipped.Int

	// The claim (both broadcasts at once, as a scan tick picks them up).
	h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE id = ANY($1)`, pq.Array([]int{b, off}))
	st := &store{queries: h.q, core: h.app.core}
	// The manager passes non-nil (possibly empty) arrays: a nil one binds NULL and claims nothing.
	if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
		t.Fatalf("next-campaigns: %v", err)
	}
	skipRows := func(camp int) []int {
		var ids []int
		if err := h.db.Select(&ids, `SELECT subscriber_id FROM campaign_smart_skips WHERE campaign_id = $1 ORDER BY subscriber_id`, camp); err != nil {
			t.Fatal(err)
		}
		return ids
	}
	toSend := func(camp int) int {
		var n int
		h.db.Get(&n, `SELECT to_send FROM campaigns WHERE id = $1`, camp)
		return n
	}
	if got := skipRows(b); fmt.Sprint(got) != fmt.Sprint([]int{subs[1], subs[3]}) {
		t.Fatalf("skip rows %v, want %v (to_send %d)", got, []int{subs[1], subs[3]}, toSend(b))
	}
	if before != len(skipRows(b)) {
		t.Fatalf("the before-Start count %d != the claim's %d rows", before, len(skipRows(b)))
	}
	if n := toSend(b); n != 3 {
		t.Fatalf("to_send %d, want 3", n)
	}
	if len(skipRows(off)) != 0 || toSend(off) != 5 {
		t.Fatalf("switched off: %d skip rows, to_send %d, want 0 / 5", len(skipRows(off)), toSend(off))
	}
	var jid int
	h.db.Get(&jid, `SELECT journey_campaign_id FROM campaign_smart_skips WHERE campaign_id = $1 LIMIT 1`, b)
	if jid != j {
		t.Fatalf("journey_campaign_id %d, want %d", jid, j)
	}

	// I5 (after the claim): a running broadcast carries neither audience nor audience_skipped.
	cm, _ = h.app.core.GetCampaign(b, "", "")
	h.app.core.FillAudience(&cm)
	if cm.Audience.Valid || cm.AudienceSkipped.Valid {
		t.Fatalf("running: audience %v skipped %v, want both null", cm.Audience, cm.AudienceSkipped)
	}

	// The batches: exactly the audience minus the recorded set, never a recorded subscriber.
	batches := func(camp int) []int {
		var got []int
		for i := 0; i < 10; i++ {
			out, err := st.NextSubscribers(camp, 2)
			if err != nil {
				t.Fatalf("next-campaign-subscribers: %v", err)
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
	if got, want := batches(b), []int{subs[0], subs[2], subs[4]}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("batches %v, want %v", got, want)
	}
	if got := batches(off); len(got) != 5 {
		t.Fatalf("switched-off batches %v, want all 5", got)
	}

	// The stats (the Campaigns list) carry the recorded count.
	camps := models.Campaigns{{Base: models.Base{ID: b}}, {Base: models.Base{ID: off}}}
	if err := camps.LoadStats(h.q.GetCampaignStats); err != nil {
		t.Fatal(err)
	}
	if camps[0].Skipped != 2 || camps[1].Skipped != 0 {
		t.Fatalf("stats skipped %d / %d, want 2 / 0", camps[0].Skipped, camps[1].Skipped)
	}

	// The matrix: a broadcast in the window with something sent (sent is the manager's counter,
	// set here), the count, and smart_off on the switched-off one; a journey's skipped is null.
	h.db.MustExec(`UPDATE campaigns SET sent = 3 WHERE id = ANY($1)`, pq.Array([]int{b, off}))
	h.db.MustExec(`INSERT INTO campaign_send_hours (campaign_id, hour, n) VALUES ($1, DATE_TRUNC('hour', NOW(), 'UTC'), 1)`, j)
	raw, err := h.app.core.GetCampaignMatrix(true, nil, false, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var mx struct {
		Rows []struct {
			ID      int  `json:"id"`
			Skipped *int `json:"skipped"`
			// A pointer, so a NULL smart_off (not a real boolean) fails the pin (Stage 4 R2).
			SmartOff *bool `json:"smart_off"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &mx); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, r := range mx.Rows {
		switch r.ID {
		case b:
			seen++
			if r.Skipped == nil || *r.Skipped != 2 || r.SmartOff == nil || *r.SmartOff {
				t.Fatalf("matrix broadcast %+v (smart_off must be false, not null)", r)
			}
		case off:
			seen++
			if r.Skipped == nil || *r.Skipped != 0 || r.SmartOff == nil || !*r.SmartOff {
				t.Fatalf("matrix switched-off %+v", r)
			}
		case j:
			seen++
			if r.Skipped != nil || r.SmartOff == nil || *r.SmartOff {
				t.Fatalf("matrix journey %+v", r)
			}
		}
	}
	if seen != 3 {
		t.Fatalf("matrix rows %s", raw)
	}

	// I4 -- a re-claim after a pause: the checkpoint is past subs[0..2], and subs[0] (at or below it,
	// already sent) gets a fresh journey send. The re-claim records nothing for subs[0], keeps the
	// earlier skips (subs[1] is below the checkpoint now and in the table only), and to_send stays 3.
	h.db.MustExec(`UPDATE campaigns SET status = 'running', last_subscriber_id = $2 WHERE id = $1`, b, subs[2])
	f.send(j, subs[0], "5 minutes")
	if _, err := st.NextCampaigns([]int64{}, []int64{}); err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if got := skipRows(b); fmt.Sprint(got) != fmt.Sprint([]int{subs[1], subs[3]}) {
		t.Fatalf("after the re-claim skip rows %v, want %v", got, []int{subs[1], subs[3]})
	}
	if n := toSend(b); n != 3 {
		t.Fatalf("after the re-claim to_send %d, want 3", n)
	}
	if got, want := batches(b), []int{subs[4]}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("after the re-claim batches %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------------------------
// Fork (smart sending look-ahead) -- integrations listmonk/SMART-SENDING-LOOKAHEAD-SPEC L1-L8
// against a real database, through the REAL statements: brand_journey_due_24h,
// brand_journey_skips and smart_sending_brand (schema.sql), the claim (next-campaigns), the batches
// (next-campaign-subscribers) and get-campaign-lang-audience (core.FillAudience).

// lookahead sets the switch row app.smart_sending_lookahead to the JSONB value v ("" deletes it).
func (f ssFixture) lookahead(v string) {
	f.t.Helper()
	f.h.db.MustExec(`DELETE FROM settings WHERE key = 'app.smart_sending_lookahead'`)
	if v != "" {
		f.h.db.MustExec(`INSERT INTO settings (key, value) VALUES ('app.smart_sending_lookahead', $1::JSONB)`, v)
	}
}

// evergreen inserts an evergreen of the status, started `startedAgo` before now, with the send
// delay and attribs, on the lists.
func (f ssFixture) evergreen(name, status, startedAgo string, delaySecs int, attribs string, lists ...int) int {
	f.t.Helper()
	id := f.camp(name, true, "regular", status, attribs, lists...)
	f.h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - $2::INTERVAL, send_delay_secs = $3 WHERE id = $1`, id, startedAgo, delaySecs)
	return id
}

// confirmed joins sub to list as confirmed, the join stamped `ago` before now.
func (f ssFixture) confirmed(sub, list int, ago string) {
	f.t.Helper()
	f.join(sub, list, "confirmed")
	f.h.db.MustExec(`UPDATE subscriber_lists SET confirmed_at = NOW() - $3::INTERVAL WHERE subscriber_id = $1 AND list_id = $2`, sub, list, ago)
}

// due writes a forecast row -- the run at now + in (a Postgres interval, may be negative) will join
// sub to list.
func (f ssFixture) due(list, sub int, in string) {
	f.t.Helper()
	f.h.db.MustExec(`INSERT INTO journey_due (source, list_id, subscriber_id, expected_at) VALUES ('series', $1, $2, NOW() + $3::INTERVAL)`, list, sub, in)
}

// fnDue is brand_journey_due_24h(camp) as "sub:list" pairs, sorted.
func (f ssFixture) fnDue(camp int) string {
	f.t.Helper()
	var rows []struct {
		Sub  int `db:"subscriber_id"`
		List int `db:"due_list_id"`
	}
	if err := f.h.db.Select(&rows, `SELECT subscriber_id, due_list_id FROM brand_journey_due_24h($1)`, camp); err != nil {
		f.t.Fatalf("brand_journey_due_24h(%d): %v", camp, err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%d:%d", r.Sub, r.List))
	}
	sort.Strings(out)
	return fmt.Sprint(out)
}

// fnSkips is brand_journey_skips(camp) as "sub:journey:list" triples (0 for a NULL), sorted, and
// the number of rows (to catch a subscriber returned twice).
func (f ssFixture) fnSkips(camp int) string {
	f.t.Helper()
	var rows []struct {
		Sub     int  `db:"subscriber_id"`
		Journey *int `db:"journey_campaign_id"`
		List    *int `db:"due_list_id"`
	}
	if err := f.h.db.Select(&rows, `SELECT subscriber_id, journey_campaign_id, due_list_id FROM brand_journey_skips($1)`, camp); err != nil {
		f.t.Fatalf("brand_journey_skips(%d): %v", camp, err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		j, l := 0, 0
		if r.Journey != nil {
			j = *r.Journey
		}
		if r.List != nil {
			l = *r.List
		}
		out = append(out, fmt.Sprintf("%d:%d:%d", r.Sub, j, l))
	}
	sort.Strings(out)
	return fmt.Sprint(out)
}

func triples(p ...[3]int) string {
	out := make([]string, 0, len(p))
	for _, x := range p {
		out = append(out, fmt.Sprintf("%d:%d:%d", x[0], x[1], x[2]))
	}
	sort.Strings(out)
	return fmt.Sprint(out)
}

// laFixture is the L1-L4 world: brand acme's newsletter (the broadcasts' list) and its journey
// lists, a beta list, and one fixture per L2/L3 case. want* are the expected function rows when the
// switch is on.
type laFixture struct {
	news, step, welcome, delayed, enOnly, paused, late, beta int
	welcomeEv                                                int
	wantDue                                                  string // brand_journey_due_24h, switch on
	wantSends                                                string // brand_journey_sends_24h
	wantSkipsOn                                              string // brand_journey_skips, switch on
}

func newLAFixture(f ssFixture) laFixture {
	var x laFixture
	x.news = f.list("Acme newsletter", "brand:acme")
	x.step = f.list("Acme no purchase 2", "brand:acme")
	x.welcome = f.list("Acme welcome", "brand:acme")
	x.delayed = f.list("Acme delayed", "brand:acme")
	x.enOnly = f.list("Acme EN step", "brand:acme")
	x.paused = f.list("Acme paused step", "brand:acme")
	x.late = f.list("Acme late start", "brand:acme")
	x.beta = f.list("Beta step", "brand:beta")

	f.evergreen("Acme step 2", "running", "10 days", 0, `{}`, x.step)
	x.welcomeEv = f.evergreen("Acme welcome", "running", "10 days", 0, `{}`, x.welcome)
	f.evergreen("Acme delayed", "running", "10 days", 3*86400, `{}`, x.delayed)
	f.evergreen("Acme EN step", "running", "10 days", 0, `{"lang": "en"}`, x.enOnly)
	f.evergreen("Acme paused step", "paused", "10 days", 0, `{}`, x.paused)
	f.evergreen("Acme late start", "running", "1 minute", 0, `{}`, x.late)
	f.evergreen("Beta step", "running", "10 days", 0, `{}`, x.beta)

	sub := func(email, lang string) int {
		id := f.sub(email, "enabled")
		if lang != "" {
			f.h.db.MustExec(`UPDATE subscribers SET attribs = JSONB_BUILD_OBJECT('lang', $2::TEXT) WHERE id = $1`, id, lang)
		}
		return id
	}

	// L2 -- forecast rows.
	inWin := sub("in-window@acme.test", "")
	f.due(x.step, inWin, "2 hours")
	f.due(x.step, inWin, "5 hours") // a second, later forecast: one row, the earliest
	f.due(x.step, sub("ahead-25h@acme.test", ""), "25 hours")
	f.due(x.step, sub("past-3h@acme.test", ""), "-3 hours")
	f.due(x.paused, sub("paused@acme.test", ""), "2 hours")
	f.due(x.beta, sub("beta@acme.test", ""), "2 hours")
	f.due(x.enOnly, sub("fr@acme.test", "fr"), "2 hours")
	enNoLang := sub("no-lang@acme.test", "")
	f.due(x.enOnly, enNoLang, "2 hours") // the control: no language is English
	graceIn := sub("grace@acme.test", "")
	f.due(x.step, graceIn, "-1 hour") // inside the 2-hour grace

	// L3 -- actual joins not yet sent.
	inHold := sub("in-hold@acme.test", "")
	f.confirmed(inHold, x.welcome, "5 minutes") // a welcome inside its hold
	delayedIn := sub("delayed-2h@acme.test", "")
	f.confirmed(delayedIn, x.delayed, "70 hours")                        // due 3 d after, i.e. in 2 hours
	f.confirmed(sub("delayed-30h@acme.test", ""), x.delayed, "42 hours") // due in 30 hours
	f.confirmed(sub("before-start@acme.test", ""), x.late, "5 minutes")  // joined before started_at
	// Stage 4 R1 -- an absurd send delay (an API write of microseconds for seconds) on a RUNNING
	// same-brand evergreen must never make the function raise (timestamp out of range inside the
	// claim would stall every broadcast of the brand); it simply contributes no row.
	absurd := f.list("Acme absurd delay", "brand:acme")
	f.evergreen("Acme absurd delay", "running", "10 days", 1_000_000_000_000, `{}`, absurd)
	f.due(absurd, sub("absurd-forecast@acme.test", ""), "2 hours")
	f.confirmed(sub("absurd-join@acme.test", ""), absurd, "5 minutes")
	sent := sub("sent@acme.test", "")
	f.confirmed(sent, x.welcome, "10 minutes")
	f.send(x.welcomeEv, sent, "5 minutes") // already sent: a look-back row, not a look-ahead one

	x.wantDue = pairs([2]int{inWin, x.step}, [2]int{enNoLang, x.enOnly}, [2]int{graceIn, x.step},
		[2]int{inHold, x.welcome}, [2]int{delayedIn, x.delayed})
	x.wantSends = pairs([2]int{sent, x.welcomeEv})
	x.wantSkipsOn = triples([3]int{inWin, 0, x.step}, [3]int{enNoLang, 0, x.enOnly}, [3]int{graceIn, 0, x.step},
		[3]int{inHold, 0, x.welcome}, [3]int{delayedIn, 0, x.delayed}, [3]int{sent, x.welcomeEv, 0})
	return x
}

// L2 / L3 -- switched on, the look-ahead returns exactly the forecast rows that pass all three
// conditions and the actual unsent joins due in the window; one row per subscriber (the earliest);
// someone already sent is a look-back row and appears once in brand_journey_skips.
func TestLookaheadFunction(t *testing.T) {
	f := ssFixture{t: t, h: newLinkHarness(t)}
	x := newLAFixture(f)
	f.lookahead("true")
	b := f.camp("Acme broadcast", false, "regular", "draft", `{}`, x.news)

	if got := f.fnDue(b); got != x.wantDue {
		t.Fatalf("brand_journey_due_24h %s, want %s", got, x.wantDue)
	}
	var due string
	f.h.db.Get(&due, `SELECT (due_at - NOW() BETWEEN INTERVAL '119 minutes' AND INTERVAL '121 minutes')::TEXT FROM brand_journey_due_24h($1) d
		JOIN subscribers s ON s.id = d.subscriber_id WHERE s.email = 'in-window@acme.test'`, b)
	if due != "true" {
		t.Fatalf("the in-window subscriber's due_at is not the earliest forecast (+2 h)")
	}
	if got := f.fnSkips(b); got != x.wantSkipsOn {
		t.Fatalf("brand_journey_skips %s, want %s", got, x.wantSkipsOn)
	}
	// I2's twin -- the beta broadcast sees beta's forecast only.
	bb := f.camp("Beta broadcast", false, "regular", "draft", `{}`, x.beta)
	var betaSub int
	f.h.db.Get(&betaSub, `SELECT id FROM subscribers WHERE email = 'beta@acme.test'`)
	if got, want := f.fnDue(bb), pairs([2]int{betaSub, x.beta}); got != want {
		t.Fatalf("beta broadcast %s, want %s", got, want)
	}
}

// L1 -- dark by default: with the switch row absent, false or any non-true value, brand_journey_skips
// is exactly brand_journey_sends_24h, whatever journey_due and the pending joins hold.
func TestLookaheadDark(t *testing.T) {
	f := ssFixture{t: t, h: newLinkHarness(t)}
	x := newLAFixture(f)
	b := f.camp("Acme broadcast", false, "regular", "draft", `{}`, x.news)

	var sent int
	f.h.db.Get(&sent, `SELECT id FROM subscribers WHERE email = 'sent@acme.test'`)
	wantDark := triples([3]int{sent, x.welcomeEv, 0})
	for _, v := range []string{"", "false", `"true"`, "1", "null", `{}`, `[true]`} {
		f.lookahead(v)
		if got := f.fn(b); got != x.wantSends {
			t.Fatalf("switch %q: brand_journey_sends_24h %s, want %s", v, got, x.wantSends)
		}
		if got := f.fnDue(b); got != "[]" {
			t.Fatalf("switch %q: brand_journey_due_24h %s, want no rows", v, got)
		}
		if got := f.fnSkips(b); got != wantDark {
			t.Fatalf("switch %q: brand_journey_skips %s, want the look-back rows only %s", v, got, wantDark)
		}
	}
	// The fixture does hold look-ahead rows: the switch is what hides them.
	f.lookahead("true")
	if got := f.fnSkips(b); got != x.wantSkipsOn {
		t.Fatalf("switched on: %s, want %s", got, x.wantSkipsOn)
	}
}

// L4 -- the off switch disables BOTH halves and journeys stay exempt: smart_sending false, an
// evergreen, an opt-in campaign and an untagged campaign get no row from either half, with
// look-back and look-ahead fixtures both present and the switch on.
func TestSmartSendingOffBothHalves(t *testing.T) {
	f := ssFixture{t: t, h: newLinkHarness(t)}
	x := newLAFixture(f)
	f.lookahead("true")
	scratch := f.list("Scratch")

	on := f.camp("Acme broadcast", false, "regular", "draft", `{}`, x.news)
	if f.fn(on) == "[]" || f.fnDue(on) == "[]" {
		t.Fatalf("the control broadcast lacks a half: back %s ahead %s", f.fn(on), f.fnDue(on))
	}
	for name, id := range map[string]int{
		"smart_sending false": f.camp("Launch", false, "regular", "draft", `{"smart_sending": false}`, x.news),
		"evergreen":           f.camp("Journey", true, "regular", "running", `{}`, x.news),
		"opt-in":              f.camp("Optin", false, "optin", "draft", `{}`, x.news),
		"untagged":            f.camp("Untagged", false, "regular", "draft", `{}`, scratch),
	} {
		var brand *string
		f.h.db.Get(&brand, `SELECT smart_sending_brand($1)`, id)
		if brand != nil {
			t.Errorf("%s: smart_sending_brand %q, want NULL", name, *brand)
		}
		if got := f.fn(id); got != "[]" {
			t.Errorf("%s: look-back %s, want no rows", name, got)
		}
		if got := f.fnDue(id); got != "[]" {
			t.Errorf("%s: look-ahead %s, want no rows", name, got)
		}
		if got := f.fnSkips(id); got != "[]" {
			t.Errorf("%s: brand_journey_skips %s, want no rows", name, got)
		}
	}
}

// L5 / L6 / L7 / L8 (the both-halves fixture) -- the claim records every skip of both kinds with its
// pair of columns; to_send = audience - skip rows; the batches return exactly the audience minus the
// recorded set and number to_send; a forecast for someone outside the audience records nothing; a
// subscriber with both a look-back send and a forecast is one row with the look-back pair; the
// before-Start count equals the claim's row count switch off and on; a re-claim after a pause
// records no skip for a new forecast at or below the checkpoint and leaves to_send unchanged.
func TestLookaheadClaim(t *testing.T) {
	h := newLinkHarness(t)
	f := ssFixture{t: t, h: h}
	news := f.list("Acme newsletter", "brand:acme")
	welcome := f.list("Acme welcome list", "brand:acme")
	step := f.list("Acme step list", "brand:acme")
	j := f.evergreen("Acme welcome", "running", "10 days", 0, `{}`, welcome)
	f.evergreen("Acme step", "running", "10 days", 0, `{}`, step)

	var a []int
	for i := 1; i <= 6; i++ {
		s := f.sub(fmt.Sprintf("a%d@acme.test", i), "enabled")
		f.join(s, news, "unconfirmed")
		a = append(a, s)
	}
	blocked := f.sub("blocked@acme.test", "blocklisted")
	f.join(blocked, news, "unconfirmed")

	f.send(j, a[1], "2 hours")              // a2 -- look-back
	f.due(step, a[2], "2 hours")            // a3 -- forecast
	f.confirmed(a[3], welcome, "5 minutes") // a4 -- an actual join not yet sent
	f.send(j, a[4], "3 hours")              // a5 -- BOTH halves (L8): one row, the look-back pair
	f.due(step, a[4], "1 hour")
	f.due(step, blocked, "2 hours") // outside the audience: records nothing

	b1 := f.camp("Acme broadcast dark", false, "regular", "draft", `{}`, news)
	b2 := f.camp("Acme broadcast lit", false, "regular", "draft", `{}`, news)
	st := &store{queries: h.q, core: h.app.core}

	preview := func(camp int) int {
		cm, err := h.app.core.GetCampaign(camp, "", "")
		if err != nil {
			t.Fatal(err)
		}
		h.app.core.FillAudience(&cm)
		if !cm.Audience.Valid || cm.Audience.Int != 6 || !cm.AudienceSkipped.Valid {
			t.Fatalf("campaign %d preview audience %v skipped %v", camp, cm.Audience, cm.AudienceSkipped)
		}
		return int(cm.AudienceSkipped.Int)
	}
	type skipRow struct {
		Sub     int     `db:"subscriber_id"`
		Journey *int    `db:"journey_campaign_id"`
		SentAt  *string `db:"journey_sent_at"`
		DueList *int    `db:"due_list_id"`
		DueAt   *string `db:"due_at"`
	}
	skipRows := func(camp int) []skipRow {
		var rows []skipRow
		if err := h.db.Select(&rows, `SELECT subscriber_id, journey_campaign_id, journey_sent_at::TEXT, due_list_id, due_at::TEXT
			FROM campaign_smart_skips WHERE campaign_id = $1 ORDER BY subscriber_id`, camp); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	toSend := func(camp int) int {
		var n int
		h.db.Get(&n, `SELECT to_send FROM campaigns WHERE id = $1`, camp)
		return n
	}
	batches := func(camp int) []int {
		var got []int
		for i := 0; i < 20; i++ {
			out, err := st.NextSubscribers(camp, 2)
			if err != nil {
				t.Fatalf("next-campaign-subscribers: %v", err)
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
	claim := func(camp int, inFlight ...int64) {
		h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE id = $1`, camp)
		sent := make([]int64, len(inFlight))
		if inFlight == nil {
			inFlight = []int64{}
		}
		if _, err := st.NextCampaigns(inFlight, sent); err != nil {
			t.Fatalf("next-campaigns: %v", err)
		}
	}

	// Switch off (L7, dark): the preview and the claim agree on the look-back half alone.
	f.lookahead("false")
	if n := preview(b1); n != 2 {
		t.Fatalf("dark preview skipped %d, want 2 (a2, a5)", n)
	}
	claim(b1)
	if rows := skipRows(b1); len(rows) != 2 || rows[0].Sub != a[1] || rows[1].Sub != a[4] {
		t.Fatalf("dark claim rows %+v", rows)
	}
	if n := toSend(b1); n != 4 {
		t.Fatalf("dark to_send %d, want 4", n)
	}

	// Switch on (L5, L7, L8).
	f.lookahead("true")
	before := preview(b2)
	claim(b2, int64(b1)) // b1 is in flight: not re-claimed
	rows := skipRows(b2)
	if before != len(rows) || before != 4 {
		t.Fatalf("lit preview %d, claim rows %d, want 4 / 4: %+v", before, len(rows), rows)
	}
	want := map[int]string{a[1]: "back", a[2]: "ahead", a[3]: "ahead", a[4]: "back"}
	for _, r := range rows {
		kind := ""
		switch {
		case r.Journey != nil && r.SentAt != nil && r.DueList == nil && r.DueAt == nil:
			kind = "back"
			if *r.Journey != j {
				t.Fatalf("subscriber %d journey %d, want %d", r.Sub, *r.Journey, j)
			}
		case r.Journey == nil && r.SentAt == nil && r.DueList != nil && r.DueAt != nil:
			kind = "ahead"
			wantList := step
			if r.Sub == a[3] {
				wantList = welcome
			}
			if *r.DueList != wantList {
				t.Fatalf("subscriber %d due_list %d, want %d", r.Sub, *r.DueList, wantList)
			}
		}
		if want[r.Sub] != kind {
			t.Fatalf("subscriber %d recorded as %q, want %q (%+v)", r.Sub, kind, want[r.Sub], r)
		}
	}
	if n := toSend(b2); n != 2 {
		t.Fatalf("lit to_send %d, want 2", n)
	}
	got := batches(b2)
	if fmt.Sprint(got) != fmt.Sprint([]int{a[0], a[5]}) {
		t.Fatalf("batches %v, want %v", got, []int{a[0], a[5]})
	}
	if len(got) != toSend(b2) {
		t.Fatalf("sent ids %d != to_send %d (hazard 56)", len(got), toSend(b2))
	}
	camps := models.Campaigns{{Base: models.Base{ID: b2}}}
	if err := camps.LoadStats(h.q.GetCampaignStats); err != nil {
		t.Fatal(err)
	}
	if camps[0].Skipped != 4 {
		t.Fatalf("stats skipped %d, want 4 (both kinds)", camps[0].Skipped)
	}

	// L6 -- a re-claim after a pause: the checkpoint is past a1..a3; a fresh forecast for a1 (at or
	// below it) records nothing and to_send is unchanged.
	h.db.MustExec(`UPDATE campaigns SET status = 'running', last_subscriber_id = $2 WHERE id = $1`, b2, a[2])
	f.due(step, a[0], "30 minutes")
	claim(b2, int64(b1))
	if after := skipRows(b2); len(after) != 4 {
		t.Fatalf("after the re-claim %d skip rows, want 4: %+v", len(after), after)
	}
	if n := toSend(b2); n != 2 {
		t.Fatalf("after the re-claim to_send %d, want 2", n)
	}
}
