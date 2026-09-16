package models

import (
	"errors"
	"strings"
)

// Fork (holds, integrations SUNSET-SPEC D1/D3). A hold is a subscriber_lists row that is
// 'unsubscribed' and carries meta.hold, on a subscriber who is not blocklisted. The hold
// object names the SYSTEM that wrote it (source) and why (reason, rule) — never a person,
// because the privacy export shows it to the data subject.

// HoldReasons are the accepted values of hold.reason.
var HoldReasons = []string{"never-engaged", "deliverability", "manual"}

// HoldSkip is one id the hold action did not write, and why: not_on_list,
// updated_after_bound, opt_out (unsubscribed without a hold), already_held, or the row's
// status (confirmed / unconfirmed, relabel mode only).
type HoldSkip struct {
	ID  int    `json:"id" db:"id"`
	Why string `json:"why" db:"why"`
}

// HoldResult is the response of PUT /api/subscribers/lists {action:"hold"}.
type HoldResult struct {
	Held    int        `json:"held"`
	Skipped []HoldSkip `json:"skipped"`
}

// ValidateHold checks a caller-supplied hold object and returns the copy to store. at is
// always server-stamped, so a supplied one is dropped.
func ValidateHold(h map[string]any) (map[string]any, error) {
	if len(h) == 0 {
		return nil, errors.New("hold is required")
	}
	reason, _ := h["reason"].(string)
	ok := false
	for _, r := range HoldReasons {
		if reason == r {
			ok = true
			break
		}
	}
	if !ok {
		return nil, errors.New("hold.reason must be one of " + strings.Join(HoldReasons, ", "))
	}
	if src, _ := h["source"].(string); strings.TrimSpace(src) == "" {
		return nil, errors.New("hold.source is required")
	}

	out := make(map[string]any, len(h))
	for k, v := range h {
		if k == "at" {
			continue
		}
		out[k] = v
	}
	return out, nil
}
