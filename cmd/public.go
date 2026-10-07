package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"html/template"
	"image"
	"image/png"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/knadh/listmonk/internal/captcha"
	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/internal/linkresolve"
	"github.com/knadh/listmonk/internal/manager"
	"github.com/knadh/listmonk/internal/notifs"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

const (
	tplMessage = "message"
)

// tplRenderer wraps a template.tplRenderer for echo.
type tplRenderer struct {
	templates           *template.Template
	SiteName            string
	RootURL             string
	LogoURL             string
	FaviconURL          string
	AssetVersion        string
	EnablePublicSubPage bool
	EnablePublicArchive bool
	IndividualTracking  bool
}

// tplData is the data container that is injected
// into public templates for accessing data.
type tplData struct {
	SiteName            string
	RootURL             string
	LogoURL             string
	FaviconURL          string
	AssetVersion        string
	EnablePublicSubPage bool
	EnablePublicArchive bool
	IndividualTracking  bool
	// AuthPage is true for the admin auth pages (login, forgot/reset password, first-run
	// setup): light-mode pages that take the bundled light logo and no vendor footer.
	AuthPage bool
	Data     any
	L        *i18n.I18n
}

type publicTpl struct {
	Title       string
	Description string
}

type unsubTpl struct {
	publicTpl
	Subscriber       models.Subscriber
	Subscriptions    []models.Subscription
	SubUUID          string
	AllowBlocklist   bool
	AllowExport      bool
	AllowWipe        bool
	AllowPreferences bool
	ShowManage       bool

	// Names of the lists the campaign in the URL targeted (empty when the campaign is
	// unknown), shown on the simple unsubscribe form.
	Lists []string
}

// unsubbedTpl is the post-unsubscribe confirmation page.
type unsubbedTpl struct {
	publicTpl
	Email string
	Lists []string
}

type optinReq struct {
	SubUUID   string
	ListUUIDs []string      `query:"l" form:"l"`
	Lists     []models.List `query:"-" form:"-"`
}

type optinTpl struct {
	publicTpl
	optinReq
}

type msgTpl struct {
	publicTpl
	MessageTitle string
	Message      string
}

type subFormTpl struct {
	publicTpl
	Lists   []models.List
	Captcha struct {
		Enabled    bool
		Provider   string
		Key        string
		Complexity int
	}
}

var (
	pixelPNG = drawTransparentImage(3, 14)
)

// Render executes and renders a template for echo.
func (t *tplRenderer) Render(w io.Writer, name string, data any, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, tplData{
		SiteName:            t.SiteName,
		RootURL:             t.RootURL,
		LogoURL:             t.LogoURL,
		FaviconURL:          t.FaviconURL,
		AssetVersion:        t.AssetVersion,
		EnablePublicSubPage: t.EnablePublicSubPage,
		EnablePublicArchive: t.EnablePublicArchive,
		IndividualTracking:  t.IndividualTracking,
		AuthPage:            strings.HasPrefix(name, "admin-"),
		Data:                data,
		L:                   c.Get("app").(*App).i18n,
	})
}

// GetPublicLists returns the list of public lists with minimal fields
// required to submit a subscription.
func (a *App) GetPublicLists(c echo.Context) error {
	// Get all public lists.
	lists, err := a.core.GetLists(models.ListTypePublic, models.ListStatusActive, true, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("public.errorFetchingLists"))
	}

	type list struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	}

	out := make([]list, 0, len(lists))
	for _, l := range lists {
		out = append(out, list{
			UUID: l.UUID,
			Name: l.Name,
		})
	}

	return c.JSON(http.StatusOK, out)
}

// ViewCampaignMessage renders the HTML view of a campaign message.
// This is the view the {{ MessageURL }} template tag links to in e-mail campaigns.
func (a *App) ViewCampaignMessage(c echo.Context) error {
	// Get the campaign.
	campUUID := c.Param("campUUID")
	camp, err := a.core.GetCampaign(0, campUUID, "")
	if err != nil {
		if er, ok := err.(*echo.HTTPError); ok {
			if er.Code == http.StatusBadRequest {
				return c.Render(http.StatusNotFound, tplMessage,
					makeMsgTpl(a.i18n.T("public.notFoundTitle"), "", a.i18n.T("public.campaignNotFound")))
			}
		}

		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorFetchingCampaign")))
	}

	// Get the subscriber.
	subUUID := c.Param("subUUID")
	sub, err := a.core.GetSubscriber(0, subUUID, "")
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Render(http.StatusNotFound, tplMessage,
				makeMsgTpl(a.i18n.T("public.notFoundTitle"), "", a.i18n.T("public.errorFetchingEmail")))
		}

		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorFetchingCampaign")))
	}

	// Compile the template.
	if err := camp.CompileTemplate(a.manager.TemplateFuncs(&camp)); err != nil {
		a.log.Printf("error compiling template: %v", err)
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorFetchingCampaign")))
	}

	// Render the message body.
	msg, err := a.manager.NewCampaignMessage(&camp, sub)
	if err != nil {
		a.log.Printf("error rendering message: %v", err)
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorFetchingCampaign")))
	}

	return sandboxedHTML(c, publicPageCSP, string(msg.Body()))
}

