package migrations

// Fork (holds) -- integrations SUNSET-SPEC I5, I6, I7 against a scratch database built from
// schema.sql with V6_2_10 run twice on top (newEvergreenHarness's ladder). Opt-in, e.g.
//
//	LISTMONK_TEST_PG='postgres://listmonk-dev:listmonk-dev@localhost:5432/listmonk-dev?sslmode=disable' go test ./internal/migrations/ -run 'ListStats|Engagement' -v

import (
	"database/sql"
	"log"
	"os"
	"strconv"
	"testing"
	"time"
)

// oldListStatsDDL is the pre-v6.2.10 view, the one a host upgrading to v6.2.10 carries.
const oldListStatsDDL = `
DROP INDEX IF EXISTS mat_list_subscriber_stats_idx;
DROP MATERIALIZED VIEW IF EXISTS mat_list_subscriber_stats;
CREATE MATERIALIZED VIEW mat_list_subscriber_stats AS
    SELECT NOW() AS updated_at, lists.id AS list_id, subscriber_lists.status, COUNT(subscriber_lists.status) AS subscriber_count FROM lists
    LEFT JOIN subscriber_lists ON (subscriber_lists.list_id = lists.id)
    GROUP BY lists.id, subscriber_lists.status
    UNION ALL
    SELECT NOW() AS updated_at, 0 AS list_id, NULL AS status, COUNT(id) AS subscriber_count FROM subscribers;
CREATE UNIQUE INDEX mat_list_subscriber_stats_idx ON mat_list_subscriber_stats (list_id, status);`

