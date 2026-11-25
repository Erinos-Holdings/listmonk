package migrations

// Fork (automation main-list rule) -- integrations lambda/integrations/komigo/SKU-JOURNEYS-SPEC I1,
// I2, I3: through the REAL goyesql-parsed next-evergreen-subscribers statement (the harness of
// evergreen_db_test.go, LISTMONK_TEST_PG opt-in).
//   I1 -- an automation on a non-exempt brand list does not select a subscriber whose main-list row
//         is not confirmed (unsubscribed, held, final-held, unconfirmed, absent); does select one who is.
//   I2 -- the exemptions (the main list itself, a reengage queue, a transactional list, an untagged
//         list) select as before; a brand with no single public list selects nobody on its private lists.
//   I3 -- the function's own DB test (brand_opted_in returns the rule), the migration is schema.sql's
//         verbatim and idempotent, and next-evergreen-subscribers is the ONLY query that calls it.
// Fixture names are acme.test / example.test only.

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/knadh/goyesql/v2"
	"github.com/lib/pq"
)

func (h *evergreenHarness) mlList(name, typ string, transactional bool, tags ...string) int {
	h.t.Helper()
	var id int
	if tags == nil {
		tags = []string{}
	}
	if err := h.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags, transactional) VALUES (gen_random_uuid(), $1, $2, 'single', $3, $4) RETURNING id`,
		name, typ, pq.StringArray(tags), transactional); err != nil {
		h.t.Fatal(err)
	}
	return id
}

// mlStart starts an automation with its watermark two hours back, so rows confirmed an hour ago are
// past it and past the public-list hold.
func (h *evergreenHarness) mlStart(name string, list int) int {
	h.t.Helper()
	c := h.campaign(name, true, 0, list)
	h.setStatus(c, "running")
	h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - INTERVAL '2 hours' WHERE id = $1`, c)
	return c
}

