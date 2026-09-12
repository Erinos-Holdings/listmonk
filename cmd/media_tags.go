package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// Fork (media tags) -- MEDIA-TAGS-SPEC 3.4. Handlers for the tag autocomplete and the tile
// editor, plus the strict normalization every tag-carrying request goes through. Writes need
// `media:manage`, reads `media:get` (D11) -- the routes in handlers.go carry that split.

// splitMediaTagsField splits the upload form's comma-separated `tags` value. Empty entries
// are left for NormalizeMediaTags to drop.
func splitMediaTagsField(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.Split(v, ",")
}

// normalizeMediaTags applies models.NormalizeMediaTags and turns a rejected tag into a 400
// naming it. The server is always strict: an invalid tag is never silently dropped.
func (a *App) normalizeMediaTags(in []string) ([]string, error) {
	out, err := models.NormalizeMediaTags(in)
	if err != nil {
		var te *models.MediaTagInvalidError
		if errors.As(err, &te) {
			return nil, echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts(models.MediaTagInvalidKey, "tag", te.Tag))
		}
		return nil, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return out, nil
}

// GetMediaTags returns every distinct media tag with its row count.
func (a *App) GetMediaTags(c echo.Context) error {
	out, err := a.core.GetMediaTags(a.cfg.MediaUpload.Provider)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// UpdateMediaTags replaces a media item's tag set. Body: {"tags": [...]}.
func (a *App) UpdateMediaTags(c echo.Context) error {
	var req struct {
		Tags []string `json:"tags"`
	}
	if err := c.Bind(&req); err != nil {
		return err
	}

	tags, err := a.normalizeMediaTags(req.Tags)
	if err != nil {
		return err
	}

	out, err := a.core.UpdateMediaTags(getID(c), tags, a.media)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}
