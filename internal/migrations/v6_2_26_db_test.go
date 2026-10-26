package migrations

// Fork (campaign send series) -- integrations CAMPAIGN-SEND-SERIES-SPEC I1/I2/I3/I5/I8, against a
// real Postgres through the shipped query files. Same opt-in harness as evergreen_db_test.go
// (LISTMONK_TEST_PG).
//   I1  the migration's backfill (broadcasts at started_at, evergreens from campaign_sends, paused
//       and started_at-less broadcasts skipped and counted in the log, a re-run duplicates nothing)
//       and the schema.sql mirror;
//   I2  the three live write points (next-campaigns' tick flush, update-campaign-counts,
//       mark-evergreen-sent) each count exactly once, evergreens only through the mark;
//   I3  get-campaign-send-counts buckets in Mountain wall-clock (a fixture straddling Mountain
//       midnight), and the typed bounce series sum to the untyped one;
//   I5  get-campaign-stats' hard_bounces / complaints in both tracking modes, bounces unchanged;
//   I8  the campaign 217 seed guard -- five rows summing 46,272, or none.

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
)

func (h *evergreenHarness) prepNamed(name string) *sqlx.Stmt {
	q, ok := h.qs[name]
	if !ok {
		h.t.Fatalf("query %s not found", name)
	}
	st, err := h.db.Preparex(q.Query)
	if err != nil {
		h.t.Fatalf("prepare %s: %v", name, err)
	}
	return st
}

// sendHours returns a campaign's rows as "RFC3339(UTC)=n", ordered by hour.
func (h *evergreenHarness) sendHours(camp int) []string {
	var rows []struct {
		Hour time.Time `db:"hour"`
		N    int       `db:"n"`
	}
	if err := h.db.Select(&rows, `SELECT hour, n FROM campaign_send_hours WHERE campaign_id = $1 ORDER BY hour`, camp); err != nil {
		h.t.Fatal(err)
	}
	out := []string{}
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s=%d", r.Hour.UTC().Format(time.RFC3339), r.N))
	}
	return out
}

func (h *evergreenHarness) sendSum(camp int) int {
	var n int
	if err := h.db.Get(&n, `SELECT COALESCE(SUM(n), 0) FROM campaign_send_hours WHERE campaign_id = $1`, camp); err != nil {
		h.t.Fatal(err)
	}
	return n
}

