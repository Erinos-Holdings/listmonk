package decisions

// Fork (usage decisions) -- integrations USAGE-DECISIONS-SPEC I1/I2, amended by
// LIFECYCLE-DOCS-SPEC I1-I3/I7/I8. lifecycle.json carries exactly §0's 23 keys in order (14 + the 9
// Curated Sunset Policy items, USAGE-REENGAGEMENT-SPEC D7), with §0's sections, numbers, derived
// since dates and header, the sunset-policy table, and each item's changelog. The acknowledgement
// is structural (D2): parse refuses a text whose hash is not the newest hash-bearing changelog
// entry's, and the literal pin here is each item's CONCEPT hash (its newest `reapprove` entry's) --
// it changes only when a re-approval is requested. Word-for-word agreement with the spec's §0 is
// untestable across repos: edit §0 and lifecycle.json in one change.

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func intp(n int) *int { return &n }

func TestItemsShape(t *testing.T) {
	d, err := Lifecycle()
	if err != nil {
		t.Fatal(err)
	}
	if d.Approver != "Kaylinn Brophy" || d.Published != "2026-10-30" {
		t.Fatalf("header = %q / %q", d.Approver, d.Published)
	}
	// `since` is each item's concept date (LIFECYCLE-DOCS-SPEC D3, Since -- the concept entry's
	// date): 2026-10-30 at publication; 2026-10-31 on the thirteen items the user's concision pass
	// reworded (no decision existed, so nothing was voided); 2026-11-01 on item 13 and on 19/21
	// (SUNSET-V4 D8 — still no decision, nothing voided).
	want := []struct {
		key, section string
		n            *int
		since        string
	}{
		{"np-entry", SectionNoPurchase, intp(1), "2026-10-30"},
		{"np-exit-on-purchase", SectionNoPurchase, intp(2), "2026-10-30"},
		{"np-same-day-double", SectionNoPurchase, intp(3), "2026-10-30"},
		{"np-launch-overlap", SectionNoPurchase, intp(4), "2026-10-31"},
		{"np-no-restarts", SectionNoPurchase, intp(5), "2026-10-30"},
		{"np-unsubscribe-stops", SectionNoPurchase, intp(6), "2026-10-30"},
		{"np-ruze-15d", SectionNoPurchase, intp(7), "2026-10-31"},
		{"pp-entry", SectionPostPurchase, intp(8), "2026-10-30"},
		{"pp-delivery-clock", SectionPostPurchase, intp(9), "2026-10-31"},
		{"pp-final-step-skip", SectionPostPurchase, intp(10), "2026-10-30"},
		{"pp-returning-thanks", SectionPostPurchase, intp(11), "2026-10-30"},
		{"both-multi-brand", SectionBoth, intp(12), "2026-10-31"},
		{"both-per-brand", SectionBoth, intp(13), "2026-11-01"},
		{"both-language", SectionBoth, intp(14), "2026-10-31"},
		// USAGE-REENGAGEMENT-SPEC D7: the Curated Sunset Policy section.
		{"sp-scope", SectionSunsetPolicy, intp(15), "2026-10-30"},
		{"sp-engagement", SectionSunsetPolicy, intp(16), "2026-11-02"},
		{"sp-normal", SectionSunsetPolicy, intp(17), "2026-11-02"},
		{"sp-hold", SectionSunsetPolicy, intp(18), "2026-10-31"},
		{"sp-asks", SectionSunsetPolicy, intp(19), "2026-11-02"},        // SUNSET-V4 D8; its NOTE became item 20 (2026-11-02)
		{"sp-winback", SectionSunsetPolicy, intp(20), "2026-11-02"},     // the win-back line + the 360d/365d refusal (FINAL-QUEUE-GUARD-SPEC)
		{"sp-coming-back", SectionSunsetPolicy, intp(21), "2026-11-02"}, // the journey-candidate sentence (2026-11-02)
		{"sp-suppressed", SectionSunsetPolicy, intp(22), "2026-11-01"},  // SUNSET-V4 D8
		{"sp-safety", SectionSunsetPolicy, intp(23), "2026-10-31"},
	}
	if len(d.Items) != len(want) {
		t.Fatalf("%d items, want %d", len(d.Items), len(want))
	}
	for i, w := range want {
		it := d.Items[i]
		if it.Key != w.key || it.Section != w.section {
			t.Errorf("item %d = %s/%s, want %s/%s", i, it.Key, it.Section, w.key, w.section)
		}
		if (it.N == nil) != (w.n == nil) || (it.N != nil && *it.N != *w.n) {
			t.Errorf("%s: n = %v, want %v", it.Key, it.N, w.n)
		}
		if got := Since(it); got != w.since {
			t.Errorf("%s: since = %s, want %s", it.Key, got, w.since)
		}
		checkShippedChangelog(t, it)
		// D2's canonical form: valid UTF-8, single \n separators, no blank line, no leading or
		// trailing newline, no trailing whitespace on any line.
		if !utf8.ValidString(it.Text) {
			t.Errorf("%s: text is not UTF-8", it.Key)
		}
		if strings.HasPrefix(it.Text, "\n") || strings.HasSuffix(it.Text, "\n") || strings.Contains(it.Text, "\n\n") || strings.Contains(it.Text, "\r") {
			t.Errorf("%s: text is not canonical (newlines)", it.Key)
		}
		for _, ln := range strings.Split(it.Text, "\n") {
			if ln != strings.TrimRight(ln, " \t") {
				t.Errorf("%s: trailing whitespace on %q", it.Key, ln)
			}
		}
	}
	// USAGE-REENGAGEMENT-SPEC D8: the one table, sunset-policy's, 3 rows x 2 columns plus its note.
	if len(d.Tables) != 1 {
		t.Fatalf("%d tables, want 1", len(d.Tables))
	}
	tb, ok := d.Tables[SectionSunsetPolicy]
	if !ok {
		t.Fatal("no sunset-policy table")
	}
	if len(tb.Columns) != 2 || len(tb.Rows) != 3 || tb.Note == "" {
		t.Fatalf("sunset-policy table: %d columns, %d rows, note %q", len(tb.Columns), len(tb.Rows), tb.Note)
	}
	for i, r := range tb.Rows {
		if len(r) != 2 {
			t.Errorf("row %d has %d cells", i, len(r))
		}
	}
	// Not an item: no item carries a table's name as its key.
	if _, ok := d.Find(SectionSunsetPolicy); ok {
		t.Fatal("a table name is an item key")
	}
}

