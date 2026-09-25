package main

import (
	"errors"
	"html/template"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

const (
	// tplTag is the template tag that should be present in a template
	// as the placeholder for campaign bodies.
	tplTag = `{{ template "content" . }}`

	dummyTpl = `
		<p>Hi there</p>
		<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Duis et elit ac elit sollicitudin condimentum non a magna. Sed tempor mauris in facilisis vehicula. Aenean nisl urna, accumsan ac tincidunt vitae, interdum cursus massa. Interdum et malesuada fames ac ante ipsum primis in faucibus. Aliquam varius turpis et turpis lacinia placerat. Aenean id ligula a orci lacinia blandit at eu felis. Phasellus vel lobortis lacus. Suspendisse leo elit, luctus sed erat ut, venenatis fermentum ipsum. Donec bibendum neque quis.</p>

		<h3>Sub heading</h3>
		<p>Nam luctus dui non placerat mattis. Morbi non accumsan orci, vel interdum urna. Duis faucibus id nunc ut euismod. Curabitur et eros id erat feugiat fringilla in eget neque. Aliquam accumsan cursus eros sed faucibus.</p>

		<p>Here is a link to <a href="https://listmonk.app" target="_blank">listmonk</a>.</p>`
)

var (
	regexpTplTag = regexp.MustCompile(`{{(\s+)?template\s+?"content"(\s+)?\.(\s+)?}}`)
)

// GetTemplate handles the retrieval of a template
func (a *App) GetTemplate(c echo.Context) error {
	// If no_body is true, blank out the body of the template from the response.
	noBody, _ := strconv.ParseBool(c.QueryParam("no_body"))

	// Get the template from the DB.
	id := getID(c)
	out, err := a.core.GetTemplate(id, noBody)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// GetTemplates handles retrieval of templates.
func (a *App) GetTemplates(c echo.Context) error {
	// If no_body is true, blank out the body of the template from the response.
	noBody, _ := strconv.ParseBool(c.QueryParam("no_body"))

	// Fetch templates from the DB.
	out, err := a.core.GetTemplates("", noBody)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// PreviewTemplate renders the HTML preview of a template in the DB.
func (a *App) PreviewTemplate(c echo.Context) error {
	// Fetch one template from the DB.
	id := getID(c)
	tpl, err := a.core.GetTemplate(id, false)
	if err != nil {
		return err
	}

	// Render the template.
	out, err := a.previewTemplate(tpl)
	if err != nil {
		return err
	}

	return c.HTML(http.StatusOK, string(out))
}

// PreviewTemplateBody renders the HTML preview of a template given its type and body.
func (a *App) PreviewTemplateBody(c echo.Context) error {
	tpl := models.Template{
		Type: c.FormValue("template_type"),
		Body: c.FormValue("body"),
	}

	// Body is posted with the request.
	if tpl.Type == "" {
		tpl.Type = models.TemplateTypeCampaign
	}

	if tpl.Type == models.TemplateTypeCampaign && !regexpTplTag.MatchString(tpl.Body) {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("templates.placeholderHelp", "placeholder", tplTag))
	}

	// Render the template.
	out, err := a.previewTemplate(tpl)
	if err != nil {
		return err
	}

	return c.HTML(http.StatusOK, string(out))
}

// CreateTemplate handles template creation.
func (a *App) CreateTemplate(c echo.Context) error {
	var o models.Template
	if err := c.Bind(&o); err != nil {
		return err
	}
	if err := a.validateTemplate(&o); err != nil {
		return err
	}
	if err := a.guardOfficialTemplate(nil, &o); err != nil {
		return err
	}

	// Subject is only relevant for fixed tx templates. For campaigns,
	// the subject changes per campaign and is on models.Campaign.
	var funcs template.FuncMap
	if o.Type == models.TemplateTypeCampaign || o.Type == models.TemplateTypeCampaignVisual {
		o.Subject = ""
		funcs = a.manager.TemplateFuncs(nil)
	} else {
		funcs = a.manager.GenericTemplateFuncs()
	}

	// Compile the template and validate.
	if err := o.Compile(funcs); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Create the template the in the DB.
	out, err := a.core.CreateTemplate(o.Name, o.Type, o.Subject, []byte(o.Body), o.BodySource, o.Brand, o.Lang)
	if err != nil {
		return err
	}

	// If it's a transactional template, cache it in the manager
	// to be used for arbitrary incoming tx message pushes.
	if o.Type == models.TemplateTypeTx {
		a.manager.CacheTpl(out.ID, &o)
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateTemplate handles template modification.
func (a *App) UpdateTemplate(c echo.Context) error {
	var o models.Template
	if err := c.Bind(&o); err != nil {
		return err
	}
	if err := a.validateTemplate(&o); err != nil {
		return err
	}

	// Fork (official footer): the refusals read the stored row (rename off the prefix).
	id := getID(c)
	stored, err := a.core.GetTemplate(id, true)
	if err != nil {
		return err
	}
	if err := a.guardOfficialTemplate(&stored, &o); err != nil {
		return err
	}

	// Subject is only relevant for fixed tx templates. For campaigns,
	// the subject changes per campaign and is on models.Campaign.
	var funcs template.FuncMap
	if o.Type == models.TemplateTypeCampaign || o.Type == models.TemplateTypeCampaignVisual {
		o.Subject = ""
		funcs = a.manager.TemplateFuncs(nil)
	} else {
		funcs = a.manager.GenericTemplateFuncs()
	}

	// Compile the template and validate.
	if err := o.Compile(funcs); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Update the template in the DB.
	out, err := a.core.UpdateTemplate(id, o.Name, o.Subject, []byte(o.Body), o.BodySource, o.Brand, o.Lang)
	if err != nil {
		return err
	}

	// If it's a transactional template, cache it.
	if out.Type == models.TemplateTypeTx {
		a.manager.CacheTpl(out.ID, &o)
	}

	return c.JSON(http.StatusOK, okResp{out})

}

// TemplateSetDefault handles template modification.
func (a *App) TemplateSetDefault(c echo.Context) error {
	// Update the template in the DB.
	id := getID(c)
	if err := a.core.SetDefaultTemplate(id); err != nil {
		return err
	}

	return a.GetTemplates(c)
}

// DeleteTemplate handles template deletion.
func (a *App) DeleteTemplate(c echo.Context) error {
	// Delete the template from the DB.
	id := getID(c)

	// Fork (official footer, OFFICIAL-FOOTER-SPEC D10): an official block is removed from the
	// seed file first -- the seed re-creates it at the next bootstrap anyway. A lookup failure
	// falls through to the delete, which reports a missing id as before.
	if stored, err := a.core.GetTemplate(id, true); err == nil && models.IsOfficialTemplate(stored.Name) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("templates.officialDelete"))
	}

	if err := a.core.DeleteTemplate(id); err != nil {
		return err
	}

	// Delete cached in-memory template.
	a.manager.DeleteTpl(id)

	return c.JSON(http.StatusOK, okResp{true})
}

// officialTemplateRefusal is the pure half of the official-block rules (integrations
// OFFICIAL-FOOTER-SPEC D10): stored is the row being updated (nil on create), o the incoming
// template, all every stored template. It returns the i18n key of the refusal, or "".
//   - an official template may not be renamed away from the Official_ prefix;
//   - a create, or a rename, onto an Official_ name that already exists is refused (the
//     resolver needs one row per name; the templates table has no unique constraint);
//   - an official template's body_source may not hold an OfficialFooter block (a reference
//     must never recurse).
//
// `type` needs no guard: the update SQL never writes it.
func officialTemplateRefusal(stored *models.Template, o *models.Template, all []models.Template) string {
	if stored != nil && models.IsOfficialTemplate(stored.Name) && !models.IsOfficialTemplate(o.Name) {
		return "templates.officialRename"
	}
	if !models.IsOfficialTemplate(o.Name) {
		return ""
	}
	if stored == nil || stored.Name != o.Name {
		for _, t := range all {
			if t.Name == o.Name && (stored == nil || t.ID != stored.ID) {
				return "templates.officialDuplicate"
			}
		}
	}
	if o.BodySource.Valid && models.HasOfficialFooterBlock(o.BodySource.String) {
		return "templates.officialNested"
	}
	return ""
}

// guardOfficialTemplate applies officialTemplateRefusal as a 400. The template list is read only
// when an official name is involved.
func (a *App) guardOfficialTemplate(stored *models.Template, o *models.Template) error {
	if !models.IsOfficialTemplate(o.Name) && (stored == nil || !models.IsOfficialTemplate(stored.Name)) {
		return nil
	}

	var all []models.Template
	if models.IsOfficialTemplate(o.Name) {
		var err error
		if all, err = a.core.GetTemplates("", true); err != nil {
			return err
		}
	}

	switch key := officialTemplateRefusal(stored, o, all); key {
	case "":
		return nil
	case "templates.officialDuplicate":
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts(key, "name", o.Name))
	default:
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T(key))
	}
}

