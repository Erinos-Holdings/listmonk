package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	null "gopkg.in/volatiletech/null.v6"
)

// Fork (brand health, integrations BRAND-HEALTH-SPEC D1/D2/D6/D10). The fork stores and renders
// the per-brand, per-day health documents the integrations BrandHealth Lambda computes; it
// knows nothing of Google, DNSBLs or the brand registry, and validates only the keys it indexes
// (v, brand, domain, day, status, default). Everything else is stored verbatim.

const (
	brandHealthMaxRows     = 1000
	brandHealthMaxKeyLen   = 200
	brandHealthDefaultDays = 30
	brandHealthMaxDays     = 400
)

var (
	brandHealthStatuses = map[string]bool{"ok": true, "warn": true, "issues": true, "unknown": true}
	reISODay            = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// validateBrandHealthRows checks a PUT batch. It returns an error naming the first bad row and
// field, and refuses the whole batch on any one bad row (all-or-nothing), an empty batch, an
// oversized batch, or the same (brand, day) twice (which one upsert statement cannot apply).
func validateBrandHealthRows(rows []json.RawMessage) error {
	if len(rows) == 0 {
		return fmt.Errorf("rows is empty")
	}
	if len(rows) > brandHealthMaxRows {
		return fmt.Errorf("too many rows (%d > %d)", len(rows), brandHealthMaxRows)
	}

	seen := map[string]bool{}
	for i, raw := range rows {
		var d map[string]json.RawMessage
		if err := json.Unmarshal(raw, &d); err != nil || d == nil {
			return fmt.Errorf("row %d is not a JSON object", i)
		}

		var v float64
		if err := json.Unmarshal(d["v"], &v); err != nil || v != 1 {
			return fmt.Errorf("row %d: v must be 1", i)
		}

		brand, ok := jsonString(d["brand"])
		if !ok || strings.TrimSpace(brand) == "" || brand != strings.TrimSpace(brand) || len(brand) > brandHealthMaxKeyLen {
			return fmt.Errorf("row %d: brand must be a non-empty trimmed string", i)
		}
		domain, ok := jsonString(d["domain"])
		if !ok || strings.TrimSpace(domain) == "" || len(domain) > brandHealthMaxKeyLen {
			return fmt.Errorf("row %d: domain must be a non-empty string", i)
		}

		day, ok := jsonString(d["day"])
		if !ok || !reISODay.MatchString(day) {
			return fmt.Errorf("row %d: day must be an ISO date (YYYY-MM-DD)", i)
		}
		if _, err := time.Parse("2006-01-02", day); err != nil {
			return fmt.Errorf("row %d: day must be an ISO date (YYYY-MM-DD)", i)
		}

		status, ok := jsonString(d["status"])
		if !ok || !brandHealthStatuses[status] {
			return fmt.Errorf("row %d: status must be one of ok, warn, issues, unknown", i)
		}

		var def bool
		if err := json.Unmarshal(d["default"], &def); err != nil || len(d["default"]) == 0 || string(d["default"]) == "null" {
			return fmt.Errorf("row %d: default must be a boolean", i)
		}

		key := brand + "\x00" + day
		if seen[key] {
			return fmt.Errorf("row %d: duplicate (brand, day) %s %s", i, brand, day)
		}
		seen[key] = true
	}

	return nil
}

// jsonString decodes a JSON string value; ok is false for a missing key or any other type.
func jsonString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// PutBrandHealth upserts a batch of health documents. Body: {"rows": [document, ...]}.
func (a *App) PutBrandHealth(c echo.Context) error {
	var req struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "rows"))
	}
	if err := validateBrandHealthRows(req.Rows); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	batch, err := json.Marshal(req.Rows)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "rows"))
	}

	n, err := a.core.UpsertBrandHealth(batch)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{map[string]int64{"rows": n}})
}