// The shipped changelog shape (LIFECYCLE-DOCS-SPEC I7, D11-D16; generalised by integrations
// SMART-SENDING-SPEC D10/I9). Each item's lineage after its concept entry, in file order, as one
// token per entry: "slack" = Kaylinn's hash-less Slack 2026-11-06 entry; "relayed" = her hash-less
// relayed 2026-11-07 entry; "page" = a hash-less page entry (a change of meaning folded into the next
// edit); "page#" = a page entry carrying a text hash. The six items rewritten on 2026-11-07 carry a
// page# entry (item 1 two, the Stage 4 R3 rewording); items 1, 9 and 13 the Slack entry before it;
// item 13 (Smart Sending implemented) then the relayed request, the analysis and the implementation
// edit. Every other item has only its concept entry. The check keys on the NEWEST HASH-BEARING entry
// (its hash is the text's), never the last entry, so a later hash-less entry (gate G7's decision
// entry) keeps the shape test meaningful; and it asserts what the file guarantees -- a `who` entry
// is hash-less, a hash-bearing entry is source "page".
var shippedLineage = map[string][]string{
	"np-entry":            {"slack", "page#", "page#"},
	"np-same-day-double":  {"page#"},
	"np-no-restarts":      {"page#"},
	"pp-delivery-clock":   {"slack", "page#"},
	"pp-returning-thanks": {"page#"},
	"both-per-brand":      {"slack", "page#", "relayed", "page", "page#"},
}

