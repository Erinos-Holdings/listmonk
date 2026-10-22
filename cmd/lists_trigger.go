package main

// Fork (list join triggers) -- integrations LIST-JOIN-TRIGGER-SPEC D1-D6. A list may carry a join
// trigger ({"kind":"first_order"}, {"kind":"skus","skus":[...]} or -- NO-PURCHASE-STEPS-SPEC D1 --
// {"kind":"no_purchase","days":N}) and a transactional flag. The
// fork validates SHAPE only and stores; an external reconciler reads the lists, gives the trigger
// its meaning and writes its state back through PUT /api/lists/:id/trigger-sync. The fork stores
// and renders, never computes.
//
// The rules on PUT /api/lists/:id (UpdateList):
//   - D5 -- a transactional value that DIFFERS from the stored one needs lists:transactional, and
//     so does a join_trigger or brand change on a list whose STORED transactional is true. Both
//     refusals are 403 before anything on the request is applied. A round-tripped stored value
//     is not a change and is ignored.
//   - D3 -- join_trigger is tri-state (absent keeps, null clears, an object is validated and
//     normalized); transactional is absent (nil, also a JSON null) or a boolean. Validation runs
//     before the main update, so a 400 leaves the whole list unchanged.
//   - D2 -- a non-null RESULTING join_trigger needs a RESULTING brand.
//   - D4 -- the write is the one update-list-trigger statement, run after update-list, which moves
//     the stamps only on a real change and nulls trigger_sync only when join_trigger changed.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

const (
	// maxJoinTriggerSkus and maxJoinTriggerSkuLen bound a skus trigger (D1).
	maxJoinTriggerSkus   = 200
	maxJoinTriggerSkuLen = 100

	// maxTriggerSyncBody bounds the write-back body; maxTriggerSyncError its error text (D6).
	maxTriggerSyncBody  = 4096
	maxTriggerSyncError = 500

	joinTriggerFirstOrder = "first_order"
	joinTriggerSkus       = "skus"
	// joinTriggerNoPurchase is the "still deciding" step (integrations NO-PURCHASE-STEPS-SPEC D1):
	// {"kind":"no_purchase","days":N}, N an integer 1-365. Shape only -- the reconciler owns meaning.
	joinTriggerNoPurchase = "no_purchase"

	// minJoinTriggerDays and maxJoinTriggerDays bound a no_purchase trigger's days.
	minJoinTriggerDays = 1
	maxJoinTriggerDays = 365
)

// joinTriggerKindErr is the refusal for a missing or unknown kind.
var joinTriggerKindErr = fmt.Errorf("join_trigger.kind must be %q, %q or %q", joinTriggerFirstOrder, joinTriggerSkus, joinTriggerNoPurchase)

// normalizedJoinTrigger is the stored form; field order fixes the stored key order.
type normalizedJoinTrigger struct {
	Kind string   `json:"kind"`
	Skus []string `json:"skus,omitempty"`
	Days int      `json:"days,omitempty"`
}

