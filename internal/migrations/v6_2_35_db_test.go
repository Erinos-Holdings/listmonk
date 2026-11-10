package migrations

// Fork (hold scope) -- integrations listmonk/HOLD-SCOPE-SPEC I7 and I8: brand_eligible_sends counts
// the six broadcasts as sent across the brand, and v6.2.35 brings a v6.2.34 database to exactly
// schema.sql's shape (the two functions and the switch row, inserted false), idempotently, with
// subscription_engagement unchanged. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG).
// The brand-hold readers are cmd/brand_hold_db_test.go.

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
)

func (h *evergreenHarness) holdScopeShape() string {
	var fn []string
	if err := h.db.Select(&fn, `SELECT pg_get_functiondef(p.oid) FROM pg_proc p WHERE p.proname IN ('brand_hold_rows', 'brand_eligible_sends') ORDER BY p.proname`); err != nil {
		h.t.Fatal(err)
	}
	return strings.Join(fn, "; ")
}

func (h *evergreenHarness) engagementDef() string {
	var def string
	if err := h.db.Get(&def, `SELECT pg_get_functiondef(p.oid) FROM pg_proc p WHERE p.proname = 'subscription_engagement'`); err != nil {
		h.t.Fatal(err)
	}
	return def
}

func (h *evergreenHarness) holdScopeSetting() string {
	var v sql.NullString
	h.db.Get(&v, `SELECT value::TEXT FROM settings WHERE key = 'app.brand_hold_scope'`)
	return v.String
}

// I8 -- idempotent, the function texts equal schema.sql's (and each DDL constant is in schema.sql
// verbatim), the row is inserted false and never overwritten, subscription_engagement unchanged.
func TestHoldScopeMigration_V6_2_35(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for name, ddl := range map[string]string{"brandHoldScopeDDL": brandHoldScopeDDL, "brandEligibleSendsDDL": brandEligibleSendsDDL} {
		if !strings.Contains(string(schema), strings.TrimSpace(ddl)) {
			t.Fatalf("%s is not in schema.sql verbatim", name)
		}
	}

	fresh := h.holdScopeShape()
	for _, want := range []string{
		"brand_hold_rows(p_campaign_id integer)", "brand_eligible_sends(p_list_id integer)",
		"app.brand_hold_scope", "campaign_smart_skips", "STABLE",
	} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh schema lacks %q: %s", want, fresh)
		}
	}
	if v := h.holdScopeSetting(); v != "false" {
		t.Fatalf("fresh switch row %q, want false", v)
	}
	engBefore := h.engagementDef()

	// A v6.2.34 database: neither function nor the row exists.
	h.db.MustExec(`DROP FUNCTION brand_hold_rows(INT)`)
	h.db.MustExec(`DROP FUNCTION brand_eligible_sends(INT)`)
	h.db.MustExec(`DELETE FROM settings WHERE key = 'app.brand_hold_scope'`)
	if got := h.holdScopeShape(); got != "" {
		t.Fatalf("functions survived the drop: %s", got)
	}

	for i := 0; i < 2; i++ {
		if err := V6_2_35(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_35 run %d: %v", i+1, err)
		}
	}
	if got := h.holdScopeShape(); got != fresh {
		t.Fatalf("migration shape differs from schema.sql\n migration %s\n schema    %s", got, fresh)
	}
	if v := h.holdScopeSetting(); v != "false" {
		t.Fatalf("migrated switch row %q, want false", v)
	}
	// A flipped switch survives a re-run (ON CONFLICT DO NOTHING).
	h.db.MustExec(`UPDATE settings SET value = 'true' WHERE key = 'app.brand_hold_scope'`)
	if err := V6_2_35(h.db, nil, nil, lo); err != nil {
		t.Fatal(err)
	}
	if v := h.holdScopeSetting(); v != "true" {
		t.Fatalf("a re-run overwrote the switch: %q", v)
	}
	if got := h.engagementDef(); got != engBefore {
		t.Fatalf("subscription_engagement changed\n before %s\n after  %s", engBefore, got)
	}
}

type eligRow struct {
	SubscriberID int          `db:"subscriber_id"`
	Sends        int          `db:"eligible_sends"`
	FirstAt      sql.NullTime `db:"first_eligible_send_at"`
}

func (h *evergreenHarness) eligible(list int) map[int]eligRow {
	var rows []eligRow
	if err := h.db.Select(&rows, `SELECT * FROM brand_eligible_sends($1)`, list); err != nil {
		h.t.Fatalf("brand_eligible_sends: %v", err)
	}
	out := map[int]eligRow{}
	for _, r := range rows {
		out[r.SubscriberID] = r
	}
	return out
}