// Fork (uploads hardening) -- integrations UPLOADS-HARDENING-SPEC D4. A campaign or template body
// is user-supplied HTML served from the admin host, so every response returning one as a page
// is sandboxed: script off and an opaque origin, so it never runs with an administrator's
// session. Only the stored-body answer carries it; the error pages rendered from the public
// template load the site's own script and are not sandboxed.
const (
	// previewCSP is for the admin preview routes.
	previewCSP = "sandbox"

	// publicPageCSP is for the view-in-browser and public archive pages: links still navigate,
	// target="_blank" opens a normal tab, forms still submit and a linked file still downloads.
	publicPageCSP = "sandbox allow-popups allow-popups-to-escape-sandbox allow-forms allow-downloads"
)

// sandboxedHTML answers a stored body as an HTML page under the given policy. It is the only
// c.HTML call in cmd/ (cmd/media_content_type_test.go asserts it): a new route answering a body
// goes through here.
func sandboxedHTML(c echo.Context, policy, body string) error {
	c.Response().Header().Set(echo.HeaderContentSecurityPolicy, policy)
	return c.HTML(http.StatusOK, body)
}

// SubscriptionPage renders the subscription management page and handles unsubscriptions.
// This is the view that {{ UnsubscribeURL }} in campaigns link to.
func (a *App) SubscriptionPage(c echo.Context) error {
	var (
		campUUID      = c.Param("campUUID")
		subUUID       = c.Param("subUUID")
		showManage, _ = strconv.ParseBool(c.FormValue("manage"))
	)

	// Get the subscriber from the DB.
	s, err := a.core.GetSubscriber(0, subUUID, "")
	if err != nil {
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorProcessingRequest")))
	}

	// Prepare the public template.
	out := unsubTpl{
		Subscriber:       s,
		SubUUID:          subUUID,
		publicTpl:        publicTpl{Title: a.i18n.T("public.unsubscribeTitle")},
		AllowBlocklist:   a.cfg.Privacy.AllowBlocklist,
		AllowExport:      a.cfg.Privacy.AllowExport,
		AllowWipe:        a.cfg.Privacy.AllowWipe,
		AllowPreferences: a.cfg.Privacy.AllowPreferences,
	}

	// If the subscriber is blocklisted, throw an error.
	if s.Status == models.SubscriberStatusBlockListed {
		return c.Render(http.StatusOK, tplMessage, makeMsgTpl(a.i18n.T("public.noSubTitle"), "", a.i18n.Ts("public.blocklisted")))
	}

	// Name the lists the unsubscribe will act on. Best effort, the page renders without
	// them on a lookup failure rather than turning an unsubscribe link into an error.
	if names, err := a.core.GetCampaignListNames(campUUID, subUUID); err == nil {
		out.Lists = names
	}

	// Only show preference management if it's enabled in settings.
	if a.cfg.Privacy.AllowPreferences {
		out.ShowManage = showManage

		// Get the subscriber's lists from the DB to render in the template.
		subs, err := a.core.GetSubscriptions(0, subUUID, false)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("public.errorFetchingLists"))
		}

		out.Subscriptions = make([]models.Subscription, 0, len(subs))
		for _, s := range subs {
			// Private lists shouldn't be rendered in the template.
			if s.Type == models.ListTypePrivate {
				continue
			}

			out.Subscriptions = append(out.Subscriptions, s)
		}
	}

	return c.Render(http.StatusOK, "subscription", out)
}

