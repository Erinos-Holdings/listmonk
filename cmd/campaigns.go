package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/manager"
	"github.com/knadh/listmonk/internal/notifs"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"gopkg.in/volatiletech/null.v6"
)

// campReq is a wrapper over the Campaign model for receiving
// campaign creation and update data from APIs.
type campReq struct {
	models.Campaign

	// This overrides Campaign.Lists to receive and
	// write a list of int IDs during creation and updation.
	// Campaign.Lists is JSONText for sending lists children
	// to the outside world.
	ListIDs []int `json:"lists"`

	MediaIDs []int `json:"media"`

	// This is only relevant to campaign test requests.
	SubscriberEmails pq.StringArray `json:"subscribers"`
}

// campContentReq wraps params coming from API requests for converting
// campaign content formats.
type campContentReq struct {
	models.Campaign
	From string `json:"from"`
	To   string `json:"to"`
}

var (
	reFromAddress = regexp.MustCompile(`((.+?)\s)?<(.+?)@(.+?)>`)
	reSlug        = regexp.MustCompile(`[^\p{L}\p{M}\p{N}]`)
)

// GetCampaigns handles retrieval of campaigns.
func (a *App) GetCampaigns(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	var (
		hasAllPerm     = user.HasPerm(auth.PermCampaignsGetAll)
		permittedLists []int
	)

	if !hasAllPerm {
		// Either the user has campaigns:get_all permissions and can view all campaigns,
		// or the campaigns are filtered by the lists the user has get|manage access to.
		hasAllPerm, permittedLists = user.GetPermittedLists(auth.PermTypeGet | auth.PermTypeManage)
	}

	var (
		pg = a.pg.NewFromURL(c.Request().URL.Query())

		status    = c.QueryParams()["status"]
		tags      = c.QueryParams()["tag"]
		query     = strings.TrimSpace(c.FormValue("query"))
		orderBy   = c.FormValue("order_by")
		order     = c.FormValue("order")
		noBody, _ = strconv.ParseBool(c.QueryParam("no_body"))
	)

	// Fork (evergreen) -- optional scope on the campaign kind (absent = no filter).
	evergreen, err := a.parseEvergreenParam(c)
	if err != nil {
		return err
	}

	// Fork (list filter) -- optional, repeatable list_id (absent = no filter). Intersects
	// with the permitted-list scoping above; it never widens what a user may see.
	listIDs, err := parseStringIDs(c.QueryParams()["list_id"])
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}

	// Query and retrieve campaigns from the DB.
	res, total, err := a.core.QueryCampaigns(query, status, tags, orderBy, order, hasAllPerm, permittedLists, evergreen, listIDs, pg.Offset, pg.Limit)
	if err != nil {
		return err
	}

	// Remove the body from the response if requested.
	if noBody {
		for i := range res {
			res[i].Body = ""
			res[i].BodySource.Valid = false
		}
	}

	// Paginate the response.
	if len(res) == 0 {
		return c.JSON(http.StatusOK, okResp{models.PageResults{Results: []models.Campaign{}}})
	}

	out := models.PageResults{
		Query:   query,
		Results: res,
		Total:   total,
		Page:    pg.Page,
		PerPage: pg.PerPage,
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// GetCampaign handles retrieval of campaigns.
func (a *App) GetCampaign(c echo.Context) error {
	// Get the campaign ID.
	id := getID(c)

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeGet, id, c); err != nil {
		return err
	}

	// Get the campaign from the DB.
	out, err := a.core.GetCampaign(id, "", "")
	if err != nil {
		return err
	}

	// Blank out the body if requested.
	noBody, _ := strconv.ParseBool(c.QueryParam("no_body"))
	if noBody {
		out.Body = ""
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// PreviewCampaign renders the HTML preview of a campaign body.
func (a *App) PreviewCampaign(c echo.Context) error {
	// Get the campaign ID.
	id := getID(c)

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeGet, id, c); err != nil {
		return err
	}

	var (
		isPost      = c.Request().Method == http.MethodPost
		contentType = c.FormValue("content_type")
		tplID, _    = strconv.Atoi(c.FormValue("template_id"))
	)
	// For visual content, template ID for previewing is irrelevant.
	if contentType == models.CampaignContentTypeVisual || tplID < 1 {
		tplID = 0
	}

	// Get the campaign from the DB for previewing with the `template_body` field.
	camp, err := a.core.GetCampaignForPreview(id, tplID)
	if err != nil {
		return err
	}

	// There's a body in the request to preview instead of the body in the DB.
	if isPost {
		camp.ContentType = contentType
		camp.Body = c.FormValue("body")

		// Fork (multi-language campaigns) -- preview the attribs on screen (lang, preheader),
		// not the last saved ones; the footer conditional reads .Campaign.Attribs.lang.
		if raw := c.FormValue("attribs"); raw != "" {
			var attribs models.JSON
			if err := json.Unmarshal([]byte(raw), &attribs); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("subscribers.invalidJSON"))
			}
			camp.Attribs = attribs
		}

		// For visual campaigns, template body from the DB shouldn't be used.
		if contentType == models.CampaignContentTypeVisual {
			camp.TemplateBody = ""
		}
	}

	// Use a dummy campaign ID to prevent views and clicks from {{ TrackView }}
	// and {{ TrackLink }} being registered on preview.
	camp.UUID = dummySubscriber.UUID
	if err := camp.CompileTemplate(a.manager.TemplateFuncs(&camp)); err != nil {
		a.log.Printf("error compiling template: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("templates.errorCompiling", "error", err.Error()))
	}

	// Render the message body.
	msg, err := a.manager.NewCampaignMessage(&camp, dummySubscriber)
	if err != nil {
		a.log.Printf("error rendering message: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("templates.errorRendering", "error", err.Error()))
	}

	// Plaintext headers for plain body.
	if camp.ContentType == models.CampaignContentTypePlain {
		return c.String(http.StatusOK, string(msg.Body()))
	}

	return c.HTML(http.StatusOK, string(msg.Body()))
}

// PreviewCampaignArchive renders the public campaign archives page.
func (a *App) PreviewCampaignArchive(c echo.Context) error {
	// Get the campaign ID.
	id := getID(c)

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeGet, id, c); err != nil {
		return err
	}

	// Fetch the campaign body from the DB.
	tplID, _ := strconv.Atoi(c.FormValue("template_id"))
	camp, err := a.core.GetCampaignForPreview(id, tplID)
	if err != nil {
		return err
	}

	camp.ArchiveMeta = json.RawMessage([]byte(c.FormValue("archive_meta")))

	// "Compile" the campaign template with appropriate data.
	res, err := a.compileArchiveCampaigns([]models.Campaign{camp})
	if err != nil {
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorFetchingCampaign")))
	}

	// Render the campaign body.
	out := res[0].Campaign
	msg, err := a.manager.NewCampaignMessage(out, res[0].Subscriber)
	if err != nil {
		a.log.Printf("error rendering campaign: %v", err)
		return c.Render(http.StatusInternalServerError, tplMessage,
			makeMsgTpl(a.i18n.T("public.errorTitle"), "", a.i18n.Ts("public.errorFetchingCampaign")))
	}

	return c.HTML(http.StatusOK, string(msg.Body()))
}