// compileTemplate validates template fields.
func (a *App) validateTemplate(o *models.Template) error {
	if !strHasLen(o.Name, 1, stdInputMaxLen) {
		return errors.New(a.i18n.T("campaigns.fieldInvalidName"))
	}

	if o.Type == models.TemplateTypeCampaign && !regexpTplTag.MatchString(o.Body) {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("templates.placeholderHelp", "placeholder", tplTag))
	}

	if o.Type == models.TemplateTypeTx && strings.TrimSpace(o.Subject) == "" {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.missingFields", "name", "subject"))
	}

	// Fork (editor polish, EDITOR-POLISH-SPEC D4): '' (no brand) or a slug matching the same
	// regex list tags and the theme proxy use (cmd/campaigns_brand.go:71 -> models.ReBrandSlug),
	// lowercase-folded — the frontend roster is already folded, so this only guards direct API
	// callers.
	if o.Brand != "" {
		brand := strings.ToLower(o.Brand)
		if !models.ReBrandSlug.MatchString(brand) {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.invalidFields", "name", "brand"))
		}
		o.Brand = brand
	}

	// Fork (LIST-GRID-SPEC D12): absent (stored "en" on create, kept on update) or exactly one of
	// models.CampaignLangs -- the same closed set, and the same exact match, as a campaign's.
	if o.Lang != "" && !models.IsCampaignLang(o.Lang) {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("campaigns.fieldInvalidLang", "langs", strings.Join(models.CampaignLangs, ", ")))
	}

	return nil
}

