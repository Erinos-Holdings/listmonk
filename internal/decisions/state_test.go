package decisions

// Fork (usage decisions) -- integrations USAGE-DECISIONS-SPEC I7 / D4: an item's current state is
// its newest row only when that row's hash is the current hash; history is every row newest first,
// each marked voided when its hash is not the current one.

import (
	"testing"
	"time"
)

func TestCurrentState(t *testing.T) {
	const cur, old = "c", "o"
	at := time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)
	note := "not yet"
	row := func(id int64, hash, decision string, n *string) Row {
		return Row{ID: id, ItemKey: "np-entry", TextHash: hash, Decision: decision, Note: n, UserID: 4, UserName: "Kaylinn", CreatedAt: at.Add(time.Duration(id) * time.Minute)}
	}

	// No rows: Awaiting, empty (non-nil) history.
	c, h := State(nil, cur)
	if c != nil || h == nil || len(h) != 0 {
		t.Fatalf("no rows: current %v history %v", c, h)
	}

	// One accept on the current text.
	c, h = State([]Row{row(1, cur, Accept, nil)}, cur)
	if c == nil || c.Decision != Accept || c.Voided || len(h) != 1 {
		t.Fatalf("accept: current %+v history %+v", c, h)
	}

	// Reject then accept (rows given out of order): the newest wins, history newest first.
	c, h = State([]Row{row(3, cur, Accept, nil), row(2, cur, Reject, &note)}, cur)
	if c == nil || c.Decision != Accept {
		t.Fatalf("reject then accept: current %+v", c)
	}
	if len(h) != 2 || h[0].Decision != Accept || h[1].Decision != Reject || h[1].Note == nil || *h[1].Note != note {
		t.Fatalf("history order: %+v", h)
	}
	// Accept then reject: the reject is current, with its note.
	c, _ = State([]Row{row(5, cur, Accept, nil), row(6, cur, Reject, &note)}, cur)
	if c == nil || c.Decision != Reject || c.Note == nil || *c.Note != note {
		t.Fatalf("accept then reject: current %+v", c)
	}

	// A stale-hash NEWEST row -> Awaiting, even with an older current-hash row beneath it; voided
	// marks follow each row's own hash.
	c, h = State([]Row{row(7, cur, Accept, nil), row(8, old, Reject, &note)}, cur)
	if c != nil {
		t.Fatalf("stale newest: current %+v, want Awaiting", c)
	}
	if !h[0].Voided || h[1].Voided {
		t.Fatalf("stale newest: voided marks %+v", h)
	}

	// A text edit voids without a write: the same rows read under a new hash are Awaiting...
	rows := []Row{row(9, cur, Accept, nil)}
	if c, h = State(rows, "edited"); c != nil || !h[0].Voided {
		t.Fatalf("edited text: current %+v history %+v", c, h)
	}
	// ...and reverting the text revives that row.
	if c, _ = State(rows, cur); c == nil || c.Decision != Accept {
		t.Fatalf("reverted text: current %+v", c)
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
	for _, it := range v.Items[1:] {
		if it.Current != nil || len(it.History) != 0 {
			t.Fatalf("%s: rows of another key leaked in", it.Key)
		}
	}
}