// CampaignContent handles campaign content (body) format conversions.
func (a *App) CampaignContent(c echo.Context) error {
	var camp campContentReq
	if err := c.Bind(&camp); err != nil {
		return err
	}

	// Convert formats, eg: markdown to HTML.
	out, err := camp.ConvertContent(camp.From, camp.To)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// CreateCampaign handles campaign creation.
// Newly created campaigns are always drafts.
func (a *App) CreateCampaign(c echo.Context) error {
	var o campReq
	if err := c.Bind(&o); err != nil {
		return err
	}

	// Filter lists against the current user's permitted lists.
	user := auth.GetUser(c)
	o.ListIDs = user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, o.ListIDs)

	// If the campaign's 'opt-in', prepare a default message.
	switch o.Type {
	case models.CampaignTypeOptin:
		op, err := a.makeOptinCampaignMessage(o)
		if err != nil {
			return err
		}
		o = op
	case "":
		o.Type = models.CampaignTypeRegular
	}

	if o.Messenger == "" {
		o.Messenger = "email"
	}

	// Validate.
	if c, err := a.validateCampaignFields(o); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	} else {
		o = c
	}

	if o.ArchiveTemplateID.Valid && o.ArchiveTemplateID.Int != 0 {
		o.ArchiveTemplateID = o.TemplateID
	}

	out, err := a.core.CreateCampaign(o.Campaign, o.ListIDs, o.MediaIDs)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateCampaign handles campaign modification.
