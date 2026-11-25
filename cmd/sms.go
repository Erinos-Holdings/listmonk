package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
)

// Fork (SMS Admin, integrations listmonk/SMS-LINKS-ADMIN-SPEC). The SMS Admin page's API half:
//
//   Short links (D3, D6) -- a proxy to the integrations ShortlinksProd IAM admin Function URL
//   (config sms.admin_url, read once at boot), every request SigV4-signed (service lambda, the
//   URL's region) with the default AWS credential chain -- on the listmonk host, the instance role
//   over IMDSv2. Empty or ill-shaped sms.admin_url turns the link routes off (503 sms.notConfigured).
//   The proxy decodes the client body into a typed struct and re-marshals it, so only D6's fields
//   reach the Lambda; created_by is set here (listmonk:<username>), never read from the client. Slug
//   and target validation are the Lambda's alone.
//     GET    /api/sms/links         admin list
//     POST   /api/sms/links         admin put mode create (slug given) or create (generated code)
//     PUT    /api/sms/links/:slug   admin put mode update (the whole record; omitted = cleared)
//     DELETE /api/sms/links/:slug   admin delete (deactivate)
//   Lambda 400/404/409 pass through with its error text (<= 200 characters); any other status, a
//   timeout, a transport or a credential error is 502 sms.adminUnavailable, the cause logged only.
//
//   Promo codes (D8-D10) -- sms_promo_codes marks which promotion values are SMS codes; the report
//   aggregates promo_purchases per marked code (queries/sms.sql).
//     GET    /api/sms/promo-codes      {rows, feed, known}
//     POST   /api/sms/promo-codes      {code, brand?} -> 201 the row; 409 duplicate; 400 invalid
//     DELETE /api/sms/promo-codes/:id  unmark; 404 when absent
//
// Every route is sms:manage (handlers.go).

// lambdaURLRe is the accepted Function URL shape (SMS Admin D3; the SKU count's
// lists.sku_count_url, integrations SKU-LIVE-COUNT-SPEC D5); group 1 is the region.
var lambdaURLRe = regexp.MustCompile(`^https://[a-z0-9]+\.lambda-url\.([a-z0-9-]+)\.on\.aws/?$`)

const (
	smsAdminTimeout  = 10 * time.Second
	smsAdminErrorMax = 200
	smsCreatedByPfx  = "listmonk:"
)

// sigv4Lambda is the shared half of the host's SigV4 proxies to an integrations AWS_IAM Function URL
// (SMS Admin; the Lists form's SKU journey count, integrations SKU-LIVE-COUNT-SPEC D5): the URL, its
// region, the default-chain credentials and one signed JSON POST to the URL's root. Each feature
// constructs its own (its own config key, timeout and error mapping).
type sigv4Lambda struct {
	endpoint string // the configured URL; every request POSTs to its root
	region   string
	creds    aws.CredentialsProvider // cached by the SDK; nil with credsErr
	credsErr error
	signer   *v4.Signer
	client   *http.Client
	now      func() time.Time
}

// smsAdmin is the configured SMS Admin proxy target. nil on the App = the feature is off.
type smsAdmin struct {
	*sigv4Lambda
}

// parseLambdaURL returns the URL's region, or "" and why the URL (config key `key`) is refused.
func parseLambdaURL(key, raw string) (region, reason string) {
	if raw == "" {
		return "", key + " is empty"
	}
	m := lambdaURLRe.FindStringSubmatch(raw)
	if m == nil {
		return "", key + " is ill-shaped (want https://<id>.lambda-url.<region>.on.aws/)"
	}
	return m[1], ""
}

// parseSMSAdminURL returns the URL's region, or "" and why the URL is refused.
func parseSMSAdminURL(raw string) (region, reason string) {
	return parseLambdaURL("sms.admin_url", raw)
}

// newSigV4Lambda builds the signer half for an accepted URL with the given credentials and timeout.
func newSigV4Lambda(raw, region string, creds aws.CredentialsProvider, timeout time.Duration) *sigv4Lambda {
	return &sigv4Lambda{
		endpoint: raw,
		region:   region,
		creds:    creds,
		signer:   v4.NewSigner(),
		client:   &http.Client{Timeout: timeout},
		now:      time.Now,
	}
}

// newSMSAdmin builds the proxy for an accepted URL with the given credentials.
func newSMSAdmin(raw, region string, creds aws.CredentialsProvider) *smsAdmin {
	return &smsAdmin{newSigV4Lambda(raw, region, creds, smsAdminTimeout)}
}

