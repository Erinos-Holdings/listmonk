package migrations

// Fork (erinos evergreen campaigns) — DB harness for the trigger, the eligibility
// query and the status/collision queries, run through the REAL goyesql-parsed
// statements against a scratch database built from schema.sql (then V6_2_2 on top,
// proving idempotency). Opt-in: set LISTMONK_TEST_PG to a superuser-ish DSN on a
// throwaway Postgres, e.g. the dev suite's
//
//	LISTMONK_TEST_PG='postgres://listmonk-dev:listmonk-dev@localhost:5432/listmonk-dev?sslmode=disable' go test ./internal/migrations/ -run Evergreen -v
//
// The test creates and drops a database named listmonk_evergreen_test.

import (
	"database/sql"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/goyesql/v2"
	"github.com/lib/pq"
)

const evergreenTestDB = "listmonk_evergreen_test"

type evergreenHarness struct {
	t     *testing.T
	db    *sqlx.DB
	qs    goyesql.Queries
	next  *sqlx.Stmt // next-evergreen-subscribers
	coll  *sqlx.Stmt // get-evergreen-collision
	stat  *sqlx.Stmt // update-campaign-status
	listA int
}

func newEvergreenHarness(t *testing.T) *evergreenHarness {
	dsn := os.Getenv("LISTMONK_TEST_PG")
	if dsn == "" {
		t.Skip("LISTMONK_TEST_PG not set")
	}

	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	admin.MustExec("DROP DATABASE IF EXISTS " + evergreenTestDB)
	admin.MustExec("CREATE DATABASE " + evergreenTestDB)
	admin.Close()

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("dsn: %v", err)
	}
	u.Path = "/" + evergreenTestDB
	db, err := sqlx.Connect("postgres", u.String())
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		admin, err := sqlx.Connect("postgres", dsn)
		if err == nil {
			admin.Exec("DROP DATABASE IF EXISTS " + evergreenTestDB)
			admin.Close()
		}
	})

	root := filepath.Join("..", "..")
	schema, err := os.ReadFile(filepath.Join(root, "schema.sql"))
	if err != nil {
		t.Fatalf("schema.sql: %v", err)
	}
	db.MustExec(string(schema))

	// The migration must be a no-op on a fresh schema and idempotent on itself.
	lo := log.New(os.Stderr, "", 0)
	for i := 0; i < 2; i++ {
		if err := V6_2_2(db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_2 run %d: %v", i+1, err)
		}
		// Fork (multi-language campaigns) -- v6.2.3 is idempotent too.
		if err := V6_2_3(db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_3 run %d: %v", i+1, err)
		}
		// Fork (footer guard) -- and v6.2.4.
		if err := V6_2_4(db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_4 run %d: %v", i+1, err)
		}
	}

	// Parse every shipped query file exactly as the app does and prepare the ones under test.
	files, _ := filepath.Glob(filepath.Join(root, "queries", "*.sql"))
	qs := goyesql.Queries{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		mp, err := goyesql.ParseBytes(b)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for k, v := range mp {
			qs[k] = v
		}
	}
	prep := func(name string) *sqlx.Stmt {
		q, ok := qs[name]
		if !ok {
			t.Fatalf("query %s not found", name)
		}
		st, err := db.Preparex(q.Query)
		if err != nil {
			t.Fatalf("prepare %s: %v", name, err)
		}
		return st
	}

	h := &evergreenHarness{t: t, db: db, qs: qs,
		next: prep("next-evergreen-subscribers"),
		coll: prep("get-evergreen-collision"),
		stat: prep("update-campaign-status"),
	}
	db.Get(&h.listA, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'A', 'private', 'single') RETURNING id`)
	db.MustExec(`INSERT INTO templates (name, type, subject, body, is_default) VALUES ('tpl', 'campaign', '', 'TPL-V1', true)`)
	return h
}

func (h *evergreenHarness) campaign(name string, evergreen bool, delaySecs int64, lists ...int) int {
	var id int
	if err := h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, evergreen, send_delay_secs, template_id)
		VALUES (gen_random_uuid(), $1, 's', 'f@x', 'b', 'email', $2, $3, (SELECT id FROM templates LIMIT 1)) RETURNING id`, name, evergreen, delaySecs); err != nil {
		h.t.Fatal(err)
	}
	for _, l := range lists {
		h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'x')`, id, l)
	}
	return id
}

func (h *evergreenHarness) subscriber(email string) int {
	var id int
	if err := h.db.Get(&id, `INSERT INTO subscribers (uuid, email, name) VALUES (gen_random_uuid(), $1, $1) RETURNING id`, email); err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *evergreenHarness) join(sub, list int, status string) {
	h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, $3)
		ON CONFLICT (subscriber_id, list_id) DO UPDATE SET status = EXCLUDED.status`, sub, list, status)
}