// GetBrandHealth returns the latest document per brand, each carrying lists[] (the lists the
// user may see that the brand owns).
func (a *App) GetBrandHealth(c echo.Context) error {
	user := auth.GetUser(c)
	hasAllPerm, permittedIDs := user.GetPermittedLists(auth.PermTypeGet)

	out, err := a.core.GetBrandHealthLatest(hasAllPerm, permittedIDs)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// GetBrandHealthHistory returns one brand's documents, newest first. ?days=N (default 30, max 400).
func (a *App) GetBrandHealthHistory(c echo.Context) error {
	brand := strings.TrimSpace(c.Param("brand"))
	if brand == "" || len(brand) > brandHealthMaxKeyLen {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "brand"))
	}

	days, err := parseBrandHealthDays(c.QueryParam("days"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "days"))
	}

	out, err := a.core.GetBrandHealthHistory(brand, days)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// parseBrandHealthDays: absent -> 30; a positive integer is capped at 400; anything else errors.
func parseBrandHealthDays(s string) (int, error) {
	if s == "" {
		return brandHealthDefaultDays, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("days must be a positive integer")
	}
	if n > brandHealthMaxDays {
		n = brandHealthMaxDays
	}
	return n, nil
}

// Fork (brand picker, integrations BRAND-PICKER-SPEC D1). The brands rows: a brand's sending
// identity, which every list of the brand carries as its brand:/from:/site: projection. Readable
// by every logged-in user (the posture of GetLists and /api/brands/:slug/theme -- the data is the
// From every email already shows; a per-list list:manage holder must load it to save the list
// form). Written under lists:manage_all, the permission that could write the tags before.

// brandReq is the POST/PUT /api/brands body. On PUT the slug comes from the path.
type brandReq struct {
	Slug      string `json:"slug"`
	FromEmail string `json:"from_email"`
	Site      string `json:"site"`
}

// brandOf validates a request with the one rule body, models.BrandProblem, configured exactly as
// the list form's tag check was: the SMTP from_addresses lookup and the importer's sanitizer.
func (a *App) brandOf(r brandReq) (models.Brand, error) {
	b := models.Brand{
		Slug:      strings.TrimSpace(r.Slug),
		FromEmail: strings.TrimSpace(r.FromEmail),
		Site:      null.NewString(strings.TrimSpace(r.Site), strings.TrimSpace(r.Site) != ""),
	}
	if key, args := models.BrandProblem(b.Slug, b.FromEmail, b.Site.String, configuredFromLookup(), a.importer.SanitizeEmail); key != "" {
		return models.Brand{}, echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts(key, args...))
	}
	return b, nil
}

// GetBrands returns every brand row, sorted by slug: [{slug, from_email, site, display_name}].
func (a *App) GetBrands(c echo.Context) error {
	out, err := a.core.GetBrands()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// CreateBrand creates a brand row. A slug taken exactly or by case is refused (409).
func (a *App) CreateBrand(c echo.Context) error {
	var r brandReq
	if err := json.NewDecoder(c.Request().Body).Decode(&r); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "brand"))
	}
	b, err := a.brandOf(r)
	if err != nil {
		return err
	}

	out, created, err := a.core.CreateBrand(b)
	if err != nil {
		return err
	}
	if !created {
		return echo.NewHTTPError(http.StatusConflict, a.i18n.Ts("brands.exists", "brand", b.Slug))
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateBrand changes a brand row's From and site and re-projects them onto every list of the
// brand in the same transaction. No UI calls it in this spec; it exists so the projection cannot
// drift from the row by any API path.
func (a *App) UpdateBrand(c echo.Context) error {
	var r brandReq
	if err := json.NewDecoder(c.Request().Body).Decode(&r); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "brand"))
	}
	r.Slug = c.Param("slug")
	b, err := a.brandOf(r)
	if err != nil {
		return err
	}

	// Fork (persona From, PERSONA-FROM-SPEC D4) -- the stored personas must still be valid under
	// the new From (each must contain its display name). The operator empties or edits the set
	// first, then changes the From. A site-only change keeps the From and so always passes.
	cur, found, err := a.core.GetBrand(b.Slug)
	if err != nil {
		return err
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, a.i18n.Ts("globals.messages.notFound", "name", b.Slug))
	}
	if key, _ := models.PersonasProblem(cur.Personas, b.FromEmail); key != "" {
		failing := ""
		for _, p := range cur.Personas {
			if k, _ := models.PersonaProblem(p, b.FromEmail); k != "" {
				failing = p
				break
			}
		}
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("brands.personasInvalidForFrom", "persona", failing, "from", b.FromEmail))
	}

	out, found, err := a.core.UpdateBrand(b)
	if err != nil {
		return err
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, a.i18n.Ts("globals.messages.notFound", "name", b.Slug))
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// Fork (persona From, integrations PERSONA-FROM-SPEC D4). A brand's approved persona display
// names: the picklist a campaign's From may be chosen from, beside the brand From itself. Both
// routes need lists:manage_all, the brand-write permission -- the GET too, because it names
// campaigns a campaign-scoped user may not be allowed to see. Every rule on the values is
// models.PersonasProblem; the Campaign page has no mirror of it.