// Campaigns that are done cannot be modified.
func (a *App) UpdateCampaign(c echo.Context) error {
	// Get the campaign ID.
	id := getID(c)

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeManage, id, c); err != nil {
		return err
	}

	// Retrieve the campaign from the DB.
	cm, err := a.core.GetCampaign(id, "", "")
	if err != nil {
		return err
	}

	if !canEditCampaign(cm.Status) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("campaigns.cantUpdate"))
	}

	// Fork (multi-language campaigns) -- the stored language, read before attribs are cleared
	// below, for the started-campaign lock after binding.
	prevLang := cm.Lang()

	// Clear attribs to avoid merging old and new values as json.Unmarshal in JSON.scan() merges maps,
	// merging values already in the DB and incoming values. If this is nil, the DB value is kept
	// (fork -- update-campaign COALESCEs a NULL bind; upstream wrote JSON null and wiped it).
	cm.Attribs = nil

	// Read the incoming params into the existing campaign fields from the DB.
	// This allows updating of values that have been sent whereas fields
	// that are not in the request retain the old values.
	o := campReq{Campaign: cm}
	if err := c.Bind(&o); err != nil {
		return err
	}

	// Filter lists against the current user's permitted lists.
	user := auth.GetUser(c)
	o.ListIDs = user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, o.ListIDs)

	if c, err := a.validateCampaignFields(o); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	} else {
		o = c
	}

	// Fork (evergreen) -- once a campaign has started, its evergreen flag and its lists
	// are frozen (a paused evergreen flipped to regular would blast the whole list on
	// resume; a swapped list would welcome its whole post-watermark membership).
	if core.EvergreenLockedChange(cm, o.Evergreen, o.ListIDs) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("campaigns.evergreenLocked"))
	}
	// Fork (multi-language campaigns) -- the language is frozen once started (the checkpoint
	// window was computed for the old population). Clone to change it.
	if core.LangLockedChange(cm, prevLang, o.Attribs) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("campaigns.langLocked"))
	}

	// Fork (footer guard) -- a scheduled or paused campaign sends again with no further
	// guarded status transition, so the edit itself is the guarded event. Pre-persist:
	// a refusal leaves the last compliant version stored. Checked on the INCOMING body,
	// not the stored one.
	if manager.FooterGuardOnUpdate(cm.Status) {
		if err := a.footerGuardOnEdit(id, o); err != nil {
			return err
		}
	}

	out, err := a.core.UpdateCampaign(id, o.Campaign, o.ListIDs, o.MediaIDs)
	if err != nil {
		return err
	}

	// Ephemeral send-quality warnings on the saved state (Gmail clip size, embed lint).
	out.Warnings = a.campaignWarningsByID(id)

	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateCampaignStatus handles campaign status modification.
func (a *App) UpdateCampaignStatus(c echo.Context) error {
	// Get the campaign ID.
	id := getID(c)

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeManage, id, c); err != nil {
		return err
	}

	req := struct {
		Status string `json:"status"`
	}{}
	if err := c.Bind(&req); err != nil {
		return err
	}

	// Fork (footer guard) -- refuse a start or a schedule whose rendered body carries no
	// unsubscribe link (or is missing required footer content). BEFORE the DB write: the
	// refusal must leave the campaign in its previous status, not merely report on one
	// already dispatched. Resumes and automation restarts are never guarded.
	if err := a.footerGuardOnStatus(id, req.Status); err != nil {
		return err
	}

	// Update the campaign status in the DB.
	out, err := a.core.UpdateCampaignStatus(id, req.Status)
	if err != nil {
		return err
	}

	// If the campaign is being stopped, send the signal to the manager to stop it in flight.
	if req.Status == models.CampaignStatusPaused || req.Status == models.CampaignStatusCancelled {
		a.manager.StopCampaign(id)
	}

	// Ephemeral send-quality warnings when the campaign is actually being dispatched
	// (started or scheduled): the render checks, plus the preheader adoption nudge —
	// empty stays allowed (transactional-style sends may not want one), it just warns.
	if req.Status == models.CampaignStatusRunning || req.Status == models.CampaignStatusScheduled {
		w := a.campaignWarningsByID(id)
		if out.Preheader() == "" {
			w = append(w, manager.WarnNoPreheader)
		}
		// Fork (multi-language campaigns) -- a language-scoped broadcast with nobody to send
		// to would otherwise finish instantly with sent 0 and nothing saying why. Warns, never
		// blocks. Evergreens are skipped (their audience is future joiners).
		if lang := out.Lang(); lang != "" && !out.Evergreen {
			if n, err := a.core.CampaignLangAudience(id); err != nil {
				a.log.Printf("error counting language audience for campaign %d: %v", id, err)
			} else if n == 0 {
				w = append(w, a.i18n.Ts("campaigns.warnNoLangAudience", "lang", strings.ToUpper(lang)))
			}
		}
		out.Warnings = w
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateCampaignArchive handles campaign status modification.
func (a *App) UpdateCampaignArchive(c echo.Context) error {
	id := getID(c)

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeManage, id, c); err != nil {
		return err
	}

	req := struct {
		Archive     bool        `json:"archive"`
		TemplateID  int         `json:"archive_template_id"`
		Meta        models.JSON `json:"archive_meta"`
		ArchiveSlug string      `json:"archive_slug"`
	}{}
	if err := c.Bind(&req); err != nil {
		return err
	}

	if req.ArchiveSlug != "" {
		// Format the slug to be alpha-numeric-dash.
		s := strings.ToLower(req.ArchiveSlug)
		s = strings.TrimSpace(reSlug.ReplaceAllString(s, " "))
		s = regexpSpaces.ReplaceAllString(s, "-")
		req.ArchiveSlug = s
	}

	if err := a.core.UpdateCampaignArchive(id, req.Archive, req.TemplateID, req.Meta, req.ArchiveSlug); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{req})
}

// DeleteCampaign handles campaign deletion.
// Only scheduled campaigns that have not started yet can be deleted.
func (a *App) DeleteCampaign(c echo.Context) error {
	// Get the campaign ID.
	id := getID(c)

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeManage, id, c); err != nil {
		return err
	}

	// Delete the campaign from the DB.
	if err := a.core.DeleteCampaign(id); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// parseEvergreenParam reads the fork's optional `evergreen` query param on
