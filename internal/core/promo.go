package core

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"gopkg.in/volatiletech/null.v6"
)

// Fork (promo series, integrations listmonk/PROMO-SERIES-SPEC D4). The purchase feed's store: the
// integrations order leg PUTs every paid order's promotion values and SKUs; promo_purchasers
// (schema.sql) reads them at the claim. The fork stores and counts; it decides no purchaser here.

// PromoPurchaseRow is one feed row to upsert (already validated by the handler).
type PromoPurchaseRow struct {
	OrderID   string
	Kind      string
	Value     string
	EmailHash string
	OrderedAt time.Time
}

// UpsertPromoPurchases writes every row in one statement (one transaction): ON CONFLICT updates the
// hash, the order time, value_lc and received_at. Idempotent; never deletes. Returns the rows written.
func (c *Core) UpsertPromoPurchases(rows []PromoPurchaseRow) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	var (
		ids   = make([]string, len(rows))
		kinds = make([]string, len(rows))
		vals  = make([]string, len(rows))
		hs    = make([]string, len(rows))
		ats   = make([]time.Time, len(rows))
	)
	for i, r := range rows {
		ids[i], kinds[i], vals[i], hs[i], ats[i] = r.OrderID, r.Kind, r.Value, r.EmailHash, r.OrderedAt
	}
	res, err := c.q.UpsertPromoPurchases.Exec(pq.Array(ids), pq.Array(kinds), pq.Array(vals), pq.Array(hs), pq.Array(ats))
	if err != nil {
		c.log.Printf("error upserting promo_purchases: %v", err)
		return 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "promo_purchases", "error", pqErrMsg(err)))
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// PromoPurchasesSummary is the freshness read and the dark proof (no parameter).
type PromoPurchasesSummary struct {
	Rows            int       `db:"rows" json:"rows"`
	Orders          int       `db:"orders" json:"orders"`
	LastReceivedAt  null.Time `db:"last_received_at" json:"last_received_at"`
	Stale           bool      `db:"stale" json:"stale"`
	OldestOrderedAt null.Time `db:"oldest_ordered_at" json:"oldest_ordered_at"`
}

// GetPromoPurchasesSummary returns {rows, orders, last_received_at, stale, oldest_ordered_at}.
func (c *Core) GetPromoPurchasesSummary() (PromoPurchasesSummary, error) {
	var out PromoPurchasesSummary
	if err := c.q.GetPromoPurchasesSummary.Get(&out); err != nil {
		c.log.Printf("error fetching promo_purchases summary: %v", err)
		return out, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "promo_purchases", "error", pqErrMsg(err)))
	}
	return out, nil
}

// PromoPurchaseOut is one row of the ?order_id= read (the backfill's canary read-back).
type PromoPurchaseOut struct {
	Kind       string    `db:"kind" json:"kind"`
	Value      string    `db:"value" json:"value"`
	EmailHash  string    `db:"email_hash" json:"email_hash"`
	OrderedAt  time.Time `db:"ordered_at" json:"ordered_at"`
	ReceivedAt time.Time `db:"received_at" json:"received_at"`
}

// GetPromoPurchasesOrder returns one order's rows ([] when none).
func (c *Core) GetPromoPurchasesOrder(orderID string) ([]PromoPurchaseOut, error) {
	out := []PromoPurchaseOut{}
	if err := c.q.GetPromoPurchasesOrder.Select(&out, orderID); err != nil {
		c.log.Printf("error fetching promo_purchases order: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "promo_purchases", "error", pqErrMsg(err)))
	}
	return out, nil
}