// loadLambdaCreds is the default AWS credential chain for a Function URL's region: env -> shared
// config -> IMDS (the host resolves to its instance role, a dev box to its own profile). Nothing is
// fetched here; the SDK's cached provider resolves on first use.
func loadLambdaCreds(region string) (aws.CredentialsProvider, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return cfg.Credentials, nil
}

// initSMSAdmin reads sms.admin_url once at boot and logs one line saying whether the link proxy is
// configured (and its region) or off (and why). It never fails the boot.
func initSMSAdmin(raw string, lo *log.Logger) *smsAdmin {
	region, reason := parseSMSAdminURL(raw)
	if reason != "" {
		lo.Printf("sms admin: off (%s); the short-link routes answer 503", reason)
		return nil
	}
	creds, err := loadLambdaCreds(region)
	if err != nil {
		s := newSMSAdmin(raw, region, nil)
		s.credsErr = err
		lo.Printf("sms admin: configured, region %s; AWS config failed to load (every link request will answer 502): %v", region, err)
		return s
	}
	lo.Printf("sms admin: configured, region %s", region)
	return newSMSAdmin(raw, region, creds)
}

// smsAdminReq is the ONLY shape forwarded to the Lambda (D6): the typed fields, nothing else.
type smsAdminReq struct {
	Action      string  `json:"action"`
	Mode        string  `json:"mode,omitempty"`
	Slug        string  `json:"slug,omitempty"`
	Target      *string `json:"target,omitempty"`
	UTMSource   string  `json:"utm_source,omitempty"`
	UTMMedium   string  `json:"utm_medium,omitempty"`
	UTMCampaign string  `json:"utm_campaign,omitempty"`
	UTMTerm     string  `json:"utm_term,omitempty"`
	UTMContent  string  `json:"utm_content,omitempty"`
	Brand       string  `json:"brand,omitempty"`
	Campaign    string  `json:"campaign,omitempty"`
	Notes       string  `json:"notes,omitempty"`
	CreatedBy   string  `json:"created_by,omitempty"`
	Active      *bool   `json:"active,omitempty"`
}

// smsLinkReq is the client body of POST and PUT /api/sms/links. Unknown keys (clicks, previews,
// created_by, action, mode, ...) are dropped by the decode.
type smsLinkReq struct {
	Slug        string `json:"slug"`
	Target      string `json:"target"`
	UTMSource   string `json:"utm_source"`
	UTMMedium   string `json:"utm_medium"`
	UTMCampaign string `json:"utm_campaign"`
	UTMTerm     string `json:"utm_term"`
	UTMContent  string `json:"utm_content"`
	Brand       string `json:"brand"`
	Campaign    string `json:"campaign"`
	Notes       string `json:"notes"`
	Active      *bool  `json:"active"`
}

func (r smsLinkReq) fields(action, mode string) smsAdminReq {
	t := r.Target
	return smsAdminReq{
		Action: action, Mode: mode, Target: &t,
		UTMSource: r.UTMSource, UTMMedium: r.UTMMedium, UTMCampaign: r.UTMCampaign, UTMTerm: r.UTMTerm, UTMContent: r.UTMContent,
		Brand: r.Brand, Campaign: r.Campaign, Notes: r.Notes,
	}
}