// SubscriptionPrefs renders the subscription management page and
// s unsubscriptions. This is the view that {{ UnsubscribeURL }} in
// campaigns link to.
func (a *App) SubscriptionPrefs(c echo.Context) error {
	// Read the form.
	var req struct {
		Name      string   `form:"name" json:"name"`
		ListUUIDs []string `form:"l" json:"list_uuids"`
		Blocklist bool     `form:"blocklist" json:"blocklist"`
		Manage    bool     `form:"manage" json:"manage"`
	}
	if err := c.Bind(&req); err != nil {
		return c.Render(http.StatusBadRequest, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("globals.messages.invalidData")))
	}

	// Simple unsubscribe.
	var (
		campUUID  = c.Param("campUUID")
		subUUID   = c.Param("subUUID")
		blocklist = a.cfg.Privacy.AllowBlocklist && req.Blocklist
	)
	if !req.Manage || blocklist {
		if err := a.core.UnsubscribeByCampaign(subUUID, campUUID, blocklist); err != nil {
			return c.Render(http.StatusInternalServerError, tplMessage,
				makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("public.errorProcessingRequest")))
		}

		// Confirmation names the subscriber and the lists acted on. Both lookups are best
		// effort. The unsubscribe has already happened, so a failure here must still
		// confirm it, just with less detail. A blocklist opt-out acted on every list, not
		// the campaign's, so it keeps the generic confirmation rather than naming a subset.
		out := unsubbedTpl{publicTpl: publicTpl{Title: a.i18n.T("public.unsubbedTitle")}}
		if s, err := a.core.GetSubscriber(0, subUUID, ""); err == nil {
			out.Email = s.Email
		}
		if !blocklist {
			if names, err := a.core.GetCampaignListNames(campUUID, subUUID); err == nil {
				out.Lists = names
			}
		}

		return c.Render(http.StatusOK, "unsubscribed", out)
	}

	// Is preference management enabled?
	if !a.cfg.Privacy.AllowPreferences {
		return c.Render(http.StatusBadRequest, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("public.invalidFeature")))
	}

	// Manage preferences.
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 256 {
		return c.Render(http.StatusBadRequest, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("subscribers.invalidName")))
	}

	// Get the subscriber from the DB.
	sub, err := a.core.GetSubscriber(0, subUUID, "")
	if err != nil {
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("globals.messages.pFound",
				"name", a.i18n.T("globals.terms.subscriber"))))
	}
	sub.Name = req.Name

	// Update the subscriber properties in the DB.
	if _, err := a.core.UpdateSubscriber(sub.ID, sub); err != nil {
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("public.errorProcessingRequest")))
	}

	// Get the subscriber's lists and whatever is not sent in the request (unchecked),
	// unsubscribe them.
	reqUUIDs := make(map[string]struct{})
	for _, u := range req.ListUUIDs {
		reqUUIDs[u] = struct{}{}
	}

	// Get subscription from teh DB.
	subs, err := a.core.GetSubscriptions(0, subUUID, false)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("public.errorFetchingLists"))
	}

	// Filter the lists in the request against the subscriptions in the DB.
	unsubUUIDs := make([]string, 0, len(req.ListUUIDs))
	for _, s := range subs {
		if s.Type == models.ListTypePrivate {
			continue
		}
		if _, ok := reqUUIDs[s.UUID]; !ok {
			unsubUUIDs = append(unsubUUIDs, s.UUID)
		}
	}

	// Unsubscribe from lists.
	if err := a.core.UnsubscribeLists([]int{sub.ID}, nil, unsubUUIDs); err != nil {
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("public.errorProcessingRequest")))

	}

	return c.Render(http.StatusOK, tplMessage,
		makeMsgTpl(a.i18n.T("globals.messages.done"), "", a.i18n.T("public.prefsSaved")))
}

// OptinPage renders the double opt-in confirmation page that subscribers
// see when they click on the "Confirm subscription" button in double-optin
// notifications.
func (a *App) OptinPage(c echo.Context) error {
	var (
		subUUID    = c.Param("subUUID")
		confirm, _ = strconv.ParseBool(c.FormValue("confirm"))
		req        optinReq
	)
	if err := c.Bind(&req); err != nil {
		return err
	}

	// Validate list UUIDs if there are incoming UUIDs in the request.
	if len(req.ListUUIDs) > 0 {
		for _, l := range req.ListUUIDs {
			if !reUUID.MatchString(l) {
				return c.Render(http.StatusBadRequest, tplMessage,
					makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("globals.messages.invalidUUID")))
			}
		}
	}

	// Get the list of subscription lists where the subscriber hasn't confirmed.
	lists, err := a.core.GetSubscriberLists(0, subUUID, nil, req.ListUUIDs, models.SubscriptionStatusUnconfirmed, "")
	if err != nil {
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorFetchingLists")))
	}

	// There are no lists to confirm.
	if len(lists) == 0 {
		return c.Render(http.StatusOK, tplMessage,
			makeMsgTpl(a.i18n.T("public.noSubTitle"), "", a.i18n.Ts("public.noSubInfo")))
	}

	if confirm || !a.cfg.ShowOptinPage {
		return a.confirmOptinSubscription(c, subUUID, req.ListUUIDs, lists)
	}

	var out optinTpl
	out.Lists = lists
	out.SubUUID = subUUID
	out.Title = a.i18n.T("public.confirmOptinSubTitle")

	return c.Render(http.StatusOK, "optin", out)
}