// TestListStats_HeldPseudoStatus -- I5. The view counts held rows under 'held' (a blocklisted
// subscriber's held row stays under 'unsubscribed'), the per-list SUM equals the row count, and
// query-subscribers-count-all answers for every status value against the TEXT column.
func TestListStats_HeldPseudoStatus(t *testing.T) {
	h := newEvergreenHarness(t)

	// The upgrade path: an old-shape view replaced by V6_2_10, twice.
	h.db.MustExec(oldListStatsDDL)
	lo := log.New(os.Stderr, "", 0)
	for i := 0; i < 2; i++ {
		if err := V6_2_10(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_10 run %d over the old view: %v", i+1, err)
		}
	}

	add := func(email, status, meta string, blocklisted bool) {
		id := h.subscriber(email)
		if blocklisted {
			h.db.MustExec(`UPDATE subscribers SET status='blocklisted' WHERE id=$1`, id)
		}
		h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status, meta) VALUES ($1, $2, $3, $4)`, id, h.listA, status, meta)
	}
	add("c1@x", "confirmed", `{}`, false)
	add("c2@x", "confirmed", `{}`, false)
	add("u1@x", "unconfirmed", `{}`, false)
	add("o1@x", "unsubscribed", `{}`, false)
	add("h1@x", "unsubscribed", `{"hold":{"reason":"never-engaged"}}`, false)
	add("h2@x", "unsubscribed", `{"hold":{"reason":"deliverability"}}`, false)
	add("hb@x", "unsubscribed", `{"hold":{"reason":"never-engaged"}}`, true)
	add("rel@x", "unsubscribed", `{"hold_released":{"to":"unsubscribed"}}`, false)
	h.db.MustExec(`REFRESH MATERIALIZED VIEW mat_list_subscriber_stats`)

	got := map[string]int{}
	rows, err := h.db.Query(`SELECT status, subscriber_count FROM mat_list_subscriber_stats WHERE list_id=$1`, h.listA)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s sql.NullString
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			t.Fatal(err)
		}
		got[s.String] = n
	}
	rows.Close()
	want := map[string]int{"confirmed": 2, "unconfirmed": 1, "unsubscribed": 3, "held": 2}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("stats %v, want %v", got, want)
		}
	}
	var sum, total int
	h.db.Get(&sum, `SELECT SUM(subscriber_count) FROM mat_list_subscriber_stats WHERE list_id=$1`, h.listA)
	h.db.Get(&total, `SELECT COUNT(*) FROM subscriber_lists WHERE list_id=$1`, h.listA)
	if sum != total || total != 8 {
		t.Fatalf("SUM %d != rows %d", sum, total)
	}

	countAll, err := h.db.Preparex(h.qs["query-subscribers-count-all"].Query)
	if err != nil {
		t.Fatalf("prepare count-all: %v", err)
	}
	for status, n := range map[string]int{"": 8, "confirmed": 2, "unconfirmed": 1, "unsubscribed": 3, "held": 2, "blocklisted": 0} {
		var c int
		if err := countAll.Get(&c, "{"+strconv.Itoa(h.listA)+"}", status); err != nil {
			t.Fatalf("count-all %q: %v", status, err)
		}
		if c != n {
			t.Fatalf("count-all %q = %d, want %d", status, c, n)
		}
	}
}

type engRow struct {
	SubscriberID  int          `db:"subscriber_id"`
	AnchorAt      time.Time    `db:"anchor_at"`
	EligibleSends int          `db:"eligible_sends"`
	FirstSendAt   sql.NullTime `db:"first_eligible_send_at"`
	LastSendAt    sql.NullTime `db:"last_eligible_send_at"`
	LastViewAt    sql.NullTime `db:"last_view_at"`
	LastClickAt   sql.NullTime `db:"last_click_at"`
}

func (h *evergreenHarness) engagement(list int) map[int]engRow {
	var rows []engRow
	if err := h.db.Select(&rows, `SELECT * FROM subscription_engagement($1)`, list); err != nil {
		h.t.Fatalf("subscription_engagement: %v", err)
	}
	out := map[int]engRow{}
	for _, r := range rows {
		out[r.SubscriberID] = r
	}
	return out
}

// sent inserts a campaign on list in the given end state.
func (h *evergreenHarness) sent(list int, typ, status string, evergreen bool, startedAt time.Time, maxSub int, lang string) int {
	attribs := `{}`
	if lang != "" {
		attribs = `{"lang":"` + lang + `"}`
	}
	var id int
	if err := h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, type, status, evergreen, started_at, max_subscriber_id, attribs, template_id)
		VALUES (gen_random_uuid(), 'c', 's', 'f@x', 'b', 'email', $1, $2, $3, $4, $5, $6, (SELECT id FROM templates LIMIT 1)) RETURNING id`,
		typ, status, evergreen, startedAt, maxSub, attribs); err != nil {
		h.t.Fatal(err)
	}
	h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'x')`, id, list)
	return id
}

func (h *evergreenHarness) anchorAt(sub, list int, at time.Time) {
	h.db.MustExec(`UPDATE subscriber_lists SET confirmed_at=$3, created_at=$3 WHERE subscriber_id=$1 AND list_id=$2`, sub, list, at)
}

// TestEngagementFn_EligibleSends -- I6, seven cases. eligible_sends counts the finished regular
// broadcasts whose send predicate includes the row NOW, and excludes a cancelled campaign, an
// evergreen, a campaign started before the anchor, a subscriber above max_subscriber_id, a
// language mismatch and a recorded send failure; first_eligible_send_at is the earliest.
func TestEngagementFn_EligibleSends(t *testing.T) {
	h := newEvergreenHarness(t)
	now := time.Now().UTC().Truncate(time.Second)
	t0 := now.Add(-100 * 24 * time.Hour)

	sub := h.subscriber("s@x")
	h.join(sub, h.listA, "confirmed")
	h.anchorAt(sub, h.listA, t0)
	fr := h.subscriber("fr@x")
	h.db.MustExec(`UPDATE subscribers SET attribs='{"lang":"FR-ca"}' WHERE id=$1`, fr)
	h.join(fr, h.listA, "confirmed")
	h.anchorAt(fr, h.listA, t0)
	high := h.subscriber("high@x") // highest id: above every max_subscriber_id below
	h.join(high, h.listA, "confirmed")
	h.anchorAt(high, h.listA, t0)
	maxSub := high - 1

	day := func(n int) time.Time { return t0.Add(time.Duration(n) * 24 * time.Hour) }

	// 1. Counts: two finished regular broadcasts after the anchor, no lang (everyone).
	h.sent(h.listA, "regular", "finished", false, day(10), maxSub, "")
	h.sent(h.listA, "regular", "finished", false, day(20), maxSub, "")
	// 2. Cancelled.
	h.sent(h.listA, "regular", "cancelled", false, day(30), maxSub, "")
	// 3. Evergreen (and an opt-in campaign, not regular).
	h.sent(h.listA, "regular", "finished", true, day(31), maxSub, "")
	h.sent(h.listA, "optin", "finished", false, day(32), maxSub, "")
	// 4. Started before the anchor.
	h.sent(h.listA, "regular", "finished", false, t0.Add(-time.Hour), maxSub, "")
	// 6. An en broadcast (fr subscriber excluded, s included) and an fr broadcast (fr only).
	h.sent(h.listA, "regular", "finished", false, day(40), maxSub, "en")
	h.sent(h.listA, "regular", "finished", false, day(41), maxSub, "fr")
	// 7. A send failure for s on a campaign that would otherwise count.
	failed := h.sent(h.listA, "regular", "finished", false, day(50), maxSub, "")
	h.db.MustExec(`INSERT INTO campaign_send_failures (campaign_id, subscriber_id, email, stage, error) VALUES ($1, $2, 's@x', 'send', 'x')`, failed, sub)
	// Another list's broadcast never counts.
	var listB int
	h.db.Get(&listB, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'B', 'private', 'single') RETURNING id`)
	h.join(sub, listB, "confirmed")
	h.sent(listB, "regular", "finished", false, day(60), maxSub, "")

	e := h.engagement(h.listA)

	// s: day10, day20, day40(en) = 3 (the day50 failure excluded).
	if e[sub].EligibleSends != 3 {
		t.Fatalf("s eligible_sends = %d, want 3", e[sub].EligibleSends)
	}
	if !e[sub].FirstSendAt.Valid || !e[sub].FirstSendAt.Time.Equal(day(10)) || !e[sub].LastSendAt.Time.Equal(day(40)) {
		t.Fatalf("s first/last = %v/%v", e[sub].FirstSendAt, e[sub].LastSendAt)
	}
	if !e[sub].AnchorAt.Equal(t0) {
		t.Fatalf("anchor = %v, want %v", e[sub].AnchorAt, t0)
	}
	// fr: day10, day20, day41(fr), day50 = 4 (en broadcast excluded; FR-ca normalises to fr).
	if e[fr].EligibleSends != 4 {
		t.Fatalf("fr eligible_sends = %d, want 4", e[fr].EligibleSends)
	}
	// 5. Above max_subscriber_id: nothing.
	if e[high].EligibleSends != 0 || e[high].FirstSendAt.Valid {
		t.Fatalf("high eligible_sends = %d, want 0", e[high].EligibleSends)
	}

	// A backfilled row (NULL confirmed_at) anchors at created_at.
	bf := h.subscriber("bf@x")
	h.join(bf, h.listA, "confirmed")
	h.db.MustExec(`UPDATE subscriber_lists SET confirmed_at=NULL, created_at=$3 WHERE subscriber_id=$1 AND list_id=$2`, bf, h.listA, day(15))
	h.db.MustExec(`UPDATE campaigns SET max_subscriber_id=$1`, bf)
	e = h.engagement(h.listA)
	if !e[bf].AnchorAt.Equal(day(15)) || e[bf].EligibleSends != 3 { // day20, day40(en), day50
		t.Fatalf("backfilled row: anchor %v sends %d", e[bf].AnchorAt, e[bf].EligibleSends)
	}

	// Only confirmed, non-blocklisted rows are returned.
	h.join(bf, h.listA, "unsubscribed")
	h.db.MustExec(`UPDATE subscribers SET status='blocklisted' WHERE id=$1`, fr)
	e = h.engagement(h.listA)
	if _, ok := e[bf]; ok {
		t.Fatal("unsubscribed row returned")
	}
	if _, ok := e[fr]; ok {
		t.Fatal("blocklisted subscriber returned")
	}
}

