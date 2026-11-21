package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// Fork (promo series, integrations listmonk/PROMO-SERIES-SPEC D4/D5). The purchase feed and the
// Promo series group's live preview. The fork stores and counts; who is a purchaser is the SQL
// function promo_purchasers (schema.sql), and whether a campaign's fields apply is
// promo_applicable_tags -- nothing here re-implements either.
//
//   PUT /api/promo/purchases  subscribers:manage (the listmonk_sync user the order leg writes with
//       holds it -- no role edit). Body {"orders": [{"order_id", "email_hash", "ordered_at",
//       "promotions": [...], "skus": [...]}]}. Any refusal is a 400 and nothing is written; else every
//       (order, kind, value) is upserted in one statement. Idempotent; never deletes. {rows}.
//   GET /api/promo/purchases  subscribers:get_all -- {rows, orders, last_received_at, stale,
//       oldest_ordered_at}; ?order_id=<id> that order's rows. No address is stored, so none is served.
//   GET /api/campaigns/:id/promo-audience  campaigns:manage plus the campaign's list permission --
//       {orders, skipped, stale, last_received_at} for the stored or the given code / step / tags.

const (
	promoMaxOrders      = 1000
	promoMaxValues      = 200
	promoMaxOrderIDLen  = 128
	promoMaxValueLen    = 128
	promoMaxFutureAhead = 24 * time.Hour
)

var promoHashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

type promoOrderReq struct {
	OrderID    string   `json:"order_id"`
	EmailHash  string   `json:"email_hash"`
	OrderedAt  string   `json:"ordered_at"`
	Promotions []string `json:"promotions"`
	SKUs       []string `json:"skus"`
}

type promoPurchasesReq struct {
	Orders *[]promoOrderReq `json:"orders"`
}

// promoValueOK -- 1-128 characters after trimming, no control character.
func promoValueOK(v string) bool {
	n := utf8.RuneCountInString(v)
	if n < 1 || n > promoMaxValueLen {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// validatePromoPurchases checks a PUT body against now (D4) and returns the rows to upsert,
// de-duplicated by (order, kind, trimmed value), first kept. Any refusal returns an error and no rows.
func validatePromoPurchases(req promoPurchasesReq, now time.Time) ([]core.PromoPurchaseRow, error) {
	if req.Orders == nil {
		return nil, fmt.Errorf("orders is required")
	}
	orders := *req.Orders
	if len(orders) > promoMaxOrders {
		return nil, fmt.Errorf("too many orders (%d > %d)", len(orders), promoMaxOrders)
	}
	type key struct{ o, k, v string }
	seen := map[key]bool{}
	out := []core.PromoPurchaseRow{}
	for i, o := range orders {
		if n := utf8.RuneCountInString(o.OrderID); n < 1 || n > promoMaxOrderIDLen {
			return nil, fmt.Errorf("order %d: order_id must be 1-%d characters", i, promoMaxOrderIDLen)
		}
		if !promoHashRe.MatchString(o.EmailHash) {
			return nil, fmt.Errorf("order %d: email_hash must be 64 lower-case hex characters", i)
		}
		at, err := time.Parse(time.RFC3339Nano, o.OrderedAt)
		if err != nil {
			return nil, fmt.Errorf("order %d: ordered_at must be an RFC 3339 timestamp", i)
		}
		if at.After(now.Add(promoMaxFutureAhead)) {
			return nil, fmt.Errorf("order %d: ordered_at is more than 1 day in the future", i)
		}
		if len(o.Promotions)+len(o.SKUs) > promoMaxValues {
			return nil, fmt.Errorf("order %d: more than %d values", i, promoMaxValues)
		}
		for _, kv := range []struct {
			kind string
			vals []string
		}{{"promotion", o.Promotions}, {"sku", o.SKUs}} {
			for _, v := range kv.vals {
				v = strings.TrimSpace(v)
				if !promoValueOK(v) {
					return nil, fmt.Errorf("order %d: each %s must be 1-%d characters with no control character", i, kv.kind, promoMaxValueLen)
				}
				k := key{o.OrderID, kv.kind, v}
				if seen[k] {
					continue
				}
				seen[k] = true
				out = append(out, core.PromoPurchaseRow{OrderID: o.OrderID, Kind: kv.kind, Value: v, EmailHash: o.EmailHash, OrderedAt: at.UTC()})
			}
		}
	}
	return out, nil
}

// PutPromoPurchases upserts the feed (D4). Any refusal is a 400 and nothing is written.
func (a *App) PutPromoPurchases(c echo.Context) error {
	var req promoPurchasesReq
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "orders"))
	}
	rows, err := validatePromoPurchases(req, time.Now())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	n, err := a.core.UpsertPromoPurchases(rows)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{map[string]int64{"rows": n}})
}

// GetPromoPurchases is the summary, or with ?order_id= one order's rows (D4).
func (a *App) GetPromoPurchases(c echo.Context) error {
	if c.QueryParams().Has("order_id") {
		out, err := a.core.GetPromoPurchasesOrder(c.QueryParam("order_id"))
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, okResp{out})
	}
	out, err := a.core.GetPromoPurchasesSummary()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// promoPreviewParams turns the preview's optional code / step / tags query values into the JSONB
// texts the statement takes (nil = absent, use the stored value). Validated by D1's rule (a blank
// value is allowed: it stands for a cleared field); a malformed value is a 400.
func promoPreviewParams(q map[string][]string) (code, step, tags *string, err error) {
	has := func(k string) bool { _, ok := q[k]; return ok }
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	if has("code") {
		v := strings.TrimSpace(get("code"))
		if v != "" && models.PromoCodeProblem(v) != "" {
			return nil, nil, nil, fmt.Errorf("code")
		}
		b, _ := json.Marshal(v)
		s := string(b)
		code = &s
	}
	if has("step") {
		var s string
		switch strings.TrimSpace(get("step")) {
		case "":
			s = "null"
		case "1", "2", "3":
			s = strings.TrimSpace(get("step"))
		default:
			return nil, nil, nil, fmt.Errorf("step")
		}
		step = &s
	}
	if has("tags") {
		list, ok := models.ParsePromoTagsParam(get("tags"))
		if !ok {
			return nil, nil, nil, fmt.Errorf("tags")
		}
		b, _ := json.Marshal(list)
		s := string(b)
		tags = &s
	}
	return code, step, tags, nil
}

// GetCampaignPromoAudience is the Promo series group's live preview (D5): {orders, skipped, stale,
// last_received_at}. campaigns:manage (the route) plus the campaign's list permission (the campaign
// GET's list rule, for a manager). Read-only; counts only.
func (a *App) GetCampaignPromoAudience(c echo.Context) error {
	id := getID(c)

	if err := a.checkCampaignPerm(auth.PermTypeManage, id, c); err != nil {
		return err
	}

	code, step, tags, err := promoPreviewParams(c.QueryParams())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", err.Error()))
	}

	out, err := a.core.CampaignPromoAudience(id, code, step, tags)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}
