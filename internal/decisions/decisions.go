// Package decisions is the fork's home for Marketing's lifecycle-series decision record on
// Brands > Usage (integrations USAGE-DECISIONS-SPEC D1/D2/D4). The item texts live in the embedded
// lifecycle.json -- the fork's text home for them, word for word the spec's §0 -- and each
// decision row in the append-only usage_decisions table (internal/migrations/v6.2.29.go).
//
// The fork STORES these; nothing in the integrations repo computes or reads them. An item's hash
// is the lowercase hex sha256 of its text bytes: editing a text voids the decision on record
// without any write, and the literal hash pin in decisions_test.go is the acknowledgement ritual
// (bump the item's `since` and its pin together).
package decisions

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

//go:embed lifecycle.json
var lifecycleJSON []byte

// Sections, in display order.
const (
	SectionNoPurchase   = "no-purchase"
	SectionPostPurchase = "post-purchase"
	SectionBoth         = "both"
)

// The two decisions a row may carry.
const (
	Accept = "accept"
	Reject = "reject"
)

// Item is one lifecycle item. N is the display number (nil for Both series) and never part of
// the item's identity (Key is) nor of its hash.
type Item struct {
	Key     string `json:"key"`
	Section string `json:"section"`
	N       *int   `json:"n"`
	Since   string `json:"since"`
	Text    string `json:"text"`
}

// Doc is lifecycle.json. Approver and Published are display only, never enforcement.
type Doc struct {
	Approver  string `json:"approver"`
	Published string `json:"published"`
	Items     []Item `json:"items"`
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
		case SectionNoPurchase, SectionPostPurchase, SectionBoth:
		default:
			return Doc{}, fmt.Errorf("decisions: lifecycle.json: %s: unknown section %q", it.Key, it.Section)
		}
		if _, err := time.Parse("2006-01-02", it.Since); err != nil {
			return Doc{}, fmt.Errorf("decisions: lifecycle.json: %s: since %q is not a date", it.Key, it.Since)
		}
		if it.Text == "" {
			return Doc{}, fmt.Errorf("decisions: lifecycle.json: %s: empty text", it.Key)
		}
	}
	return d, nil
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

// Entry is one row as the GET shows it. Voided means the row's hash is not the item's current
// hash (its text changed after the decision).
type Entry struct {
	Decision  string    `json:"decision"`
	Note      *string   `json:"note"`
	UserName  string    `json:"user_name"`
	CreatedAt time.Time `json:"created_at"`
	Voided    bool      `json:"voided"`
}

// ItemView is one item of the GET.
type ItemView struct {
	Key     string  `json:"key"`
	Section string  `json:"section"`
	N       *int    `json:"n"`
	Since   string  `json:"since"`
	Text    string  `json:"text"`
	Hash    string  `json:"hash"`
	Current *Entry  `json:"current"`
	History []Entry `json:"history"`
}

// View is the GET /api/usage/decisions body.
type View struct {
	Approver  string     `json:"approver"`
	Published string     `json:"published"`
	CanDecide bool       `json:"can_decide"`
	Items     []ItemView `json:"items"`
}

// State is D4's current-state rule over one item's rows: the newest row (max id) counts only when
// its text_hash equals the item's current hash, else the item is Awaiting (nil). A text edit
// therefore voids the decision without any write; reverting the text revives that row. History is
// every row, newest first, each marked voided when its hash is not the current one.
func State(rows []Row, hash string) (*Entry, []Entry) {
	sorted := append([]Row(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID > sorted[j].ID })

	history := make([]Entry, 0, len(sorted))
	for _, r := range sorted {
		history = append(history, Entry{
			Decision:  r.Decision,
			Note:      r.Note,
			UserName:  r.UserName,
			CreatedAt: r.CreatedAt,
			Voided:    r.TextHash != hash,
		})
	}
	if len(history) == 0 || history[0].Voided {
		return nil, history
	}
	cur := history[0]
	return &cur, history
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
	cur, hist := State(own, h)
	return ItemView{Key: it.Key, Section: it.Section, N: it.N, Since: it.Since, Text: it.Text, Hash: h, Current: cur, History: hist}
}

// Build is the whole GET body.
func Build(d Doc, rows []Row, canDecide bool) View {
	v := View{Approver: d.Approver, Published: d.Published, CanDecide: canDecide, Items: make([]ItemView, 0, len(d.Items))}
	for _, it := range d.Items {
		v.Items = append(v.Items, ItemOf(it, rows))
	}
	return v
}