func (h *evergreenHarness) confirmedAt(sub, list int) sql.NullTime {
	var ts sql.NullTime
	if err := h.db.Get(&ts, `SELECT confirmed_at FROM subscriber_lists WHERE subscriber_id=$1 AND list_id=$2`, sub, list); err != nil {
		h.t.Fatal(err)
	}
	return ts
}

func (h *evergreenHarness) setStatus(camp int, status string) {
	if _, err := h.stat.Exec(camp, status); err != nil {
		h.t.Fatalf("update-campaign-status %s: %v", status, err)
	}
}

// batch runs the eligibility query and returns the subscriber ids it chose (and recorded).
func (h *evergreenHarness) batch(camp, limit int) []int {
	rows, err := h.next.Queryx(camp, limit)
	if err != nil {
		h.t.Fatalf("next-evergreen-subscribers: %v", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		m := map[string]interface{}{}
		if err := rows.MapScan(m); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, int(m["id"].(int64)))
	}
	return out
}

func (h *evergreenHarness) collides(camp int, delay int64, lists []int, vg *string) bool {
	return h.collidesLang(camp, delay, lists, vg, "")
}

// collidesLang -- lang "" is an everyone (no attribs.lang) campaign. Fork (multi-language).
func (h *evergreenHarness) collidesLang(camp int, delay int64, lists []int, vg *string, lang string) bool {
	var row struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	var vgv sql.NullString
	if vg != nil {
		vgv = sql.NullString{String: *vg, Valid: true}
	}
	err := h.coll.Get(&row, camp, delay, pq.Array(lists), vgv, sql.NullString{String: lang, Valid: lang != ""})
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		h.t.Fatalf("get-evergreen-collision: %v", err)
	}
	return true
}

func eq(t *testing.T, name string, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %v want %v", name, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: got %v want %v", name, got, want)
		}
	}
}

