// Package decisions is the fork's home for Marketing's lifecycle-series decision record on
// Brands > Usage (integrations USAGE-DECISIONS-SPEC D1/D2/D4). The item texts live in the embedded
// lifecycle.json -- the fork's text home for them, word for word the spec's §0 -- and each
// decision row in the append-only usage_decisions table (internal/migrations/v6.2.29.go).
//
// The fork STORES these; nothing in the integrations repo computes or reads them. An item's hash
// is the lowercase hex sha256 of its text bytes. Since LIFECYCLE-DOCS-SPEC D1-D3 the approval is of
// the item's CONCEPT: every item carries a changelog (its lineage), an ordinary text edit appends an
// entry carrying the new hash and KEEPS the decision on record, and only an entry with
// `reapprove: true` returns the item to Awaiting. parse refuses a text whose hash is not the newest
// hash-bearing entry's (the structural acknowledgement), and the literal pin in decisions_test.go
// is each item's concept hash -- it changes only when a re-approval is requested.
package decisions

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

//go:embed lifecycle.json
var lifecycleJSON []byte

// Sections, in display order.
const (
	SectionNoPurchase   = "no-purchase"
	SectionPostPurchase = "post-purchase"
	SectionBoth         = "both"
	// SectionSunsetPolicy is the Curated Sunset Policy block (integrations USAGE-REENGAGEMENT-SPEC
	// D7) -- ordinary items, plus the one non-decision table (Doc.Tables, D8).
	SectionSunsetPolicy = "sunset-policy"
)

// The decisions a row may carry. An item with a "Recommended:" line (HasRecommended) is decided
// Current | Recommended | Reject; every other item Accept | Reject (Choices; 2026-10-31 polish,
// migration v6.2.30).
const (
	Accept      = "accept"
	Reject      = "reject"
	Current     = "current"
	Recommended = "recommended"
)

// RecommendedLabel opens the line that offers an item's recommended option.
const RecommendedLabel = "Recommended:"

// HasRecommended reports whether an item's text offers a recommended option: a line (after the
// first) that starts with RecommendedLabel. The first line is the item's lead, never an option.
func HasRecommended(text string) bool {
	lines := strings.Split(text, "\n")
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, RecommendedLabel) {
			return true
		}
	}
	return false
}

// Choices is the ONE rule for which decisions an item takes, in button order.
func Choices(it Item) []string {
	if HasRecommended(it.Text) {
		return []string{Current, Recommended, Reject}
	}
	return []string{Accept, Reject}
}

// Item is one lifecycle item. N is the display number (nil for Both series) and never part of
// the item's identity (Key is) nor of its hash. Changelog is the item's lineage
// (LIFECYCLE-DOCS-SPEC D1), in chronological order; its first entry is the concept entry.
type Item struct {
	Key       string   `json:"key"`
	Section   string   `json:"section"`
	N         *int     `json:"n"`
	Text      string   `json:"text"`
	Changelog []Change `json:"changelog"`
}

// Change is one changelog entry (LIFECYCLE-DOCS-SPEC D1): Date the day of the change; Source where
// it came from ("page" for an edit made here, or a pointer such as "Slack 2026-11-06"); Who the
// approver's name when the approver originated the change (empty for an edit of ours); Line one
// concise line; Hash the item's text hash AFTER this entry's edit (empty on an entry recording a
// change of meaning folded into a later text edit); Reapprove the only way back to Awaiting (D3).
// Callout (integrations SMART-SENDING-SPEC D11) is an optional one-line note the page shows under
// the item's state line -- only the LAST entry's, so the next entry clears it (whoever appends one
// repeats it while the matter is open). parse adds no rule for it.
type Change struct {
	Date      string `json:"date"`
	Source    string `json:"source"`
	Who       string `json:"who,omitempty"`
	Line      string `json:"line"`
	Hash      string `json:"hash,omitempty"`
	Reapprove bool   `json:"reapprove,omitempty"`
	Callout   string `json:"callout,omitempty"`
}

