package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// Fork (smart sending look-ahead, integrations listmonk/SMART-SENDING-LOOKAHEAD-SPEC D6). The
// forecast feed of the look-ahead half: the integrations NoPurchaseSweep (source "series") and
// ListmonkSunset (source "sunset") Lambdas compute the joins their NEXT run will make and PUT them
// here; brand_journey_due_24h reads them at the claim and the preview. The fork stores and counts;
// it computes no row.
//
//   PUT /api/journeys/due  subscribers:manage (both Lambda users hold it -- no role edit). Body
//       {"source": "series"|"sunset", "expected_at": "<RFC 3339>", "rows": [[list_id, subscriber_id], ...]}.
//       Replaces exactly (source, expected_at) and drops rows past the retention, in one
//       transaction. Idempotent.
//   GET /api/journeys/due  subscribers:get_all -- {"lookahead": <the switch>, "brand_hold": <the
//       brand-wide hold switch, integrations HOLD-SCOPE-SPEC D2>, "feeds": [...]}, the
//       dark proof and the feed-freshness read. No subscriber ids.

const (
	// journeyDueMaxRows caps one PUT body (D6).
	journeyDueMaxRows = 500000
	// journeyDueMaxAhead is how far ahead a feed's expected_at may be (D6 -- (now, now + 48 h]).
	journeyDueMaxAhead = 48 * time.Hour
)

var journeyDueSources = map[string]bool{"series": true, "sunset": true}

// journeyDueReq is the PUT body. Rows is a pointer so an absent rows is told apart from [].
type journeyDueReq struct {
	Source     string     `json:"source"`
	ExpectedAt string     `json:"expected_at"`
	Rows       *[][]int64 `json:"rows"`
}

// validateJourneyDue checks a PUT body against now (D6): a known source, an RFC 3339 expected_at in
// (now, now + 48 h], rows present (an empty list is valid -- it removes the feed), at most 500,000
// of them, each exactly [list_id, subscriber_id] with both ids positive INT values. It returns the
// expected time and the two parallel id columns.
func validateJourneyDue(req journeyDueReq, now time.Time) (time.Time, []int64, []int64, error) {
	if !journeyDueSources[req.Source] {
		return time.Time{}, nil, nil, fmt.Errorf("source must be series or sunset")
	}
	at, err := time.Parse(time.RFC3339Nano, req.ExpectedAt)
	if err != nil {
		return time.Time{}, nil, nil, fmt.Errorf("expected_at must be an RFC 3339 timestamp")
	}
	if !at.After(now) || at.After(now.Add(journeyDueMaxAhead)) {
		return time.Time{}, nil, nil, fmt.Errorf("expected_at must be after now and at most 48 hours ahead")
	}
	if req.Rows == nil {
		return time.Time{}, nil, nil, fmt.Errorf("rows is required")
	}
	rows := *req.Rows
	if len(rows) > journeyDueMaxRows {
		return time.Time{}, nil, nil, fmt.Errorf("too many rows (%d > %d)", len(rows), journeyDueMaxRows)
	}
	lists := make([]int64, 0, len(rows))
	subs := make([]int64, 0, len(rows))
	for i, r := range rows {
		if len(r) != 2 {
			return time.Time{}, nil, nil, fmt.Errorf("row %d must be [list_id, subscriber_id]", i)
		}
		if r[0] < 1 || r[1] < 1 || r[0] > math.MaxInt32 || r[1] > math.MaxInt32 {
			return time.Time{}, nil, nil, fmt.Errorf("row %d: ids must be positive integers", i)
		}
		lists = append(lists, r[0])
		subs = append(subs, r[1])
	}
	return at.UTC(), lists, subs, nil
}

// PutJourneyDue replaces one forecast feed (D6). Any refusal is a 400 and nothing is written.
func (a *App) PutJourneyDue(c echo.Context) error {
	var req journeyDueReq
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "rows"))
	}
	at, lists, subs, err := validateJourneyDue(req, time.Now())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	n, err := a.core.ReplaceJourneyDue(req.Source, at, lists, subs)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{map[string]int64{"rows": n}})
}

// GetJourneyDue is the switch and the per-feed counts (D6/D7). No subscriber ids.
func (a *App) GetJourneyDue(c echo.Context) error {
	out, err := a.core.GetJourneyDueFeeds()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}