func TestCampaignSendHoursMigration_V6_2_26(t *testing.T) {
	h := newEvergreenHarness(t)

	// A v6.2.25 database with history: no send-hours table yet.
	h.db.MustExec(`DROP TABLE IF EXISTS campaign_send_hours`)

	fin := h.campaign("finished", false, 0, h.listA)
	can := h.campaign("cancelled", false, 0, h.listA)
	pau := h.campaign("paused", false, 0, h.listA)
	noStart := h.campaign("no-start", false, 0, h.listA)
	zero := h.campaign("zero", false, 0, h.listA)
	ever := h.campaign("evergreen", true, 0, h.listA)
	h.db.MustExec(`UPDATE campaigns SET status = 'finished', sent = 100, started_at = '2026-10-01 15:42:10+00' WHERE id = $1`, fin)
	h.db.MustExec(`UPDATE campaigns SET status = 'cancelled', sent = 7, started_at = '2026-10-02 08:00:00+00' WHERE id = $1`, can)
	h.db.MustExec(`UPDATE campaigns SET status = 'paused', sent = 50, started_at = '2026-10-03 09:10:00+00' WHERE id = $1`, pau)
	h.db.MustExec(`UPDATE campaigns SET status = 'finished', sent = 30, started_at = NULL WHERE id = $1`, noStart)
	h.db.MustExec(`UPDATE campaigns SET status = 'finished', sent = 0, started_at = '2026-10-04 09:10:00+00' WHERE id = $1`, zero)
	h.db.MustExec(`UPDATE campaigns SET status = 'running', sent = 3, started_at = '2026-10-01 00:00:00+00' WHERE id = $1`, ever)
	s1, s2, s3, s4 := h.subscriber("a@x"), h.subscriber("b@x"), h.subscriber("c@x"), h.subscriber("d@x")
	h.db.MustExec(`INSERT INTO campaign_sends (campaign_id, subscriber_id, sent_at) VALUES
		($1, $2, '2026-10-05 10:05:00+00'), ($1, $3, '2026-10-05 10:50:00+00'), ($1, $4, '2026-10-05 11:10:00+00'), ($1, $5, NULL)`,
		ever, s1, s2, s3, s4)

	var buf bytes.Buffer
	lo := log.New(&buf, "", 0)
	if err := V6_2_26(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_26: %v", err)
	}

	want := map[int][]string{
		fin:     {"2026-10-01T15:00:00Z=100"},
		can:     {"2026-10-02T08:00:00Z=7"},
		pau:     {},
		noStart: {},
		zero:    {},
		ever:    {"2026-10-05T10:00:00Z=2", "2026-10-05T11:00:00Z=1"},
	}
	check := func(when string) {
		for id, w := range want {
			if got := h.sendHours(id); strings.Join(got, ",") != strings.Join(w, ",") {
				t.Errorf("%s: campaign %d rows %v, want %v", when, id, got, w)
			}
		}
	}
	check("first run")

	logs := buf.String()
	if !strings.Contains(logs, fmt.Sprintf("2 broadcast(s) with sent > 0 skipped by the generic backfill (status not finished/cancelled, or no started_at): [%d %d]", pau, noStart)) {
		t.Errorf("the skipped broadcasts are not logged as expected:\n%s", logs)
	}
	if !strings.Contains(logs, "seed guard did not match") {
		t.Errorf("the missed 217 seed guard is not logged:\n%s", logs)
	}

	// A re-run finds the table and writes nothing (twice).
	for i := 0; i < 2; i++ {
		if err := V6_2_26(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_26 re-run %d: %v", i+1, err)
		}
	}
	check("re-run")

	// Shape -- the D1 columns, the primary key, cascade from campaigns.
	cols := map[string]string{"campaign_id": "integer NO", "hour": "timestamp with time zone NO", "n": "integer NO"}
	for col, w := range cols {
		var got string
		h.db.Get(&got, `SELECT data_type || ' ' || is_nullable FROM information_schema.columns WHERE table_name = 'campaign_send_hours' AND column_name = $1`, col)
		if got != w {
			t.Errorf("campaign_send_hours.%s = %q, want %q", col, got, w)
		}
	}
	var pk string
	h.db.Get(&pk, `SELECT pg_get_indexdef(i.indexrelid) FROM pg_index i WHERE i.indrelid = 'campaign_send_hours'::regclass AND i.indisprimary`)
	if !strings.Contains(pk, "(campaign_id, hour)") {
		t.Errorf("primary key = %q, want (campaign_id, hour)", pk)
	}
	h.db.MustExec(`DELETE FROM campaigns WHERE id = $1`, fin)
	if n := h.sendSum(fin); n != 0 {
		t.Errorf("deleting the campaign left %d sends (want ON DELETE CASCADE)", n)
	}

	// schema.sql contains the DROP line + the migration's DDL, byte-equal.
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	mirror := "DROP TABLE IF EXISTS campaign_send_hours CASCADE;\n" + strings.TrimSpace(campaignSendHoursDDL)
	if !strings.Contains(string(schema), mirror) {
		t.Fatal("schema.sql does not contain the DROP line followed by the v6.2.26 DDL, byte-equal")
	}
}

// I8 -- the winback seed guard.
func TestCampaignSendHoursWinbackSeed_V6_2_26(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	h.db.MustExec(`INSERT INTO campaigns (id, uuid, name, subject, from_email, body, messenger, template_id)
		VALUES (217, gen_random_uuid(), 'Winback', 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1))`)

	seed := []string{
		"2026-10-22T00:00:00Z=8107",
		"2026-10-22T23:00:00Z=10999",
		"2026-10-23T22:00:00Z=8433",
		"2026-10-23T23:00:00Z=5587",
		"2026-10-25T00:00:00Z=13146",
	}
	cases := []struct {
		name   string
		sent   int
		status string
		want   []string
	}{
		{"guard matches", 46272, "paused", seed},
		{"wrong sent", 46000, "paused", []string{}},
		{"cancelled", 46272, "cancelled", []string{}},
		{"running", 46272, "running", []string{}},
	}
	for _, c := range cases {
		h.db.MustExec(`DROP TABLE IF EXISTS campaign_send_hours`)
		// started_at NULL keeps the generic broadcast backfill out of the cancelled case, so the
		// rows counted here are the seed's alone.
		h.db.MustExec(`UPDATE campaigns SET sent = $1, status = $2, started_at = NULL WHERE id = 217`, c.sent, c.status)
		if err := V6_2_26(h.db, nil, nil, lo); err != nil {
			t.Fatalf("%s: V6_2_26: %v", c.name, err)
		}
		got := h.sendHours(217)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: rows %v, want %v", c.name, got, c.want)
		}
		if c.name == "guard matches" {
			if n := h.sendSum(217); n != 46272 {
				t.Errorf("seed sums to %d, want 46272", n)
			}
			// A re-run never seeds again.
			if err := V6_2_26(h.db, nil, nil, lo); err != nil {
				t.Fatal(err)
			}
			if n := h.sendSum(217); n != 46272 {
				t.Errorf("re-run: seed sums to %d, want 46272", n)
			}
		}
	}
}

