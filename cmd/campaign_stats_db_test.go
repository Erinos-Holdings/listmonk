package main

// Fork (campaign list rates) -- integrations CAMPAIGN-RATES-SPEC I1 and I2 against a real database:
// get-campaign-stats, prepared through prepareQueries exactly as cmd/init.go does at boot, counts
// views and clicks per distinct non-NULL subscriber with individual tracking ON and raw rows with
// it OFF; bounces are raw in both. Rows come back in input order and an unknown id yields a zero
// row. Shares newLinkHarness (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go).

import (
	"fmt"
	"testing"

	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
)

func TestCampaignStatsCounting(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db

	newCamp := func(name string) int {
		var id int
		db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id) VALUES (gen_random_uuid(), $1, 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id`, name)
		return id
	}
	newSub := func(email string) int {
		var id int
		db.Get(&id, `INSERT INTO subscribers (uuid, email, name) VALUES (gen_random_uuid(), $1, 'S') RETURNING id`, email)
		return id
	}
	var link int
	db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://x.test') RETURNING id`)

	// sub 0 = a NULL-subscriber row.
	view := func(camp, sub, times int) {
		for i := 0; i < times; i++ {
			db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id) VALUES ($1, NULLIF($2, 0))`, camp, sub)
		}
	}
	click := func(camp, sub, times int) {
		for i := 0; i < times; i++ {
			db.MustExec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id) VALUES ($1, NULLIF($2, 0), $3)`, camp, sub, link)
		}
	}
	bounce := func(camp, sub, times int) {
		for i := 0; i < times; i++ {
			db.MustExec(`INSERT INTO bounces (campaign_id, subscriber_id, source) VALUES ($1, $2, 'test')`, camp, sub)
		}
	}

	known, known2 := newCamp("Known"), newCamp("Known2")
	if known == 0 || known2 == 0 {
		t.Fatal("campaign seed failed")
	}
	s1, s2 := newSub("s1@x"), newSub("s2@x")

	// I1's seed on the known campaign: one subscriber with three views and two clicks, a second
	// with one view, one NULL-subscriber view, two bounce rows (the same subscriber, so a distinct
	// bounce count would read 1).
	view(known, s1, 3)
	click(known, s1, 2)
	view(known, s2, 1)
	view(known, 0, 1)
	bounce(known, s1, 2)
	// known2 gets a distinguishable, mode-independent shape so the order check means something.
	view(known2, s2, 1)

	ids := []int{known, 999999, known2}
	stats := func(tracking bool) string {
		ko.Set("privacy.individual_tracking", tracking)
		defer ko.Set("privacy.individual_tracking", true)
		q := prepareQueries(loadQueryMap(t), db, ko)

		var rows []models.CampaignMeta
		if err := q.GetCampaignStats.Select(&rows, pq.Array(ids)); err != nil {
			t.Fatalf("tracking=%v: %v", tracking, err)
		}
		out := ""
		for _, r := range rows {
			id := r.CampaignID
			switch id {
			case known:
				id = -1
			case known2:
				id = -2
			}
			out += fmt.Sprintf("[%d v%d c%d b%d]", id, r.Views, r.Clicks, r.Bounces)
		}
		return out
	}

	// ON (I1): views = distinct non-NULL subscribers (s1, s2), clicks = s1, bounces raw. I2: input
	// order, the unknown id as a zero row between the two known ones.
	if got, want := stats(true), "[-1 v2 c1 b2][999999 v0 c0 b0][-2 v1 c0 b0]"; got != want {
		t.Fatalf("individual tracking ON\n got  %s\n want %s", got, want)
	}
	// OFF (I1): all three raw.
	if got, want := stats(false), "[-1 v5 c2 b2][999999 v0 c0 b0][-2 v1 c0 b0]"; got != want {
		t.Fatalf("individual tracking OFF\n got  %s\n want %s", got, want)
	}
}