func (a *App) confirmOptinSubscription(c echo.Context, subUUID string, listUUIDs []string, lists []models.List) error {
	if len(listUUIDs) == 0 {
		listUUIDs = make([]string, 0, len(lists))
		for _, l := range lists {
			listUUIDs = append(listUUIDs, l.UUID)
		}
	}

	meta := models.JSON{}
	if a.cfg.Privacy.RecordOptinIP {
		if h := c.Request().Header.Get("X-Forwarded-For"); h != "" {
			meta["optin_ip"] = h
		} else if h := c.Request().RemoteAddr; h != "" {
			meta["optin_ip"] = strings.Split(h, ":")[0]
		}
	}

	if err := a.core.ConfirmOptionSubscription(subUUID, listUUIDs, meta); err != nil {
		a.log.Printf("error confirming opt-in subscription: %v", err)
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorProcessingRequest")))
	}

	return c.Render(http.StatusOK, tplMessage,
		makeMsgTpl(a.i18n.T("public.subConfirmedTitle"), "", a.i18n.Ts("public.subConfirmed")))
}

// SubscriptionFormPage handles subscription requests coming from public
// HTML subscription forms.
func (a *App) SubscriptionFormPage(c echo.Context) error {
	if !a.cfg.EnablePublicSubPage {
		return c.Render(http.StatusNotFound, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.invalidFeature")))
	}

	// Get all public lists from the DB.
	lists, err := a.core.GetLists(models.ListTypePublic, models.ListStatusActive, true, nil)
	if err != nil {
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorFetchingLists")))
	}

	// There are no public lists available for subscription.
	if len(lists) == 0 {
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.noListsAvailable")))
	}

	out := subFormTpl{}
	out.Title = a.i18n.T("public.sub")
	out.Lists = lists

	// Captcha configuration for template rendering.
	if a.cfg.Security.Captcha.Altcha.Enabled {
		out.Captcha.Enabled = true
		out.Captcha.Provider = "altcha"
		out.Captcha.Complexity = a.cfg.Security.Captcha.Altcha.Complexity
	} else if a.cfg.Security.Captcha.HCaptcha.Enabled {
		out.Captcha.Enabled = true
		out.Captcha.Provider = "hcaptcha"
		out.Captcha.Key = a.cfg.Security.Captcha.HCaptcha.Key
	}

	return c.Render(http.StatusOK, "subscription-form", out)
}

// SubscriptionForm handles subscription requests coming from public
// HTML subscription forms.
func (a *App) SubscriptionForm(c echo.Context) error {
	if !a.cfg.EnablePublicSubPage {
		return echo.NewHTTPError(http.StatusNotFound, a.i18n.T("public.invalidFeature"))

	}

	// If there's a nonce value, a bot could've filled the form.
	if c.FormValue("nonce") != "" {
		return echo.NewHTTPError(http.StatusBadGateway, a.i18n.T("public.invalidFeature"))
	}

	// Process CAPTCHA.
	if a.captcha.IsEnabled() {
		var val string

		// Get the appropriate captcha response field based on provider.
		switch a.captcha.GetProvider() {
		case captcha.ProviderHCaptcha:
			val = c.FormValue("h-captcha-response")
		case captcha.ProviderAltcha:
			val = c.FormValue("altcha")
		default:
			return c.Render(http.StatusBadRequest, tplMessage,
				makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("public.invalidCaptcha")))
		}

		if val == "" {
			return c.Render(http.StatusBadRequest, tplMessage,
				makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("public.invalidCaptcha")))
		}

		err, ok := a.captcha.Verify(val)
		if err != nil {
			a.log.Printf("captcha request failed: %v", err)
		}

		if !ok {
			return c.Render(http.StatusBadRequest, tplMessage,
				makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.T("public.invalidCaptcha")))
		}
	}

	hasOptin, err := a.processSubForm(c)
	if err != nil {
		e, ok := err.(*echo.HTTPError)
		if !ok {
			return err
		}

		return c.Render(e.Code, tplMessage, makeMsgTpl(a.i18n.T("public.errorTitle"), "", fmt.Sprintf("%s", e.Message)))
	}

	// Redirect to a custom page if a trusted '?next' is set.
	if nextURL := strings.TrimSpace(c.FormValue("next")); nextURL != "" {
		for _, d := range a.cfg.Security.TrustedURLs {
			if d != "*" && nextURL == d {
				return c.Redirect(http.StatusSeeOther, nextURL)
			}
		}
	}

	// If there were double optin lists, show the opt-in pending message instead of
	// the subscription confirmation message.
	msg := "public.subConfirmed"
	if hasOptin {
		msg = "public.subOptinPending"
	}

	return c.Render(http.StatusOK, tplMessage, makeMsgTpl(a.i18n.T("public.subTitle"), "", a.i18n.Ts(msg)))
}