// validateJoinTrigger checks a non-null join_trigger object and returns its normalized JSON: kind
// first_order with no other key; kind skus with a skus array of strings -- trimmed, lower-cased,
// de-duplicated in order, none empty, each at most 100 characters, at most 200 of them; or kind
// no_purchase with days a JSON integer 1-365 and no other key. Anything else is refused.
func validateJoinTrigger(raw []byte) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil || doc == nil {
		return nil, fmt.Errorf("join_trigger must be a JSON object")
	}
	var kind string
	if k, ok := doc["kind"]; !ok || json.Unmarshal(k, &kind) != nil {
		return nil, joinTriggerKindErr
	}
	switch kind {
	case joinTriggerNoPurchase:
		for k := range doc {
			if k != "kind" && k != "days" {
				return nil, fmt.Errorf("join_trigger has an unknown key: %s", k)
			}
		}
		days, ok := jsonInt(doc["days"])
		if !ok || days < minJoinTriggerDays || days > maxJoinTriggerDays {
			return nil, fmt.Errorf("join_trigger.days must be a whole number from %d to %d", minJoinTriggerDays, maxJoinTriggerDays)
		}
		return json.Marshal(normalizedJoinTrigger{Kind: kind, Days: days})

	case joinTriggerFirstOrder:
		for k := range doc {
			if k != "kind" {
				return nil, fmt.Errorf("join_trigger has an unknown key: %s", k)
			}
		}
		return json.Marshal(normalizedJoinTrigger{Kind: kind})

	case joinTriggerSkus:
		for k := range doc {
			if k != "kind" && k != "skus" {
				return nil, fmt.Errorf("join_trigger has an unknown key: %s", k)
			}
		}
		var in []any
		if s, ok := doc["skus"]; !ok || json.Unmarshal(s, &in) != nil || in == nil {
			return nil, fmt.Errorf("join_trigger.skus must be an array of SKUs")
		}
		seen := map[string]bool{}
		out := make([]string, 0, len(in))
		for _, v := range in {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("join_trigger.skus must hold strings only")
			}
			s = strings.ToLower(strings.TrimSpace(s))
			if s == "" {
				return nil, fmt.Errorf("join_trigger.skus holds an empty SKU")
			}
			if utf8.RuneCountInString(s) > maxJoinTriggerSkuLen {
				return nil, fmt.Errorf("a SKU is longer than %d characters", maxJoinTriggerSkuLen)
			}
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("join_trigger.skus must hold at least one SKU")
		}
		if len(out) > maxJoinTriggerSkus {
			return nil, fmt.Errorf("join_trigger.skus holds more than %d SKUs", maxJoinTriggerSkus)
		}
		return json.Marshal(normalizedJoinTrigger{Kind: kind, Skus: out})
	}
	return nil, joinTriggerKindErr
}

// jsonInt reads a JSON number literal written as a plain integer (digits, optional leading
// minus): 7 passes; 7.0, 7e0, "7", null and an absent value do not.
func jsonInt(raw json.RawMessage) (int, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || len(s) > 6 {
		return 0, false
	}
	n := 0
	for i, r := range s {
		if i == 0 && r == '-' && len(s) > 1 {
			continue
		}
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	if s[0] == '-' {
		n = -n
	}
	return n, true
}

// sameJSON compares two JSON documents by value (the stored JSONB comes back re-formatted). An
// empty side is SQL NULL.
func sameJSON(a, b []byte) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == 0 && len(b) == 0
	}
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return bytes.Equal(a, b)
	}
	return reflect.DeepEqual(x, y)
}

// listTriggerCarried reports whether the request carries any field the trigger rules read.
func listTriggerCarried(r listReq) bool {
	return r.JoinTrigger != nil || r.Transactional != nil
}

// listTriggerPlan is the validated outcome of a request's trigger fields.
type listTriggerPlan struct {
	write         bool   // run update-list-trigger after update-list
	joinTrigger   []byte // the resulting join_trigger, nil = NULL
	transactional bool   // the resulting transactional
}

// checkListTriggerPerm is the D5 check, run FIRST on an update: 403 when the request changes
// transactional, or changes join_trigger or the brand of a list whose stored transactional is
// true, without lists:transactional. A join_trigger that does not parse counts as a change.
func (a *App) checkListTriggerPerm(user auth.User, r listReq, cur models.List) error {
	if user.HasPerm(auth.PermListTransactional) {
		return nil
	}
	if r.Transactional != nil && *r.Transactional != cur.Transactional {
		return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("lists.transactionalLocked"))
	}
	if !cur.Transactional {
		return nil
	}
	brandChanged := r.Brand != nil && strings.TrimSpace(*r.Brand) != cur.Brand
	triggerChanged := false
	if r.JoinTrigger != nil {
		var next []byte
		if !isJSONNull(r.JoinTrigger) {
			n, err := validateJoinTrigger(r.JoinTrigger)
			if err != nil {
				triggerChanged = true
			}
			next = n
		}
		triggerChanged = triggerChanged || !sameJSON(next, cur.JoinTrigger)
	}
	if brandChanged || triggerChanged {
		return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("lists.transactionalLocked"))
	}
	return nil
}