func checkShippedChangelog(t *testing.T, it Item) {
	t.Helper()
	want := shippedLineage[it.Key]
	cl := it.Changelog
	if len(cl) != 1+len(want) {
		t.Errorf("%s: %d changelog entries, want %d", it.Key, len(cl), 1+len(want))
		return
	}
	if c := cl[0]; !c.Reapprove || c.Source != "page" || c.Line != "Published for approval" || c.Who != "" || c.Hash != pinnedHashes[it.Key] {
		t.Errorf("%s: concept entry %+v", it.Key, c)
	}
	newestHashed := 0
	for i, tok := range want {
		c := cl[i+1]
		if c.Reapprove || c.Line == "" {
			t.Errorf("%s: entry %d %+v", it.Key, i+1, c)
		}
		// What the file guarantees for every entry: a `who` entry is hash-less; a hash-bearing entry
		// is source "page".
		if c.Who != "" && c.Hash != "" {
			t.Errorf("%s: entry %d carries both who and a hash", it.Key, i+1)
		}
		if c.Hash != "" && c.Source != "page" {
			t.Errorf("%s: entry %d is hash-bearing but source %q", it.Key, i+1, c.Source)
		}
		ok := false
		switch tok {
		case "slack":
			ok = c.Date == "2026-11-06" && c.Source == "Slack 2026-11-06" && c.Who == "Kaylinn Brophy" && c.Hash == ""
		case "relayed":
			ok = c.Date == "2026-11-07" && c.Source == "relayed 2026-11-07" && c.Who == "Kaylinn Brophy" && c.Hash == ""
		case "page":
			ok = c.Date == "2026-11-07" && c.Source == "page" && c.Who == "" && c.Hash == ""
		case "page#":
			ok = c.Date == "2026-11-07" && c.Source == "page" && c.Who == "" && c.Hash != ""
		}
		if !ok {
			t.Errorf("%s: entry %d %+v, want a %s entry", it.Key, i+1, c, tok)
		}
		if c.Hash != "" {
			newestHashed = i + 1
		}
	}
	if len(want) > 0 {
		if newestHashed == 0 || cl[newestHashed].Hash != Hash(it.Text) {
			t.Errorf("%s: the newest hash-bearing entry (%d) does not carry the text's hash", it.Key, newestHashed)
		}
	}
}

// Integrations SMART-SENDING-SPEC I9 -- item 13 as implemented: no "Recommended:" line (Accept |
// Reject), the concept pin unchanged, a decision on the concept text reads "approved as modified" by
// Kaylinn on 2026-11-07 (her relayed request is the newest `who` entry after the concept), the last
// entry carries the callout and the newest hash-bearing entry the text's hash.
func TestSmartSendingItem(t *testing.T) {
	d, err := Lifecycle()
	if err != nil {
		t.Fatal(err)
	}
	it, ok := d.Find("both-per-brand")
	if !ok {
		t.Fatal("no item 13")
	}
	if HasRecommended(it.Text) || strings.Join(Choices(it), ",") != Accept+","+Reject {
		t.Fatalf("item 13 choices %v", Choices(it))
	}
	if Concept(it).Hash != pinnedHashes[it.Key] {
		t.Fatal("item 13's concept pin moved")
	}
	if m := Modified(it, Concept(it).Hash); m == nil || m.Who != "Kaylinn Brophy" || m.Date != "2026-11-07" {
		t.Fatalf("Modified(concept) = %+v", m)
	}
	// A row on the new text's hash sits at the last entry: nothing after it.
	if m := Modified(it, Hash(it.Text)); m != nil {
		t.Fatalf("Modified(current text) = %+v", m)
	}
	last := it.Changelog[len(it.Changelog)-1]
	if last.Hash != Hash(it.Text) || last.Callout == "" {
		t.Fatalf("item 13's last entry %+v", last)
	}
	v := ItemOf(it, nil)
	if c := v.Changelog[len(v.Changelog)-1].Callout; c != last.Callout {
		t.Fatalf("the GET's last entry callout = %q", c)
	}
	for i, c := range v.Changelog[:len(v.Changelog)-1] {
		if c.Callout != "" {
			t.Fatalf("entry %d carries a callout: %q", i, c.Callout)
		}
	}
}