// personaCampaignRefs renders blocking campaigns for a message: `"<id> <name>" (status)`, joined.
func personaCampaignRefs(camps []models.PersonaCampaign) string {
	refs := make([]string, 0, len(camps))
	for _, c := range camps {
		refs = append(refs, fmt.Sprintf("%q (%s)", strconv.Itoa(c.ID)+" "+c.Name, c.Status))
	}
	return strings.Join(refs, ", ")
}

// GetBrandPersonas returns [{name, campaigns: [{id, name, status}]}] per persona of the brand,
// in stored order: campaigns are the BLOCKING carriers -- those whose From is the persona's
// canonical From and whose status is editable or running. Finished and cancelled never block.
func (a *App) GetBrandPersonas(c echo.Context) error {
	slug := c.Param("slug")
	row, found, err := a.core.GetBrand(slug)
	if err != nil {
		return err
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, a.i18n.Ts("globals.messages.notFound", "name", slug))
	}

	out := make([]models.PersonaUse, 0, len(row.Personas))
	for _, p := range row.Personas {
		camps, err := a.core.GetCampaignsCarryingFrom(models.PersonaFrom(p, row.FromEmail))
		if err != nil {
			return err
		}
		out = append(out, models.PersonaUse{Name: p, Campaigns: camps})
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// PutBrandPersonas replaces the brand's persona set. Body: {"personas": ["Natasha at Acme", ...]}.
// Each element is trimmed, the set validated against the row's From (400), and a persona being
// REMOVED that an editable or running campaign still carries is refused (409 naming the persona
// and the campaigns) with nothing written. Returns the brand row.
func (a *App) PutBrandPersonas(c echo.Context) error {
	var req struct {
		Personas *[]string `json:"personas"`
	}
	// A body without the personas array is refused rather than read as "remove them all".
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil || req.Personas == nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "personas"))
	}

	slug := c.Param("slug")
	row, found, err := a.core.GetBrand(slug)
	if err != nil {
		return err
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, a.i18n.Ts("globals.messages.notFound", "name", slug))
	}

	next := make([]string, 0, len(*req.Personas))
	for _, p := range *req.Personas {
		next = append(next, strings.TrimSpace(p))
	}
	if key, args := models.PersonasProblem(next, row.FromEmail); key != "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts(key, args...))
	}

	// removed = current - new, as exact strings.
	keep := make(map[string]struct{}, len(next))
	for _, p := range next {
		keep[p] = struct{}{}
	}
	for _, p := range row.Personas {
		if _, ok := keep[p]; ok {
			continue
		}
		camps, err := a.core.GetCampaignsCarryingFrom(models.PersonaFrom(p, row.FromEmail))
		if err != nil {
			return err
		}
		if len(camps) > 0 {
			return echo.NewHTTPError(http.StatusConflict,
				a.i18n.Ts("brands.personaInUse", "persona", p, "campaigns", personaCampaignRefs(camps)))
		}
	}

	out, found, err := a.core.UpdateBrandPersonas(row.Slug, next)
	if err != nil {
		return err
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, a.i18n.Ts("globals.messages.notFound", "name", slug))
	}
	return c.JSON(http.StatusOK, okResp{out})
}
