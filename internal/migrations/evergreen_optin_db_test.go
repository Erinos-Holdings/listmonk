package migrations

// Fork (erinos evergreen campaigns) — DB harness for the single opt-in default: a
// membership created on a single opt-in list with no explicit confirmed status is stored
// confirmed (and so stamps confirmed_at, the welcome anchor); double opt-in lists keep
// upstream behaviour. Exercises the REAL insert-subscriber, update-subscriber-with-lists,
// add-subscribers-to-lists and add-subscribers-to-lists-by-query statements. Shares
// newEvergreenHarness (LISTMONK_TEST_PG opt-in, see evergreen_db_test.go).

import (
	"strings"
	"testing"

	"github.com/lib/pq"
)

func TestEvergreenSingleOptinDefault(t *testing.T) {
	h := newEvergreenHarness(t)
	prep := func(name string) string {
		q, ok := h.qs[name]
		if !ok {
			t.Fatalf("query %s not found", name)
		}
		return q.Query
	}
	ins := prep("insert-subscriber")
	upd := prep("update-subscriber-with-lists")
	add := prep("add-subscribers-to-lists")
	addQ := strings.ReplaceAll(prep("add-subscribers-to-lists-by-query"), "%query%", "SELECT id FROM subscribers WHERE id = ANY($1::INT[]) AND $2::TEXT IS NOT NULL AND $3::INT IS NOT NULL AND $4::INT IS NOT NULL")

	var listB int // double opt-in
	h.db.Get(&listB, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'B', 'private', 'double') RETURNING id`)
	status := func(sub, list int) (string, bool) {
		var st string
		var ts pq.NullTime
		if err := h.db.QueryRow(`SELECT status, confirmed_at FROM subscriber_lists WHERE subscriber_id=$1 AND list_id=$2`, sub, list).Scan(&st, &ts); err != nil {
			t.Fatalf("status %d/%d: %v", sub, list, err)
		}
		return st, ts.Valid
	}
	want := func(name string, sub, list int, st string, stamped bool) {
		gs, gt := status(sub, list)
		if gs != st || gt != stamped {
			t.Errorf("%s: got %s stamped=%v, want %s stamped=%v", name, gs, gt, st, stamped)
		}
	}

	// 1. insert-subscriber with the admin default ($8 = unconfirmed) onto A (single) and B (double).
	var s1 int
	if err := h.db.Get(&s1, ins, "11111111-1111-1111-1111-111111111111", "one@x", "one", "enabled", "{}", pq.Array([]int{h.listA, listB}), pq.Array([]string{}), "unconfirmed"); err != nil {
		t.Fatalf("insert-subscriber: %v", err)
	}
	want("insert single -> confirmed", s1, h.listA, "confirmed", true)
	want("insert double -> unconfirmed", s1, listB, "unconfirmed", false)

	// 2. update-subscriber-with-lists (admin edit form: no preconfirm, deleteLists true):
	//    drop A, then re-add A -> fresh row, confirmed again.
	if _, err := h.db.Exec(upd, s1, "", "", "enabled", "", pq.Array([]int{listB}), pq.Array([]string{}), "unconfirmed", true, pq.Array([]int{}), false); err != nil {
		t.Fatalf("update drop A: %v", err)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM subscriber_lists WHERE subscriber_id=$1 AND list_id=$2`, s1, h.listA)
	if n != 0 {
		t.Fatalf("drop A: row still present")
	}
	if _, err := h.db.Exec(upd, s1, "", "", "enabled", "", pq.Array([]int{h.listA, listB}), pq.Array([]string{}), "unconfirmed", true, pq.Array([]int{}), false); err != nil {
		t.Fatalf("update re-add A: %v", err)
	}
	want("edit form re-add single -> confirmed", s1, h.listA, "confirmed", true)
	want("edit form keeps double unconfirmed", s1, listB, "unconfirmed", false)
	// An unsubscribed row is retained by the admin edit form (no resubscribe flag).
	h.join(s1, h.listA, "unsubscribed")
	if _, err := h.db.Exec(upd, s1, "", "", "enabled", "", pq.Array([]int{h.listA, listB}), pq.Array([]string{}), "unconfirmed", true, pq.Array([]int{}), false); err != nil {
		t.Fatal(err)
	}
	want("edit form retains unsubscribed", s1, h.listA, "unsubscribed", true)
	// An existing unconfirmed row (legacy shape) is NOT promoted by an unrelated admin edit.
	h.db.MustExec(`UPDATE subscriber_lists SET status='unconfirmed', confirmed_at=NULL WHERE subscriber_id=$1 AND list_id=$2`, s1, h.listA)
	if _, err := h.db.Exec(upd, s1, "", "renamed", "", "", pq.Array([]int{h.listA, listB}), pq.Array([]string{}), "unconfirmed", true, pq.Array([]int{}), false); err != nil {
		t.Fatal(err)
	}
	want("edit form does not promote legacy unconfirmed", s1, h.listA, "unconfirmed", false)
	h.join(s1, h.listA, "unsubscribed")
	// The public form (allow resubscribe) flips it back to confirmed on a single opt-in list.
	var uuidA string
	h.db.Get(&uuidA, `SELECT uuid FROM lists WHERE id=$1`, h.listA)
	if _, err := h.db.Exec(upd, s1, "", "", "enabled", "", pq.Array([]int{}), pq.Array([]string{uuidA}), "unconfirmed", false, pq.Array([]int{}), true); err != nil {
		t.Fatal(err)
	}
	want("public form resubscribe single -> confirmed", s1, h.listA, "confirmed", true)

	// 3. add-subscribers-to-lists with no status: single -> confirmed, double -> unconfirmed;
	//    an existing unconfirmed row on single is promoted; unsubscribed is retained; explicit status wins.
	s2 := h.subscriber("two@x")
	if _, err := h.db.Exec(add, pq.Array([]int{s2}), pq.Array([]int{h.listA, listB}), ""); err != nil {
		t.Fatalf("add: %v", err)
	}
	want("bulk add single -> confirmed", s2, h.listA, "confirmed", true)
	want("bulk add double -> unconfirmed", s2, listB, "unconfirmed", false)
	h.db.MustExec(`UPDATE subscriber_lists SET status='unconfirmed', confirmed_at=NULL WHERE subscriber_id=$1 AND list_id=$2`, s2, h.listA)
	if _, err := h.db.Exec(add, pq.Array([]int{s2}), pq.Array([]int{h.listA}), ""); err != nil {
		t.Fatal(err)
	}
	want("bulk add does not promote legacy unconfirmed", s2, h.listA, "unconfirmed", false)
	// Unknown list id still fails on the foreign key (API error contract unchanged).
	if _, err := h.db.Exec(add, pq.Array([]int{s2}), pq.Array([]int{999999}), ""); err == nil {
		t.Errorf("bulk add unknown list: expected FK error, got nil")
	}
	h.join(s2, h.listA, "unsubscribed")
	if _, err := h.db.Exec(add, pq.Array([]int{s2}), pq.Array([]int{h.listA}), ""); err != nil {
		t.Fatal(err)
	}
	want("bulk add retains unsubscribed", s2, h.listA, "unsubscribed", false)
	if _, err := h.db.Exec(add, pq.Array([]int{s2}), pq.Array([]int{h.listA}), "confirmed"); err != nil {
		t.Fatal(err)
	}
	want("bulk add explicit confirmed wins", s2, h.listA, "confirmed", true)

	// 3b. backfill (SET LOCAL) on the bulk statement: confirmed on single, but NOT stamped.
	s2b := h.subscriber("twob@x")
	tx := h.db.MustBegin()
	tx.MustExec(`SET LOCAL listmonk.backfill = 'true'`)
	if _, err := tx.Exec(add, pq.Array([]int{s2b}), pq.Array([]int{h.listA, listB}), ""); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	want("backfill bulk add single -> confirmed, unstamped", s2b, h.listA, "confirmed", false)
	want("backfill bulk add double -> unconfirmed", s2b, listB, "unconfirmed", false)

	// 3c. A blocklisted subscriber never gets a confirmed row (edit form, bulk add, by-query).
	sb := h.subscriber("blocked@x")
	h.db.MustExec(`UPDATE subscribers SET status='blocklisted' WHERE id=$1`, sb)
	if _, err := h.db.Exec(upd, sb, "", "", "", "", pq.Array([]int{h.listA}), pq.Array([]string{}), "unconfirmed", true, pq.Array([]int{}), false); err != nil {
		t.Fatal(err)
	}
	want("edit form blocklisted -> unsubscribed", sb, h.listA, "unsubscribed", false)
	if _, err := h.db.Exec(add, pq.Array([]int{sb}), pq.Array([]int{listB}), ""); err != nil {
		t.Fatal(err)
	}
	want("bulk add blocklisted -> unsubscribed", sb, listB, "unsubscribed", false)

	// 3d. Public form re-signup on a DOUBLE list after unsubscribe stays upstream (unconfirmed, unstamped).
	var uuidB string
	h.db.Get(&uuidB, `SELECT uuid FROM lists WHERE id=$1`, listB)
	h.join(s1, listB, "unsubscribed")
	if _, err := h.db.Exec(upd, s1, "", "", "enabled", "", pq.Array([]int{}), pq.Array([]string{uuidB}), "unconfirmed", false, pq.Array([]int{}), true); err != nil {
		t.Fatal(err)
	}
	want("public form resubscribe double -> unconfirmed", s1, listB, "unconfirmed", false)

	// 4. add-subscribers-to-lists-by-query, no status.
	s3 := h.subscriber("three@x")
	if _, err := h.db.Exec(addQ, pq.Array([]int{s3}), "", 0, 0, pq.Array([]int{h.listA, listB}), ""); err != nil {
		t.Fatalf("add by query: %v", err)
	}
	want("by-query single -> confirmed", s3, h.listA, "confirmed", true)
	want("by-query double -> unconfirmed", s3, listB, "unconfirmed", false)
	if _, err := h.db.Exec(addQ, pq.Array([]int{sb}), "", 0, 0, pq.Array([]int{h.listA}), ""); err != nil {
		t.Fatal(err)
	}
	want("by-query blocklisted -> unsubscribed", sb, h.listA, "unsubscribed", false)
}

