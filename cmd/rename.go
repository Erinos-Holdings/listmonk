package main

// Fork (naming standard, integrations listmonk/NAMING-STANDARD-SPEC D3/D10/D13 §4.2/§4.3): the
// name-only rename endpoints and the name rule the create/update paths apply.
//
//	PUT /api/campaigns/:id/name   campaigns:manage_all + campaigns:review_structure
//	PUT /api/lists/:id/name       lists:manage_all + campaigns:review_structure (+ lists:journeys
//	                              for a journey-locked list)
//	PUT /api/templates/:id/name   templates:manage + campaigns:review_structure
//
// body {"name": "<new>"}. A rename writes the name ONLY, in every status (finished, cancelled,
// running included): never canEditCampaign, status, lists, body, attribs, from_email, schedule,
// the review guard or the send pipeline, and updated_at is not bumped. Marketing renames an
// editable record through the existing forms (which apply the rule and record the former name).

import (
	"net/http"
	"strings"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

type renameReq struct {
	Name string `json:"name"`
}

// nameRuleErr applies models.NameProblem as a 400 (nil when valid). brandSlug/listType matter for
// a list only; the brand's display name is read from its brands row.
func (a *App) nameRuleErr(kind, name, brandSlug, listType string) error {
	display := ""
	if kind == models.NameKindList && brandSlug != "" {
		b, ok, err := a.core.GetBrand(brandSlug)
		if err != nil {
			return err
		}
		if ok {
			display = models.BrandDisplayName(b.FromEmail)
		}
	}
	if key, args := models.NameProblem(kind, name, brandSlug, display, listType); key != "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts(key, args...))
	}
	return nil
}

// listReqBrand is the brand a list will carry after the request: the request's brand when it names
// one ("" clears it), else stored.
func listReqBrand(r listReq, stored string) string {
	if r.Brand != nil {
		return strings.TrimSpace(*r.Brand)
	}
	return stored
}

// listTypeOr is the list's type after the request: the request's when set, else stored, else
// private (the create default).
func listTypeOr(typ, stored string) string {
	if typ != "" {
		return typ
	}
	if stored != "" {
		return stored
	}
	return models.ListTypePrivate
}

// renameRefusal is the code-named guard (§4.2 step 2): a `CATALOG ` campaign or an `Official_`
// template is never renamed, and no record is renamed INTO a `CATALOG `/`Official_` name (those
// records are found by name).
func (a *App) renameRefusal(kind, stored, next string) error {
	if models.IsExemptName(kind, stored) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("naming.renameExempt", "name", stored))
	}
	n := strings.TrimSpace(next)
	if strings.HasPrefix(n, models.CatalogCampaignPrefix) || strings.HasPrefix(n, models.OfficialTemplatePrefix) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("naming.renameIntoExempt", "name", n))
	}
	return nil
}

// renameTarget is §4.2 step 3: a name byte-for-byte equal to the record's former_names name is
// accepted as-is (the reverse run restores names over 30 or carrying whitespace); every other name
// is trimmed and validated by the rule.
func (a *App) renameTarget(kind string, id int, name, brandSlug, listType string) (string, error) {
	former, ok, err := a.core.GetFormerName(kind, id)
	if err != nil {
		return "", err
	}
	if ok && name == former {
		return name, nil
	}
	n := strings.TrimSpace(name)
	if err := a.nameRuleErr(kind, n, brandSlug, listType); err != nil {
		return "", err
	}
	return n, nil
}

func (a *App) requireRenamePerm(c echo.Context) error {
	user := auth.GetUser(c)
	if !user.HasPerm(auth.PermCampaignsReviewStructure) {
		return echo.NewHTTPError(http.StatusForbidden, "permission denied: "+auth.PermCampaignsReviewStructure)
	}
	return nil
}

// as404 turns the core getters' not-found answer (upstream's 400) into the endpoint's 404 (§4.2
// step 1). The getters return 400 for nothing else.
func as404(err error) error {
	if he, ok := err.(*echo.HTTPError); ok && he.Code == http.StatusBadRequest {
		return echo.NewHTTPError(http.StatusNotFound, he.Message)
	}
	return err
}

func bindRename(c echo.Context) (string, error) {
	var r renameReq
	if err := c.Bind(&r); err != nil {
		return "", err
	}
	return r.Name, nil
}

// RenameCampaign handles PUT /api/campaigns/:id/name.
func (a *App) RenameCampaign(c echo.Context) error {
	if err := a.requireRenamePerm(c); err != nil {
		return err
	}
	id := getID(c)
	name, err := bindRename(c)
	if err != nil {
		return err
	}
	cm, err := a.core.GetCampaign(id, "", "")
	if err != nil {
		return as404(err)
	}
	if err := a.renameRefusal(models.NameKindCampaign, cm.Name, name); err != nil {
		return err
	}
	target, err := a.renameTarget(models.NameKindCampaign, id, name, "", "")
	if err != nil {
		return err
	}
	if err := a.core.RenameRecord(models.NameKindCampaign, id, target); err != nil {
		return err
	}
	out, err := a.core.GetCampaign(id, "", "")
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// RenameList handles PUT /api/lists/:id/name. Beside the rename permission it carries the list
// update path's guards: the caller's manage access to the list, the standard-journey-list lock
// (lists:journeys), the locked-list lock and no rename ONTO a locked list name.
func (a *App) RenameList(c echo.Context) error {
	if err := a.requireRenamePerm(c); err != nil {
		return err
	}
	user := auth.GetUser(c)
	id := getID(c)
	if err := user.HasListPerm(auth.PermTypeManage, id); err != nil {
		return err
	}
	name, err := bindRename(c)
	if err != nil {
		return err
	}
	cur, err := a.core.GetList(id, "")
	if err != nil {
		return as404(err)
	}
	if !canManageJourneys(user) && isJourneyLockedList(cur) {
		return a.journeyLockedErr(cur.Name)
	}
	if err := a.refuseLocked(id); err != nil {
		return err
	}
	if models.IsLockedListName(strings.TrimSpace(name)) || models.IsLockedListName(name) {
		return a.lockedListErr(name)
	}
	if err := a.renameRefusal(models.NameKindList, cur.Name, name); err != nil {
		return err
	}
	target, err := a.renameTarget(models.NameKindList, id, name, cur.Brand, cur.Type)
	if err != nil {
		return err
	}
	if err := a.core.RenameRecord(models.NameKindList, id, target); err != nil {
		return err
	}
	out, err := a.core.GetList(id, "")
	if err != nil {
		return err
	}
	if err := a.decorateList(&out, false); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// RenameTemplate handles PUT /api/templates/:id/name. An `Official_` template is never renamed
// and no template is renamed onto an `Official_` name (renameRefusal).
func (a *App) RenameTemplate(c echo.Context) error {
	if err := a.requireRenamePerm(c); err != nil {
		return err
	}
	id := getID(c)
	name, err := bindRename(c)
	if err != nil {
		return err
	}
	stored, err := a.core.GetTemplate(id, true)
	if err != nil {
		return as404(err)
	}
	if err := a.renameRefusal(models.NameKindTemplate, stored.Name, name); err != nil {
		return err
	}
	target, err := a.renameTarget(models.NameKindTemplate, id, name, "", "")
	if err != nil {
		return err
	}
	if err := a.core.RenameRecord(models.NameKindTemplate, id, target); err != nil {
		return err
	}
	out, err := a.core.GetTemplate(id, true)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}
