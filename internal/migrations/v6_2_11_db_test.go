package migrations

// Fork (list grid) -- integrations LIST-GRID-SPEC I1, the SQL half of I12 (L1) and the
// migration cases of I8, against a real database. Same opt-in harness as evergreen_db_test.go
// (LISTMONK_TEST_PG). The fixture and the expected segment/bucket of each row come from
// internal/testfixtures/listgrid, an independent Go statement of the spec's D1/D2 tables.

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/testfixtures/listgrid"
	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
)

// sendLangExpr is the send queries' language expression (queries/campaigns.sql), verbatim.
const sendLangExpr = `COALESCE(NULLIF(LOWER(LEFT(attribs->>'lang', 2)), ''), 'en')`

// gridCounts is [bucket or send-language key][segment] -> count.
type gridCounts map[string]map[string]int

func (g gridCounts) add(key, segment string, n int) {
	if g[key] == nil {
		g[key] = map[string]int{}
	}
	g[key][segment] += n
}

// runV6_2_11OverOldView is review L7: v6.2.10's tests (and the harness ladder's V6_2_10)
// recreate the OLD-shape view, so this test puts the old shape there on purpose and runs
// V6_2_11 itself, twice, rather than trusting whatever a previous step left behind.
func runV6_2_11OverOldView(t *testing.T, h *evergreenHarness) {
	t.Helper()
	lo := log.New(os.Stderr, "", 0)
	if err := V6_2_10(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_10: %v", err)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM pg_attribute WHERE attrelid = 'mat_list_subscriber_stats'::regclass AND attname = 'segment'`)
	if n != 0 {
		t.Fatal("fixture: V6_2_10 must leave the old-shape view (no segment column)")
	}
	for i := 0; i < 2; i++ {
		if err := V6_2_11(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_11 run %d over the old view: %v", i+1, err)
		}
	}
}

// TestGridPartitions -- I1.
func TestGridPartitions(t *testing.T) {
	h := newEvergreenHarness(t)
	runV6_2_11OverOldView(t, h)

	single, double, err := listgrid.Load(h.db)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	rows, err := listgrid.Rows(h.db)
	if err != nil || len(rows) == 0 {
		t.Fatalf("fixture rows: %d, %v", len(rows), err)
	}
	h.db.MustExec(`REFRESH MATERIALIZED VIEW mat_list_subscriber_stats`)

	// (a) subscriber_lang() per row: absent, JSON-null attribs and "" are none; " ", " fr", a
	// non-string and pt are other; EN/FR/fr-CA fold to their code. And it agrees with the send
	// expression on every row: inside the five codes and none the two are equal after the
	// fold; an other row's send expression is outside the five.
	var langs []struct {
		Email  string `db:"email"`
		Bucket string `db:"bucket"`
		Folded string `db:"folded"`
		Send   string `db:"send"`
	}
	if err := h.db.Select(&langs, `SELECT email, subscriber_lang(attribs) AS bucket,
		send_lang(subscriber_lang(attribs)) AS folded, `+sendLangExpr+` AS send
		FROM subscribers WHERE email LIKE '%@grid.test'`); err != nil {
		t.Fatal(err)
	}
	byEmail := map[string]listgrid.Row{}
	for _, r := range rows {
		byEmail[r.Email] = r
	}
	seenBuckets := map[string]bool{}
	for _, l := range langs {
		want := byEmail[l.Email].Bucket()
		seenBuckets[l.Bucket] = true
		if l.Bucket != want {
			t.Fatalf("subscriber_lang(%s): got %q want %q", l.Email, l.Bucket, want)
		}
		if want == "other" {
			for _, c := range models.CampaignLangs {
				if l.Send == c {
					t.Fatalf("%s is other but the send expression mails it as %q", l.Email, c)
				}
			}
		} else if l.Folded != l.Send {
			t.Fatalf("%s: send_lang(subscriber_lang) = %q but the send expression = %q", l.Email, l.Folded, l.Send)
		}
	}
	for _, b := range listgrid.Buckets {
		if !seenBuckets[b] {
			t.Fatalf("fixture: no row landed in bucket %q", b)
		}
	}

	for _, li := range []struct {
		id     int
		double bool
	}{{single, false}, {double, true}} {
		// Expected, from the e-mails alone.
		want, wantAll := gridCounts{}, 0
		for _, r := range rows {
			want.add(r.Bucket(), r.Segment(li.double), 1)
			wantAll++
		}

		// (b) The view, cell by cell, in storage buckets.
		var cells []struct {
			Segment string `db:"segment"`
			Lang    string `db:"lang"`
			N       int    `db:"n"`
		}
		if err := h.db.Select(&cells, `SELECT segment, lang, SUM(subscriber_count)::INT AS n
			FROM mat_list_subscriber_stats WHERE list_id = $1 GROUP BY 1, 2`, li.id); err != nil {
			t.Fatal(err)
		}
		got, gotAll := gridCounts{}, 0
		for _, c := range cells {
			got.add(c.Lang, c.Segment, c.N)
			gotAll += c.N
		}
		for _, b := range listgrid.Buckets {
			for _, s := range listgrid.Segments {
				if got[b][s] != want[b][s] {
					t.Fatalf("list %d view cell %s/%s: got %d want %d", li.id, b, s, got[b][s], want[b][s])
				}
			}
		}
		// The seven buckets sum to all, which is every subscriber_lists row of the list.
		var slRows int
		h.db.Get(&slRows, `SELECT COUNT(*) FROM subscriber_lists WHERE list_id = $1`, li.id)
		if gotAll != wantAll || gotAll != slRows {
			t.Fatalf("list %d: view total %d, expected %d, subscriber_lists rows %d", li.id, gotAll, wantAll, slRows)
		}
		if !li.double && len(got) > 0 {
			for b := range got {
				if got[b]["pending"] != 0 {
					t.Fatalf("single opt-in list has pending rows in %s", b)
				}
			}
		}
		if li.double && want["en"]["pending"] == 0 {
			t.Fatal("fixture: the double opt-in list must have pending rows")
		}

		// (c) query-lists' subscriber_grid: total = the five segments in every row; all = en +
		// fr + es + de + it + other with en FOLDED; none <= en cell by cell.
		grid := queryListGrid(t, h, li.id)
		sum := map[string]int{}
		for key, row := range grid {
			if row["total"] != row["active"]+row["held"]+row["unsubscribed"]+row["pending"]+row["blocked"] {
				t.Fatalf("list %d grid row %s does not partition: %v", li.id, key, row)
			}
			if key != "all" && key != "none" {
				for k, v := range row {
					sum[k] += v
				}
			}
		}
		for k, v := range grid["all"] {
			if sum[k] != v {
				t.Fatalf("list %d: all.%s = %d but the language rows sum to %d", li.id, k, v, sum[k])
			}
		}
		for _, s := range append([]string{"total"}, listgrid.Segments...) {
			if grid["none"][s] > grid["en"][s] {
				t.Fatalf("list %d: none.%s %d > en.%s %d", li.id, s, grid["none"][s], s, grid["en"][s])
			}
		}
		for _, s := range listgrid.Segments {
			if grid["en"][s] != want["en"][s]+want["none"][s] {
				t.Fatalf("list %d: en.%s = %d, want en %d + none %d", li.id, s, grid["en"][s], want["en"][s], want["none"][s])
			}
			if grid["none"][s] != want["none"][s] || grid["other"][s] != want["other"][s] || grid["fr"][s] != want["fr"][s] {
				t.Fatalf("list %d: none/other/fr .%s = %d/%d/%d", li.id, s, grid["none"][s], grid["other"][s], grid["fr"][s])
			}
		}
		if grid["all"]["total"] != slRows {
			t.Fatalf("list %d: all.total %d != %d rows", li.id, grid["all"]["total"], slRows)
		}
	}

	// (d) An empty list still has an all row of zeros and no language rows; the view stays
	// refreshable CONCURRENTLY over its NULL-keyed rows (the list_id = 0 row, empty lists).
	grid := queryListGrid(t, h, h.listA)
	if len(grid) != 1 || grid["all"]["total"] != 0 {
		t.Fatalf("empty list grid: %v", grid)
	}
	if _, err := h.db.Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY mat_list_subscriber_stats`); err != nil {
		t.Fatalf("concurrent refresh: %v", err)
	}
}

// queryListGrid runs the shipped query-lists for one list and decodes subscriber_grid.
func queryListGrid(t *testing.T, h *evergreenHarness, listID int) map[string]map[string]int {
	t.Helper()
	q := strings.ReplaceAll(h.qs["query-lists"].Query, "%order%", "id ASC")
	var row struct {
		Grid models.ListGrid `db:"subscriber_grid"`
	}
	if err := h.db.Unsafe().Get(&row, q, listID, "", "", "", "", "", pq.StringArray{}, true, pq.Array([]int{}), 0, 1); err != nil {
		t.Fatalf("query-lists %d: %v", listID, err)
	}
	out := map[string]map[string]int{}
	for k, r := range row.Grid {
		out[k] = map[string]int{"active": r.Active, "held": r.Held, "unsubscribed": r.Unsubscribed,
			"pending": r.Pending, "blocked": r.Blocked, "total": r.Total}
	}
	return out
}

// TestSubscriberLangCoversCampaignLangs -- I12 (review L1). The five codes are literals in the
// SQL function; this walks EVERY two-letter code so drift shows in both directions: a language
// added to models.CampaignLangs that the function does not know lands in other, and a code the
// function knows that Go does not comes back as itself.
func TestSubscriberLangCoversCampaignLangs(t *testing.T) {
	h := newEvergreenHarness(t)

	isLang := map[string]bool{}
	for _, l := range models.CampaignLangs {
		isLang[l] = true
	}
	for a := 'a'; a <= 'z'; a++ {
		for b := 'a'; b <= 'z'; b++ {
			code := string(a) + string(b)
			var got, gotUpper string
			if err := h.db.QueryRow(`SELECT subscriber_lang(JSONB_BUILD_OBJECT('lang', $1::TEXT)),
				subscriber_lang(JSONB_BUILD_OBJECT('lang', UPPER($1::TEXT) || '-XX'))`, code).Scan(&got, &gotUpper); err != nil {
				t.Fatal(err)
			}
			want := "other"
			if isLang[code] {
				want = code
			}
			if got != want || gotUpper != want {
				t.Fatalf("subscriber_lang(%q): got %q / %q want %q", code, got, gotUpper, want)
			}
		}
	}
	// send_lang folds none and only none.
	for _, b := range listgrid.Buckets {
		var got string
		h.db.Get(&got, `SELECT send_lang($1)`, b)
		want := b
		if b == "none" {
			want = "en"
		}
		if got != want {
			t.Fatalf("send_lang(%q) = %q want %q", b, got, want)
		}
	}
}

// langlessCampaign inserts a language-less campaign by SQL (past core, which would default it).
func langlessCampaign(t *testing.T, h *evergreenHarness, typ, status string) int {
	t.Helper()
	var id int
	if err := h.db.Get(&id, fmt.Sprintf(`INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, type, status, attribs, template_id)
		VALUES (gen_random_uuid(), '%s-%s', 's', 'f@x', 'b', 'email', '%s', '%s', '{"preheader": "p"}', (SELECT id FROM templates LIMIT 1)) RETURNING id`,
		typ, status, typ, status)); err != nil {
		t.Fatal(err)
	}
	return id
}

func campaignAttribs(t *testing.T, h *evergreenHarness, id int) string {
	t.Helper()
	var s string
	if err := h.db.Get(&s, `SELECT attribs::TEXT FROM campaigns WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestV6_2_11CampaignLang -- I8, migration cases: regular drafts are stamped en (other attribs
// kept); finished, cancelled and every opt-in campaign are untouched; a draft that already has
// a language keeps it; templates.lang arrives as en.
func TestV6_2_11CampaignLang(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	draft := langlessCampaign(t, h, "regular", "draft")
	untouched := []int{
		langlessCampaign(t, h, "regular", "finished"),
		langlessCampaign(t, h, "regular", "cancelled"),
	}
	for _, st := range []string{"draft", "scheduled", "running", "paused", "cancelled", "finished"} {
		untouched = append(untouched, langlessCampaign(t, h, "optin", st))
	}
	frDraft := langlessCampaign(t, h, "regular", "draft")
	h.db.MustExec(`UPDATE campaigns SET attribs = '{"lang": "fr"}' WHERE id = $1`, frDraft)
	// Implementation review L2 -- a draft whose attribs is JSON null (not an object). A bare
	// `attribs || '{"lang": "en"}'` would store the ARRAY [null, {...}], which no longer
	// unmarshals into the campaign's attribs map.
	nullDraft := langlessCampaign(t, h, "regular", "draft")
	h.db.MustExec(`UPDATE campaigns SET attribs = 'null'::JSONB WHERE id = $1`, nullDraft)

	h.db.MustExec(`ALTER TABLE templates DROP COLUMN lang`)
	for i := 0; i < 2; i++ {
		if err := V6_2_11(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_11 run %d: %v", i+1, err)
		}
	}

	if got := campaignAttribs(t, h, draft); got != `{"lang": "en", "preheader": "p"}` {
		t.Fatalf("regular draft: got %s", got)
	}
	for _, id := range untouched {
		if got := campaignAttribs(t, h, id); got != `{"preheader": "p"}` {
			t.Fatalf("campaign %d must be untouched, got %s", id, got)
		}
	}
	if got := campaignAttribs(t, h, nullDraft); got != `{"lang": "en"}` {
		t.Fatalf("JSON-null draft: got %s", got)
	}
	if got := campaignAttribs(t, h, frDraft); got != `{"lang": "fr"}` {
		t.Fatalf("fr draft: got %s", got)
	}
	var tplLang string
	if err := h.db.Get(&tplLang, `SELECT lang FROM templates LIMIT 1`); err != nil || tplLang != "en" {
		t.Fatalf("templates.lang: %q, %v", tplLang, err)
	}
}

// TestV6_2_11AbortsOnLiveLanglessCampaign -- I8 / review M6: a language-less regular campaign
// that is scheduled, running or paused aborts the migration with NOTHING else applied -- no
// draft stamped, no templates.lang, the old view still in place -- and names the campaign.
func TestV6_2_11AbortsOnLiveLanglessCampaign(t *testing.T) {
	for _, status := range []string{"scheduled", "running", "paused"} {
		t.Run(status, func(t *testing.T) {
			h := newEvergreenHarness(t)
			lo := log.New(os.Stderr, "", 0)

			// The pre-v6.2.11 state: old view, no templates.lang, no functions.
			if err := V6_2_10(h.db, nil, nil, lo); err != nil {
				t.Fatal(err)
			}
			h.db.MustExec(`ALTER TABLE templates DROP COLUMN lang`)
			h.db.MustExec(`DROP FUNCTION subscription_segment(subscription_status, JSONB, subscriber_status, list_optin)`)

			draft := langlessCampaign(t, h, "regular", "draft")
			live := langlessCampaign(t, h, "regular", status)

			err := V6_2_11(h.db, nil, nil, lo)
			if err == nil {
				t.Fatalf("a language-less %s regular campaign must abort the migration", status)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("id %d", live)) {
				t.Fatalf("the abort must name campaign %d, got %v", live, err)
			}
			if got := campaignAttribs(t, h, draft); got != `{"preheader": "p"}` {
				t.Fatalf("abort must not stamp drafts, got %s", got)
			}
			var n int
			h.db.Get(&n, `SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'templates' AND column_name = 'lang'`)
			if n != 0 {
				t.Fatal("abort must not add templates.lang")
			}
			h.db.Get(&n, `SELECT COUNT(*) FROM pg_attribute WHERE attrelid = 'mat_list_subscriber_stats'::regclass AND attname = 'segment'`)
			if n != 0 {
				t.Fatal("abort must leave the old view in place")
			}
			h.db.Get(&n, `SELECT COUNT(*) FROM pg_proc WHERE proname = 'subscription_segment'`)
			if n != 0 {
				t.Fatal("abort must not create the functions")
			}

			// Giving it a language clears the way.
			h.db.MustExec(`UPDATE campaigns SET attribs = attribs || '{"lang": "en"}' WHERE id = $1`, live)
			if err := V6_2_11(h.db, nil, nil, lo); err != nil {
				t.Fatalf("V6_2_11 after the repair: %v", err)
			}
		})
	}
}