// PublicSubscription handles subscription requests coming from public
// API calls.
func (a *App) PublicSubscription(c echo.Context) error {
	if !a.cfg.EnablePublicSubPage {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("public.invalidFeature"))
	}

	hasOptin, err := a.processSubForm(c)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{struct {
		HasOptin bool `json:"has_optin"`
	}{hasOptin}})
}

// LinkRedirect redirects a link UUID to its original underlying link
// after recording the link click for a particular subscriber in the particular
// campaign. These links are generated by {{ TrackLink }} tags in campaigns.
func (a *App) LinkRedirect(c echo.Context) error {
	var (
		linkUUID = c.Param("linkUUID")
		campUUID = c.Param("campUUID")
		subUUID  = c.Param("subUUID")

		url string
		err error
	)

	switch {
	// If tracking is globally disabled, resolve the URL without recording a click.
	case a.cfg.Privacy.DisableTracking:
		url, err = a.core.GetLinkURL(linkUUID)

	default:
		// If individual tracking is disabled, do not record the subscriber ID.
		if !a.cfg.Privacy.IndividualTracking {
			subUUID = ""
		}

		// Fork (visual tracking) -- exclude dummy hits (template previews, archive
		// pages) from click registration, mirroring RegisterCampaignView's exclusion:
		// the individual-tracking reassignment above runs first, so with tracking OFF
		// an anonymous archive click still records — upstream parity with the pixel.
		if campUUID == dummyUUID || subUUID == dummyUUID {
			url, err = a.core.GetLinkURL(linkUUID)
		} else {
			// Fork (location stats) -- the CDN-resolved country code, NULL when absent/invalid.
			// Fork (client stats) -- the classified User-Agent token, NULL when absent/junk; never the raw UA.
			url, err = a.core.RegisterCampaignLinkClick(linkUUID, campUUID, subUUID, viewerCountry(c), viewerClient(c))
		}
	}
	if err != nil {
		e := err.(*echo.HTTPError)
		return c.Render(e.Code, tplMessage, makeMsgTpl(a.i18n.T("public.errorTitle"), "", e.Error()))
	}

	// Fork (click tracking, CLICK-TRACKING-SPEC §3.2) -- a dynamic link (stored as its
	// unexpanded expression) is resolved for the clicking subscriber now; UTM parameters are
	// appended at redirect time. The click above is already recorded either way.
	dest := a.resolveLinkDestination(url, linkUUID, campUUID, subUUID)
	if dest == "" {
		return c.Render(http.StatusNotFound, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.linkUnavailable")))
	}

	return c.Redirect(http.StatusTemporaryRedirect, dest)
}

// resolveLinkDestination turns a stored link URL into the redirect destination: a dynamic
// expression is rendered against the subscriber (D1/D4) and validated (D3), falling back to the
// campaign's brand site (D2) when it cannot be; then UTM tagging applies (D7). subUUID must
// already reflect the individual-tracking setting ("" when off). Returns "" only when a
// dynamic link has no fallback at all (the public error page).
func (a *App) resolveLinkDestination(url, linkUUID, campUUID, subUUID string) string {
	camp, haveCamp := a.linkFallbacks.campaignByUUID(campUUID)

	if linkresolve.IsDynamic(url) {
		dest := ""
		canResolve := a.cfg.Privacy.IndividualTracking && subUUID != "" && subUUID != dummyUUID
		if canResolve {
			if sub, err := a.core.GetSubscriber(0, subUUID, ""); err != nil {
				a.log.Printf("link %s (campaign %s): subscriber not found for dynamic link, falling back", linkUUID, campUUID)
			} else if d, err := linkresolve.Resolve(url, &sub); err == nil {
				dest = d
			} else if e, ok := err.(*linkresolve.Error); ok && e.Kind == linkresolve.KindEmpty {
				// The common case: this subscriber has no value for the attribute.
				a.log.Printf("link %s (campaign %d): dynamic link empty for subscriber, falling back", linkUUID, camp.ID)
			} else {
				// Never the subscriber's email -- logs are read more widely than the click.
				a.log.Printf("warn: link %s (campaign %d): dynamic link could not be resolved: %v", linkUUID, camp.ID, err)
			}
		}
		if dest == "" {
			if haveCamp {
				dest = a.linkFallbacks.forCampaign(&camp)
			} else {
				dest = a.linkFallbacks.forCampaign(nil)
			}
		}
		if dest == "" {
			return ""
		}
		url = dest
	}

	if haveCamp {
		url = a.linkFallbacks.applyUTM(url, camp)
	}
	return url
}

// Fork (location stats, integrations LOCATION-STATS-SPEC D1/D4) -- the viewer's country for the
// tracking handlers comes from the CloudFront-Viewer-Country header the CDN adds on the pixel and
// click-redirect behaviors. Those behaviors forward an allow-list of request headers: a tracking
// handler that starts reading another header or cookie gets nothing through the edge. No IP is
// read or stored for this feature.
const hdrViewerCountry = "CloudFront-Viewer-Country"

// viewerCountry returns the normalized country code of the request, or "" (stored as NULL).
func viewerCountry(c echo.Context) string {
	return normalizeCountry(c.Request().Header.Get(hdrViewerCountry))
}

// normalizeCountry accepts only a two-letter ASCII code after trimming and upper-casing; anything
// else is "" (unknown). The length and ASCII checks run BEFORE upper-casing, so a non-ASCII letter
// that upper-cases to ASCII (dotless i, long s) is refused rather than folded into a code.
func normalizeCountry(s string) string {
	s = strings.TrimSpace(s)
	if len(s) != 2 {
		return ""
	}
	for i := 0; i < 2; i++ {
		if b := s[i] | 0x20; b < 'a' || b > 'z' {
			return ""
		}
	}
	return strings.ToUpper(s)
}

// Fork (client stats, integrations CLIENT-STATS-SPEC D1/D2) -- the email client (rendering
// environment) of a tracking request, classified from its User-Agent into a CLOSED vocabulary at
// insert time. The CDN's pixel and click-redirect behaviors already forward User-Agent (the same
// origin-request allow-list as the country). Only the token is stored: the raw User-Agent is never
// persisted or logged (unbounded cardinality, a fingerprinting surface). A click's User-Agent is
// the browser the link opened in, not the mail client -- hence the browser-* tokens.
//
// The token set is the contract shared with the frontend's frontend/src/clientRows.mjs label map:
// extending the vocabulary means touching classifyClient, its test table and clientRows in the
// same change (D2: deliberately not pinned by a cross-language test).
const (
	clientGmailProxy     = "gmail-proxy"
	clientYahoo          = "yahoo"
	clientOutlookWindows = "outlook-windows"
	clientOutlookMac     = "outlook-mac"
	clientThunderbird    = "thunderbird"
	clientOutlookMobile  = "outlook-mobile"
	clientAppleMail      = "apple-mail"
	clientBrowserIOS     = "browser-ios"
	clientBrowserAndroid = "browser-android"
	clientBrowserWindows = "browser-windows"
	clientBrowserMac     = "browser-mac"
	clientBrowserLinux   = "browser-linux"
	clientOther          = "other"
)

// viewerClient returns the classified client token of the request, or "" (stored as NULL).
func viewerClient(c echo.Context) string {
	return classifyClient(c.Request().UserAgent())
}

// classifyClient maps a User-Agent to one vocabulary token, "" (unknown, stored NULL) for an
// empty or junk value. It is total: plain substring tests over the bytes, no regex, no indexing
// that can go out of range, so no input panics. Ordered first-match rules: image proxies, then
// named mail clients (incl. the Outlook mobile apps, before the browser rows since the Android
// app's UA contains "Android"), then Apple Mail's terminal WebKit signature, then browsers by
// platform, then "other" for any UA seen but not classified (its growth is the signal to extend
// this table). Matching is case-insensitive.
func classifyClient(ua string) string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return ""
	}
	// Junk: a real User-Agent is visible ASCII (RFC 9110 field-value). Control bytes, non-ASCII
	// and invalid UTF-8 carry no signal -- unknown, not "other".
	letters := false
	for i := 0; i < len(ua); i++ {
		b := ua[i]
		if b < 0x20 || b > 0x7e {
			return ""
		}
		if l := b | 0x20; l >= 'a' && l <= 'z' {
			letters = true
		}
	}
	if !letters {
		return ""
	}

	s := strings.ToLower(ua)
	has := func(sub string) bool { return strings.Contains(s, sub) }

	switch {
	// Image proxies.
	case has("googleimageproxy"):
		return clientGmailProxy
	case has("yahoomailproxy"), has("yahoo") && has("mailproxy"):
		return clientYahoo

	// Named mail clients.
	case has("msoffice"), has("microsoft office") && has("outlook") && !has("macintosh"):
		return clientOutlookWindows
	case has("outlook-mac"), has("outlook") && has("macintosh"):
		return clientOutlookMac
	case has("thunderbird/"):
		return clientThunderbird
	case has("outlook-ios"), has("outlook-android"):
		return clientOutlookMobile
	case isAppleMailUA(s):
		return clientAppleMail

	// Browsers (clicks, webmail pixel fetches), by platform.
	case (has("iphone") || has("ipad") || has("ipod")) && (has("safari") || has("crios")):
		return clientBrowserIOS
	case has("android"):
		return clientBrowserAndroid
	case has("windows nt") && has("mozilla/"):
		return clientBrowserWindows
	case has("macintosh") && (has("chrome") || has("firefox") || has("version/") && has("safari")):
		return clientBrowserMac
	case (has("x11") || has("linux")) && has("mozilla/"):
		return clientBrowserLinux
	}
	return clientOther
}