// post signs and sends one JSON request to the URL's root; it returns the Lambda's status and body.
func (s *sigv4Lambda) post(ctx context.Context, payload any) (int, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	u, err := url.Parse(s.endpoint)
	if err != nil {
		return 0, nil, err
	}
	u.Path, u.RawPath, u.RawQuery, u.Fragment = "/", "", "", ""

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	if s.creds == nil {
		return 0, nil, fmt.Errorf("no AWS credentials: %v", s.credsErr)
	}
	cr, err := s.creds.Retrieve(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("retrieving AWS credentials: %w", err)
	}
	sum := sha256.Sum256(body)
	if err := s.signer.SignHTTP(ctx, cr, req, hex.EncodeToString(sum[:]), "lambda", s.region, s.now()); err != nil {
		return 0, nil, fmt.Errorf("signing: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, b, nil
}

// lambdaError is the admin API's error text ({status, error}), at most 200 characters.
func lambdaError(body []byte, status int) string {
	var e struct {
		Error string `json:"error"`
	}
	msg := ""
	if json.Unmarshal(body, &e) == nil {
		msg = e.Error
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	if utf8.RuneCountInString(msg) > smsAdminErrorMax {
		msg = string([]rune(msg)[:smsAdminErrorMax])
	}
	return msg
}

// smsProxy runs one admin request and maps its answer (D6). okStatus is the success status the
// route answers (201 on create, 200 otherwise); list returns the Lambda's links array.
func (a *App) smsProxy(c echo.Context, payload smsAdminReq, okStatus int, list bool) error {
	if a.smsAdmin == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, a.i18n.T("sms.notConfigured"))
	}
	unavailable := func(cause string) error {
		a.log.Printf("sms admin: %s failed: %s", payload.Action, cause)
		return echo.NewHTTPError(http.StatusBadGateway, a.i18n.T("sms.adminUnavailable"))
	}

	status, body, err := a.smsAdmin.post(c.Request().Context(), payload)
	if err != nil {
		return unavailable(err.Error())
	}
	switch status {
	case http.StatusOK, http.StatusCreated:
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict:
		return echo.NewHTTPError(status, lambdaError(body, status))
	default:
		return unavailable(fmt.Sprintf("HTTP %d: %s", status, lambdaError(body, status)))
	}

	if list {
		var out struct {
			Links []json.RawMessage `json:"links"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return unavailable("undecodable list response: " + err.Error())
		}
		if out.Links == nil {
			out.Links = []json.RawMessage{}
		}
		return c.JSON(okStatus, okResp{out.Links})
	}
	var rec map[string]json.RawMessage
	if err := json.Unmarshal(body, &rec); err != nil || rec == nil {
		return unavailable("undecodable record response")
	}
	delete(rec, "status") // the Lambda's outcome word; the record is the rest
	return c.JSON(okStatus, okResp{rec})
}

func smsCreatedBy(c echo.Context) string {
	return smsCreatedByPfx + auth.GetUser(c).Username
}

// smsPathSlug is the route's :slug, unescaped.
func smsPathSlug(c echo.Context) (string, error) {
	return url.PathUnescape(c.Param("slug"))
}

func (a *App) smsDecodeLink(c echo.Context) (smsLinkReq, error) {
	var req smsLinkReq
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return req, echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "body"))
	}
	return req, nil
}

// GetSMSLinks is GET /api/sms/links -- the admin list.
func (a *App) GetSMSLinks(c echo.Context) error {
	return a.smsProxy(c, smsAdminReq{Action: "list"}, http.StatusOK, true)
}

// CreateSMSLink is POST /api/sms/links: a non-empty slug is put mode create (an existing slug is
// the Lambda's 409, nothing written); none is create (a generated code).
func (a *App) CreateSMSLink(c echo.Context) error {
	req, err := a.smsDecodeLink(c)
	if err != nil {
		return err
	}
	var p smsAdminReq
	if strings.TrimSpace(req.Slug) != "" {
		p = req.fields("put", "create")
		p.Slug = req.Slug
	} else {
		p = req.fields("create", "")
	}
	p.CreatedBy = smsCreatedBy(c)
	return a.smsProxy(c, p, http.StatusCreated, false)
}

// UpdateSMSLink is PUT /api/sms/links/:slug -- put mode update with the path's slug; created_by is
// never sent (the Lambda keeps it). Omitted optional fields are cleared, so the page sends the
// whole record.
func (a *App) UpdateSMSLink(c echo.Context) error {
	slug, err := smsPathSlug(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "slug"))
	}
	req, err := a.smsDecodeLink(c)
	if err != nil {
		return err
	}
	p := req.fields("put", "update")
	p.Slug = slug
	p.Active = req.Active
	return a.smsProxy(c, p, http.StatusOK, false)
}

// DeleteSMSLink is DELETE /api/sms/links/:slug -- the admin's soft delete (deactivate).
func (a *App) DeleteSMSLink(c echo.Context) error {
	slug, err := smsPathSlug(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "slug"))
	}
	return a.smsProxy(c, smsAdminReq{Action: "delete", Slug: slug}, http.StatusOK, false)
}

// GetSMSPromoCodes is GET /api/sms/promo-codes -- {rows, feed, known} (D9).
func (a *App) GetSMSPromoCodes(c echo.Context) error {
	out, err := a.core.GetSMSPromoReport()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// CreateSMSPromoCode is POST /api/sms/promo-codes {code, brand?} (D10). The code is trimmed and
// validated as the feed's values are (promoValueOK).
func (a *App) CreateSMSPromoCode(c echo.Context) error {
	var req struct {
		Code  string `json:"code"`
		Brand string `json:"brand"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "body"))
	}
	code := strings.TrimSpace(req.Code)
	if !promoValueOK(code) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("sms.invalidCode"))
	}
	out, err := a.core.InsertSMSPromoCode(code, strings.TrimSpace(req.Brand), smsCreatedBy(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, okResp{out})
}

// DeleteSMSPromoCode is DELETE /api/sms/promo-codes/:id -- unmark; 404 when absent.
func (a *App) DeleteSMSPromoCode(c echo.Context) error {
	if err := a.core.DeleteSMSPromoCode(getID(c)); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{true})
}