// TestEngagementView_ScopedToListAndAnchor -- I7. last_view_at / last_click_at consider only this
// list's campaigns and only events after the anchor; a re-confirm (new confirmed_at) resets both.
func TestEngagementView_ScopedToListAndAnchor(t *testing.T) {
	h := newEvergreenHarness(t)
	now := time.Now().UTC().Truncate(time.Second)
	t0 := now.Add(-10 * 24 * time.Hour)

	var listB int
	h.db.Get(&listB, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'B', 'private', 'single') RETURNING id`)
	sub := h.subscriber("s@x")
	h.join(sub, h.listA, "confirmed")
	h.anchorAt(sub, h.listA, t0)

	campA := h.sent(h.listA, "regular", "finished", false, t0.Add(time.Hour), sub, "")
	campB := h.sent(listB, "regular", "finished", false, t0.Add(time.Hour), sub, "")
	var link int
	h.db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://x.test') RETURNING id`)

	view := func(camp int, at time.Time) {
		h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id, created_at) VALUES ($1, $2, $3)`, camp, sub, at)
	}
	click := func(camp int, at time.Time) {
		h.db.MustExec(`INSERT INTO link_clicks (campaign_id, link_id, subscriber_id, created_at) VALUES ($1, $2, $3, $4)`, camp, link, sub, at)
	}

	// Other list's events, and this list's events before the anchor: invisible.
	view(campB, t0.Add(2*time.Hour))
	click(campB, t0.Add(2*time.Hour))
	view(campA, t0.Add(-time.Hour))
	click(campA, t0.Add(-time.Hour))
	if r := h.engagement(h.listA)[sub]; r.LastViewAt.Valid || r.LastClickAt.Valid {
		t.Fatalf("out-of-scope events counted: %+v", r)
	}

	// This list's events after the anchor: the latest wins.
	view(campA, t0.Add(3*time.Hour))
	view(campA, t0.Add(4*time.Hour))
	click(campA, t0.Add(5*time.Hour))
	r := h.engagement(h.listA)[sub]
	if !r.LastViewAt.Time.Equal(t0.Add(4*time.Hour)) || !r.LastClickAt.Time.Equal(t0.Add(5*time.Hour)) {
		t.Fatalf("in-scope events: %+v", r)
	}

	// Re-confirm: unsubscribe then confirm again restamps confirmed_at to NOW, after every event.
	h.join(sub, h.listA, "unsubscribed")
	h.join(sub, h.listA, "confirmed")
	r = h.engagement(h.listA)[sub]
	if !r.AnchorAt.After(t0.Add(5*time.Hour)) || r.LastViewAt.Valid || r.LastClickAt.Valid || r.EligibleSends != 0 {
		t.Fatalf("re-confirm did not reset: %+v", r)
	}

	// A HOLD re-permission re-confirms under listmonk.backfill (no welcome), which never restamps
	// confirmed_at -- so the anchor must follow hold_released.at, or every broadcast sent while
	// the person was held counts as an eligible send and the rule re-holds them the next day
	// (implementation review C1). Back to t0 so campA and a view count again, hold, then
	// release under backfill.
	h.anchorAt(sub, h.listA, t0)
	view(campA, t0.Add(3*time.Hour))
	if r := h.engagement(h.listA)[sub]; r.EligibleSends != 1 || !r.LastViewAt.Valid {
		t.Fatalf("pre-hold state: %+v", r)
	}
	h.db.MustExec(`UPDATE subscriber_lists SET status='unsubscribed', meta='{"hold":{"rule":"r","reason":"never-engaged","source":"t"}}' WHERE subscriber_id=$1 AND list_id=$2`, sub, h.listA)
	tx := h.db.MustBegin()
	tx.MustExec(`SET LOCAL listmonk.backfill = 'true'`)
	tx.MustExec(`UPDATE subscriber_lists SET status='confirmed' WHERE subscriber_id=$1 AND list_id=$2`, sub, h.listA)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if ca := h.confirmedAt(sub, h.listA); !ca.Valid || !ca.Time.Equal(t0) {
		t.Fatalf("backfill re-confirm restamped confirmed_at: %v", ca)
	}
	r = h.engagement(h.listA)[sub]
	if !r.AnchorAt.After(now.Add(-time.Minute)) || r.EligibleSends != 0 || r.LastViewAt.Valid {
		t.Fatalf("hold re-permission under backfill did not move the anchor: %+v", r)
	}
}