func (h *evergreenHarness) taggedList(name, optin string, transactional bool, tags ...string) int {
	var id int
	if err := h.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags, transactional) VALUES (gen_random_uuid(), $1, 'private', $2::list_optin, $3, $4) RETURNING id`,
		name, optin, pq.StringArray(tags), transactional); err != nil {
		h.t.Fatal(err)
	}
	return id
}

// rowAt writes a row at status with created_at (and confirmed_at for a confirmed row) set to at.
func (h *evergreenHarness) rowAt(sub, list int, status string, at time.Time) {
	h.join(sub, list, status)
	h.db.MustExec(`UPDATE subscriber_lists SET created_at = $3::TIMESTAMPTZ,
		confirmed_at = CASE WHEN status = 'confirmed' THEN $3::TIMESTAMPTZ ELSE NULL END
		WHERE subscriber_id = $1 AND list_id = $2`, sub, list, at)
}

func (h *evergreenHarness) addList(camp, list int) {
	h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'x')`, camp, list)
}

// I7 -- brand_eligible_sends: a campaign counts once across lists; a send failure, a Smart Sending
// skip, a non-finished, evergreen or opt-in campaign, one started at or before the anchor, another
// language's, a transactional-list broadcast, another brand's, and a sibling list where the member
// has no row / an unsubscribed row / a row created after the send are excluded; a sibling-list
// broadcast with a qualifying earlier row (single and double opt-in) is included; an untagged list
// counts its own only.
func TestBrandEligibleSends(t *testing.T) {
	h := newEvergreenHarness(t)
	now := time.Now().UTC().Truncate(time.Second)
	t0 := now.Add(-100 * 24 * time.Hour)
	day := func(n int) time.Time { return t0.Add(time.Duration(n) * 24 * time.Hour) }
	const max = 1 << 30

	news := h.taggedList("Acme newsletter", "single", false, "brand:acme")
	sib := h.taggedList("Acme segment", "single", false, "brand:acme")
	sibDbl := h.taggedList("Acme buyers", "double", false, "brand:acme")
	noRow := h.taggedList("Acme other segment", "single", false, "brand:acme")
	unsub := h.taggedList("Acme lapsed", "single", false, "brand:acme")
	late := h.taggedList("Acme late", "single", false, "brand:acme")
	txn := h.taggedList("Acme receipts", "single", true, "brand:acme")
	beta := h.taggedList("Beta newsletter", "single", false, "brand:beta")

	m := h.subscriber("m@acme.test")
	h.rowAt(m, news, "confirmed", t0)
	h.rowAt(m, sib, "confirmed", t0)
	h.rowAt(m, sibDbl, "confirmed", t0)
	h.rowAt(m, unsub, "unsubscribed", t0)
	h.rowAt(m, late, "confirmed", day(50))
	h.rowAt(m, txn, "confirmed", t0)
	h.rowAt(m, beta, "confirmed", t0)
	other := h.subscriber("other@acme.test") // on the newsletter only, anchor later
	h.rowAt(other, news, "confirmed", day(20))

	reg := func(list int, start time.Time) int { return h.sent(list, "regular", "finished", false, start, max, "") }
	// Counted for m: four.
	cNews := reg(news, day(10))
	cBoth := reg(news, day(11))
	h.addList(cBoth, sib) // on two lists of the brand: counted once
	cSib := reg(sib, day(5))
	cDbl := reg(sibDbl, day(12))
	// Excluded.
	cFail := reg(news, day(13))
	h.db.MustExec(`INSERT INTO campaign_send_failures (campaign_id, subscriber_id, email, stage, error) VALUES ($1, $2, 'm@acme.test', 'send', 'x')`, cFail, m)
	cSkip := reg(news, day(14))
	h.db.MustExec(`INSERT INTO campaign_smart_skips (campaign_id, subscriber_id) VALUES ($1, $2)`, cSkip, m)
	h.sent(news, "regular", "cancelled", false, day(15), max, "")
	h.sent(news, "regular", "finished", true, day(16), max, "")
	h.sent(news, "optin", "finished", false, day(17), max, "")
	reg(news, t0)                                                  // at the anchor
	h.sent(news, "regular", "finished", false, day(18), max, "fr") // another language
	reg(txn, day(19))
	reg(beta, day(21))
	reg(noRow, day(22))
	reg(unsub, day(23))
	reg(late, day(24)) // m's row on it was created at day 50, after the send

	got := h.eligible(news)
	if r := got[m]; r.Sends != 4 || !r.FirstAt.Valid || !r.FirstAt.Time.Equal(day(5)) {
		t.Fatalf("m: %+v, want 4 sends, first %v (cNews %d, cBoth %d, cSib %d, cDbl %d)", r, day(5), cNews, cBoth, cSib, cDbl)
	}
	// other joined at day 20, after every newsletter broadcast, and holds no sibling rows.
	if r := got[other]; r.Sends != 0 || r.FirstAt.Valid {
		t.Fatalf("other: %+v, want 0 sends", r)
	}
	if len(got) != 2 {
		t.Fatalf("members %v, want exactly m and other", got)
	}

	// An untagged list counts its own only: a broadcast on another untagged list the member holds a
	// row on does not count.
	plain2 := h.taggedList("Scratch 2", "single", false)
	u := h.subscriber("u@x")
	h.rowAt(u, h.listA, "confirmed", t0)
	h.rowAt(u, plain2, "confirmed", t0)
	reg(h.listA, day(30))
	reg(plain2, day(31))
	if r := h.eligible(h.listA)[u]; r.Sends != 1 || !r.FirstAt.Time.Equal(day(30)) {
		t.Fatalf("untagged: %+v, want 1 send at %v", r, day(30))
	}
}