// ChangeView is one changelog entry as the GET shows it (D6): the hash is a file mechanism and is
// not sent. Callout is carried as stored (SMART-SENDING-SPEC D11); the page reads the last entry's.
type ChangeView struct {
	Date      string `json:"date"`
	Source    string `json:"source"`
	Who       string `json:"who,omitempty"`
	Line      string `json:"line"`
	Reapprove bool   `json:"reapprove"`
	Callout   string `json:"callout,omitempty"`
}

// conceptIndex is the position of the item's newest reapprove entry, or -1 when it has none (parse
// refuses that file).
func conceptIndex(it Item) int {
	for i := len(it.Changelog) - 1; i >= 0; i-- {
		if it.Changelog[i].Reapprove {
			return i
		}
	}
	return -1
}

// Concept is the item's concept entry: its newest `reapprove` entry (D3). Zero when it has none.
func Concept(it Item) Change {
	if i := conceptIndex(it); i >= 0 {
		return it.Changelog[i]
	}
	return Change{}
}

// Since is the concept entry's date -- the GET's `since` and the "Awaiting since" line (D3).
func Since(it Item) string {
	return Concept(it).Date
}

// Accepted is the item's accepted hashes, in lineage order: the concept entry's, then every later
// hash-bearing entry's (D3). A row carrying any of them is current.
func Accepted(it Item) []string {
	i := conceptIndex(it)
	if i < 0 {
		return nil
	}
	var out []string
	for _, c := range it.Changelog[i:] {
		if c.Hash != "" {
			out = append(out, c.Hash)
		}
	}
	return out
}

// ModifiedBy is D5's "approved as modified" marker: the approver who changed the concept off-page
// after the decision, and the day.
type ModifiedBy struct {
	Who  string `json:"who"`
	Date string `json:"date"`
}

// Modified is D5: for a decision row carrying `hash`, the newest changelog entry with a Who that
// sits AFTER the hash's position in the lineage, else nil. The position is the concept entry or
// the later hash-bearing entry whose hash equals `hash` (the earliest such position, so a text
// revert never hides an off-page modification); hash-less entries sit at their file position. A
// hash outside the accepted set has no position: nil.
func Modified(it Item, hash string) *ModifiedBy {
	start := conceptIndex(it)
	if start < 0 {
		return nil
	}
	pos := -1
	for i := start; i < len(it.Changelog); i++ {
		if it.Changelog[i].Hash != "" && it.Changelog[i].Hash == hash {
			pos = i
			break
		}
	}
	if pos < 0 {
		return nil
	}
	for i := len(it.Changelog) - 1; i > pos; i-- {
		if c := it.Changelog[i]; c.Who != "" {
			return &ModifiedBy{Who: c.Who, Date: c.Date}
		}
	}
	return nil
}

// Table is a rendered, never-approvable table above a section's items (USAGE-REENGAGEMENT-SPEC
// D8): no key, no hash, no decision. Every row has len(Columns) cells.
type Table struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
	Note    string     `json:"note"`
}

// Doc is lifecycle.json. Approver and Published are display only, never enforcement. Tables is
// keyed by section; only SectionSunsetPolicy may carry one (D8).
type Doc struct {
	Approver  string           `json:"approver"`
	Published string           `json:"published"`
	Items     []Item           `json:"items"`
	Tables    map[string]Table `json:"tables"`
}

// Hash is the lowercase hex sha256 of an item's text bytes (D2).
func Hash(text string) string {
	s := sha256.Sum256([]byte(text))
	return hex.EncodeToString(s[:])
}

// Lifecycle parses the embedded lifecycle.json and checks its shape: unique keys, a known
// section, a parseable since date. It is called once at boot; a bad file fails the boot.
func Lifecycle() (Doc, error) {
	return parse(lifecycleJSON)
}