// pinnedHashes are each item's CONCEPT hash -- its newest `reapprove` entry's (LIFECYCLE-DOCS-SPEC
// D2 (b)). An ordinary text edit needs no change here (its changelog entry is the
// acknowledgement); requesting a re-approval -- an entry with `reapprove: true` and the new hash --
// is the deliberate act that voids Marketing's decision, and it replaces the item's pin here in the
// same change. The values are the item texts' hashes as Kaylinn approved them on 2026-11-05.
var pinnedHashes = map[string]string{
	"np-entry":             "853f6bd91015c71f5616a4d103963fa55feff23e56feeb2ae01530b46f52466f",
	"np-exit-on-purchase":  "5b6c4ce9a2fb62225f37b22f31a10012d10eb30f8070893645fd7c9818a08d08",
	"np-same-day-double":   "c92f801864a2a20e0d2622cabf2cc9ba60f7009b31bc619b6c35c80669c3855f",
	"np-launch-overlap":    "d7ede5ae8e9efe4e5ff95f24ed8bb3d74244ff533904bda48eb8400a7218d1b3",
	"np-no-restarts":       "9939b17029272c8dccd51147c2eae5e484373939283f3cf64115a2a65b78e0c2",
	"np-unsubscribe-stops": "010ae60d9a0a8ef0a920e0c465a38776a457a322be531027b7b07c642f9ebf8c",
	"np-ruze-15d":          "4a00e902003dcfd0a30ea88c0de8857fd93cbb1d390b869e0edd23cc59fb3447",
	"pp-entry":             "04c9677b5342fae0df4fc80eea6cd7ff4d683c029194dedf08eeb9bcd119219a",
	"pp-delivery-clock":    "076286fe5369766d0bdd7f9088bda7beae753099ec94f055b7583c9c16db7d1c",
	"pp-final-step-skip":   "3f068c366f42b25026a83d1503499af293c02791c7ca4784e4e1dc3a85b65e95",
	"pp-returning-thanks":  "96bad4eae80e5bbb49a8517ec66c8b9daae5d2458142e0bc7bc7a78fb572e64b",
	"both-multi-brand":     "cce91d6c3233d5109155fd799daa3cd573b13f283be22c296c8c9f3a6d821bd6",
	"both-per-brand":       "fffd6054bb4f91b4c3e0e2cd09a38bdf326bc3bc4e7bed46a93fb50d0e1ae9ef", // NOTE line dropped, Klaviyo aside kept (2026-11-01)
	"both-language":        "578c218c31eaab2268ed5b8f3fdbf306f4573c3ac4211b697f86e92392cb9d0f",
	"sp-scope":             "b82a9e15281891935c2c3afc621e29db5f26497c40d8988ba87f32615ba5a54b",
	"sp-engagement":        "7c179904c1332c8b4065b6b0c0f17bb0bd9b7857ef109b0c02d83fc13f2794fc", // Klaviyo aside parenthesized (2026-11-02)
	"sp-normal":            "019dcdef0d1a6ca90c79b7808b27a885d03586fa7ee398dabc818cf3a0214a83", // justification reworded (2026-11-02)
	"sp-hold":              "82038c303d156fcd91d49b849be02297ad47855fa76b036714042ba93c3d52c1",
	"sp-asks":              "3138349591e68f7ca4675f4a13a676d75b65ba92e7ef77d98126fd4943d91e8a", // SUNSET-V4 D8; its NOTE split out as item 20 (2026-11-02)
	"sp-winback":           "c18615cfff81bf60a8fa66f12dd36eaefac1278cce6a526ddc7d41ab291685e3", // the win-back line + the 360d/365d refusal (2026-11-02)
	"sp-coming-back":       "a618929fb8f9f59a1b92953eacf813614c1ce9e1afbdfd2f862711984bb0d86f", // the journey-candidate sentence (2026-11-02)
	"sp-suppressed":        "36cfe7f4776d4db972dbdba74e1639b1f784e7670f9e5fced74101f9c91fcc8e", // SUNSET-V4 D8: unsubscribed at 365, with the confirmation (2026-11-01)
	"sp-safety":            "0170340a622e3402a6e022dc13baa67062567c9d2cdce51af2bc1d0ebcb46dfb",
}