// isAppleMailUA reports the Apple Mail / Mail Privacy Protection signature on a lower-cased UA:
// an Apple platform and AppleWebKit, with the UA TERMINATING at "(KHTML, like Gecko)", optionally
// followed only by a "Mobile/<build>" token (the Mac MPP form and the on-device iOS Mail form).
// Any further product token (FxiOS, DuckDuckGo, FBAN/FBIOS, Safari, Version/, ...) is a browser or
// an in-app WebView and falls through to the browser rows. MPP and real Apple Mail opens cannot be
// told apart by UA (CLIENT-STATS-SPEC D3) -- one bucket, labelled as such in the UI.
func isAppleMailUA(s string) bool {
	if !strings.Contains(s, "applewebkit/") ||
		!(strings.Contains(s, "macintosh") || strings.Contains(s, "iphone") || strings.Contains(s, "ipad") || strings.Contains(s, "ipod")) {
		return false
	}
	const sig = "(khtml, like gecko)"
	i := strings.LastIndex(s, sig)
	if i < 0 {
		return false
	}
	rest := strings.TrimSpace(s[i+len(sig):])
	if rest == "" {
		return true
	}
	build, ok := strings.CutPrefix(rest, "mobile/")
	if !ok || build == "" {
		return false
	}
	for i := 0; i < len(build); i++ {
		if b := build[i]; !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9') {
			return false
		}
	}
	return true
}