// previewTemplate renders the HTML preview of a template.
func (a *App) previewTemplate(tpl models.Template) ([]byte, error) {
	var out []byte
	if tpl.Type == models.TemplateTypeCampaign || tpl.Type == models.TemplateTypeCampaignVisual {
		camp := models.Campaign{
			UUID:         dummyUUID,
			Name:         a.i18n.T("templates.dummyName"),
			Subject:      a.i18n.T("templates.dummySubject"),
			FromEmail:    "dummy-campaign@listmonk.app",
			TemplateBody: tpl.Body,
			Body:         dummyTpl,
		}

		if err := camp.CompileTemplate(a.manager.TemplateFuncs(&camp)); err != nil {
			return nil, echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("templates.errorCompiling", "error", err.Error()))
		}

		// Render the message body.
		msg, err := a.manager.NewCampaignMessage(&camp, dummySubscriber)
		if err != nil {
			return nil, echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("templates.errorRendering", "error", err.Error()))
		}
		out = msg.Body()
	} else {
		// Compile transactional template.
		if err := tpl.Compile(a.manager.GenericTemplateFuncs()); err != nil {
			return nil, echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		m := models.TxMessage{
			Subject: tpl.Subject,
		}

		// Render the message.
		if err := m.Render(dummySubscriber, &tpl, a.manager.GenericTemplateFuncs()); err != nil {
			return nil, echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		out = m.Body
	}

	return out, nil
}