func TestEvergreenTriggerAndEligibility(t *testing.T) {
	h := newEvergreenHarness(t)
	qs := h.qs
	L := h.listA
	welcome := h.campaign("welcome", true, 0, L)

	// --- Trigger: stamped on the transition into confirmed, not on an overwrite, not
	// under the backfill setting, not for unconfirmed rows.
	pre := h.subscriber("pre@x")
	h.join(pre, L, "confirmed")
	if !h.confirmedAt(pre, L).Valid {
		t.Fatal("confirmed insert must stamp confirmed_at")
	}
	unc := h.subscriber("unc@x")
	h.join(unc, L, "unconfirmed")
	if h.confirmedAt(unc, L).Valid {
		t.Fatal("unconfirmed insert must not stamp confirmed_at")
	}

	// --- Watermark: nothing before start is eligible; a never-started evergreen sends nothing.
	eq(t, "unstarted", h.batch(welcome, 100), nil)
	h.setStatus(welcome, "running")
	var started sql.NullTime
	h.db.Get(&started, `SELECT started_at FROM campaigns WHERE id=$1`, welcome)
	if !started.Valid {
		t.Fatal("update-campaign-status must stamp started_at for an evergreen")
	}
	eq(t, "pre-start joiner", h.batch(welcome, 100), nil)

	// --- A post-start join is welcomed exactly once.
	time.Sleep(5 * time.Millisecond)
	b := h.subscriber("b@x")
	h.join(b, L, "confirmed")
	eq(t, "post-start joiner", h.batch(welcome, 100), []int{b})
	eq(t, "already sent", h.batch(welcome, 100), nil)
	var sends int
	h.db.Get(&sends, `SELECT COUNT(*) FROM campaign_sends WHERE campaign_id=$1 AND subscriber_id=$2`, welcome, b)
	if sends != 1 {
		t.Fatalf("campaign_sends rows for b: %d", sends)
	}

	// --- Overwrite of an already-confirmed row does not move confirmed_at or re-welcome.
	before := h.confirmedAt(b, L)
	h.join(b, L, "confirmed")
	if !h.confirmedAt(b, L).Time.Equal(before.Time) {
		t.Fatal("overwrite of a confirmed row moved confirmed_at")
	}
	eq(t, "overwrite", h.batch(welcome, 100), nil)

	// --- Backfill: rows written under the setting have no confirmed_at and are never eligible.
	bf := h.subscriber("backfill@x")
	tx := h.db.MustBegin()
	tx.MustExec(`SET LOCAL listmonk.backfill = 'true'`)
	tx.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, bf, L)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if h.confirmedAt(bf, L).Valid {
		t.Fatal("backfill row must have NULL confirmed_at")
	}
	eq(t, "backfill", h.batch(welcome, 100), nil)
	// And a backfilled unsubscribed->confirmed flip is likewise silent.
	tx = h.db.MustBegin()
	tx.MustExec(`SET LOCAL listmonk.backfill = 'true'`)
	tx.MustExec(`UPDATE subscriber_lists SET status='unsubscribed' WHERE subscriber_id=$1`, bf)
	tx.MustExec(`UPDATE subscriber_lists SET status='confirmed' WHERE subscriber_id=$1`, bf)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if h.confirmedAt(bf, L).Valid {
		t.Fatal("backfill flip must not stamp confirmed_at")
	}

	// --- Re-opt-in: unsubscribed -> confirmed advances confirmed_at and welcomes again.
	h.join(b, L, "unsubscribed")
	eq(t, "unsubscribed exit", h.batch(welcome, 100), nil)
	time.Sleep(5 * time.Millisecond)
	h.join(b, L, "confirmed")
	if !h.confirmedAt(b, L).Time.After(before.Time) {
		t.Fatal("re-opt-in must advance confirmed_at")
	}
	eq(t, "re-opt-in", h.batch(welcome, 100), []int{b})
	h.db.Get(&sends, `SELECT COUNT(*) FROM campaign_sends WHERE campaign_id=$1 AND subscriber_id=$2`, welcome, b)
	if sends != 2 {
		t.Fatalf("campaign_sends is append-only; rows for b: %d", sends)
	}

	// --- Blocklisted subscribers are never eligible.
	bl := h.subscriber("bl@x")
	h.join(bl, L, "confirmed")
	h.db.MustExec(`UPDATE subscribers SET status='blocklisted' WHERE id=$1`, bl)
	eq(t, "blocklisted", h.batch(welcome, 100), nil)

	// --- Batch cap and ordering by join time.
	c1 := h.subscriber("c1@x")
	h.join(c1, L, "confirmed")
	time.Sleep(5 * time.Millisecond)
	c2 := h.subscriber("c2@x")
	h.join(c2, L, "confirmed")
	eq(t, "cap 1", h.batch(welcome, 1), []int{c1})
	eq(t, "cap next", h.batch(welcome, 1), []int{c2})
	eq(t, "drained", h.batch(welcome, 100), nil)

	// --- Pause / resume: started_at is preserved, the template snapshot is re-taken.
	h.db.MustExec(`UPDATE campaigns SET frozen_template_body='STALE' WHERE id=$1`, welcome)
	h.db.MustExec(`UPDATE templates SET body='TPL-V2'`)
	h.setStatus(welcome, "paused")
	h.setStatus(welcome, "running")
	var after struct {
		StartedAt sql.NullTime   `db:"started_at"`
		Frozen    sql.NullString `db:"frozen_template_body"`
	}
	h.db.Get(&after, `SELECT started_at, frozen_template_body FROM campaigns WHERE id=$1`, welcome)
	if !after.StartedAt.Time.Equal(started.Time) {
		t.Fatal("resume moved started_at (the watermark)")
	}
	if after.Frozen.String != "TPL-V2" {
		t.Fatalf("resume must re-freeze the template for an evergreen, got %q", after.Frozen.String)
	}
	// A regular campaign keeps upstream's once-only freeze.
	reg := h.campaign("regular", false, 0, L)
	h.setStatus(reg, "running")
	h.db.MustExec(`UPDATE templates SET body='TPL-V3'`)
	h.setStatus(reg, "paused")
	h.setStatus(reg, "running")
	h.db.Get(&after, `SELECT started_at, frozen_template_body FROM campaigns WHERE id=$1`, reg)
	if after.Frozen.String != "TPL-V2" {
		t.Fatalf("regular campaign freeze must stay once-only, got %q", after.Frozen.String)
	}
	if after.StartedAt.Valid {
		t.Fatal("update-campaign-status must not stamp started_at for a regular campaign (next-campaigns does)")
	}

	// --- Delay: a day-3 step on the same list.
	day3 := h.campaign("day3", true, 3*86400, L)
	h.setStatus(day3, "running")
	time.Sleep(5 * time.Millisecond)
	d := h.subscriber("d@x")
	h.join(d, L, "confirmed")
	eq(t, "delay not elapsed", h.batch(day3, 100), nil)
	h.db.MustExec(`UPDATE subscriber_lists SET confirmed_at = NOW() - INTERVAL '4 days' WHERE subscriber_id=$1`, d)
	eq(t, "delay elapsed", h.batch(day3, 100), nil) // joined before day3's watermark once backdated
	h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - INTERVAL '10 days' WHERE id=$1`, day3)
	eq(t, "delay elapsed, after watermark", h.batch(day3, 100), []int{d})

	// --- Collision: same list + same delay = collision; different delay is not; same variant group is not.
	draft := h.campaign("draft-dup", true, 0, L)
	if h.collides(welcome, 0, []int{L}, nil) {
		t.Fatal("welcome must not collide with itself")
	}
	if !h.collides(draft, 0, []int{L}, nil) {
		t.Fatal("a new delay-0 evergreen on L must collide with the running welcome")
	}
	if h.collides(draft, 86400, []int{L}, nil) {
		t.Fatal("a different delay must not collide")
	}
	vg := "11111111-1111-1111-1111-111111111111"
	h.db.MustExec(`UPDATE campaigns SET variant_group_id=$2 WHERE id=$1`, welcome, vg)
	if h.collides(draft, 0, []int{L}, &vg) {
		t.Fatal("same variant group must not collide")
	}
	other := "22222222-2222-2222-2222-222222222222"
	if !h.collides(draft, 0, []int{L}, &other) {
		t.Fatal("a different variant group must collide")
	}

	// --- Variant-group exclusion: a send from any campaign in the group excludes the subscriber.
	vb := h.campaign("welcome-B", true, 0, L)
	h.db.MustExec(`UPDATE campaigns SET variant_group_id=$2 WHERE id=$1`, vb, vg)
	h.setStatus(vb, "running")
	time.Sleep(5 * time.Millisecond)
	e := h.subscriber("e@x")
	h.join(e, L, "confirmed")
	eq(t, "variant A sends", h.batch(welcome, 100), []int{e})
	eq(t, "variant B excluded", h.batch(vb, 100), nil)

	// --- Claims (review C1): a fetched row is claimed with no sent_at; a recent unattempted
	// claim excludes the subscriber; a released claim (dropped unattempted) makes them
	// eligible again; a claim older than an hour with no attempt is abandoned; an attempted
	// claim (sent_at set, success or failure) is final.
	mark := func(camp, sub int) { h.db.MustExec(qs["mark-evergreen-sent"].Query, camp, sub) }
	release := func(camp, sub int) { h.db.MustExec(qs["release-evergreen-claim"].Query, camp, sub) }
	cl := h.subscriber("claim@x")
	h.join(cl, L, "confirmed")
	eq(t, "claimed", h.batch(welcome, 100), []int{cl})
	var sentNull bool
	h.db.Get(&sentNull, `SELECT sent_at IS NULL FROM campaign_sends WHERE campaign_id=$1 AND subscriber_id=$2`, welcome, cl)
	if !sentNull {
		t.Fatal("a fetch must claim (sent_at NULL), not mark sent")
	}
	eq(t, "recent claim excludes", h.batch(welcome, 100), nil)
	release(welcome, cl)
	eq(t, "released claim re-eligible", h.batch(welcome, 100), []int{cl})
	h.db.MustExec(`UPDATE campaign_sends SET claimed_at = NOW() - INTERVAL '2 hours' WHERE campaign_id=$1 AND subscriber_id=$2`, welcome, cl)
	eq(t, "abandoned claim re-eligible", h.batch(welcome, 100), []int{cl})
	mark(welcome, cl)
	h.db.MustExec(`UPDATE campaign_sends SET claimed_at = NOW() - INTERVAL '2 hours' WHERE campaign_id=$1 AND subscriber_id=$2`, welcome, cl)
	eq(t, "attempted claim is final", h.batch(welcome, 100), nil)
	release(welcome, cl)
	eq(t, "release never touches an attempted row", h.batch(welcome, 100), nil)

	// --- Cancel is terminal for eligibility: a cancelled evergreen returns nothing.
	h.setStatus(welcome, "cancelled")
	f := h.subscriber("f@x")
	h.join(f, L, "confirmed")
	eq(t, "cancelled", h.batch(welcome, 100), nil)
}