// I2 -- the live write points.
func TestCampaignSendHoursWrites(t *testing.T) {
	h := newEvergreenHarness(t)
	next := h.prepNamed("next-campaigns")
	counts := h.prepNamed("update-campaign-counts")
	mark := h.prepNamed("mark-evergreen-sent")

	// Paused/finished, so next-campaigns claims nothing: only its updateCounts flush acts.
	bc := h.campaign("broadcast", false, 0, h.listA)
	ev := h.campaign("evergreen", true, 0, h.listA)
	h.db.MustExec(`UPDATE campaigns SET status = 'paused' WHERE id = ANY($1)`, pq.Array([]int{bc, ev}))

	sentOf := func(id int) int {
		var n int
		h.db.Get(&n, `SELECT sent FROM campaigns WHERE id = $1`, id)
		return n
	}

	// The tick flush -- a positive broadcast delta lands; the evergreen's does not (its sent still
	// moves, the existing behavior).
	if _, err := next.Exec(pq.Int64Array{int64(bc), int64(ev)}, pq.Int64Array{5, 4}); err != nil {
		t.Fatalf("next-campaigns: %v", err)
	}
	if got := h.sendSum(bc); got != 5 {
		t.Errorf("tick flush: broadcast sends %d, want 5", got)
	}
	if got := h.sendSum(ev); got != 0 {
		t.Errorf("tick flush: evergreen sends %d, want 0 (counted by the mark only)", got)
	}
	if sentOf(bc) != 5 || sentOf(ev) != 4 {
		t.Errorf("campaigns.sent = %d / %d, want 5 / 4", sentOf(bc), sentOf(ev))
	}
	// A second flush in the same hour adds to the hour's row; a zero writes nothing.
	if _, err := next.Exec(pq.Int64Array{int64(bc)}, pq.Int64Array{2}); err != nil {
		t.Fatal(err)
	}
	if _, err := next.Exec(pq.Int64Array{int64(bc)}, pq.Int64Array{0}); err != nil {
		t.Fatal(err)
	}
	if got := h.sendSum(bc); got != 7 {
		t.Errorf("after +2 and +0: broadcast sends %d, want 7", got)
	}

	// The pipe cleanup's residual.
	if _, err := counts.Exec(bc, 0, 3, 0); err != nil {
		t.Fatalf("update-campaign-counts: %v", err)
	}
	if _, err := counts.Exec(bc, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := counts.Exec(ev, 0, 2, 0); err != nil {
		t.Fatal(err)
	}
	if got := h.sendSum(bc); got != 10 {
		t.Errorf("after the residual: broadcast sends %d, want 10", got)
	}
	if got := h.sendSum(ev); got != 0 {
		t.Errorf("update-campaign-counts counted an evergreen (%d)", got)
	}
	if sentOf(bc) != 10 {
		t.Errorf("campaigns.sent = %d, want 10", sentOf(bc))
	}

	// The evergreen mark -- 1 per matched claim; a repeat mark (no unmarked claim) writes nothing;
	// two unmarked claims of one subscriber are ONE delivery.
	s1, s2 := h.subscriber("e1@x"), h.subscriber("e2@x")
	h.db.MustExec(`INSERT INTO campaign_sends (campaign_id, subscriber_id) VALUES ($1, $2), ($1, $3), ($1, $3)`, ev, s1, s2)
	for _, s := range []int{s1, s1, s2} {
		if _, err := mark.Exec(ev, s); err != nil {
			t.Fatalf("mark-evergreen-sent %d: %v", s, err)
		}
	}
	if got := h.sendSum(ev); got != 2 {
		t.Errorf("evergreen sends %d after marks of s1, s1 again, s2 (two claims), want 2", got)
	}
	// A mark on a subscriber with no claim at all writes nothing.
	if _, err := mark.Exec(ev, h.subscriber("e3@x")); err != nil {
		t.Fatal(err)
	}
	if got := h.sendSum(ev); got != 2 {
		t.Errorf("a no-op mark wrote a send (%d)", got)
	}

	// Every hour is a UTC hour boundary, and no zero rows exist.
	var bad int
	h.db.Get(&bad, `SELECT COUNT(*) FROM campaign_send_hours WHERE n <= 0 OR hour <> DATE_TRUNC('hour', hour, 'UTC')`)
	if bad != 0 {
		t.Errorf("%d rows with n <= 0 or an hour off the UTC hour", bad)
	}
}

// I3 -- Mountain buckets for sends; typed bounce series sum to the untyped one.
func TestCampaignSendAndBounceCounts(t *testing.T) {
	h := newEvergreenHarness(t)
	sends := h.prepNamed("get-campaign-send-counts")
	bounces := h.prepNamed("get-campaign-bounce-counts")

	c := h.campaign("c", false, 0, h.listA)
	// 05:00Z on Oct 22 is 23:00 MDT on Oct 21; 06:00Z is Mountain midnight of Oct 22.
	h.db.MustExec(`INSERT INTO campaign_send_hours (campaign_id, hour, n) VALUES
		($1, '2026-10-22 05:00:00+00', 10), ($1, '2026-10-22 06:00:00+00', 20), ($1, '2026-10-22 07:00:00+00', 5)`, c)

	type row struct {
		Count int
		TS    string
	}
	read := func(st *sqlx.Stmt, args ...any) []row {
		var out []models.CampaignAnalyticsCount
		if err := st.Select(&out, args...); err != nil {
			t.Fatal(err)
		}
		rs := []row{}
		for _, r := range out {
			rs = append(rs, row{r.Count, r.Timestamp.Format(models.AnalyticsBucketLayout)})
		}
		return rs
	}

	// Day buckets (a >= 7-day range): Mountain days, not UTC ones (UTC would put all three on Oct 22).
	day := read(sends, pq.Array([]int{c}), "2026-10-15T00:00:00Z", "2026-10-25T00:00:00Z")
	if fmt.Sprint(day) != fmt.Sprint([]row{{10, "2026-10-21T00:00:00"}, {25, "2026-10-22T00:00:00"}}) {
		t.Errorf("day buckets %v, want Oct 21 = 10, Oct 22 = 25 (Mountain)", day)
	}
	// Hour buckets: Mountain wall-clock hours.
	hour := read(sends, pq.Array([]int{c}), "2026-10-21T00:00:00Z", "2026-10-23T00:00:00Z")
	if fmt.Sprint(hour) != fmt.Sprint([]row{{10, "2026-10-21T23:00:00"}, {20, "2026-10-22T00:00:00"}, {5, "2026-10-22T01:00:00"}}) {
		t.Errorf("hour buckets %v, want 23:00 / 00:00 / 01:00 Mountain", hour)
	}
	// The window bounds the hours.
	if got := read(sends, pq.Array([]int{c}), "2026-10-22T06:00:00Z", "2026-10-22T06:30:00Z"); fmt.Sprint(got) != fmt.Sprint([]row{{20, "2026-10-22T00:00:00"}}) {
		t.Errorf("windowed buckets %v", got)
	}

	// Bounces: per type separately, the typed series summing to the untyped one per bucket.
	s1, s2 := h.subscriber("b1@x"), h.subscriber("b2@x")
	h.db.MustExec(`INSERT INTO bounces (subscriber_id, campaign_id, type, created_at) VALUES
		($1, $3, 'hard', '2026-10-22 05:30:00+00'), ($2, $3, 'hard', '2026-10-22 06:30:00+00'),
		($1, $3, 'soft', '2026-10-22 05:40:00+00'), ($2, $3, 'complaint', '2026-10-22 06:10:00+00'),
		($2, $3, 'soft', '2026-10-22 06:20:00+00')`, s1, s2, c)
	from, to := "2026-10-15T00:00:00Z", "2026-10-25T00:00:00Z"
	all := read(bounces, pq.Array([]int{c}), from, to, "")
	if fmt.Sprint(all) != fmt.Sprint([]row{{2, "2026-10-21T00:00:00"}, {3, "2026-10-22T00:00:00"}}) {
		t.Errorf("untyped bounce buckets %v", all)
	}
	sum := map[string]int{}
	for typ, w := range map[string][]row{
		"hard":      {{1, "2026-10-21T00:00:00"}, {1, "2026-10-22T00:00:00"}},
		"soft":      {{1, "2026-10-21T00:00:00"}, {1, "2026-10-22T00:00:00"}},
		"complaint": {{1, "2026-10-22T00:00:00"}},
	} {
		got := read(bounces, pq.Array([]int{c}), from, to, typ)
		if fmt.Sprint(got) != fmt.Sprint(w) {
			t.Errorf("%s bounce buckets %v, want %v", typ, got, w)
		}
		for _, r := range got {
			sum[r.TS] += r.Count
		}
	}
	for _, r := range all {
		if sum[r.TS] != r.Count {
			t.Errorf("bucket %s: typed series sum to %d, untyped %d", r.TS, sum[r.TS], r.Count)
		}
	}

	// The view/click counts statements bucket the same way (both tracking modes' statements).
	h.db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id, created_at) VALUES ($1, $2, '2026-10-22 05:30:00+00'), ($1, $3, '2026-10-22 06:30:00+00')`, c, s1, s2)
	for _, name := range []string{"get-campaign-analytics-counts", "get-campaign-analytics-unique-counts"} {
		st, err := h.db.Preparex(fmt.Sprintf(h.qs[name].Query, "campaign_views"))
		if err != nil {
			t.Fatalf("prepare %s: %v", name, err)
		}
		if got := read(st, pq.Array([]int{c}), from, to); fmt.Sprint(got) != fmt.Sprint([]row{{1, "2026-10-21T00:00:00"}, {1, "2026-10-22T00:00:00"}}) {
			t.Errorf("%s day buckets %v, want one view on each Mountain day", name, got)
		}
	}
}

// I5 -- get-campaign-stats' per-type fields in both tracking modes.
func TestCampaignStatsBounceTypes(t *testing.T) {
	h := newEvergreenHarness(t)
	c := h.campaign("c", false, 0, h.listA)
	other := h.campaign("other", false, 0, h.listA)
	s1, s2 := h.subscriber("s1@x"), h.subscriber("s2@x")
	h.db.MustExec(`INSERT INTO bounces (subscriber_id, campaign_id, type) VALUES
		($1, $3, 'hard'), ($1, $3, 'hard'), ($2, $3, 'soft'), ($2, $3, 'complaint'), ($2, $4, 'hard')`, s1, s2, c, other)

	for _, sel := range []string{"campaign_id", "DISTINCT subscriber_id"} {
		st, err := h.db.Preparex(fmt.Sprintf(h.qs["get-campaign-stats"].Query, sel))
		if err != nil {
			t.Fatalf("prepare get-campaign-stats (%s): %v", sel, err)
		}
		var meta []models.CampaignMeta
		if err := st.Select(&meta, pq.Array([]int{c, other})); err != nil {
			t.Fatalf("get-campaign-stats (%s): %v", sel, err)
		}
		if len(meta) != 2 {
			t.Fatalf("%d rows", len(meta))
		}
		m := meta[0]
		if m.Bounces != 4 || m.HardBounces != 2 || m.Complaints != 1 {
			t.Errorf("%s: bounces %d, hard %d, complaints %d -- want 4, 2 (raw rows), 1", sel, m.Bounces, m.HardBounces, m.Complaints)
		}
		if o := meta[1]; o.Bounces != 1 || o.HardBounces != 1 || o.Complaints != 0 {
			t.Errorf("%s: other campaign bounces %d, hard %d, complaints %d -- want 1, 1, 0", sel, o.Bounces, o.HardBounces, o.Complaints)
		}
	}
}
