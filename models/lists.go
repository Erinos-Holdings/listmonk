package models

import (
	"encoding/json"
	"fmt"

	"github.com/lib/pq"
	null "gopkg.in/volatiletech/null.v6"
)

const (
	ListTypePrivate    = "private"
	ListTypePublic     = "public"
	ListOptinSingle    = "single"
	ListOptinDouble    = "double"
	ListStatusActive   = "active"
	ListStatusArchived = "archived"
)

// List represents a mailing list.
type List struct {
	Base

	UUID             string         `db:"uuid" json:"uuid"`
	Name             string         `db:"name" json:"name"`
	Type             string         `db:"type" json:"type"`
	Optin            string         `db:"optin" json:"optin"`
	Status           string         `db:"status" json:"status"`
	Tags             pq.StringArray `db:"tags" json:"tags"`
	Description      string         `db:"description" json:"description"`
	SubscriberCount  int            `db:"subscriber_count" json:"subscriber_count"`
	SubscriberCounts StringIntMap   `db:"subscriber_statuses" json:"subscriber_statuses"`
	SubscriberID     int            `db:"subscriber_id" json:"-"`

	// Fork (list grid, LIST-GRID-SPEC D5/D9). SubscriberGrid is the segment x send-language
	// grid (query-lists only); the *_count fields are its "all" row, selected as columns so
	// the Lists page can sort on them server-side.
	SubscriberGrid    ListGrid `db:"subscriber_grid" json:"subscriber_grid,omitempty"`
	ActiveCount       int      `db:"active_count" json:"-"`
	HeldCount         int      `db:"held_count" json:"-"`
	UnsubscribedCount int      `db:"unsubscribed_count" json:"-"`
	PendingCount      int      `db:"pending_count" json:"-"`
	BlockedCount      int      `db:"blocked_count" json:"-"`

	// This is only relevant when querying the lists of a subscriber.
	SubscriptionStatus    string    `db:"subscription_status" json:"subscription_status,omitempty"`
	SubscriptionCreatedAt null.Time `db:"subscription_created_at" json:"subscription_created_at,omitempty"`
	SubscriptionUpdatedAt null.Time `db:"subscription_updated_at" json:"subscription_updated_at,omitempty"`

	// Pseudofield for getting the total number of subscribers
	// in searches and queries.
	Total int `db:"total" json:"-"`
}

// Fork (list grid, integrations LIST-GRID-SPEC D1/D2/D6). The five segments partition every
// subscriber_lists row and the language values name the rows of the Lists-page grid. The
// DEFINITIONS live in SQL (subscription_segment(), subscriber_lang(), send_lang() -- v6.2.11);
// these are only the allowlists for the segment= and lang= filter params, so no free text
// reaches the server-authored predicate.
const (
	SegmentActive       = "active"
	SegmentHeld         = "held"
	SegmentUnsubscribed = "unsubscribed"
	SegmentPending      = "pending"
	SegmentBlocked      = "blocked"

	// SubscriberLangNone is the storage bucket of a subscriber with no usable attribs.lang,
	// SubscriberLangOther of one whose value is outside CampaignLangs.
	SubscriberLangNone  = "none"
	SubscriberLangOther = "other"
)

// SubscriberFilter is the server-authored segment=/lang= filter on the subscriber query and
// by-query endpoints. Zero value = no filter. See core.subscriberFilterExp.
type SubscriberFilter struct {
	Segment string
	Lang    string
}

// Empty reports whether no filter is set.
func (f SubscriberFilter) Empty() bool { return f.Segment == "" && f.Lang == "" }

// Segments is the closed set of the segment= filter, in column order.
var Segments = []string{SegmentActive, SegmentHeld, SegmentUnsubscribed, SegmentPending, SegmentBlocked}

// IsSegment reports whether s is a valid segment= value.
func IsSegment(s string) bool {
	for _, v := range Segments {
		if v == s {
			return true
		}
	}
	return false
}

// IsSubscriberLangFilter reports whether s is a valid lang= value: a CampaignLangs code (a
// SEND language -- en includes the none bucket), none, or other.
func IsSubscriberLangFilter(s string) bool {
	return s == SubscriberLangNone || s == SubscriberLangOther || IsCampaignLang(s)
}

// ListGridRow is one row of the grid. Total is the sum of the five segments.
type ListGridRow struct {
	Active       int `json:"active"`
	Held         int `json:"held"`
	Unsubscribed int `json:"unsubscribed"`
	Pending      int `json:"pending"`
	Blocked      int `json:"blocked"`
	Total        int `json:"total"`
}

// ListGrid is keyed all (always), the send languages and other (where total > 0; en already
// includes none), and none (where total > 0) as an informational subset of en.
type ListGrid map[string]ListGridRow

// Scan implements the sql.Scanner interface.
func (g *ListGrid) Scan(src any) error {
	if src == nil {
		*g = ListGrid{}
		return nil
	}
	if data, ok := src.([]byte); ok {
		return json.Unmarshal(data, g)
	}
	return fmt.Errorf("could not not decode type %T -> %T", src, g)
}