// RegisterCampaignView registers a campaign view which comes in
// the form of an pixel image request. Regardless of errors, this handler
// should always render the pixel image bytes. The pixel URL is generated by
// the {{ TrackView }} template tag in campaigns.
func (a *App) RegisterCampaignView(c echo.Context) error {
	// If tracking is globally disabled, return the pixel without recording.
	if a.cfg.Privacy.DisableTracking {
		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.Blob(http.StatusOK, "image/png", pixelPNG)
	}

	// If individual tracking is disabled, do not record the subscriber ID.
	subUUID := c.Param("subUUID")
	if !a.cfg.Privacy.IndividualTracking {
		subUUID = ""
	}

	// Exclude dummy hits from template previews.
	campUUID := c.Param("campUUID")
	if campUUID != dummyUUID && subUUID != dummyUUID {
		// Fork (location stats) -- the CDN-resolved country code, NULL when absent/invalid.
		// Fork (client stats) -- the classified User-Agent token, NULL when absent/junk; never the raw UA.
		if err := a.core.RegisterCampaignView(campUUID, subUUID, viewerCountry(c), viewerClient(c)); err != nil {
			a.log.Printf("error registering campaign view: %s", err)
		}
	}

	c.Response().Header().Set("Cache-Control", "no-cache")
	return c.Blob(http.StatusOK, "image/png", pixelPNG)
}

