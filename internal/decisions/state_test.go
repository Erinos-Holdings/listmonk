package decisions

// Fork (usage decisions) -- integrations USAGE-DECISIONS-SPEC I7 / D4, amended by LIFECYCLE-DOCS-SPEC
// I4/I5: an item's current state is its newest row when that row's hash is one of the item's
// accepted hashes (the concept entry's or a later edit's); history is every row newest first, each
// marked voided when its hash is not accepted; a current approval carries D5's "modified" marker.

import (
	"testing"
	"time"
)

func TestCurrentState(t *testing.T) {
	// The concept hash c0, a later ordinary edit c1; "o" a hash from before a re-approval.
	accepted := []string{"c0", "c1"}
	at := time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)
	note := "not yet"
	row := func(id int64, hash, decision string, n *string) Row {
		return Row{ID: id, ItemKey: "np-entry", TextHash: hash, Decision: decision, Note: n, UserID: 4, UserName: "Kaylinn", CreatedAt: at.Add(time.Duration(id) * time.Minute)}
	}

	// No rows: Awaiting, empty (non-nil) history.
	c, h := State(nil, accepted)
	if c != nil || h == nil || len(h) != 0 {
		t.Fatalf("no rows: current %v history %v", c, h)
	}

	// One accept on the concept text, and one on the later edit's text: both current.
	for _, hash := range accepted {
		c, h = State([]Row{row(1, hash, Accept, nil)}, accepted)
		if c == nil || c.Decision != Accept || c.Voided || len(h) != 1 {
			t.Fatalf("accept on %s: current %+v history %+v", hash, c, h)
		}
	}

	// Reject then accept (rows given out of order): the newest wins, history newest first.
	c, h = State([]Row{row(3, "c1", Accept, nil), row(2, "c0", Reject, &note)}, accepted)
	if c == nil || c.Decision != Accept {
		t.Fatalf("reject then accept: current %+v", c)
	}
	if len(h) != 2 || h[0].Decision != Accept || h[1].Decision != Reject || h[1].Note == nil || *h[1].Note != note || h[1].Voided {
		t.Fatalf("history order: %+v", h)
	}
	// Accept then reject: the reject is current, with its note.
	c, _ = State([]Row{row(5, "c0", Accept, nil), row(6, "c0", Reject, &note)}, accepted)
	if c == nil || c.Decision != Reject || c.Note == nil || *c.Note != note {
		t.Fatalf("accept then reject: current %+v", c)
	}

	// An older-concept NEWEST row -> Awaiting, even with an accepted row beneath it; voided marks
	// follow each row's own hash.
	c, h = State([]Row{row(7, "c0", Accept, nil), row(8, "o", Reject, &note)}, accepted)
	if c != nil {
		t.Fatalf("older-concept newest: current %+v, want Awaiting", c)
	}
	if !h[0].Voided || h[1].Voided {
		t.Fatalf("older-concept newest: voided marks %+v", h)
	}

	// An ordinary text edit (its entry adds c2 to the accepted set) keeps the decision...
	rows := []Row{row(9, "c0", Accept, nil)}
	if c, h = State(rows, []string{"c0", "c1", "c2"}); c == nil || h[0].Voided {
		t.Fatalf("ordinary edit: current %+v history %+v", c, h)
	}
	// ...and a reapprove entry (the accepted set restarts at the new concept r0) voids every
	// earlier row.
	if c, h = State([]Row{row(10, "c0", Accept, nil), row(11, "c2", Accept, nil)}, []string{"r0"}); c != nil || !h[0].Voided || !h[1].Voided {
		t.Fatalf("re-approval: current %+v history %+v", c, h)
	}
	// No accepted hashes at all: Awaiting.
	if c, _ = State(rows, nil); c != nil {
		t.Fatalf("nothing accepted: current %+v", c)
	}

	// D4: a decision on the current text of every shipped item is never Awaiting.
	d, err := Lifecycle()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range d.Items {
		if c, _ := State([]Row{{ID: 1, ItemKey: it.Key, TextHash: Hash(it.Text), Decision: Accept}}, Accepted(it)); c == nil {
			t.Errorf("%s: a decision on the current text is Awaiting", it.Key)
		}
		// Kaylinn's 2026-11-05 rows carry each item's concept hash: every one still counts.
		if c, _ := State([]Row{{ID: 1, ItemKey: it.Key, TextHash: Concept(it).Hash, Decision: Accept}}, Accepted(it)); c == nil {
			t.Errorf("%s: a decision on the concept text is Awaiting", it.Key)
		}
	}
}

