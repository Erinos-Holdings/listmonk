package main

// Fork (SKU journey live count) -- integrations lambda/integrations/komigo/SKU-LIVE-COUNT-SPEC D5.
// POST /api/lists/sku-count is the Lists form's on-demand count for a Replenish / First purchase of
// SKUs trigger: a proxy to the integrations SkuJourneyCount AWS_IAM Function URL (config
// lists.sku_count_url, env LISTMONK_lists__sku_count_url, read once at boot -- the SMS Admin URL
// shape; empty or ill-shaped turns the route off: 503 lists.skuCountNotConfigured), every request
// SigV4-signed with the host's default credential chain through the shared sigv4Lambda (sms.go).
// The fork stores nothing and computes nothing.
//
//   - Body (typed decode, unknown keys dropped): {list_id, brand, kind, skus, days}. kind is
//     replenish or first_sku, skus normalized as a SKU trigger's (normalizeTriggerSkus), days null or
//     a whole number 1-365 -- shape only (hazard 110); the Lambda owns the meaning. 400 otherwise.
//   - The role boundary: list_id > 0 needs manage on THAT list (HasListPerm), the list must exist,
//     and the brand sent to the Lambda is the LIST's brand (a body brand is ignored; a list with no
//     brand is 400). list_id absent or 0 (a list being created) needs lists:manage_all -- what
//     POST /api/lists requires -- and the body's brand must be a brands row. HasListPerm is never
//     called with 0 (it skips ids <= 0 and would pass vacuously).
//   - The response is decoded into a typed struct and re-marshalled: counts, the two booleans, the
//     timestamps and the suggestion strings only, so an address cannot pass through even if the
//     Lambda regressed. Lambda 400 passes through with its error text (<= 200 characters); any other
//     status, a timeout (25 s), a transport or credential error is 502 lists.skuCountUnavailable,
//     the cause logged only.

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

const (
	skuCountTimeout = 25 * time.Second
	maxSkuCountBody = 64 << 10
)

// skuCountProxy is the configured count target. nil on the App = the route is off.
type skuCountProxy struct {
	*sigv4Lambda
}

func newSkuCountProxy(raw, region string, creds aws.CredentialsProvider) *skuCountProxy {
	return &skuCountProxy{newSigV4Lambda(raw, region, creds, skuCountTimeout)}
}

// initSkuCount reads lists.sku_count_url once at boot and logs one line saying whether the count
// proxy is configured (and its region) or off (and why). It never fails the boot.
func initSkuCount(raw string, lo *log.Logger) *skuCountProxy {
	region, reason := parseLambdaURL("lists.sku_count_url", raw)
	if reason != "" {
		lo.Printf("sku count: off (%s); POST /api/lists/sku-count answers 503", reason)
		return nil
	}
	creds, err := loadLambdaCreds(region)
	if err != nil {
		s := newSkuCountProxy(raw, region, nil)
		s.credsErr = err
		lo.Printf("sku count: configured, region %s; AWS config failed to load (every count will answer 502): %v", region, err)
		return s
	}
	lo.Printf("sku count: configured, region %s", region)
	return newSkuCountProxy(raw, region, creds)
}

// skuCountBody is the client body. Unknown keys are dropped by the decode.
type skuCountBody struct {
	ListID *int            `json:"list_id"`
	Brand  string          `json:"brand"`
	Kind   string          `json:"kind"`
	Skus   json.RawMessage `json:"skus"`
	Days   json.RawMessage `json:"days"`
}

// skuCountReq is the ONLY shape forwarded to the Lambda (D1): days null when not entered.
type skuCountReq struct {
	Brand  string   `json:"brand"`
	ListID int      `json:"list_id"`
	Kind   string   `json:"kind"`
	Skus   []string `json:"skus"`
	Days   *int     `json:"days"`
}

// The typed response (D1): counts, booleans, timestamps and suggestion strings -- nothing else.
type skuCountSuggestion struct {
	Sku      string `json:"sku"`
	BrandSku string `json:"brand_sku"`
	Title    string `json:"title"`
}

type skuCountSku struct {
	Sku         string               `json:"sku"`
	BrandSku    bool                 `json:"brand_sku"`
	Buyers      *int                 `json:"buyers"`
	Suggestions []skuCountSuggestion `json:"suggestions"`
}

type skuCountBlock struct {
	Buyers   int            `json:"buyers"`
	Eligible int            `json:"eligible"`
	Skips    map[string]int `json:"skips"`
	Consent  map[string]int `json:"consent"`
}

type skuCountAge struct {
	FromDays int `json:"from_days"`
	ToDays   int `json:"to_days"`
	Joins    int `json:"joins"`
}

type skuCountDue struct {
	Days    int            `json:"days"`
	Joins   int            `json:"joins"`
	ByAge   []skuCountAge  `json:"by_age"`
	ByLang  map[string]int `json:"by_lang"`
	Skips   map[string]int `json:"skips"`
	Consent map[string]int `json:"consent"`
}

type skuCountResp struct {
	AsOf                string         `json:"as_of"`
	NextRunAt           string         `json:"next_run_at"`
	AutomationsRunning  int            `json:"automations_running"`
	AssumedAllLanguages bool           `json:"assumed_all_languages"`
	Skus                []skuCountSku  `json:"skus"`
	Window              *skuCountBlock `json:"window"`
	Due                 *skuCountDue   `json:"due"`
}