func TestItemHashesPinned(t *testing.T) {
	d, err := Lifecycle()
	if err != nil {
		t.Fatal(err)
	}
	if len(pinnedHashes) != len(d.Items) {
		t.Fatalf("%d pins for %d items", len(pinnedHashes), len(d.Items))
	}
	for _, it := range d.Items {
		pin, ok := pinnedHashes[it.Key]
		if !ok {
			t.Errorf("%s: no pinned hash", it.Key)
			continue
		}
		if got := Concept(it).Hash; got != pin {
			t.Errorf("%s: concept hash %s != pinned %s -- a reapprove entry voids Marketing's decision on this item: replace this pin in the same change (LIFECYCLE-DOCS-SPEC D2)", it.Key, got, pin)
		}
		// D4's invariant: the current text's hash is always accepted, so a fresh decision is never
		// Awaiting.
		if !slices.Contains(Accepted(it), Hash(it.Text)) {
			t.Errorf("%s: the current text hash is not accepted", it.Key)
		}
		// The shipped file requests no re-approval: exactly one reapprove entry, the first.
		n := 0
		for _, c := range it.Changelog {
			if c.Reapprove {
				n++
			}
		}
		if n != 1 || !it.Changelog[0].Reapprove {
			t.Errorf("%s: %d reapprove entries (want exactly the first)", it.Key, n)
		}
	}
	// The hash covers the text only: key, n and the changelog are not hashed.
	if Hash("a") != "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb" {
		t.Fatal("Hash is not lowercase hex sha256 of the text bytes")
	}
}

// 2026-10-31 polish: Choices -- Current | Recommended | Reject exactly on the items whose text has a
// "Recommended:" line after the first; Accept | Reject on every other. The shipped items with a
// Recommended option are pinned, so a text edit that adds or drops one is a deliberate change
// (LIFECYCLE-DOCS-SPEC I8: items 3 and 9 lost theirs when implemented, 2026-11-07; integrations
// SMART-SENDING-SPEC D8/I9: item 13 lost its when Smart Sending was implemented, 2026-11-07).
func TestChoices(t *testing.T) {
	if HasRecommended("Recommended: lead line only") {
		t.Fatal("a first-line Recommended: is the lead, not an option")
	}
	if !HasRecommended("Lead.\nRecommended:\n1. x") || HasRecommended("Lead.\nNot Recommended: x") {
		t.Fatal("HasRecommended reads a later line's prefix")
	}
	d, err := Lifecycle()
	if err != nil {
		t.Fatal(err)
	}
	var three []string
	for _, it := range d.Items {
		c := Choices(it)
		switch len(c) {
		case 3:
			if c[0] != Current || c[1] != Recommended || c[2] != Reject {
				t.Fatalf("%s: %v", it.Key, c)
			}
			three = append(three, it.Key)
		case 2:
			if c[0] != Accept || c[1] != Reject {
				t.Fatalf("%s: %v", it.Key, c)
			}
		default:
			t.Fatalf("%s: %v", it.Key, c)
		}
	}
	want := []string{"np-launch-overlap", "np-ruze-15d", "both-multi-brand"}
	if strings.Join(three, ",") != strings.Join(want, ",") {
		t.Fatalf("items with three choices %v, want %v", three, want)
	}
}

// Synthetic-file helpers: a changelog entry, a changelog, and a one-item file keyed "item-a".
func entry(date, source, who, line, hash string, reapprove bool) string {
	b, _ := json.Marshal(Change{Date: date, Source: source, Who: who, Line: line, Hash: hash, Reapprove: reapprove})
	return string(b)
}

func changelog(entries ...string) string { return "[" + strings.Join(entries, ",") + "]" }

func itemJSON(key, text, cl string) string {
	return fmt.Sprintf(`{"key":%q,"section":"both","text":%q,"changelog":%s}`, key, text, cl)
}

func oneItem(text, cl string) string { return `{"items":[` + itemJSON("item-a", text, cl) + `]}` }