// GET/DELETE /api/campaigns. "" (absent) returns nil (no filter); any other
// value must parse as a bool, otherwise a 400 is returned.
func (a *App) parseEvergreenParam(c echo.Context) (*bool, error) {
	v := c.QueryParam("evergreen")
	if v == "" {
		return nil, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
	}
	return &b, nil
}

// DeleteCampaigns deletes multiple campaigns by IDs or by query.
func (a *App) DeleteCampaigns(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	var (
		hasAllPerm     = user.HasPerm(auth.PermCampaignsManageAll)
		permittedLists []int
	)

	if !hasAllPerm {
		// Either the user has campaigns:manage_all permissions and can manage all campaigns,
		// or the campaigns are filtered by the lists the user has get|manage access to.
		hasAllPerm, permittedLists = user.GetPermittedLists(auth.PermTypeGet | auth.PermTypeManage)
	}

	var (
		ids   []int
		query string
		all   bool
	)

	// Check for IDs in query params.
	if len(c.Request().URL.Query()["id"]) > 0 {
		var err error
		ids, err = parseStringIDs(c.Request().URL.Query()["id"])
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
		}
	} else {
		// Check for query param.
		query = strings.TrimSpace(c.FormValue("query"))
		all = c.FormValue("all") == "true"
	}

	// Validate that either IDs or query is provided.
	if len(ids) == 0 && (query == "" && !all) {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", "id or query required"))
	}

	// Fork (evergreen) -- the by-query delete carries the list page's scope so a
	// "select all" on Broadcasts never deletes the automations (or vice versa).
	evergreen, err := a.parseEvergreenParam(c)
	if err != nil {
		return err
	}

	// Delete the campaigns from the DB.
	if err := a.core.DeleteCampaigns(ids, query, hasAllPerm, permittedLists, evergreen); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// GetRunningCampaignStats returns stats of a given set of campaign IDs.
func (a *App) GetRunningCampaignStats(c echo.Context) error {
	// Get the running campaign stats from the DB.
	out, err := a.core.GetRunningCampaignStats()
	if err != nil {
		return err
	}

	if len(out) == 0 {
		return c.JSON(http.StatusOK, okResp{[]struct{}{}})
	}

	// Compute rate.
	for i, c := range out {
		if c.Started.Valid && c.UpdatedAt.Valid {
			diff := max(int(c.UpdatedAt.Time.Sub(c.Started.Time).Minutes()), 1)

			rate := c.Sent / diff
			if rate > c.Sent || rate > c.ToSend {
				rate = c.Sent
			}

			// Rate since the starting of the campaign.
			out[i].NetRate = rate

			// Realtime running rate over the last minute.
			out[i].Rate = a.manager.GetCampaignStats(c.ID).SendRate
		}
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// TestCampaign handles the sending of a campaign message to
// arbitrary subscribers for testing.
func (a *App) TestCampaign(c echo.Context) error {
	// Get the campaign ID.
	id := getID(c)

	// Check if the user has access to the campaign.
	if err := a.checkCampaignPerm(auth.PermTypeManage, id, c); err != nil {
		return err
	}

	// Get and validate fields.
	var req campReq
	if err := c.Bind(&req); err != nil {
		return err
	}

	// Validate.
	if c, err := a.validateCampaignFields(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	} else {
		req = c
	}
	if len(req.SubscriberEmails) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("campaigns.noSubsToTest"))
	}

	// Sanitize subscriber e-mails.
	for i := range req.SubscriberEmails {
		req.SubscriberEmails[i] = strings.ToLower(strings.TrimSpace(req.SubscriberEmails[i]))
	}

	// Get the subscribers from the DB by their e-mails.
	subs, err := a.core.GetSubscribersByEmail(req.SubscriberEmails)
	if err != nil {
		return err
	}

	// Count BEFORE the permission filter below, which reuses this slice's backing array.
	numResolved := len(subs)

	// Exclude subscribers from lists that the user doesn't have access to.
	user := auth.GetUser(c)
	validSubs := subs[:0]
	for _, s := range subs {
		if err := a.hasSubPerm(user, []int{s.ID}); err == nil {
			validSubs = append(validSubs, s)
		}
	}
	subs = validSubs

	// A test send drops recipients silently, and NOTHING server-side records it. Confirmed twice:
	// once as the total case (a UI test send produced no send at all, while the same campaign and
	// recipient driven over the API delivered normally) and once as the partial case (a UI test
	// send to TWO addresses produced exactly ONE send). The loop below is synchronous and its only
	// log statement sits in the failure branch, so a successful send is never recorded and a
	// shortened recipient list leaves no trace whatsoever.
	//
	// The DISCREPANCY BETWEEN THESE THREE COUNTS IS THE DIAGNOSIS: requested is what the browser
	// actually posted (a Buefy taginput only commits an address on Enter/comma/Tab, so "typed two,
	// sent one" shows up here), resolved is what existed in `subscribers`, and permitted is what
	// survived list permissions.
	//
	// Bounded on purpose: fine for a handful of test addresses, not a PII firehose at audience
	// scale. Test sends are the only caller.
	a.log.Printf("campaign %d test send: requested=%d resolved=%d permitted=%d addresses=%s",
		id, len(req.SubscriberEmails), numResolved, len(subs), truncateList(req.SubscriberEmails, 10))

	// No subscribers.
	if len(subs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("campaigns.noKnownSubsToTest"))
	}

	// Get the campaign from the DB for previewing.
	tplID, _ := strconv.Atoi(c.FormValue("template_id"))
	camp, err := a.core.GetCampaignForPreview(id, tplID)
	if err != nil {
		return err
	}

	// Override certain values from the DB with incoming values.
	camp.Name = req.Name
	camp.Subject = req.Subject
	camp.FromEmail = req.FromEmail
	camp.Body = req.Body
	camp.AltBody = req.AltBody
	camp.Messenger = req.Messenger
	camp.ContentType = req.ContentType
	camp.Headers = req.Headers
	// Attribs carry the preheader, which should test-send as edited (like the subject),
	// not as last saved. nil-guarded so API callers that omit attribs keep the DB value.
	if req.Attribs != nil {
		camp.Attribs = req.Attribs
	}
	camp.TemplateID = req.TemplateID
	for _, id := range req.MediaIDs {
		if id > 0 {
			camp.MediaIDs = append(camp.MediaIDs, int64(id))
		}
	}

	// Ephemeral send-quality warnings on the posted payload, computed BEFORE the send
	// loop — sendTestMessage's LoadInlineImages rewrites data-embed imgs to cid: in
	// place, which would hide them from the embed lint. The test send is the earliest
	// point a campaign is rendered on the real payload, so this is the earliest catch.
	warnings := a.renderWarnings(camp)

	// Send the test messages.
	for _, s := range subs {
		sub := s

		if err := a.sendTestMessage(sub, &camp); err != nil {
			a.log.Printf("error sending test message to %s: %v", sub.Email, err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				a.i18n.Ts("campaigns.errorSendTest", "error", err.Error()))
		}

		// Per-recipient success. Without this only failures are recorded, so a send that
		// silently never happened is indistinguishable from one that worked.
		a.log.Printf("campaign %d test send: sent to %s (subscriber %d)", id, sub.Email, sub.ID)
	}

	return c.JSON(http.StatusOK, okResp{struct {
		Warnings []string `json:"warnings"`
	}{warnings}})
}