// planListTrigger validates the request's trigger fields (D3) and the D2 brand rule against the
// RESULTING brand. Run before update-list; a 400 here leaves the list unchanged.
func (a *App) planListTrigger(r listReq, cur models.List) (listTriggerPlan, error) {
	p := listTriggerPlan{joinTrigger: []byte(cur.JoinTrigger), transactional: cur.Transactional}
	if len(p.joinTrigger) == 0 {
		p.joinTrigger = nil
	}
	if r.JoinTrigger != nil {
		p.write = true
		if isJSONNull(r.JoinTrigger) {
			p.joinTrigger = nil
		} else {
			n, err := validateJoinTrigger(r.JoinTrigger)
			if err != nil {
				return p, echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("lists.triggerInvalid", "reason", err.Error()))
			}
			p.joinTrigger = n
		}
	}
	if r.Transactional != nil {
		p.write = true
		p.transactional = *r.Transactional
	}

	brand := cur.Brand
	if r.Brand != nil {
		brand = strings.TrimSpace(*r.Brand)
	}
	if p.joinTrigger != nil && brand == "" {
		return p, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.triggerNeedsBrand"))
	}
	return p, nil
}

func isJSONNull(raw []byte) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// validateTriggerSync is the shape check on the reconciler's write-back (D6): exactly synced_at
// (RFC 3339, required), state "live" with a non-empty string slug, or state "invalid" with a
// non-empty error of at most 500 characters -- and no other key.
func validateTriggerSync(body []byte) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil || doc == nil {
		return fmt.Errorf("trigger_sync must be a JSON object")
	}
	if dec.More() {
		return fmt.Errorf("trigger_sync must be one JSON object")
	}
	for k := range doc {
		switch k {
		case "synced_at", "state", "slug", "error":
		default:
			return fmt.Errorf("trigger_sync has an unknown key: %s", k)
		}
	}
	s, ok := doc["synced_at"].(string)
	if !ok {
		return fmt.Errorf("trigger_sync.synced_at must be an RFC 3339 timestamp")
	}
	if _, err := time.Parse(time.RFC3339, s); err != nil {
		return fmt.Errorf("trigger_sync.synced_at must be an RFC 3339 timestamp")
	}
	_, hasSlug := doc["slug"]
	_, hasErr := doc["error"]
	switch doc["state"] {
	case "live":
		slug, ok := doc["slug"].(string)
		if !ok || strings.TrimSpace(slug) == "" {
			return fmt.Errorf("trigger_sync.slug is required with state live")
		}
		if hasErr {
			return fmt.Errorf("trigger_sync.error is not allowed with state live")
		}
	case "invalid":
		e, ok := doc["error"].(string)
		if !ok || strings.TrimSpace(e) == "" {
			return fmt.Errorf("trigger_sync.error is required with state invalid")
		}
		if utf8.RuneCountInString(e) > maxTriggerSyncError {
			return fmt.Errorf("trigger_sync.error is longer than %d characters", maxTriggerSyncError)
		}
		if hasSlug {
			return fmt.Errorf("trigger_sync.slug is not allowed with state invalid")
		}
	default:
		return fmt.Errorf("trigger_sync.state must be \"live\" or \"invalid\"")
	}
	return nil
}

// PutListTriggerSync handles PUT /api/lists/:id/trigger-sync (lists:manage_all -- the
// reconciler's user, never a per-list role). Stored verbatim after the shape check; updated_at is
// left alone (not an edit of the list).
func (a *App) PutListTriggerSync(c echo.Context) error {
	user := auth.GetUser(c)
	if !user.HasPerm(auth.PermListManageAll) {
		return echo.NewHTTPError(http.StatusForbidden, fmt.Sprintf("permission denied: %s", auth.PermListManageAll))
	}
	id := getID(c)

	body, err := io.ReadAll(io.LimitReader(c.Request().Body, maxTriggerSyncBody+1))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(body) > maxTriggerSyncBody {
		return echo.NewHTTPError(http.StatusBadRequest, "trigger_sync body too large")
	}
	if err := validateTriggerSync(body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := a.core.UpdateListTriggerSync(id, bytes.TrimSpace(body)); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{true})
}