// LIFECYCLE-DOCS-SPEC I5 / D5: Modified is the newest Who entry after the row hash's position in
// the lineage, else nil.
func TestModified(t *testing.T) {
	ch := func(date, who, hash string, re bool) Change {
		return Change{Date: date, Source: "page", Who: who, Line: "l", Hash: hash, Reapprove: re}
	}
	// Item 9's shape: concept h0, Kaylinn's Slack entry, the page edit h1.
	nine := Item{Key: "k", Changelog: []Change{ch("2026-10-31", "", "h0", true), ch("2026-11-06", "Kaylinn Brophy", "", false), ch("2026-11-07", "", "h1", false)}}
	if m := Modified(nine, "h0"); m == nil || m.Who != "Kaylinn Brophy" || m.Date != "2026-11-06" {
		t.Fatalf("item 9 shape: %+v", m)
	}
	// A Who entry at or before the row's hash POSITION in the lineage does not set it (position,
	// not time -- LIFECYCLE-DOCS-SPEC I5 / Stage 4 R1).
	if m := Modified(nine, "h1"); m != nil {
		t.Fatalf("a who entry before the row's hash position: %+v", m)
	}
	// Item 3's shape: no Who entry at all.
	three := Item{Key: "k", Changelog: []Change{ch("2026-10-30", "", "h0", true), ch("2026-11-07", "", "h1", false)}}
	if m := Modified(three, "h0"); m != nil {
		t.Fatalf("item 3 shape: %+v", m)
	}
	// Hash-less entries count by file position; the newest Who entry wins.
	mixed := Item{Key: "k", Changelog: []Change{
		ch("2026-10-30", "", "h0", true), ch("2026-11-01", "Ana", "", false), ch("2026-11-02", "", "h1", false),
		ch("2026-11-03", "Kaylinn Brophy", "", false), ch("2026-11-04", "", "h2", false),
	}}
	if m := Modified(mixed, "h0"); m == nil || m.Who != "Kaylinn Brophy" || m.Date != "2026-11-03" {
		t.Fatalf("newest who after h0: %+v", m)
	}
	if m := Modified(mixed, "h1"); m == nil || m.Who != "Kaylinn Brophy" {
		t.Fatalf("who after h1: %+v", m)
	}
	if m := Modified(mixed, "h2"); m != nil {
		t.Fatalf("nothing after h2: %+v", m)
	}
	// A hash outside the lineage (or from before a re-approval) has no position.
	if m := Modified(mixed, "zz"); m != nil {
		t.Fatalf("unknown hash: %+v", m)
	}
	re := Item{Key: "k", Changelog: []Change{ch("2026-10-30", "", "h0", true), ch("2026-11-01", "Ana", "", false), ch("2026-11-02", "", "r0", true)}}
	if m := Modified(re, "h0"); m != nil {
		t.Fatalf("a pre-re-approval hash: %+v", m)
	}

	// Through ItemOf: an approval carries it, a reject never does.
	text := "t1"
	it := Item{Key: "k", Section: SectionBoth, Text: text, Changelog: []Change{ch("2026-10-31", "", Hash("t0"), true), ch("2026-11-06", "Kaylinn Brophy", "", false), ch("2026-11-07", "", Hash(text), false)}}
	note := "no"
	for _, tc := range []struct {
		decision string
		note     *string
		want     bool
	}{{Accept, nil, true}, {Current, nil, true}, {Recommended, nil, true}, {Reject, &note, true}} {
		v := ItemOf(it, []Row{{ID: 1, ItemKey: "k", TextHash: Hash("t0"), Decision: tc.decision, Note: tc.note}})
		if v.Current == nil || (v.Current.Modified != nil) != tc.want {
			t.Errorf("%s: current %+v", tc.decision, v.Current)
		}
		// D5 amendment 2026-11-08: a reject with a `who` entry after its hash is an ADOPTED reject
		// (its note became the text); an approval is never marked adopted.
		if v.Current.Adopted != (tc.decision == Reject) {
			t.Errorf("%s: adopted %v", tc.decision, v.Current.Adopted)
		}
		if len(v.History) != 1 || v.History[0].Modified != nil || v.History[0].Adopted {
			t.Errorf("%s: a history row carries modified", tc.decision)
		}
	}
	// A reject with no `who` entry after its hash is a plain reject: no modified, not adopted.
	plain := Item{Key: "k", Section: SectionBoth, Text: text, Changelog: []Change{ch("2026-10-31", "", Hash("t0"), true), ch("2026-11-07", "", Hash(text), false)}}
	if v := ItemOf(plain, []Row{{ID: 1, ItemKey: "k", TextHash: Hash("t0"), Decision: Reject, Note: &note}}); v.Current == nil || v.Current.Modified != nil || v.Current.Adopted {
		t.Errorf("plain reject: current %+v", v.Current)
	}

	// The shipped file: items 1 and 9 read "approved as modified" by Kaylinn on 2026-11-06 for a
	// decision on their concept text, item 13 on 2026-11-07 (her relayed Smart Sending request,
	// integrations SMART-SENDING-SPEC D9), item 22 on 2026-11-05 (her reject note asking for
	// "inactivity suppression", adopted 2026-11-08 -- her stored reject reads as approved as
	// modified); items 3, 5, 11 and 19 do not (19's 2026-11-08 edit carries no `who`).
	d, err := Lifecycle()
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"np-entry": "2026-11-06", "pp-delivery-clock": "2026-11-06", "both-per-brand": "2026-11-07", "sp-suppressed": "2026-11-05", "np-same-day-double": "", "np-no-restarts": "", "pp-returning-thanks": "", "sp-asks": ""} {
		it, _ := d.Find(key)
		m := Modified(it, Concept(it).Hash)
		if (m != nil) != (want != "") || (m != nil && (m.Who != "Kaylinn Brophy" || m.Date != want)) {
			t.Errorf("%s: modified %+v", key, m)
		}
	}
}

