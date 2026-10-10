package core

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// Fork (brand picker, integrations BRAND-PICKER-SPEC D1/D2). The brands table is the home of a
// brand's sending identity; a list's brand:/from:/site: tags are its projection
// (models.BrandProjection). Validation is the caller's (models.BrandProblem in cmd/brand_rows.go).

// GetBrands returns every brand row, sorted by slug, each with its display name.
func (c *Core) GetBrands() ([]models.Brand, error) {
	out := []models.Brand{}
	if err := c.q.GetBrands.Select(&out); err != nil {
		c.log.Printf("error fetching brands: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "brands", "error", pqErrMsg(err)))
	}
	for i := range out {
		fillBrand(&out[i])
	}
	return out, nil
}

// fillBrand sets a row's computed fields: the display name, the bare address (PERSONA-FROM-SPEC
// D1) and a non-nil persona set (JSON `[]`, never null).
func fillBrand(b *models.Brand) {
	b.DisplayName = models.BrandDisplayName(b.FromEmail)
	b.Address = models.BareAddress(b.FromEmail)
	if b.Personas == nil {
		b.Personas = pq.StringArray{}
	}
}

// UpdateBrandPersonas replaces a brand row's persona set (validated by the caller) and returns
// the row. found is false when no row has this slug.
func (c *Core) UpdateBrandPersonas(slug string, personas []string) (models.Brand, bool, error) {
	if personas == nil {
		personas = []string{}
	}
	var out string
	if err := c.q.UpdateBrandPersonas.Get(&out, slug, pq.StringArray(personas)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Brand{}, false, nil
		}
		c.log.Printf("error updating brand personas: %v", err)
		return models.Brand{}, false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "brand", "error", pqErrMsg(err)))
	}
	return c.GetBrand(out)
}

// GetCampaignsCarryingFrom returns the campaigns whose From is exactly from and whose status
// blocks a persona's removal (models.PersonaBlockingStatuses), by id.
func (c *Core) GetCampaignsCarryingFrom(from string) ([]models.PersonaCampaign, error) {
	out := []models.PersonaCampaign{}
	if err := c.q.GetCampaignsCarryingFrom.Select(&out, from, pq.StringArray(models.PersonaBlockingStatuses)); err != nil {
		c.log.Printf("error fetching campaigns carrying a From: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaigns}", "error", pqErrMsg(err)))
	}
	return out, nil
}

// GetBrand returns one brand row; ok is false when no row has this slug (compared as stored).
func (c *Core) GetBrand(slug string) (models.Brand, bool, error) {
	var b models.Brand
	if err := c.q.GetBrand.Get(&b, slug); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Brand{}, false, nil
		}
		c.log.Printf("error fetching brand: %v", err)
		return models.Brand{}, false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "brand", "error", pqErrMsg(err)))
	}
	fillBrand(&b)
	return b, true, nil
}

// CreateBrand inserts a validated brand row. created is false when the slug is taken -- exactly
// or by case (the LOWER(slug) unique index).
func (c *Core) CreateBrand(b models.Brand) (models.Brand, bool, error) {
	var slug string
	if err := c.q.CreateBrand.Get(&slug, b.Slug, b.FromEmail, b.Site.String); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Brand{}, false, nil
		}
		c.log.Printf("error creating brand: %v", err)
		return models.Brand{}, false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "brand", "error", pqErrMsg(err)))
	}
	out, _, err := c.GetBrand(slug)
	return out, true, err
}

// UpdateBrand changes a brand row's From and site and re-projects them onto every list of the
// brand, in ONE transaction: a failed re-projection leaves the row and every list unchanged.
// found is false when no row has this slug.
func (c *Core) UpdateBrand(b models.Brand) (models.Brand, bool, error) {
	tx, err := c.db.Beginx()
	if err != nil {
		c.log.Printf("error updating brand: %v", err)
		return models.Brand{}, false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "brand", "error", pqErrMsg(err)))
	}
	defer tx.Rollback()

	var slug string
	if err := tx.Stmtx(c.q.UpdateBrand).Get(&slug, b.Slug, b.FromEmail, b.Site.String); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Brand{}, false, nil
		}
		c.log.Printf("error updating brand: %v", err)
		return models.Brand{}, false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "brand", "error", pqErrMsg(err)))
	}
	if _, err := tx.Stmtx(c.q.ReprojectBrandLists).Exec(b.Slug, pq.StringArray(models.BrandProjection(b))); err != nil {
		c.log.Printf("error re-projecting brand %s onto its lists: %v", b.Slug, err)
		return models.Brand{}, false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.lists}", "error", pqErrMsg(err)))
	}
	if err := tx.Commit(); err != nil {
		c.log.Printf("error committing brand update: %v", err)
		return models.Brand{}, false, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "brand", "error", pqErrMsg(err)))
	}

	out, _, err := c.GetBrand(b.Slug)
	return out, true, err
}