// truncateList renders at most n items of a string slice for logging, reporting how many were
// elided. Keeps the test-send diagnostics above bounded.
func truncateList(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, ", ")
	}

	return fmt.Sprintf("%s (+%d more)", strings.Join(items[:n], ", "), len(items)-n)
}

// GetCampaignViewAnalytics retrieves view counts for a campaign.
func (a *App) GetCampaignViewAnalytics(c echo.Context) error {
	ids, err := parseStringIDs(c.Request().URL.Query()["id"])
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}

	if len(ids) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.missingFields", "name", "`id`"))
	}

	// Ensure the user has access to campaigns via lists.
	for _, id := range ids {
		if err := a.checkCampaignPerm(auth.PermTypeGet, id, c); err != nil {
			return err
		}
	}

	var (
		typ  = c.Param("type")
		from = c.QueryParams().Get("from")
		to   = c.QueryParams().Get("to")
	)
	if !strHasLen(from, 10, 30) || !strHasLen(to, 10, 30) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("analytics.invalidDates"))
	}

	// Campaign link stats.
	if typ == "links" {
		out, err := a.core.GetCampaignAnalyticsLinks(ids, typ, from, to)
		if err != nil {
			return err
		}

		return c.JSON(http.StatusOK, okResp{out})
	}

	// Get the analytics numbers from the DB for the campaigns.
	out, err := a.core.GetCampaignAnalyticsCounts(ids, typ, from, to)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// renderWarnings renders the campaign the way preview does — dummy subscriber, and
// the campaign UUID swapped for a dummy so {{ TrackView }}/{{ TrackLink }} register
// nothing — and returns non-blocking send-quality warnings (Gmail clip size measured
// on the QP-encoded output, embedded-image lint) for embedding in API responses.
// camp is taken by value so the caller's copy keeps its real UUID and compiled state.
// Never fails the calling request: render errors just skip the warnings.
func (a *App) renderWarnings(camp models.Campaign) []string {
	if camp.Messenger != "email" || camp.ContentType == models.CampaignContentTypePlain {
		return nil
	}

	camp.UUID = dummySubscriber.UUID
	if err := camp.CompileTemplate(a.manager.TemplateFuncs(&camp)); err != nil {
		a.log.Printf("error compiling campaign %d for warnings: %v", camp.ID, err)
		return nil
	}

	msg, err := a.manager.NewCampaignMessage(&camp, dummySubscriber)
	if err != nil {
		a.log.Printf("error rendering campaign %d for warnings: %v", camp.ID, err)
		return nil
	}

	return manager.RenderWarnings(msg.Body())
}