// skuCountKeyRe is the shape of a count map's key (a skip or consent reason, a language): never an
// address -- a key outside it is dropped from the re-marshalled response.
var skuCountKeyRe = regexp.MustCompile(`^[a-z0-9_-]{1,40}$`)

func cleanCountMap(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		if skuCountKeyRe.MatchString(k) {
			out[k] = v
		}
	}
	return out
}

// sanitize normalizes the decoded response for re-marshalling (empty slices, cleaned maps).
func (r *skuCountResp) sanitize() {
	if r.Skus == nil {
		r.Skus = []skuCountSku{}
	}
	for i := range r.Skus {
		if r.Skus[i].Suggestions == nil {
			r.Skus[i].Suggestions = []skuCountSuggestion{}
		}
	}
	if r.Window != nil {
		r.Window.Skips, r.Window.Consent = cleanCountMap(r.Window.Skips), cleanCountMap(r.Window.Consent)
	}
	if r.Due != nil {
		r.Due.ByLang, r.Due.Skips, r.Due.Consent = cleanCountMap(r.Due.ByLang), cleanCountMap(r.Due.Skips), cleanCountMap(r.Due.Consent)
		if r.Due.ByAge == nil {
			r.Due.ByAge = []skuCountAge{}
		}
	}
}

// skuCountLookups are the two reads the permission half needs (an interface so the handler is
// testable without a database).
type skuCountLookups interface {
	GetList(id int) (models.List, error)
	BrandExists(slug string) (bool, error)
}

type coreSkuCountLookups struct{ a *App }

func (l coreSkuCountLookups) GetList(id int) (models.List, error) { return l.a.core.GetList(id, "") }
func (l coreSkuCountLookups) BrandExists(slug string) (bool, error) {
	_, ok, err := l.a.core.GetBrand(slug)
	return ok, err
}

// PostListSkuCount handles POST /api/lists/sku-count (permissions in the handler -- the UpdateList
// posture; see the file comment).
func (a *App) PostListSkuCount(c echo.Context) error {
	return a.skuCount(c, coreSkuCountLookups{a})
}

func (a *App) skuCount(c echo.Context, lk skuCountLookups) error {
	bad := func(reason string) error {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("lists.skuCountInvalid", "reason", reason))
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxSkuCountBody+1))
	if err != nil || len(raw) > maxSkuCountBody {
		return bad("the body is unreadable or too large")
	}
	var body skuCountBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return bad("the body must be a JSON object")
	}
	if body.Kind != joinTriggerReplenish && body.Kind != joinTriggerFirstSku {
		return bad(fmt.Sprintf("kind must be %q or %q", joinTriggerReplenish, joinTriggerFirstSku))
	}
	skus, err := normalizeTriggerSkus(body.Skus, len(body.Skus) > 0)
	if err != nil {
		return bad(err.Error())
	}
	var days *int
	if len(body.Days) > 0 && !isJSONNull(body.Days) {
		d, ok := jsonInt(body.Days)
		if !ok || d < minJoinTriggerDays || d > maxJoinTriggerDays {
			return bad(fmt.Sprintf("days must be empty or a whole number from %d to %d", minJoinTriggerDays, maxJoinTriggerDays))
		}
		days = &d
	}

	// The role boundary.
	user := auth.GetUser(c)
	listID := 0
	if body.ListID != nil {
		listID = *body.ListID
	}
	var brand string
	switch {
	case listID < 0:
		return bad("list_id must be a list id, or 0 for a new list")
	case listID > 0:
		if err := user.HasListPerm(auth.PermTypeManage, listID); err != nil {
			return err
		}
		list, err := lk.GetList(listID)
		if err != nil {
			return err
		}
		brand = strings.TrimSpace(list.Brand)
		if brand == "" {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.skuCountNoBrand"))
		}
	default:
		// A list being created: what POST /api/lists requires. Never HasListPerm(.., 0).
		if !user.HasPerm(auth.PermListManageAll) {
			return echo.NewHTTPError(http.StatusForbidden, fmt.Sprintf("permission denied: %s", auth.PermListManageAll))
		}
		brand = strings.TrimSpace(body.Brand)
		ok := false
		if brand != "" {
			if ok, err = lk.BrandExists(brand); err != nil {
				return err
			}
		}
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("lists.skuCountNoBrand"))
		}
	}

	if a.skuCountProxy == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, a.i18n.T("lists.skuCountNotConfigured"))
	}
	unavailable := func(cause string) error {
		a.log.Printf("sku count: list %d brand %s failed: %s", listID, brand, cause)
		return echo.NewHTTPError(http.StatusBadGateway, a.i18n.T("lists.skuCountUnavailable"))
	}
	status, respBody, err := a.skuCountProxy.post(c.Request().Context(), skuCountReq{Brand: brand, ListID: listID, Kind: body.Kind, Skus: skus, Days: days})
	if err != nil {
		return unavailable(err.Error())
	}
	switch status {
	case http.StatusOK:
	case http.StatusBadRequest:
		return echo.NewHTTPError(http.StatusBadRequest, lambdaError(respBody, status))
	default:
		return unavailable(fmt.Sprintf("HTTP %d: %s", status, lambdaError(respBody, status)))
	}
	var out skuCountResp
	if err := json.Unmarshal(respBody, &out); err != nil {
		return unavailable("undecodable count response: " + err.Error())
	}
	out.sanitize()
	return c.JSON(http.StatusOK, okResp{out})
}