// I7 -- with one list and no Smart Sending skips, brand_eligible_sends equals
// subscription_engagement's eligible_sends and first_eligible_send_at row for row.
func TestBrandEligibleSends_EqualsEngagementOnOneList(t *testing.T) {
	h := newEvergreenHarness(t)
	now := time.Now().UTC().Truncate(time.Second)
	t0 := now.Add(-100 * 24 * time.Hour)
	day := func(n int) time.Time { return t0.Add(time.Duration(n) * 24 * time.Hour) }

	l := h.taggedList("Gamma newsletter", "single", false, "brand:gamma")
	a, b, fr, low := h.subscriber("a@x"), h.subscriber("b@x"), h.subscriber("fr@x"), h.subscriber("low@x")
	h.rowAt(a, l, "confirmed", t0)
	h.rowAt(b, l, "confirmed", day(15))
	h.rowAt(fr, l, "confirmed", t0)
	h.db.MustExec(`UPDATE subscribers SET attribs = '{"lang":"FR-ca"}' WHERE id = $1`, fr)
	h.rowAt(low, l, "confirmed", t0)
	blocked := h.subscriber("blocked@x")
	h.rowAt(blocked, l, "confirmed", t0)
	h.db.MustExec(`UPDATE subscribers SET status = 'blocklisted' WHERE id = $1`, blocked)
	pending := h.subscriber("pending@x")
	h.rowAt(pending, l, "unconfirmed", t0)
	// low sits above every campaign's id cap but the first one's.
	h.db.MustExec(`UPDATE subscribers SET id = 900000 WHERE id = $1`, low)
	low = 900000

	c1 := h.sent(l, "regular", "finished", false, day(10), 1<<30, "")
	h.sent(l, "regular", "finished", false, day(20), 1000, "")
	h.sent(l, "regular", "finished", false, day(30), 1000, "fr")
	failed := h.sent(l, "regular", "finished", false, day(40), 1000, "")
	h.db.MustExec(`INSERT INTO campaign_send_failures (campaign_id, subscriber_id, email, stage, error) VALUES ($1, $2, 'a@x', 'send', 'x')`, failed, a)
	h.sent(l, "regular", "running", false, day(50), 1000, "")
	h.sent(l, "regular", "finished", true, day(60), 1000, "")
	_ = c1

	eng := h.engagement(l)
	el := h.eligible(l)
	if len(eng) != len(el) || len(el) != 4 {
		t.Fatalf("members: engagement %d, eligible %d, want 4", len(eng), len(el))
	}
	for id, e := range eng {
		r, ok := el[id]
		if !ok || r.Sends != e.EligibleSends || r.FirstAt.Valid != e.FirstSendAt.Valid || (r.FirstAt.Valid && !r.FirstAt.Time.Equal(e.FirstSendAt.Time)) {
			t.Fatalf("subscriber %d: eligible %+v, engagement sends %d first %v", id, r, e.EligibleSends, e.FirstSendAt)
		}
	}
	// The fixture is not trivial: the four members' counts differ.
	if eng[a].EligibleSends == eng[b].EligibleSends && eng[b].EligibleSends == eng[fr].EligibleSends {
		t.Fatalf("degenerate fixture: %+v", eng)
	}
}