// footerGuardOnStatus applies the blocking footer guard to a campaign status change.
// It is a no-op for every transition but draft|scheduled -> running|scheduled, so a
// resume (paused -> running) and an automation restart are never blocked.
func (a *App) footerGuardOnStatus(id int, next string) error {
	// Cheap bail before any fetch or render -- pause, cancel and save-as-draft pay nothing.
	if next != models.CampaignStatusRunning && next != models.CampaignStatusScheduled {
		return nil
	}

	camp, err := a.core.GetCampaignForPreview(id, 0)
	if err != nil {
		return err
	}

	if !manager.FooterGuardOnStatus(camp.Status, next) {
		return nil
	}

	return a.footerGuard(camp)
}

// footerGuardOnEdit applies the blocking footer guard to the INCOMING campaign of an
// update, rendered against the template it is being saved with.
func (a *App) footerGuardOnEdit(id int, o campReq) error {
	// The template the campaign is being saved with, not the stored one. For visual
	// campaigns the template body is irrelevant (CompileTemplate ignores it), same as
	// in preview.
	tplID := 0
	if o.ContentType != models.CampaignContentTypeVisual && o.TemplateID.Int > 0 {
		tplID = o.TemplateID.Int
	}

	camp, err := a.core.GetCampaignForPreview(id, tplID)
	if err != nil {
		return err
	}

	// Overlay the incoming payload the way the test send does -- the guard must judge
	// what is about to be stored, not what is stored.
	camp.Subject = o.Subject
	camp.Body = o.Body
	camp.AltBody = o.AltBody
	camp.ContentType = o.ContentType
	camp.Messenger = o.Messenger
	if o.Attribs != nil {
		camp.Attribs = o.Attribs
	}

	return a.footerGuard(camp)
}

// footerGuard renders camp and turns any footer problem -- including a body that fails
// to compile or render, which is a refusal and never a pass -- into a 400.
func (a *App) footerGuard(camp models.Campaign) error {
	problems, err := a.manager.CheckFooter(camp, dummySubscriber, a.cfg.RequiredFooterMarkers)
	if err != nil {
		a.log.Printf("footer guard could not render campaign %d: %v", camp.ID, err)
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("campaigns.footerMissing", "error", err.Error()))
	}

	if len(problems) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("campaigns.footerMissing", "error", strings.Join(problems, "; ")))
	}

	return nil
}

// campaignWarningsByID fetches a stored campaign with its template body (the same
// query preview uses) and computes renderWarnings for it.
func (a *App) campaignWarningsByID(id int) []string {
	camp, err := a.core.GetCampaignForPreview(id, 0)
	if err != nil {
		a.log.Printf("error fetching campaign %d for warnings: %v", id, err)
		return nil
	}
	return a.renderWarnings(camp)
}