func parse(b []byte) (Doc, error) {
	var d Doc
	if err := json.Unmarshal(b, &d); err != nil {
		return Doc{}, fmt.Errorf("decisions: lifecycle.json: %w", err)
	}
	seen := map[string]bool{}
	for _, it := range d.Items {
		if it.Key == "" || seen[it.Key] {
			return Doc{}, fmt.Errorf("decisions: lifecycle.json: empty or duplicate key %q", it.Key)
		}
		seen[it.Key] = true
		switch it.Section {
		case SectionNoPurchase, SectionPostPurchase, SectionBoth, SectionSunsetPolicy:
		default:
			return Doc{}, fmt.Errorf("decisions: lifecycle.json: %s: unknown section %q", it.Key, it.Section)
		}
		if it.Text == "" {
			return Doc{}, fmt.Errorf("decisions: lifecycle.json: %s: empty text", it.Key)
		}
		if err := checkChangelog(it); err != nil {
			return Doc{}, fmt.Errorf("decisions: lifecycle.json: %s: %w", it.Key, err)
		}
	}
	// D8: a table only on the section that renders one, and no ragged row.
	for sec, tb := range d.Tables {
		if sec != SectionSunsetPolicy {
			return Doc{}, fmt.Errorf("decisions: lifecycle.json: a table on section %q (only %q renders one)", sec, SectionSunsetPolicy)
		}
		if len(tb.Columns) == 0 {
			return Doc{}, fmt.Errorf("decisions: lifecycle.json: table %q has no columns", sec)
		}
		for i, r := range tb.Rows {
			if len(r) != len(tb.Columns) {
				return Doc{}, fmt.Errorf("decisions: lifecycle.json: table %q row %d has %d cells, want %d", sec, i, len(r), len(tb.Columns))
			}
		}
	}
	return d, nil
}

// checkChangelog is LIFECYCLE-DOCS-SPEC D2 (a) for one item: a non-empty changelog whose first
// entry is `reapprove` with a hash; every `reapprove` entry carries a hash; every hash is 64
// lowercase hex characters; every date parses and the dates never decrease; source and line are
// non-empty; and the newest hash-bearing entry's hash is the text's -- so a text edit with no new
// entry (or with only a hash-less one) fails the boot and every test.
func checkChangelog(it Item) error {
	if len(it.Changelog) == 0 {
		return fmt.Errorf("no changelog (the first entry is the concept entry: reapprove with the text hash)")
	}
	if first := it.Changelog[0]; !first.Reapprove || first.Hash == "" {
		return fmt.Errorf("the first changelog entry must be reapprove: true with a hash")
	}
	var prev time.Time
	newest := ""
	for i, c := range it.Changelog {
		d, err := time.Parse("2006-01-02", c.Date)
		if err != nil {
			return fmt.Errorf("changelog entry %d: date %q is not a date", i, c.Date)
		}
		if i > 0 && d.Before(prev) {
			return fmt.Errorf("changelog entry %d: date %s is before the previous entry's", i, c.Date)
		}
		prev = d
		if strings.TrimSpace(c.Source) == "" || strings.TrimSpace(c.Line) == "" {
			return fmt.Errorf("changelog entry %d: empty source or line", i)
		}
		if c.Reapprove && c.Hash == "" {
			return fmt.Errorf("changelog entry %d: a reapprove entry must carry a hash", i)
		}
		if c.Hash != "" {
			if !isHexHash(c.Hash) {
				return fmt.Errorf("changelog entry %d: hash %q is not 64 lowercase hex characters", i, c.Hash)
			}
			newest = c.Hash
		}
	}
	if h := Hash(it.Text); newest != h {
		return fmt.Errorf("text hash %s is not the newest changelog hash %s -- add a changelog entry carrying the new hash", h, newest)
	}
	return nil
}

func isHexHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// Find returns the item with the key.
func (d Doc) Find(key string) (Item, bool) {
	for _, it := range d.Items {
		if it.Key == key {
			return it, true
		}
	}
	return Item{}, false
}

// Row is one stored usage_decisions row.
type Row struct {
	ID        int64     `db:"id"`
	ItemKey   string    `db:"item_key"`
	TextHash  string    `db:"text_hash"`
	Decision  string    `db:"decision"`
	Note      *string   `db:"note"`
	UserID    int       `db:"user_id"`
	UserName  string    `db:"user_name"`
	CreatedAt time.Time `db:"created_at"`
}

