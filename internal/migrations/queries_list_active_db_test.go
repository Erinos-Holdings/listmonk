package migrations

// Fork (journey lists) -- integrations listmonk/JOURNEY-LISTS-SPEC D2/D3, I5/I6 at the statement
// level: the shipped update-list-trigger (with its $5 brandChanged), update-list-active and
// get-list-running-evergreen, prepared from queries/*.sql exactly as the app parses them, against
// schema.sql. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"testing"
)

func TestListActiveStatements(t *testing.T) {
	h := newEvergreenHarness(t)
	prep := func(name string) func(args ...any) int64 {
		q, ok := h.qs[name]
		if !ok {
			t.Fatalf("query %s not found", name)
		}
		st, err := h.db.Preparex(q.Query)
		if err != nil {
			t.Fatalf("prepare %s: %v", name, err)
		}
		return func(args ...any) int64 {
			res, err := st.Exec(args...)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			n, _ := res.RowsAffected()
			return n
		}
	}
	trig := prep("update-list-trigger")
	setActive := prep("update-list-active")

	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin, join_trigger) VALUES (gen_random_uuid(), 'Buyers', 'private', 'single', '{"kind":"skus","skus":["a"]}') RETURNING id`)
	type row struct {
		Active bool    `db:"trigger_active"`
		By     *string `db:"trigger_active_by"`
		At     *string `db:"trigger_active_at"`
		SetBy  *string `db:"trigger_set_by"`
	}
	read := func() row {
		var r row
		if err := h.db.Get(&r, `SELECT trigger_active, trigger_active_by, trigger_active_at::TEXT AS trigger_active_at, trigger_set_by FROM lists WHERE id = $1`, list); err != nil {
			t.Fatal(err)
		}
		return r
	}
	on := func() {
		h.db.MustExec(`UPDATE lists SET trigger_active = true, trigger_active_by = 'earlier', trigger_active_at = '2026-01-01T00:00:00Z', trigger_set_by = 'setter' WHERE id = $1`, list)
	}
	same := `{"kind":"skus","skus":["a"]}`

	// Nothing differs, no brand change: no row.
	on()
	if n := trig(list, same, false, "u", false); n != 0 {
		t.Fatalf("no-op wrote %d rows", n)
	}
	if r := read(); !r.Active {
		t.Fatal("a no-op turned Active off")
	}
	// A brand-only change of an active list: off, stamped, the trigger stamps kept.
	if n := trig(list, same, false, "u", true); n != 1 {
		t.Fatalf("brand-only change wrote %d rows", n)
	}
	if r := read(); r.Active || r.By == nil || *r.By != "u" || r.SetBy == nil || *r.SetBy != "setter" {
		t.Fatalf("brand-only change: %+v", r)
	}
	// A brand-only change of an inactive list: a no-op.
	if n := trig(list, same, false, "v", true); n != 0 {
		t.Fatalf("brand-only change of an inactive list wrote %d rows", n)
	}
	// A trigger change of an active list: off, both stamp sets move.
	on()
	if n := trig(list, `{"kind":"skus","skus":["b"]}`, false, "w", false); n != 1 {
		t.Fatal("trigger change wrote nothing")
	}
	if r := read(); r.Active || *r.By != "w" || *r.SetBy != "w" {
		t.Fatalf("trigger change: %+v", r)
	}
	// A transactional change of an active list: off.
	on()
	trig(list, `{"kind":"skus","skus":["b"]}`, true, "x", false)
	if r := read(); r.Active || *r.By != "x" {
		t.Fatalf("transactional change: %+v", r)
	}
	// Cleared: off.
	on()
	trig(list, nil, true, "y", false)
	if r := read(); r.Active {
		t.Fatal("a cleared trigger left the list active")
	}

	// update-list-active: no trigger → 0 rows; same value → 0 rows; a change → 1 with stamps.
	if n := setActive(list, true, "m"); n != 0 {
		t.Fatalf("activating an untriggered list wrote %d rows", n)
	}
	h.db.MustExec(`UPDATE lists SET join_trigger = '{"kind":"first_order"}' WHERE id = $1`, list)
	if n := setActive(list, true, "m"); n != 1 {
		t.Fatal("activation wrote nothing")
	}
	if r := read(); !r.Active || *r.By != "m" || r.At == nil {
		t.Fatalf("activation: %+v", r)
	}
	if n := setActive(list, true, "n"); n != 0 {
		t.Fatalf("same value wrote %d rows", n)
	}

	// get-list-running-evergreen: only a running evergreen on the list counts.
	running := func() bool {
		var ok bool
		if err := h.db.Get(&ok, h.qs["get-list-running-evergreen"].Query, list); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if running() {
		t.Fatal("running with no campaign")
	}
	c := h.campaign("draft auto", true, 0, list)
	if running() {
		t.Fatal("a draft evergreen counts as running")
	}
	h.db.MustExec(`UPDATE campaigns SET status = 'paused' WHERE id = $1`, c)
	if running() {
		t.Fatal("a paused evergreen counts as running")
	}
	b := h.campaign("broadcast", false, 0, list)
	h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE id = $1`, b)
	if running() {
		t.Fatal("a running broadcast counts as an automation")
	}
	h.db.MustExec(`UPDATE campaigns SET status = 'running' WHERE id = $1`, c)
	if !running() {
		t.Fatal("a running evergreen is not counted")
	}
}
