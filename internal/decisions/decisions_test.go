package decisions

// Fork (usage decisions) -- integrations USAGE-DECISIONS-SPEC I1/I2. I1: lifecycle.json carries
// exactly §0's 14 keys in order, with §0's sections, numbers, since dates and header. I2: each
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
	want := []struct {
		key, section string
		n            *int
	}{
		{"np-entry", SectionNoPurchase, intp(1)},
		{"np-exit-on-purchase", SectionNoPurchase, intp(2)},
		{"np-same-day-double", SectionNoPurchase, intp(3)},
		{"np-launch-overlap", SectionNoPurchase, intp(4)},
		{"np-no-restarts", SectionNoPurchase, intp(5)},
		{"np-unsubscribe-stops", SectionNoPurchase, intp(6)},
		{"np-ruze-15d", SectionNoPurchase, intp(7)},
		{"pp-entry", SectionPostPurchase, intp(8)},
		{"pp-delivery-clock", SectionPostPurchase, intp(9)},
		{"pp-final-step-skip", SectionPostPurchase, intp(10)},
		{"pp-returning-thanks", SectionPostPurchase, intp(11)},
		{"both-multi-brand", SectionBoth, nil},
		{"both-per-brand", SectionBoth, nil},
		{"both-language", SectionBoth, nil},
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
		if it.Since != "2026-10-30" {
			t.Errorf("%s: since = %s", it.Key, it.Since)
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
}

// pinnedHashes are the literal sha256 of each item's text (D2). Changing a text is a deliberate
// act: bump that item's `since` to the edit date and replace its pin here in the same change.
var pinnedHashes = map[string]string{
	"np-entry":             "853f6bd91015c71f5616a4d103963fa55feff23e56feeb2ae01530b46f52466f",
	"np-exit-on-purchase":  "5b6c4ce9a2fb62225f37b22f31a10012d10eb30f8070893645fd7c9818a08d08",
	"np-same-day-double":   "c92f801864a2a20e0d2622cabf2cc9ba60f7009b31bc619b6c35c80669c3855f",
	"np-launch-overlap":    "773fa0f54393e68bff3a638b0fa412410f28968f6e90995a1355f2c299fa7b6b",
	"np-no-restarts":       "9939b17029272c8dccd51147c2eae5e484373939283f3cf64115a2a65b78e0c2",
	"np-unsubscribe-stops": "010ae60d9a0a8ef0a920e0c465a38776a457a322be531027b7b07c642f9ebf8c",
	"np-ruze-15d":          "5188c81dc5197351345d928303caaa9889e8403bde83b6aac304b953d35ad48e",
	"pp-entry":             "04c9677b5342fae0df4fc80eea6cd7ff4d683c029194dedf08eeb9bcd119219a",
	"pp-delivery-clock":    "1c746a73e6ee3db6ed484f69cecbd9f7b9835cfe41f420ed3f31619c020431f0",
	"pp-final-step-skip":   "3f068c366f42b25026a83d1503499af293c02791c7ca4784e4e1dc3a85b65e95",
	"pp-returning-thanks":  "96bad4eae80e5bbb49a8517ec66c8b9daae5d2458142e0bc7bc7a78fb572e64b",
	"both-multi-brand":     "95407fcce716865e6d984a41ca5e1ba8a592f72c897907f19dfa00dcb6941f65",
	"both-per-brand":       "7c9f2715ff814ee3d0d2a426db85098be5bbfa1ab77cd718237f493ddfbf435d",
	"both-language":        "c2d1365f1f8172d6a7f207d1d0045d54759108c8601271f9c8b14dc777b0b7de",
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

func TestParseRefusesBadFiles(t *testing.T) {
	for name, src := range map[string]string{
		"duplicate key":   `{"items":[{"key":"a","section":"both","since":"2026-10-30","text":"x"},{"key":"a","section":"both","since":"2026-10-30","text":"y"}]}`,
		"unknown section": `{"items":[{"key":"a","section":"other","since":"2026-10-30","text":"x"}]}`,
		"bad since":       `{"items":[{"key":"a","section":"both","since":"30/10/2026","text":"x"}]}`,
		"empty text":      `{"items":[{"key":"a","section":"both","since":"2026-10-30","text":""}]}`,
		"not json":        `{`,
	} {
		if _, err := parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
