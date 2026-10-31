package decisions

// Fork (usage decisions) -- integrations USAGE-DECISIONS-SPEC I1/I2. I1: lifecycle.json carries
// exactly §0's 22 keys in order (14 + the 8 Curated Sunset Policy items, USAGE-REENGAGEMENT-SPEC
// D7), with §0's sections, numbers, since dates and header, and the sunset-policy table. I2: each
// item's hash is pinned as a literal -- a text edit fails TestItemHashesPinned until the editor
// bumps the item's `since` AND its pin here, the deliberate act that voids Marketing's decision on
// that item (D2). I3 (word-for-word agreement with the spec's §0) is untestable across repos:
// edit §0 and lifecycle.json in one change.

import (
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
	// `since` is each item's text date (D2): 2026-10-30 at publication; 2026-10-31 on the thirteen
	// items the user's concision pass reworded (no decision existed, so nothing was voided).
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
		{"both-per-brand", SectionBoth, intp(13), "2026-10-31"},
		{"both-language", SectionBoth, intp(14), "2026-10-31"},
		// USAGE-REENGAGEMENT-SPEC D7: the Curated Sunset Policy section.
		{"sp-scope", SectionSunsetPolicy, intp(15), "2026-10-30"},
		{"sp-engagement", SectionSunsetPolicy, intp(16), "2026-10-31"},
		{"sp-normal", SectionSunsetPolicy, intp(17), "2026-10-31"},
		{"sp-hold", SectionSunsetPolicy, intp(18), "2026-10-31"},
		{"sp-asks", SectionSunsetPolicy, intp(19), "2026-10-31"},
		{"sp-coming-back", SectionSunsetPolicy, intp(20), "2026-10-31"},
		{"sp-suppressed", SectionSunsetPolicy, intp(21), "2026-10-31"},
		{"sp-safety", SectionSunsetPolicy, intp(22), "2026-10-31"},
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
		if it.Since != w.since {
			t.Errorf("%s: since = %s, want %s", it.Key, it.Since, w.since)
		}
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

// pinnedHashes are the literal sha256 of each item's text (D2). Changing a text is a deliberate
// act: bump that item's `since` to the edit date and replace its pin here in the same change.
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
	"both-per-brand":       "22c62880631320e478228b0ddcdb713afc79733054e1afe0a882db76d4c94865",
	"both-language":        "578c218c31eaab2268ed5b8f3fdbf306f4573c3ac4211b697f86e92392cb9d0f",
	"sp-scope":             "b82a9e15281891935c2c3afc621e29db5f26497c40d8988ba87f32615ba5a54b",
	"sp-engagement":        "062c825492e7b62fb3c206bd99ed4ab4632a2b424cb14786058d22516e78263c",
	"sp-normal":            "c2afde039a1d8a35496f2f5f23abbfea2a3b36e1659d5d4f19b06aa984a97c1e",
	"sp-hold":              "82038c303d156fcd91d49b849be02297ad47855fa76b036714042ba93c3d52c1",
	"sp-asks":              "8d61ad588e5db25063692764e4d4d8075e2b2a199a556d759fd61b8b4b70daf7",
	"sp-coming-back":       "1c1cf2c139641bffd5f23b59240691d8d334f97b17b3730cfe2b93c1a515f86b",
	"sp-suppressed":        "6631fa895a51be4629cb6516d184cf1f1a48f947e17d2f18ad3b82a0f309e75e",
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
		if got := Hash(it.Text); got != pin {
			t.Errorf("%s: text hash %s != pinned %s -- a text edit voids Marketing's decision on this item: bump its since and this pin together (USAGE-DECISIONS-SPEC D2)", it.Key, got, pin)
		}
	}
	// The hash covers the text only: key, n and since are not hashed.
	if Hash("a") != "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb" {
		t.Fatal("Hash is not lowercase hex sha256 of the text bytes")
	}
}

// 2026-10-31 polish: Choices -- Current | Recommended | Reject exactly on the items whose text has a
// "Recommended:" line after the first; Accept | Reject on every other. The six shipped items with a
// Recommended option are pinned, so a text edit that adds or drops one is a deliberate change.
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
	want := []string{"np-same-day-double", "np-launch-overlap", "np-ruze-15d", "pp-delivery-clock", "both-multi-brand", "both-per-brand"}
	if strings.Join(three, ",") != strings.Join(want, ",") {
		t.Fatalf("items with three choices %v, want %v", three, want)
	}
}

func TestParseRefusesBadFiles(t *testing.T) {
	for name, src := range map[string]string{
		"duplicate key":   `{"items":[{"key":"a","section":"both","since":"2026-10-30","text":"x"},{"key":"a","section":"both","since":"2026-10-30","text":"y"}]}`,
		"unknown section": `{"items":[{"key":"a","section":"other","since":"2026-10-30","text":"x"}]}`,
		"bad since":       `{"items":[{"key":"a","section":"both","since":"30/10/2026","text":"x"}]}`,
		"empty text":      `{"items":[{"key":"a","section":"both","since":"2026-10-30","text":""}]}`,
		"not json":        `{`,
		// USAGE-REENGAGEMENT-SPEC D8: a table only on sunset-policy, never ragged.
		"table on another section": `{"items":[],"tables":{"both":{"columns":["a","b"],"rows":[["1","2"]],"note":""}}}`,
		"table on an unknown name": `{"items":[],"tables":{"other":{"columns":["a","b"],"rows":[],"note":""}}}`,
		"ragged table row":         `{"items":[],"tables":{"sunset-policy":{"columns":["a","b"],"rows":[["1","2"],["3"]],"note":""}}}`,
		"table with no columns":    `{"items":[],"tables":{"sunset-policy":{"columns":[],"rows":[],"note":""}}}`,
	} {
		if _, err := parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// D7: sunset-policy is an accepted section, and a well-formed table on it parses.
	ok := `{"items":[{"key":"a","section":"sunset-policy","n":15,"since":"2026-10-30","text":"x"}],"tables":{"sunset-policy":{"columns":["a","b"],"rows":[["1","2"]],"note":"n"}}}`
	d, err := parse([]byte(ok))
	if err != nil {
		t.Fatalf("sunset-policy refused: %v", err)
	}
	if d.Items[0].Section != SectionSunsetPolicy || len(d.Tables[SectionSunsetPolicy].Rows) != 1 {
		t.Fatalf("parsed %+v", d)
	}
}