// Fork (import presets) I6 -- the fill upsert under the backfill flag never stamps
// confirmed_at, so no evergreen welcome can fire for a preset import.
func TestEvergreenPresetFillNeverWelcomes(t *testing.T) {
	h := newPresetHarness(t)
	h.fillOne("preset.new@example.test", "New", `{"lang":"en"}`, h.listA, "confirmed")
	if st, stamped := h.membership("preset.new@example.test", h.listA); st != "confirmed" || stamped {
		t.Errorf("I6 new membership under backfill: %s stamped=%v", st, stamped)
	}
	// An existing unconfirmed row is neither promoted nor stamped.
	old := h.subscriber("preset.old@example.test")
	h.join(old, h.listA, "unconfirmed")
	h.db.MustExec(`UPDATE subscriber_lists SET confirmed_at = NULL WHERE subscriber_id = $1`, old)
	h.fillOne("preset.old@example.test", "Old", `{}`, h.listA, "confirmed")
	if st, stamped := h.membership("preset.old@example.test", h.listA); st != "unconfirmed" || stamped {
		t.Errorf("I6 existing row: %s stamped=%v", st, stamped)
	}
	// The welcome query sees neither.
	welcome := h.campaign("welcome", true, 0, h.listA)
	h.setStatus(welcome, "running")
	eq(t, "preset rows never welcomed", h.batch(welcome, 100), nil)
}