// SelfExportSubscriberData pulls the subscriber's profile, list subscriptions,
// campaign views and clicks and produces a JSON report that is then e-mailed
// to the subscriber. This is a privacy feature and the data that's exported
// is dependent on the configuration.
func (a *App) SelfExportSubscriberData(c echo.Context) error {
	// Is export allowed?
	if !a.cfg.Privacy.AllowExport {
		return c.Render(http.StatusBadRequest, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.invalidFeature")))
	}

	// Get the subscriber's data. A single query that gets the profile,
	// list subscriptions, campaign views, and link clicks. Names of
	// private lists are replaced with "Private list".
	subUUID := c.Param("subUUID")
	data, b, err := a.exportSubscriberData(0, subUUID, a.cfg.Privacy.Exportable)
	if err != nil {
		a.log.Printf("error exporting subscriber data: %s", err)
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorProcessingRequest")))
	}

	// Prepare the attachment e-mail.
	var msg bytes.Buffer
	if err := notifs.Tpls.ExecuteTemplate(&msg, notifs.TplSubscriberData, data); err != nil {
		a.log.Printf("error compiling notification template '%s': %v", notifs.TplSubscriberData, err)
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorProcessingRequest")))
	}

	// TODO: GetTplSubject should be moved to a utils package.
	subject, body := notifs.GetTplSubject(a.i18n.Ts("email.data.title"), msg.Bytes())

	// E-mail the data as a JSON attachment to the subscriber.
	const fname = "data.json"
	if err := a.emailMsgr.Push(models.Message{
		From:    a.cfg.FromEmail,
		To:      []string{data.Email},
		Subject: subject,
		Body:    body,
		Attachments: []models.Attachment{
			{
				Name:    fname,
				Content: b,
				Header:  manager.MakeAttachmentHeader(fname, "base64", "application/json"),
			},
		},
	}); err != nil {
		a.log.Printf("error e-mailing subscriber profile: %s", err)
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorProcessingRequest")))
	}

	return c.Render(http.StatusOK, tplMessage,
		makeMsgTpl(a.i18n.T("public.dataSentTitle"), "", a.i18n.T("public.dataSent")))
}

// WipeSubscriberData allows a subscriber to delete their data. The
// profile and subscriptions are deleted, while the campaign_views and link
// clicks remain as orphan data unconnected to any subscriber.
func (a *App) WipeSubscriberData(c echo.Context) error {
	// Is wiping allowed?
	if !a.cfg.Privacy.AllowWipe {
		return c.Render(http.StatusBadRequest, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.invalidFeature")))
	}

	subUUID := c.Param("subUUID")
	if err := a.core.DeleteSubscribers(nil, []string{subUUID}); err != nil {
		a.log.Printf("error wiping subscriber data: %s", err)
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorProcessingRequest")))
	}

	return c.Render(http.StatusOK, tplMessage,
		makeMsgTpl(a.i18n.T("public.dataRemovedTitle"), "", a.i18n.T("public.dataRemoved")))
}

// AltchaChallenge generates a challenge for Altcha captcha.
func (a *App) AltchaChallenge(c echo.Context) error {
	// Check if Altcha is enabled.
	if !a.captcha.IsEnabled() || a.captcha.GetProvider() != captcha.ProviderAltcha {
		return echo.NewHTTPError(http.StatusNotFound, "captcha not enabled")
	}

	// Generate challenge.
	out, err := a.captcha.GenerateChallenge()
	if err != nil {
		a.log.Printf("error generating altcha challenge: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Error generating challenge")
	}

	// Return the challenge as JSON.
	c.Response().Header().Set("Content-Type", "application/json")
	return c.String(http.StatusOK, out)
}

// drawTransparentImage draws a transparent PNG of given dimensions
// and returns the PNG bytes.
func drawTransparentImage(h, w int) []byte {
	var (
		img = image.NewRGBA(image.Rect(0, 0, w, h))
		out = &bytes.Buffer{}
	)
	_ = png.Encode(out, img)

	return out.Bytes()
}

// processSubForm processes an incoming form/public API subscription request.
// The bool indicates whether there was subscription to an optin list so that
// an appropriate message can be shown.
func (a *App) processSubForm(c echo.Context) (bool, error) {
	// Get and validate fields.
	var req struct {
		Name          string   `form:"name" json:"name"`
		Email         string   `form:"email" json:"email"`
		FormListUUIDs []string `form:"l" json:"list_uuids"`
	}
	if err := c.Bind(&req); err != nil {
		return false, err
	}

	if len(req.FormListUUIDs) == 0 {
		return false, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("public.noListsSelected"))
	}

	// Validate fields.
	if len(req.Email) > 1000 {
		return false, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("subscribers.invalidEmail"))
	}

	em, err := a.importer.SanitizeEmail(req.Email)
	if err != nil {
		return false, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req.Email = em

	// Fork (subscriber names) -- an empty name stays empty (never the address's local part).
	req.Name = strings.TrimSpace(req.Name)
	if len(req.Name) > stdInputMaxLen {
		return false, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("subscribers.invalidName"))
	}

	listUUIDs := pq.StringArray(req.FormListUUIDs)

	// Fetch the list types and ensure that they are not private.
	listTypes, err := a.core.GetListTypes(nil, req.FormListUUIDs)
	if err != nil {
		return false, echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("%s", err.(*echo.HTTPError).Message))
	}

	for _, t := range listTypes {
		if t == models.ListTypePrivate {
			return false, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidUUID"))
		}
	}

	// Insert the subscriber into the DB.
	_, hasOptin, err := a.core.InsertSubscriber(models.Subscriber{
		Name:   req.Name,
		Email:  req.Email,
		Status: models.SubscriberStatusEnabled,
	}, nil, listUUIDs, false, true, false)
	if err == nil {
		return hasOptin, nil
	}

	// Insert returned an error. Examine it.
	var lastErr = err

	// Subscriber already exists. Update subscriptions in the DB.
	if e, ok := err.(*echo.HTTPError); ok && e.Code == http.StatusConflict {
		// Get the subscriber from the DB by their email.
		sub, err := a.core.GetSubscriber(0, "", req.Email)
		if err != nil {
			return false, err
		}

		// Update the subscriber's subscriptions in the DB.
		_, hasOptin, err := a.core.UpdateSubscriberWithLists(sub.ID, sub, nil, listUUIDs, false, false, true, nil, true)
		if err == nil {
			return hasOptin, nil
		}
		lastErr = err
	}

	// Something else went wrong.
	if e, ok := lastErr.(*echo.HTTPError); ok {
		return false, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("%s", e.Message))
	}
	return false, echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("public.errorProcessingRequest"))
}
