package main

import (
	"net/http"
	"strings"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// Fork (brand picker, integrations BRAND-PICKER-SPEC D2/D4). A list's brand mapping is a
// reference to a brands row; its brand:/from:/site: tags are the row's projection
// (models.BrandProjection), written by the server and stored verbatim by core's
// normalizeListTags. The readers (cmd/campaigns_brand.go, list_brand_tag(), the editor, the
// integrations scripts) keep reading the tags unchanged.
//
// Direct SQL writes bypass this by construction, as they bypass everything else; the
// campaign-save checks in campaigns_brand.go remain the backstop for them.

// listReq is the list API's request body: models.List plus the tri-state brand. Brand absent
// (nil) keeps the list's current projection on update and writes none on create; "" means an
// untagged list; a slug must name a brands row. The embedded List.Brand (the response field) is
// shadowed by this one in JSON decoding.
type listReq struct {
	models.List
	Brand *string `json:"brand"`
}

// listTagsFor applies the reserved-tag refusal and the brand projection to a request. It returns
// the tags to store and keepReserved (brand absent).
func (a *App) listTagsFor(r listReq) ([]string, bool, error) {
	free := make([]string, 0, len(r.Tags))
	for _, t := range r.Tags {
		if models.IsReservedListTag(t) {
			return nil, false, echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("lists.reservedTag", "tag", strings.TrimSpace(t)))
		}
		free = append(free, t)
	}

	if r.Brand == nil {
		return free, true, nil
	}
	slug := strings.TrimSpace(*r.Brand)
	if slug == "" {
		return free, false, nil
	}

	b, ok, err := a.core.GetBrand(slug)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("lists.brandUnknown", "brand", slug))
	}
	return append(free, models.BrandProjection(b)...), false, nil
}

// lockedListErr is the 409 for a locked list (models.LockedListNames).
func (a *App) lockedListErr(name string) error {
	return echo.NewHTTPError(http.StatusConflict, a.i18n.Ts("lists.lockedList", "name", name))
}

// refuseLocked returns the 409 when any of the lists is locked. Called AFTER the permission
// check, so it never reveals a list name to a caller without access to the list.
func (a *App) refuseLocked(ids ...int) error {
	names, err := a.core.LockedListsOf(ids)
	if err != nil {
		return err
	}
	if len(names) > 0 {
		return a.lockedListErr(names[0])
	}
	return nil
}

// refuseSecondLocked refuses giving a list a locked name while a list of that name exists: two
// copies would both be undeletable, and the integrations sync demands exactly one.
func (a *App) refuseSecondLocked(name string) error {
	if !models.IsLockedListName(name) {
		return nil
	}
	n, err := a.core.CountListsByName(name)
	if err != nil {
		return err
	}
	if n > 0 {
		return a.lockedListErr(name)
	}
	return nil
}
