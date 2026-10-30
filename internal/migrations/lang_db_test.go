package migrations

// Fork (erinos multi-language campaigns) -- DB harness for the language predicates in
// next-campaigns / next-campaign-subscribers / get-campaign-lang-audience, the evergreen
// language rule (exact for ES/FR/DE/IT, EN = fallback for unknown and unclaimed langs,
// paused siblings still claim), the widened exclusion set (one welcome per joiner per
// list across languages, cancelled siblings keep excluding) and the lang-aware collision
// check. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func (h *evergreenHarness) prep(name string) *sqlx.Stmt {
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

func (h *evergreenHarness) setSubLang(sub int, lang string) {
	if lang == "" {
		h.db.MustExec(`UPDATE subscribers SET attribs = attribs - 'lang' WHERE id = $1`, sub)
		return
	}
	h.db.MustExec(`UPDATE subscribers SET attribs = attribs || jsonb_build_object('lang', $2::TEXT) WHERE id = $1`, sub, lang)
}

func (h *evergreenHarness) setCampLang(camp int, lang string) {
	if lang == "" {
		h.db.MustExec(`UPDATE campaigns SET attribs = attribs - 'lang' WHERE id = $1`, camp)
		return
	}
	h.db.MustExec(`UPDATE campaigns SET attribs = attribs || jsonb_build_object('lang', $2::TEXT) WHERE id = $1`, camp, lang)
}