// mlJoin writes a membership row confirmed (or not) an hour ago, with optional meta.
func (h *evergreenHarness) mlJoin(sub, list int, status, meta string) {
	h.t.Helper()
	if meta == "" {
		meta = "{}"
	}
	h.db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status, meta) VALUES ($1, $2, $3, $4::JSONB)
		ON CONFLICT (subscriber_id, list_id) DO UPDATE SET status = EXCLUDED.status, meta = EXCLUDED.meta`, sub, list, status, meta)
	h.db.MustExec(`UPDATE subscriber_lists SET confirmed_at = NOW() - INTERVAL '1 hour' WHERE subscriber_id = $1 AND list_id = $2 AND confirmed_at IS NOT NULL`, sub, list)
}

func TestEvergreenMainListRule(t *testing.T) {
	h := newEvergreenHarness(t)
	main := h.mlList("Acme", "public", false, "brand:acme")
	step := h.mlList("ACME Replenish", "private", false, "brand:acme")
	queue := h.mlList("ACME Queue 180", "private", false, "brand:acme", "reengage:1:1")
	trans := h.mlList("ACME Receipt", "private", true, "brand:acme")
	untagged := h.mlList("Internal", "private", false)
	// A brand with no public list, and a brand with two -- no single main list: fail closed.
	solo := h.mlList("SOLO Steps", "private", false, "brand:solo")
	twoA := h.mlList("Two A", "public", false, "brand:two")
	h.mlList("Two B", "public", false, "brand:two")
	twoStep := h.mlList("TWO Steps", "private", false, "brand:two")

	onStep := h.mlStart("step automation", step)
	onMain := h.mlStart("main automation", main)
	onQueue := h.mlStart("queue automation", queue)
	onTrans := h.mlStart("transactional automation", trans)
	onUntagged := h.mlStart("untagged automation", untagged)
	onSolo := h.mlStart("solo automation", solo)
	onTwo := h.mlStart("two automation", twoStep)
	onTwoMain := h.mlStart("two public automation", twoA)

	// I1 -- the target list is a private brand list; the main row decides.
	confirmed := h.subscriber("confirmed@acme.test")
	h.mlJoin(confirmed, main, "confirmed", "")
	h.mlJoin(confirmed, step, "confirmed", "")
	unsub := h.subscriber("unsub@acme.test")
	h.mlJoin(unsub, main, "unsubscribed", "")
	h.mlJoin(unsub, step, "confirmed", "")
	held := h.subscriber("held@acme.test")
	h.mlJoin(held, main, "unsubscribed", `{"hold": {"reason": "never-engaged", "at": "2026-05-01T00:00:00Z", "stage": 1}}`)
	h.mlJoin(held, step, "confirmed", "")
	final := h.subscriber("final@acme.test")
	h.mlJoin(final, main, "unsubscribed", `{"hold": {"reason": "never-engaged", "at": "2025-11-01T00:00:00Z", "final": true}}`)
	h.mlJoin(final, step, "confirmed", "")
	unconfirmed := h.subscriber("unconfirmed@acme.test")
	h.mlJoin(unconfirmed, main, "unconfirmed", "")
	h.mlJoin(unconfirmed, step, "confirmed", "")
	absent := h.subscriber("absent@acme.test")
	h.mlJoin(absent, step, "confirmed", "")

	eq(t, "I1 step list", sorted(h.batch(onStep, 100)), []int{confirmed})

	// I2 -- the main list itself.
	eq(t, "I2 main list", sorted(h.batch(onMain, 100)), []int{confirmed})
	// A reengage queue: its members are held on the main list by definition.
	h.mlJoin(held, queue, "confirmed", "")
	h.mlJoin(absent, queue, "confirmed", "")
	eq(t, "I2 reengage queue", sorted(h.batch(onQueue, 100)), sorted([]int{held, absent}))
	// A transactional list mails buyers who never opted in.
	h.mlJoin(absent, trans, "confirmed", "")
	h.mlJoin(unsub, trans, "confirmed", "")
	eq(t, "I2 transactional", sorted(h.batch(onTrans, 100)), sorted([]int{unsub, absent}))
	// An untagged list has no brand and no main list.
	h.mlJoin(absent, untagged, "confirmed", "")
	eq(t, "I2 untagged", sorted(h.batch(onUntagged, 100)), []int{absent})
	// No public list / two public lists: nobody on the private lists -- nor on either public list.
	soloSub := h.subscriber("solo@example.test")
	h.mlJoin(soloSub, solo, "confirmed", "")
	eq(t, "I2 brand with no public list", h.batch(onSolo, 100), nil)
	twoSub := h.subscriber("two@example.test")
	h.mlJoin(twoSub, twoA, "confirmed", "")
	h.mlJoin(twoSub, twoStep, "confirmed", "")
	eq(t, "I2 brand with two public lists", h.batch(onTwo, 100), nil)
	eq(t, "I2 no single main list, its public list", h.batch(onTwoMain, 100), nil)

	// A person whose main row returns to confirmed is eligible again (D1 filters, it claims nothing).
	h.mlJoin(unsub, main, "confirmed", "")
	eq(t, "returned", sorted(h.batch(onStep, 100)), []int{unsub})
}

// I3 -- the function itself, the migration and the one reader.
func TestBrandOptedInFunction_V6_2_45(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(brandOptedInDDL)) {
		t.Fatal("brandOptedInDDL is not in schema.sql verbatim")
	}
	var fresh string
	h.db.Get(&fresh, `SELECT pg_get_functiondef('brand_opted_in(int, int)'::regprocedure)`)
	h.db.MustExec(`DROP FUNCTION brand_opted_in(INT, INT)`)
	for i := 0; i < 2; i++ {
		if err := V6_2_45(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_45 run %d: %v", i+1, err)
		}
	}
	var migrated string
	h.db.Get(&migrated, `SELECT pg_get_functiondef('brand_opted_in(int, int)'::regprocedure)`)
	if migrated != fresh {
		t.Fatalf("migration differs from schema.sql\n migration %s\n schema    %s", migrated, fresh)
	}

	main := h.mlList("Acme", "public", false, "brand:acme")
	step := h.mlList("ACME Step", "private", false, "brand:acme")
	padded := h.mlList("ACME Queue", "private", false, "brand:acme", " reengage:1:4 ")
	s := h.subscriber("fn@acme.test")
	opted := func(sub, list int) bool {
		t.Helper()
		var v bool
		if err := h.db.Get(&v, `SELECT brand_opted_in($1, $2)`, sub, list); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if opted(s, step) {
		t.Fatal("no main row: brand_opted_in must be false")
	}
	if !opted(s, main) || !opted(s, padded) {
		t.Fatal("the main list itself and a (padded) reengage queue are exempt")
	}
	if opted(s, 999999) {
		t.Fatal("an unknown list fails closed")
	}
	h.mlJoin(s, main, "confirmed", "")
	if !opted(s, step) {
		t.Fatal("a confirmed main row: brand_opted_in must be true")
	}
	h.mlJoin(s, main, "unconfirmed", "")
	if opted(s, step) {
		t.Fatal("an unconfirmed main row: brand_opted_in must be false")
	}

	// The ONE reader: next-evergreen-subscribers calls the function; no other shipped query does.
	files, _ := filepath.Glob(filepath.Join("..", "..", "queries", "*.sql"))
	call := regexp.MustCompile(`\bbrand_opted_in\s*\(`)
	readers := []string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		qs, err := goyesql.ParseBytes(b)
		if err != nil {
			t.Fatal(err)
		}
		for name, q := range qs {
			if call.MatchString(q.Query) {
				readers = append(readers, name)
			}
		}
	}
	if len(readers) != 1 || readers[0] != "next-evergreen-subscribers" {
		t.Fatalf("brand_opted_in readers = %v, want exactly [next-evergreen-subscribers]", readers)
	}
}