// sendTestMessage takes a campaign and a subscriber and sends out a sample campaign message.
func (a *App) sendTestMessage(sub models.Subscriber, camp *models.Campaign) error {
	if err := a.manager.LoadInlineImages(camp); err != nil {
		a.log.Printf("error loading inline images: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := camp.CompileTemplate(a.manager.TemplateFuncs(camp)); err != nil {
		a.log.Printf("error compiling template: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			a.i18n.Ts("templates.errorCompiling", "error", err.Error()))
	}

	// Create a sample campaign message.
	msg, err := a.manager.NewCampaignMessage(camp, sub)
	if err != nil {
		a.log.Printf("error rendering message: %v", err)
		return echo.NewHTTPError(http.StatusNotFound, a.i18n.Ts("templates.errorRendering", "error", err.Error()))
	}

	return a.manager.PushCampaignMessage(msg)
}

// validateCampaignFields validates incoming campaign field values.
func (a *App) validateCampaignFields(c campReq) (campReq, error) {
	// Normalise ONCE, so what is compared below is what actually gets stored. Trimming only at
	// comparison time would let " hello@liyorahair.com" pass a check asserting it equals the
	// list's `from:` tag and then be stored with the space -- snapshotted for the life of the
	// campaign (from_email is never re-read), emitted in the SMTP From header, and no longer
	// matching the from_addresses routing key, which normalises with TrimSpace+ToLower.
	// It also stops " " counting as a non-empty From and skipping the derivation below.
	c.FromEmail = strings.TrimSpace(c.FromEmail)

	// Resolve the brand mapping the campaign's target lists describe, BEFORE the From address is
	// defaulted below. See cmd/campaigns_brand.go for what this enforces and why.
	brand, err := a.resolveBrandMapping(c.ListIDs)
	if err != nil {
		return c, err
	}

	if c.FromEmail == "" {
		// DERIVE BEFORE DEFAULTING. Left as a plain fallback to a.cfg.FromEmail, an API client
		// that omits the From on a tagged-list campaign would get app.from_email filled in here
		// and then be rejected below by an error naming an address it never sent. Deriving first
		// makes omitting the From the CORRECT way to call the API: name the lists, and the brand
		// follows.
		//
		// This also covers listmonk's own generated opt-in campaigns, which never set a From
		// (makeOptinCampaignMessage sets only the body) and would otherwise inherit the default
		// and then be refused by validation listmonk itself had just triggered.
		if brand.mapped {
			c.FromEmail = brand.fromEmail
		} else {
			c.FromEmail = a.cfg.FromEmail
		}
	} else if !reFromAddress.Match([]byte(c.FromEmail)) {
		if _, err := a.importer.SanitizeEmail(c.FromEmail); err != nil {
			return c, errors.New(a.i18n.T("campaigns.fieldInvalidFromEmail"))
		}
	}

	// A tagged list constrains the From address to exactly its `from:` value.
	//
	// The tag carries the FULL From header -- `Curated <hello@curatedfor.you>`, not a bare address
	// -- because that is what the recipient's inbox actually shows. A bare address made every
	// brand appear as "hello" in Gmail, which falls back to the local part when no display name
	// is present.
	//
	// An exact comparison is still correct: the tag is the single source of truth, and anything
	// that differs from it was hand-typed, which is the thing this removes. So a client passing a
	// bare address on a tagged list is rejected ON PURPOSE, and so is one passing a different
	// display name. That 400 is this rule working.
	if brand.mapped && c.FromEmail != brand.fromEmail {
		return c, errors.New(a.i18n.Ts("campaigns.brandFromMismatch",
			"from", c.FromEmail, "expected", brand.fromEmail, "list", brand.listName))
	}

	// Derive the `brand` SES message tag from the same mapping, so ONE mapping populates both
	// halves of what used to be hand-typed. Enforced here rather than only in the editor because
	// this is the only path an API or scripted send goes through -- and because the editor is
	// deleted at listmonk v7. An unmapped campaign gets the DEFAULT slug, never no tag.
	brandSlug := defaultBrandSlug
	if brand.mapped {
		brandSlug = brand.brand
	}
	c.Headers = setBrandTagHeader(c.Headers, brandSlug)

	if !strHasLen(c.Name, 1, stdInputMaxLen) {
		return c, errors.New(a.i18n.T("campaigns.fieldInvalidName"))
	}

	// Larger char limit for subject as it can contain {{ go templating }} logic.
	if !strHasLen(c.Subject, 1, 5000) {
		return c, errors.New(a.i18n.T("campaigns.fieldInvalidSubject"))
	}

	// If no content-type is specified, default to richtext.
	if c.ContentType != models.CampaignContentTypeRichtext &&
		c.ContentType != models.CampaignContentTypeHTML &&
		c.ContentType != models.CampaignContentTypePlain &&
		c.ContentType != models.CampaignContentTypeVisual &&
		c.ContentType != models.CampaignContentTypeMarkdown {
		c.ContentType = models.CampaignContentTypeRichtext
	}

	if c.ContentType != models.CampaignContentTypeVisual {
		c.BodySource.Valid = false
	}

	// If there's a "send_at" date, it should be in the future.
	if c.SendAt.Valid {
		if c.SendAt.Time.Before(time.Now()) {
			return c, errors.New(a.i18n.T("campaigns.fieldInvalidSendAt"))
		}
	}

	if len(c.ListIDs) == 0 {
		return c, errors.New(a.i18n.T("campaigns.fieldInvalidListIDs"))
	}

	// Fork (evergreen) -- one list (join-time semantics are per list), never scheduled
	// (an evergreen is not claimed by next-campaigns, so a scheduled one would never
	// start), never opt-in, non-negative delay.
	if c.Evergreen {
		if c.Type == models.CampaignTypeOptin {
			return c, errors.New(a.i18n.T("campaigns.evergreenNotOptin"))
		}
		if len(c.ListIDs) != 1 {
			return c, errors.New(a.i18n.T("campaigns.evergreenOneList"))
		}
		if c.SendAt.Valid {
			return c, errors.New(a.i18n.T("campaigns.evergreenNoSchedule"))
		}
	}
	if c.SendDelaySecs < 0 {
		return c, errors.New(a.i18n.T("campaigns.evergreenInvalidDelay"))
	}

	if !a.manager.HasMessenger(c.Messenger) {
		// If it's a specific SMTP, but it's no longer available (removed/disabled), fall back to general email messenger.
		if strings.HasPrefix(c.Messenger, "email-") {
			c.Messenger = "email"
		} else {
			return c, errors.New(a.i18n.Ts("campaigns.fieldInvalidMessenger", "name", c.Messenger))
		}
	}

	camp := models.Campaign{Body: c.Body, TemplateBody: tplTag}
	if err := c.CompileTemplate(a.manager.TemplateFuncs(&camp)); err != nil {
		return c, errors.New(a.i18n.Ts("campaigns.fieldInvalidBody", "error", err.Error()))
	}

	if len(c.Headers) == 0 {
		c.Headers = make([]map[string]string, 0)
	}

	// Validate and initialize attribs.
	if c.Attribs != nil {
		if _, err := json.Marshal(c.Attribs); err != nil {
			return c, errors.New(a.i18n.T("subscribers.invalidJSON"))
		}
	}
	// Fork (multi-language campaigns) -- attribs.lang is absent or one of models.CampaignLangs.
	if !models.NormalizeLang(c.Attribs) {
		return c, errors.New(a.i18n.Ts("campaigns.fieldInvalidLang", "langs", strings.Join(models.CampaignLangs, ", ")))
	}

	if len(c.ArchiveMeta) == 0 {
		c.ArchiveMeta = json.RawMessage("{}")
	}

	if c.ArchiveSlug.String != "" {
		// Format the slug to be alpha-numeric-dash.
		s := strings.ToLower(c.ArchiveSlug.String)
		s = strings.TrimSpace(reSlug.ReplaceAllString(s, " "))
		s = regexpSpaces.ReplaceAllString(s, "-")

		c.ArchiveSlug = null.NewString(s, true)
	} else {
		// If there's no slug set, set it to NULL in the DB.
		c.ArchiveSlug.Valid = false
	}

	return c, nil
}

// makeOptinCampaignMessage makes a default opt-in campaign message body.
func (a *App) makeOptinCampaignMessage(o campReq) (campReq, error) {
	if len(o.ListIDs) == 0 {
		return o, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("campaigns.fieldInvalidListIDs"))
	}

	// Fetch double opt-in lists from the given list IDs from the DB.
	lists, err := a.core.GetListsByOptin(o.ListIDs, models.ListOptinDouble)
	if err != nil {
		return o, err
	}

	// There are no double opt-in lists.
	if len(lists) == 0 {
		return o, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("campaigns.noOptinLists"))
	}

	// Construct the opt-in URL with list IDs.
	listIDs := url.Values{}
	for _, l := range lists {
		listIDs.Add("l", l.UUID)
	}
	// optinURLFunc := template.URL("{{ OptinURL }}?" + listIDs.Encode())
	optinURLAttr := template.HTMLAttr(fmt.Sprintf(`href="{{ OptinURL }}%s"`, listIDs.Encode()))

	// Prepare sample opt-in message for the campaign.
	var b bytes.Buffer

	if err := notifs.Tpls.ExecuteTemplate(&b, "optin-campaign", struct {
		Lists        []models.List
		OptinURLAttr template.HTMLAttr
	}{lists, optinURLAttr}); err != nil {
		a.log.Printf("error compiling 'optin-campaign' template: %v", err)
		return o, echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("templates.errorCompiling", "error", err.Error()))
	}

	o.Body = b.String()

	// Opt-in campaigns are generated by listmonk itself and this function sets only the body --
	// it never sets a From address. Derive one from the lists' `from:` tag so the confirmation
	// mail, which is the FIRST message a subscriber ever receives, comes from the right brand
	// rather than the global default.
	//
	// validateCampaignFields derives an empty From the same way, so this is belt-and-braces; it is
	// explicit here so the behaviour survives a reordering of the two. Any resolution error is
	// deliberately swallowed: validation re-resolves moments later and reports it properly.
	if o.FromEmail == "" {
		if brand, err := a.resolveBrandMapping(o.ListIDs); err == nil && brand.mapped {
			o.FromEmail = brand.fromEmail
		}
	}

	return o, nil
}