func contains(ids []int, id int) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func sorted(ids []int) []int {
	out := append([]int(nil), ids...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// TestLangBroadcastPredicate -- a broadcast carrying attribs.lang counts and fetches only
// that language; EN includes subscribers with no lang; absent lang = everyone. Ties the
// to_send count (next-campaigns), the batch (next-campaign-subscribers, read from the row
// per batch) and the warning count (get-campaign-lang-audience) to one rule.
func TestLangBroadcastPredicate(t *testing.T) {
	h := newEvergreenHarness(t)
	L := h.listA
	nextCamps := h.prep("next-campaigns")
	nextSubs := h.prep("next-campaign-subscribers")
	audience := h.prep("get-campaign-lang-audience")
	running := h.prep("get-running-campaign")

	fr := h.subscriber("fr@x")
	h.setSubLang(fr, "fr-CA") // normalised to fr (finding 4)
	en := h.subscriber("en@x")
	h.setSubLang(en, "EN")
	none := h.subscriber("none@x")
	de := h.subscriber("de@x")
	h.setSubLang(de, "de")
	for _, s := range []int{fr, en, none, de} {
		h.join(s, L, "confirmed")
	}

	type want struct {
		lang string
		ids  []int
	}
	cases := []want{
		{"fr", []int{fr}},
		{"en", []int{en, none}},
		{"de", []int{de}},
		{"it", nil},
		{"", []int{fr, en, none, de}},
	}
	for _, c := range cases {
		camp := h.campaign("bc-"+c.lang, false, 0, L)
		h.setCampLang(camp, c.lang)

		var n int
		if err := audience.Get(&n, camp); err != nil {
			t.Fatalf("audience %q: %v", c.lang, err)
		}
		if n != len(c.ids) {
			t.Fatalf("audience %q: got %d want %d", c.lang, n, len(c.ids))
		}

		h.setStatus(camp, "running")
		// The claim: to_send and max_subscriber_id under the language predicate.
		if _, err := nextCamps.Exec(pq.Int64Array{}, pq.Int64Array{}); err != nil {
			t.Fatalf("next-campaigns: %v", err)
		}
		var toSend, maxID int
		if err := h.db.QueryRow(`SELECT to_send, max_subscriber_id FROM campaigns WHERE id = $1`, camp).Scan(&toSend, &maxID); err != nil {
			t.Fatal(err)
		}
		if toSend != len(c.ids) {
			t.Fatalf("to_send %q: got %d want %d", c.lang, toSend, len(c.ids))
		}

		// The batch, with lang read from the running-campaign row like the manager does.
		var rc []struct {
			CampaignID   int            `db:"campaign_id"`
			CampaignType string         `db:"campaign_type"`
			Last         int            `db:"last_subscriber_id"`
			Max          int            `db:"max_subscriber_id"`
			ListID       int            `db:"list_id"`
			Lang         sql.NullString `db:"lang"`
		}
		if err := running.Select(&rc, camp); err != nil || len(rc) == 0 {
			t.Fatalf("get-running-campaign %q: %v (%d rows)", c.lang, err, len(rc))
		}
		if rc[0].Lang.Valid != (c.lang != "") || rc[0].Lang.String != c.lang {
			t.Fatalf("running lang %q: got %+v", c.lang, rc[0].Lang)
		}
		rows, err := nextSubs.Queryx(camp, rc[0].CampaignType, rc[0].Last, rc[0].Max, pq.Array([]int{L}), 100, rc[0].Lang)
		if err != nil {
			t.Fatalf("next-campaign-subscribers %q: %v", c.lang, err)
		}
		var got []int
		for rows.Next() {
			m := map[string]interface{}{}
			if err := rows.MapScan(m); err != nil {
				t.Fatal(err)
			}
			got = append(got, int(m["id"].(int64)))
		}
		rows.Close()
		eq(t, "batch "+c.lang, sorted(got), sorted(c.ids))
		h.setStatus(camp, "cancelled")
	}
}

// TestLangEvergreen -- the evergreen language rule, the widened exclusion set, and the
// lang-aware collision check. Since integrations LIFECYCLE-SERIES-SPEC D4 (I11) the rule is the
// broadcast predicate: EN reaches en + no-language subscribers ONLY (no English fallback for a
// language with no automation of its own), ES/FR/DE/IT exact; one automation per joiner per list
// across languages; collisions unchanged.
func TestLangEvergreen(t *testing.T) {
	h := newEvergreenHarness(t)
	L := h.listA

	enW := h.campaign("welcome-en", true, 0, L)
	h.setCampLang(enW, "en")
	frW := h.campaign("welcome-fr", true, 0, L)
	h.setCampLang(frW, "fr")

	// Collision: EN and FR on the same list + delay do not collide; an everyone
	// evergreen collides with either; FR vs FR collides.
	h.setStatus(enW, "running")
	if h.collidesLang(frW, 0, []int{L}, nil, "fr") {
		t.Fatal("EN and FR evergreens must not collide")
	}
	if !h.collidesLang(frW, 0, []int{L}, nil, "") {
		t.Fatal("an everyone evergreen must collide with a running EN one")
	}
	if !h.collidesLang(frW, 0, []int{L}, nil, "en") {
		t.Fatal("a second EN evergreen must collide")
	}
	h.setStatus(frW, "running")

	// Joiners after both started.
	fr := h.subscriber("fr@x")
	h.setSubLang(fr, "fr")
	h.join(fr, L, "confirmed")
	en := h.subscriber("en@x")
	h.setSubLang(en, "en")
	h.join(en, L, "confirmed")
	none := h.subscriber("none@x")
	h.join(none, L, "confirmed")
	de := h.subscriber("de@x")
	h.setSubLang(de, "de")
	h.join(de, L, "confirmed")
	// A stored language outside en/es/fr/de/it (import or hand edit only) reaches no automation,
	// EN included (LIFECYCLE-SERIES-SPEC D4, Stage 2 F12).
	pt := h.subscriber("pt@x")
	h.setSubLang(pt, "pt")
	h.join(pt, L, "confirmed")

	eq(t, "FR exact", h.batch(frW, 10), []int{fr})
	// EN = en + no language ONLY. de has no DE automation and gets NOTHING (the old English
	// fallback is gone); pt gets nothing.
	eq(t, "EN is en + no language only (de and pt get nothing)", h.batch(enW, 10), []int{en, none})
	eq(t, "EN never reaches de or pt on a later batch", h.batch(enW, 10), nil)

	// One welcome per joiner per list -- none was welcomed in English; their order then
	// tags them fr. The FR evergreen must NOT welcome them again.
	h.setSubLang(none, "fr")
	eq(t, "no second welcome", h.batch(frW, 10), nil)

	// A DE automation takes de joiners after its watermark; EN never reaches de either way.
	deW := h.campaign("welcome-de", true, 0, L)
	h.setCampLang(deW, "de")
	h.setStatus(deW, "running")
	de2 := h.subscriber("de2@x")
	h.setSubLang(de2, "de")
	h.join(de2, L, "confirmed")
	eq(t, "EN does not reach de2", h.batch(enW, 10), nil)
	eq(t, "DE takes de2 (not de, who joined before its watermark)", h.batch(deW, 10), []int{de2})

	// Paused DE: EN still never reaches de; DE resumes and takes the joiner.
	h.setStatus(deW, "paused")
	de3 := h.subscriber("de3@x")
	h.setSubLang(de3, "de")
	h.join(de3, L, "confirmed")
	eq(t, "EN does not reach de while DE paused", h.batch(enW, 10), nil)
	h.setStatus(deW, "running")
	eq(t, "DE resumes and takes de3", h.batch(deW, 10), []int{de3})

	// Cancelling DE does NOT hand de to EN any more: a de joiner after the cancel gets nothing.
	h.setStatus(deW, "cancelled")
	de4 := h.subscriber("de4@x")
	h.setSubLang(de4, "de")
	h.join(de4, L, "confirmed")
	eq(t, "EN does not reach de after DE cancelled", h.batch(enW, 10), nil)

	// A cancelled EN sibling's sends keep excluding a joiner who later becomes fr.
	late := h.subscriber("late@x")
	h.join(late, L, "confirmed")
	eq(t, "EN welcomes late", h.batch(enW, 10), []int{late})
	h.setStatus(enW, "cancelled")
	h.setSubLang(late, "fr")
	eq(t, "cancelled EN still excludes late from FR", h.batch(frW, 10), nil)

	// Delay > 0 and an FR automation started AFTER a fr joiner's join: FR cannot welcome a
	// joiner who predates its watermark, and EN does not cover them either (no fallback, D4).
	// Exclusion set is any-delay (finding 5a): the delay-0 EN evergreen still excludes what the
	// delay-60 ones sent.
	var L3 int
	h.db.Get(&L3, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'C', 'private', 'single') RETURNING id`)
	enD := h.campaign("welcome-en-delay", true, 60, L3)
	h.setCampLang(enD, "en")
	h.setStatus(enD, "running")
	h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - INTERVAL '1 day' WHERE id = $1`, enD)
	early := h.subscriber("early-fr@x")
	h.setSubLang(early, "fr")
	h.join(early, L3, "confirmed")
	h.db.MustExec(`UPDATE subscriber_lists SET confirmed_at = NOW() - INTERVAL '120 seconds' WHERE subscriber_id = $1`, early)
	e3 := h.subscriber("e3@x") // no language -- EN's
	h.join(e3, L3, "confirmed")
	h.db.MustExec(`UPDATE subscriber_lists SET confirmed_at = NOW() - INTERVAL '120 seconds' WHERE subscriber_id = $1`, e3)
	frD := h.campaign("welcome-fr-late", true, 60, L3)
	h.setCampLang(frD, "fr")
	h.setStatus(frD, "running")
	h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - INTERVAL '60 seconds' WHERE id = $1`, frD)
	eq(t, "FR started after the join cannot welcome", h.batch(frD, 10), nil)
	eq(t, "EN takes only the no-language joiner, never the fr one FR cannot", h.batch(enD, 10), []int{e3})
	// ...and a fr joiner after FR's watermark is FR's once the delay elapses; EN never.
	later := h.subscriber("later-fr@x")
	h.setSubLang(later, "FR") // imported uppercase still counts as fr (finding 4)
	h.join(later, L3, "confirmed")
	h.db.MustExec(`UPDATE subscriber_lists SET confirmed_at = NOW() - INTERVAL '61 seconds' WHERE subscriber_id = $1`, later)
	h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - INTERVAL '90 seconds' WHERE id = $1`, frD)
	eq(t, "EN leaves a fr joiner alone", h.batch(enD, 10), nil)
	eq(t, "FR takes the joiner after its watermark", h.batch(frD, 10), []int{later})
	// Any-delay exclusion: a delay-0 EN evergreen on L3 must not re-welcome e3 (sent by the
	// delay-60 EN one), and never reaches early/later (fr).
	en0 := h.campaign("welcome-en-0", true, 0, L3)
	h.setCampLang(en0, "en")
	h.setStatus(en0, "running")
	h.db.MustExec(`UPDATE campaigns SET started_at = NOW() - INTERVAL '1 day' WHERE id = $1`, en0)
	h.setStatus(frD, "cancelled")
	eq(t, "delay-0 EN excludes what the delay-60 ones sent", h.batch(en0, 10), nil)

	// Two EN evergreens in one variant group must not starve each other (finding 2).
	var L4 int
	h.db.Get(&L4, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'D', 'private', 'single') RETURNING id`)
	vgA := h.campaign("en-A", true, 0, L4)
	vgB := h.campaign("en-B", true, 0, L4)
	h.db.MustExec(`UPDATE campaigns SET variant_group_id = 'a0000000-0000-0000-0000-000000000001' WHERE id IN ($1, $2)`, vgA, vgB)
	h.setCampLang(vgA, "en")
	h.setCampLang(vgB, "en")
	h.setStatus(vgA, "running")
	h.setStatus(vgB, "running")
	v := h.subscriber("v@x")
	h.join(v, L4, "confirmed")
	eq(t, "variant sibling does not claim", h.batch(vgA, 10), []int{v})
	eq(t, "variant group shares the send", h.batch(vgB, 10), nil)

	// Everyone evergreen (no lang) on a fresh list still takes every joiner.
	var L2 int
	h.db.Get(&L2, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'B', 'private', 'single') RETURNING id`)
	all := h.campaign("welcome-all", true, 0, L2)
	h.setStatus(all, "running")
	x := h.subscriber("x@x")
	h.setSubLang(x, "it")
	h.join(x, L2, "confirmed")
	y := h.subscriber("y@x")
	h.join(y, L2, "confirmed")
	eq(t, "everyone evergreen", h.batch(all, 10), []int{x, y})
	if contains(h.batch(all, 10), x) {
		t.Fatal("claimed rows must not repeat")
	}
}