func TestItemOfAndBuild(t *testing.T) {
	d, err := Lifecycle()
	if err != nil {
		t.Fatal(err)
	}
	first := d.Items[0]
	rows := []Row{
		{ID: 1, ItemKey: first.Key, TextHash: Hash(first.Text), Decision: Accept, UserName: "K"},
		{ID: 2, ItemKey: "other-key", TextHash: "x", Decision: Reject, UserName: "K"},
	}
	v := Build(d, rows, true)
	if !v.CanDecide || v.Approver != d.Approver || v.Published != d.Published || len(v.Items) != len(d.Items) {
		t.Fatalf("build header: %+v", v)
	}
	if v.Items[0].Hash != Hash(first.Text) || v.Items[0].Current == nil || len(v.Items[0].History) != 1 {
		t.Fatalf("item 0: %+v", v.Items[0])
	}
	// D6: the view carries the changelog in file order, and Since is the concept entry's date.
	if len(v.Items[0].Changelog) != len(first.Changelog) || v.Items[0].Changelog[0].Line != first.Changelog[0].Line || !v.Items[0].Changelog[0].Reapprove || v.Items[0].Since != Since(first) {
		t.Fatalf("item 0 changelog: %+v", v.Items[0].Changelog)
	}
	for _, it := range v.Items[1:] {
		if it.Current != nil || len(it.History) != 0 {
			t.Fatalf("%s: rows of another key leaked in", it.Key)
		}
	}
}