func TestParseRefusesBadFiles(t *testing.T) {
	hx, hy := Hash("x"), Hash("y")
	concept := entry("2026-10-30", "page", "", "Published for approval", hx, true)
	good := func(text string) string { return oneItem(text, changelog(concept)) }
	for name, src := range map[string]string{
		"duplicate key":   `{"items":[` + itemJSON("item-a", "x", changelog(concept)) + `,` + itemJSON("item-a", "x", changelog(concept)) + `]}`,
		"unknown section": strings.Replace(good("x"), `"section":"both"`, `"section":"other"`, 1),
		"empty text":      oneItem("", changelog(concept)),
		"not json":        `{`,
		// USAGE-REENGAGEMENT-SPEC D8: a table only on sunset-policy, never ragged.
		"table on another section": `{"items":[],"tables":{"both":{"columns":["a","b"],"rows":[["1","2"]],"note":""}}}`,
		"table on an unknown name": `{"items":[],"tables":{"other":{"columns":["a","b"],"rows":[],"note":""}}}`,
		"ragged table row":         `{"items":[],"tables":{"sunset-policy":{"columns":["a","b"],"rows":[["1","2"],["3"]],"note":""}}}`,
		"table with no columns":    `{"items":[],"tables":{"sunset-policy":{"columns":[],"rows":[],"note":""}}}`,
		// LIFECYCLE-DOCS-SPEC D2 (a) / I1.
		"no changelog":                    `{"items":[{"key":"item-a","section":"both","text":"x"}]}`,
		"empty changelog":                 oneItem("x", changelog()),
		"first entry not reapprove":       oneItem("x", changelog(entry("2026-10-30", "page", "", "Published", hx, false))),
		"first entry without a hash":      oneItem("x", changelog(entry("2026-10-30", "page", "", "Published", "", true))),
		"later reapprove without a hash":  oneItem("x", changelog(concept, entry("2026-10-31", "page", "", "Re-approve", "", true))),
		"uppercase hash":                  oneItem("x", changelog(entry("2026-10-30", "page", "", "Published", strings.ToUpper(hx), true))),
		"short hash":                      oneItem("x", changelog(concept, entry("2026-10-31", "page", "", "Edit", hy[:63], false))),
		"unparsable date":                 oneItem("x", changelog(entry("30/10/2026", "page", "", "Published", hx, true))),
		"decreasing date":                 oneItem("y", changelog(concept, entry("2026-10-29", "page", "", "Edit", hy, false))),
		"empty source":                    oneItem("x", changelog(entry("2026-10-30", " ", "", "Published", hx, true))),
		"empty line":                      oneItem("x", changelog(entry("2026-10-30", "page", "", "", hx, true))),
		"text edit without an entry":      oneItem("y", changelog(concept)),
		"hash-less entry after an edit":   oneItem("y", changelog(concept, entry("2026-11-06", "Slack 2026-11-06", "K", "Changed it", "", false))),
		"newest hash is an earlier text":  oneItem("x", changelog(concept, entry("2026-10-31", "page", "", "Edit", hy, false))),
		"reapprove carrying a stale hash": oneItem("y", changelog(concept, entry("2026-10-31", "page", "", "Edit", hy, false), entry("2026-11-01", "page", "", "Re-approve", hx, true))),
	} {
		_, err := parse([]byte(src))
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		// Every item refusal names the item.
		if strings.Contains(src, "item-a") && !strings.Contains(err.Error(), "item-a") {
			t.Errorf("%s: %v does not name the item", name, err)
		}
	}
	if _, err := parse([]byte(oneItem("y", changelog(concept)))); err == nil || !strings.Contains(err.Error(), "add a changelog entry carrying the new hash") {
		t.Errorf("an unacknowledged edit's message does not name the fix: %v", err)
	}
	// The controls: the base file, an edit with its entry, a hash-less Slack entry before the edit,
	// same-day entries, and a re-approval all parse.
	for name, src := range map[string]string{
		"concept only":            good("x"),
		"an edit with its entry":  oneItem("y", changelog(concept, entry("2026-10-31", "page", "", "Edit", hy, false))),
		"Slack entry, then edit":  oneItem("y", changelog(concept, entry("2026-11-06", "Slack 2026-11-06", "K", "Changed it", "", false), entry("2026-11-07", "page", "", "Edit", hy, false))),
		"same-day entries":        oneItem("y", changelog(concept, entry("2026-10-30", "page", "", "Edit", hy, false))),
		"a re-approval":           oneItem("y", changelog(concept, entry("2026-11-01", "page", "", "Re-approve", hy, true))),
		"a revert with its entry": oneItem("x", changelog(concept, entry("2026-10-31", "page", "", "Edit", hy, false), entry("2026-11-01", "page", "", "Revert", hx, false))),
	} {
		if _, err := parse([]byte(src)); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
	// D7: sunset-policy is an accepted section, and a well-formed table on it parses.
	ok := `{"items":[{"key":"a","section":"sunset-policy","n":15,"text":"x","changelog":` + changelog(concept) + `}],"tables":{"sunset-policy":{"columns":["a","b"],"rows":[["1","2"]],"note":"n"}}}`
	d, err := parse([]byte(ok))
	if err != nil {
		t.Fatalf("sunset-policy refused: %v", err)
	}
	if d.Items[0].Section != SectionSunsetPolicy || len(d.Tables[SectionSunsetPolicy].Rows) != 1 {
		t.Fatalf("parsed %+v", d)
	}
}

// LIFECYCLE-DOCS-SPEC I1: the shipped file re-parsed with one item's text altered fails, naming the
// key; with a matching changelog entry appended it parses -- and the decision on the concept text
// is still current (D3).
func TestUnacknowledgedEditFails(t *testing.T) {
	var d Doc
	if err := json.Unmarshal(lifecycleJSON, &d); err != nil {
		t.Fatal(err)
	}
	const i = 5 // np-unsubscribe-stops: one entry, its concept
	it := d.Items[i]
	conceptHash := Hash(it.Text)
	d.Items[i].Text = it.Text + " Edited."
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parse(b)
	if err == nil || !strings.Contains(err.Error(), it.Key) || !strings.Contains(err.Error(), "add a changelog entry carrying the new hash") {
		t.Fatalf("an unacknowledged edit: %v", err)
	}
	d.Items[i].Changelog = append(append([]Change(nil), it.Changelog...), Change{Date: "2026-11-08", Source: "page", Line: "Edited.", Hash: Hash(d.Items[i].Text)})
	if b, err = json.Marshal(d); err != nil {
		t.Fatal(err)
	}
	got, err := parse(b)
	if err != nil {
		t.Fatalf("an acknowledged edit was refused: %v", err)
	}
	edited, _ := got.Find(it.Key)
	cur, _ := State([]Row{{ID: 1, ItemKey: it.Key, TextHash: conceptHash, Decision: Accept}}, Accepted(edited))
	if cur == nil {
		t.Fatal("an acknowledged edit voided the decision")
	}
}

// LIFECYCLE-DOCS-SPEC I3: Accepted is the concept entry's hash then every later hash-bearing
// entry's, in order; Since is the concept entry's date; a later reapprove entry restarts both.
func TestLineage(t *testing.T) {
	c := func(date, who, hash string, re bool) Change {
		return Change{Date: date, Source: "page", Who: who, Line: "l", Hash: hash, Reapprove: re}
	}
	for _, tc := range []struct {
		name    string
		cl      []Change
		want    []string
		since   string
		concept string
	}{
		{"no edits", []Change{c("2026-10-30", "", "h0", true)}, []string{"h0"}, "2026-10-30", "h0"},
		{"two edits", []Change{c("2026-10-30", "", "h0", true), c("2026-10-31", "", "h1", false), c("2026-11-01", "", "h2", false)}, []string{"h0", "h1", "h2"}, "2026-10-30", "h0"},
		{"a Slack hash-less entry between", []Change{c("2026-10-30", "", "h0", true), c("2026-11-06", "K", "", false), c("2026-11-07", "", "h1", false)}, []string{"h0", "h1"}, "2026-10-30", "h0"},
		{"a re-approval after edits", []Change{c("2026-10-30", "", "h0", true), c("2026-10-31", "", "h1", false), c("2026-11-02", "", "h2", true), c("2026-11-03", "", "h3", false)}, []string{"h2", "h3"}, "2026-11-02", "h2"},
	} {
		it := Item{Key: "k", Changelog: tc.cl}
		if got := Accepted(it); !slices.Equal(got, tc.want) {
			t.Errorf("%s: Accepted = %v, want %v", tc.name, got, tc.want)
		}
		if got := Since(it); got != tc.since {
			t.Errorf("%s: Since = %s, want %s", tc.name, got, tc.since)
		}
		if got := Concept(it).Hash; got != tc.concept {
			t.Errorf("%s: Concept = %s, want %s", tc.name, got, tc.concept)
		}
	}
}
