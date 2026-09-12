package core

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/media"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"gopkg.in/volatiletech/null.v6"
)

// QueryMedia returns media entries optionally filtered by a query string.
//
// Fork (media tags) -- MEDIA-TAGS-SPEC D4. tags (already normalized) filters with OR; untagged
// adds rows with no tags; neither means no tag filter.
func (c *Core) QueryMedia(provider string, s media.Store, query string, tags []string, untagged bool, offset, limit int) ([]media.Media, int, error) {
	out := []media.Media{}

	if query != "" {
		query = strings.ToLower(query)
	}
	if tags == nil {
		tags = []string{}
	}

	if err := c.q.QueryMedia.Select(&out, fmt.Sprintf("%%%s%%", query), provider, offset, limit, pq.StringArray(tags), untagged); err != nil {
		return out, 0, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching",
				"name", "{globals.terms.media}", "error", pqErrMsg(err)))
	}

	total := 0
	if len(out) > 0 {
		total = out[0].Total

		for i := 0; i < len(out); i++ {
			fillMediaURLs(&out[i], s)
		}
	}

	return out, total, nil
}

// GetMedia returns a media item.
func (c *Core) GetMedia(id int, uuid, fileName string, s media.Store) (media.Media, error) {
	var uu any
	if uuid != "" {
		uu = uuid
	}

	var out media.Media
	if err := c.q.GetMedia.Get(&out, id, uu, fileName); err != nil {
		// If it's ` sql: no rows in result set`, return a 404.
		if err == sql.ErrNoRows {
			return out, ErrNotFound
		}

		return out, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.media}", "error", pqErrMsg(err)))
	}

	fillMediaURLs(&out, s)

	return out, nil
}

// fillMediaURLs sets the store-derived fields a row read from the DB lacks (url, thumb_url)
// and replaces a nil tag set with an empty one so the JSON is always an array.
func fillMediaURLs(m *media.Media, s media.Store) {
	m.URL = s.GetURL(m.Filename)
	if m.Thumb != "" {
		m.ThumbURL = null.String{Valid: true, String: s.GetURL(m.Thumb)}
	}
	if m.Tags == nil {
		m.Tags = pq.StringArray{}
	}
}

// InsertMedia inserts a new media file into the DB. tags must already be normalized
// (models.NormalizeMediaTags).
func (c *Core) InsertMedia(fileName, thumbName, contentType string, meta models.JSON, tags []string, provider string, s media.Store) (media.Media, error) {
	uu, err := uuid.NewV4()
	if err != nil {
		c.log.Printf("error generating UUID: %v", err)
		return media.Media{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUUID", "error", err.Error()))
	}
	if tags == nil {
		tags = []string{}
	}

	// Write to the DB.
	var newID int
	if err := c.q.InsertMedia.Get(&newID, uu, fileName, thumbName, contentType, provider, meta, pq.StringArray(tags)); err != nil {
		c.log.Printf("error inserting uploaded file to db: %v", err)
		return media.Media{}, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.media}", "error", pqErrMsg(err)))
	}

	return c.GetMedia(newID, "", "", s)
}

// UpdateMediaMeta merges a key set into a media row's meta JSON.
//
// Fork (dark-mode readiness) -- DARK-MODE-SPEC D4/D5. A merge, not a replace: the row
// already carries width/height from the upload, and a later re-classification must not
// drop them.
func (c *Core) UpdateMediaMeta(id int, meta models.JSON) error {
	if _, err := c.q.UpdateMediaMeta.Exec(id, meta); err != nil {
		c.log.Printf("error updating media meta: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.media}", "error", pqErrMsg(err)))
	}

	return nil
}

// UpdateMediaTags replaces a media row's tag set and returns the row with url/thumb_url
// filled the way GetMedia does, so the caller's tile keeps its thumbnail.
//
// Fork (media tags) -- MEDIA-TAGS-SPEC 3.4. tags must already be normalized. meta is
// untouched (D9).
func (c *Core) UpdateMediaTags(id int, tags []string, s media.Store) (media.Media, error) {
	if tags == nil {
		tags = []string{}
	}

	var out media.Media
	if err := c.q.UpdateMediaTags.Get(&out, id, pq.StringArray(tags)); err != nil {
		if err == sql.ErrNoRows {
			return out, ErrNotFound
		}

		c.log.Printf("error updating media tags: %v", err)
		return out, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorUpdating", "name", "{globals.terms.media}", "error", pqErrMsg(err)))
	}

	fillMediaURLs(&out, s)

	return out, nil
}

// GetMediaTags returns every distinct media tag with its row count, sorted by tag.
//
// Fork (media tags) -- MEDIA-TAGS-SPEC D8.
func (c *Core) GetMediaTags(provider string) ([]media.TagCount, error) {
	out := []media.TagCount{}
	if err := c.q.GetMediaTags.Select(&out, provider); err != nil {
		return out, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.media}", "error", pqErrMsg(err)))
	}

	return out, nil
}

// DeleteMedia deletes a given media item and returns the filename of the deleted item.
func (c *Core) DeleteMedia(id int) (string, error) {
	var fname string
	if err := c.q.DeleteMedia.Get(&fname, id); err != nil {
		c.log.Printf("error inserting uploaded file to db: %v", err)
		return "", echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorCreating", "name", "{globals.terms.media}", "error", pqErrMsg(err)))
	}

	return fname, nil
}