// Entry is one row as the GET shows it. Voided means the row's hash is not one of the item's
// accepted hashes (the row predates a re-approval, LIFECYCLE-DOCS-SPEC D3). Modified (the current
// entry only, never a reject) is D5's "approved as modified" marker.
type Entry struct {
	Decision  string      `json:"decision"`
	Note      *string     `json:"note"`
	UserName  string      `json:"user_name"`
	CreatedAt time.Time   `json:"created_at"`
	Voided    bool        `json:"voided"`
	Modified  *ModifiedBy `json:"modified,omitempty"`
}

// ItemView is one item of the GET.
type ItemView struct {
	Key     string `json:"key"`
	Section string `json:"section"`
	N       *int   `json:"n"`
	Since   string `json:"since"`
	Text    string `json:"text"`
	Hash    string `json:"hash"`
	// Choices is the item's decisions in button order (Choices) -- the page renders them, never
	// re-deriving the rule.
	Choices []string `json:"choices"`
	Current *Entry   `json:"current"`
	History []Entry  `json:"history"`
	// Changelog is the item's lineage in file order, without hashes (LIFECYCLE-DOCS-SPEC D6).
	Changelog []ChangeView `json:"changelog"`
}

// View is the GET /api/usage/decisions body.
type View struct {
	Approver  string           `json:"approver"`
	Published string           `json:"published"`
	CanDecide bool             `json:"can_decide"`
	Items     []ItemView       `json:"items"`
	Tables    map[string]Table `json:"tables"`
}

// State is LIFECYCLE-DOCS-SPEC D3's current-state rule over one item's rows: the newest row (max
// id) is current when its text_hash is one of the item's accepted hashes (Accepted -- the concept
// entry's and every later edit's), else the item is Awaiting (nil). An ordinary text edit with its
// changelog entry therefore keeps the decision; a `reapprove` entry voids every earlier row. History
// is every row, newest first, each marked voided when its hash is not accepted.
func State(rows []Row, accepted []string) (*Entry, []Entry) {
	sorted := append([]Row(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID > sorted[j].ID })

	history := make([]Entry, 0, len(sorted))
	for _, r := range sorted {
		history = append(history, Entry{
			Decision:  r.Decision,
			Note:      r.Note,
			UserName:  r.UserName,
			CreatedAt: r.CreatedAt,
			Voided:    !slices.Contains(accepted, r.TextHash),
		})
	}
	if len(history) == 0 || history[0].Voided {
		return nil, history
	}
	cur := history[0]
	return &cur, history
}

// newestRow is the row with the max id, or nil.
func newestRow(rows []Row) *Row {
	var best *Row
	for i := range rows {
		if best == nil || rows[i].ID > best.ID {
			best = &rows[i]
		}
	}
	return best
}

// ItemOf builds one item's view from every stored row (rows of other keys are ignored).
func ItemOf(it Item, rows []Row) ItemView {
	var own []Row
	for _, r := range rows {
		if r.ItemKey == it.Key {
			own = append(own, r)
		}
	}
	h := Hash(it.Text)
	cur, hist := State(own, Accepted(it))
	// D5: a current approval (never a reject -- it is not an approval to modify) carries the
	// approver's off-page modification made after it.
	if cur != nil && cur.Decision != Reject {
		cur.Modified = Modified(it, newestRow(own).TextHash)
	}
	changelog := make([]ChangeView, 0, len(it.Changelog))
	for _, c := range it.Changelog {
		changelog = append(changelog, ChangeView{Date: c.Date, Source: c.Source, Who: c.Who, Line: c.Line, Reapprove: c.Reapprove, Callout: c.Callout})
	}
	return ItemView{Key: it.Key, Section: it.Section, N: it.N, Since: Since(it), Text: it.Text, Hash: h, Choices: Choices(it), Current: cur, History: hist, Changelog: changelog}
}

// Build is the whole GET body.
func Build(d Doc, rows []Row, canDecide bool) View {
	tables := d.Tables
	if tables == nil {
		tables = map[string]Table{}
	}
	v := View{Approver: d.Approver, Published: d.Published, CanDecide: canDecide, Items: make([]ItemView, 0, len(d.Items)), Tables: tables}
	for _, it := range d.Items {
		v.Items = append(v.Items, ItemOf(it, rows))
	}
	return v
}
