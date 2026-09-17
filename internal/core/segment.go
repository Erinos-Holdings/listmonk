package core

// Fork (list grid, integrations LIST-GRID-SPEC D6/D7) -- the server-authored segment= / lang=
// subscriber filter. The DEFINITIONS of a segment and a language bucket live in SQL
// (subscription_segment(), subscriber_lang(), send_lang() -- internal/migrations/v6.2.11.go);
// this file only names them in a predicate.

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// subscriberFilterExp composes the segment/lang predicate with the caller's optional SQL
// expression and returns what goes into a subscriber query template's %query% slot.
//
// It lives in core, and takes the filter in its own argument rather than inside queryExp,
// because the delete/blocklist handlers BLANK the search and query when the request says
// all=true -- and the frontend sends all=true exactly when a grid-linked page has no search
// (select-all on a segment). A segment folded into the query by a handler would be erased
// there and "delete the 82 unsubscribed" would delete the list (spec review C1).
//
// The result is always "(<predicate>) AND (<user query>)". The templates splice
// "AND %query%" unparenthesised, so a bare user "a OR b" would otherwise bind the predicate to
// "a" only and let "b" escape the segment (review H4).
//
// Every value is allowlisted and the list id is an int, so no request text reaches the
// predicate. It is self-contained (its own subscriber_lists subquery, the list's opt-in by a
// scalar subquery on lists -- both tables the EXPLAIN validator allows) so it works whether or
// not the enclosing query joins subscriber_lists. listIDs must be the PERMITTED ids
// (auth.User.GetPermittedListIDs can substitute the user's own lists for a non-permitted id),
// so "exactly one list" is counted after that substitution.
func (c *Core) subscriberFilterExp(queryExp string, listIDs []int, f models.SubscriberFilter) (string, error) {
	if f.Segment == "" && f.Lang == "" {
		return queryExp, nil
	}

	var preds []string
	if f.Segment != "" {
		if !models.IsSegment(f.Segment) {
			return "", echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.invalidFields", "name", "segment"))
		}
		if len(listIDs) != 1 || listIDs[0] < 1 {
			return "", echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("subscribers.segmentNeedsOneList"))
		}
		preds = append(preds, fmt.Sprintf(`subscribers.id IN (SELECT seg_sl.subscriber_id FROM subscriber_lists seg_sl
			WHERE seg_sl.list_id = %d AND subscription_segment(seg_sl.status, seg_sl.meta, subscribers.status,
				(SELECT seg_l.optin FROM lists seg_l WHERE seg_l.id = %d)) = '%s')`, listIDs[0], listIDs[0], f.Segment))
	}

	if f.Lang != "" {
		if !models.IsSubscriberLangFilter(f.Lang) {
			return "", echo.NewHTTPError(http.StatusBadRequest,
				c.i18n.Ts("globals.messages.invalidFields", "name", "lang"))
		}
		if f.Lang == models.SubscriberLangNone {
			// The explicit no-language set, a subset of lang=en.
			preds = append(preds, `subscriber_lang(subscribers.attribs) = 'none'`)
		} else {
			// A SEND language -- en is the en and none buckets, which send_lang() folds.
			preds = append(preds, fmt.Sprintf(`send_lang(subscriber_lang(subscribers.attribs)) = '%s'`, f.Lang))
		}
	}

	out := "(" + strings.Join(preds, " AND ") + ")"
	if queryExp != "" {
		out += " AND (" + queryExp + ")"
	}
	return out, nil
}