// checkCampaignPerm checks if the user has get or manage access to the given campaign.
// Either the user has blanket get_all/manage_all permissions, or the campaign
// belongs to lists that the user has access to.
func (a *App) checkCampaignPerm(types auth.PermType, id int, c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	perm := auth.PermCampaignsGet
	if types&auth.PermTypeGet != 0 {
		// It's a get request and there's a blanket get all permission.
		if user.HasPerm(auth.PermCampaignsGetAll) {
			return nil
		}
	} else {
		// It's a manage request and there's a blanket manage_all permission.
		if user.HasPerm(auth.PermCampaignsManageAll) {
			return nil
		}

		perm = auth.PermCampaignsManage
	}

	// There are no *_all campaign permissions. Instead, check if the user access
	// blanket get_all/manage_all list permissions. If yes, then the user can access
	// all campaigns. If there are no *_all permissions, then ensure that the
	// campaign belongs to the lists that the user has access to.
	if hasAllPerm, permittedListIDs := user.GetPermittedLists(auth.PermTypeGet | auth.PermTypeManage); !hasAllPerm {
		if ok, err := a.core.CampaignHasLists(id, permittedListIDs); err != nil {
			return err
		} else if !ok {
			return echo.NewHTTPError(http.StatusForbidden,
				a.i18n.Ts("globals.messages.permissionDenied", "name", perm))
		}
	}

	return nil
}

// canEditCampaign returns true if a campaign is in a status where updating
// its properties is allowed.
func canEditCampaign(status string) bool {
	return status == models.CampaignStatusDraft ||
		status == models.CampaignStatusPaused ||
		status == models.CampaignStatusScheduled
}
