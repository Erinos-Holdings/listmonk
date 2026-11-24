package core

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"gopkg.in/volatiletech/null.v6"
)

// Fork (SMS Admin, integrations listmonk/SMS-LINKS-ADMIN-SPEC D8-D10). The Promo codes tab's store:
// sms_promo_codes marks which promotion values are SMS codes; the report aggregates the purchase
// feed (promo_purchases) per marked code. The fork aggregates; it decides nothing.

// SMSPromoRow is one marked code with its feed counts (D9).
type SMSPromoRow struct {
	ID           int         `db:"id" json:"id"`
	Code         string      `db:"code" json:"code"`
	Brand        null.String `db:"brand" json:"brand"`
	Orders       int         `db:"orders" json:"orders"`
	Buyers       int         `db:"buyers" json:"buyers"`
	FirstOrderAt null.Time   `db:"first_order_at" json:"first_order_at"`
	LastOrderAt  null.Time   `db:"last_order_at" json:"last_order_at"`
	CreatedBy    string      `db:"created_by" json:"created_by"`
	CreatedAt    time.Time   `db:"created_at" json:"created_at"`
}

// SMSPromoFeed is the feed's extent and freshness (D9).
type SMSPromoFeed struct {
	OldestOrderedAt null.Time `db:"oldest_ordered_at" json:"oldest_ordered_at"`
	LastReceivedAt  null.Time `db:"last_received_at" json:"last_received_at"`
	Stale           bool      `db:"stale" json:"stale"`
}

// SMSPromoKnown is one unmarked promotion value of the feed, in its most frequent spelling (D9).
type SMSPromoKnown struct {
	Value  string `db:"value" json:"value"`
	Orders int    `db:"orders" json:"orders"`
}

// SMSPromoReport is GET /api/sms/promo-codes.
type SMSPromoReport struct {
	Rows  []SMSPromoRow   `json:"rows"`
	Feed  SMSPromoFeed    `json:"feed"`
	Known []SMSPromoKnown `json:"known"`
}

// SMSPromoCode is a marking row as written (POST /api/sms/promo-codes).
type SMSPromoCode struct {
	ID        int         `db:"id" json:"id"`
	Code      string      `db:"code" json:"code"`
	Brand     null.String `db:"brand" json:"brand"`
	CreatedBy string      `db:"created_by" json:"created_by"`
	CreatedAt time.Time   `db:"created_at" json:"created_at"`
}

// GetSMSPromoReport runs the three named statements (rows, feed, known).
func (c *Core) GetSMSPromoReport() (SMSPromoReport, error) {
	out := SMSPromoReport{Rows: []SMSPromoRow{}, Known: []SMSPromoKnown{}}
	fail := func(err error) (SMSPromoReport, error) {
		c.log.Printf("error fetching sms promo report: %v", err)
		return SMSPromoReport{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "sms_promo_codes", "error", pqErrMsg(err)))
	}
	if err := c.q.GetSMSPromoRows.Select(&out.Rows); err != nil {
		return fail(err)
	}
	if err := c.q.GetSMSPromoFeed.Get(&out.Feed); err != nil {
		return fail(err)
	}
	if err := c.q.GetSMSPromoKnown.Select(&out.Known); err != nil {
		return fail(err)
	}
	return out, nil
}

// InsertSMSPromoCode marks code (already trimmed and validated by the handler). brand "" is NULL.
// A case-insensitive duplicate is 409 sms.codeExists; an unknown brand is 400.
func (c *Core) InsertSMSPromoCode(code, brand, createdBy string) (SMSPromoCode, error) {
	var out SMSPromoCode
	if err := c.q.InsertSMSPromoCode.Get(&out, code, brand, createdBy); err != nil {
		var pqe *pq.Error
		if errors.As(err, &pqe) {
			switch pqe.Code {
			case "23505":
				return out, echo.NewHTTPError(http.StatusConflict, c.i18n.T("sms.codeExists"))
			case "23503":
				return out, echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("sms.unknownBrand"))
			}
		}
		c.log.Printf("error inserting sms promo code: %v", err)
		return out, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "sms_promo_codes", "error", pqErrMsg(err)))
	}
	return out, nil
}

// DeleteSMSPromoCode unmarks id; 404 when absent. The feed is never touched.
func (c *Core) DeleteSMSPromoCode(id int) error {
	res, err := c.q.DeleteSMSPromoCode.Exec(id)
	if err != nil {
		c.log.Printf("error deleting sms promo code: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorDeleting", "name", "sms_promo_codes", "error", pqErrMsg(err)))
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return echo.NewHTTPError(http.StatusNotFound, c.i18n.Ts("globals.messages.notFound", "name", "sms_promo_codes"))
	}
	return nil
}
