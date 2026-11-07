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
