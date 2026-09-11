package main

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/knadh/listmonk/internal/media"
	"github.com/knadh/listmonk/internal/media/optimizer"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// Dark-mode classification, originals retention, and reprocessing (DARK-MODE-SPEC D4/D5).
//
// Every raster upload gets a verdict stamped into meta.darkmode, and the two classes with a
// safe repair are repaired on the way in. The untouched upload is kept under the FLAT key
// `orig_<filename>` -- flat, mirroring `thumb_`, because the filesystem provider Puts
// join(dir, filename) with no MkdirAll, so an `originals/` prefix breaks the dev suite.

const origPrefix = "orig_"

// darkmodeMetaKey is the meta key carrying the verdict; originalMetaKey records that an
// untouched copy exists under origPrefix+filename.
const (
	darkmodeMetaKey = "darkmode"
	originalMetaKey = "original"
)

// verdictMeta renders a verdict as the JSON object stored in meta.darkmode and returned on
// the upload/reprocess responses.
func verdictMeta(v optimizer.Verdict, fixed bool) models.JSON {
	warnings := v.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	return models.JSON{
		"class":    v.Class,
		"fixed":    fixed,
		"warnings": warnings,
		"version":  v.Version,
		"at":       time.Now().UTC().Format(time.RFC3339),
	}
}

// classifyAndRepair runs the classifier over an upload's bytes and applies the class's
// repair when it has one. It returns the bytes to store (the repaired encoding, or the
// input untouched), the verdict, and whether pixels changed.
//
// A repair always encodes PNG: both repairs create or preserve an alpha channel, which
// JPEG cannot carry. The optimizer's "original bytes win unless smaller" rule governs the
// COMPRESSION step only -- a pixel repair is a later, separate step whose output always
// wins, because it is the point.
func classifyAndRepair(raw []byte, ext string) ([]byte, string, optimizer.Verdict, bool, error) {
	img, v, err := optimizer.ClassifyRaw(raw, ext)
	if err != nil {
		return raw, ext, optimizer.Verdict{}, false, err
	}

	if img == nil || !optimizer.Repairable(v.Class) {
		return raw, ext, v, false, nil
	}

	repaired, changed := optimizer.Repair(img, v)
	if !changed {
		return raw, ext, v, false, nil
	}

	out, err := optimizer.EncodePNG(repaired)
	if err != nil {
		return raw, ext, v, false, err
	}

	return out, "png", v, true, nil
}

// putOriginal stores the untouched upload bytes under orig_<filename>, but NEVER over an
// existing one (H4): the first original is the only true original, and a reprocess that
// overwrote it would make the repair irreversible and, worse, make the next reprocess
// repair an already-repaired image.
func putOriginal(s media.Store, fName, contentType string, raw []byte) (string, error) {
	key := origPrefix + fName
	if b, err := s.GetBlob(key); err == nil && len(b) > 0 {
		return key, nil
	}

	return s.Put(key, contentType, bytes.NewReader(raw))
}

// originalBytes returns the untouched upload when one was kept, else the stored object.
// Every classification and repair reads through this: repairing repaired bytes would
// compound the transform.
func originalBytes(s media.Store, filename string) ([]byte, error) {
	if b, err := s.GetBlob(origPrefix + filename); err == nil && len(b) > 0 {
		return b, nil
	}

	return s.GetBlob(filename)
}

// deleteMediaObjects removes the whole object set for a filename: the file, its thumbnail,
// and the retained original. One place, so the delete path and the upload cleanup path
// cannot drift apart.
func deleteMediaObjects(s media.Store, fname string) {
	s.Delete(fname)
	s.Delete(thumbPrefix + fname)
	s.Delete(origPrefix + fname)
}

// ErrVerdictCurrent is returned by reprocessStored when the row already carries a verdict at
// the current classifier version and force was not set.
var ErrVerdictCurrent = errors.New("dark-mode verdict is already current")

// reprocessStored is the whole store-side half of a reprocess: read the original, classify,
// repair, write object + thumbnail + original. It returns the meta to merge into the row and
// the response body. The DB read and the meta write stay in the handler, so this is unit
// testable against a fake store with no database.
//
// It ALWAYS classifies and repairs from the original when one exists, and never overwrites
// an existing original.
func reprocessStored(s media.Store, m mediaLike, force bool) (models.JSON, map[string]any, error) {
	ext := rasterExt(m.Filename)
	if ext == "" {
		return nil, nil, errNotRaster
	}

	if !force {
		if v := storedVerdict(m.Meta); v != nil {
			if ver, ok := numberOf(v["version"]); ok && int(ver) >= optimizer.ClassifierVersion {
				return nil, nil, ErrVerdictCurrent
			}
		}
	}

	raw, err := originalBytes(s, m.Filename)
	if err != nil {
		return nil, nil, err
	}

	out, outExt, v, fixed, err := classifyAndRepair(raw, ext)
	if err != nil {
		return nil, nil, err
	}

	meta := models.JSON{darkmodeMetaKey: verdictMeta(v, fixed)}

	if fixed {
		// Keep the untouched bytes first (never over an existing original), then write the
		// repair over the live key and refresh the thumbnail from the new pixels.
		if _, err := putOriginal(s, m.Filename, m.ContentType, raw); err != nil {
			return nil, nil, err
		}
		meta[originalMetaKey] = true

		// A repair is a PNG at the original's dimensions; run it through the optimizer like
		// any upload (resize, quantize -- never JPEG, the repair carries alpha) so a reprocess
		// stores what an upload of the same pixels would.
		opt, err := optimizer.Optimize(out, outExt)
		if err != nil {
			return nil, nil, err
		}
		out, outExt = opt.Data, opt.Ext

		// The object keeps its filename and DB content_type: the store serves the PNG bytes
		// with the content type given here, which is what a client reads. (A still GIF that
		// repairs therefore stores PNG bytes under a .gif key -- browsers sniff, and it is a
		// case no live asset hits.)
		if _, err := s.Put(m.Filename, optimizer.RasterContentType(outExt), bytes.NewReader(out)); err != nil {
			return nil, nil, err
		}

		thumb, err := makeThumbnail(out, outExt)
		if err != nil {
			return nil, nil, err
		}
		if _, err := s.Put(thumbPrefix+m.Filename, optimizer.RasterContentType(outExt), thumb); err != nil {
			return nil, nil, err
		}
	}

	return meta, map[string]any{
		"id": m.ID, "filename": m.Filename, "class": v.Class, "fixed": fixed,
		"warnings": v.Warnings, "version": v.Version,
	}, nil
}

// mediaLike is the slice of a media row reprocessStored needs, so the fake-store tests do
// not have to build a full media.Media with a DB behind it.
type mediaLike struct {
	ID          int
	Filename    string
	ContentType string
	Meta        models.JSON
}

// numberOf reads a JSON number back out of models.JSON, which decodes to float64 through
// the driver but may be an int when the value was just constructed in Go.
func numberOf(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

// GetMediaDarkmode returns the dark-mode verdict for a media item and the action a
// reprocess would take. It writes nothing at all -- object, thumbnail and meta are
// untouched -- which is why it sits behind `media:get`: `media:manage` also grants DELETE
// on live assets, and a dry run must not need that.
func (a *App) GetMediaDarkmode(c echo.Context) error {
	id := getID(c)
	m, err := a.core.GetMedia(id, "", "", a.media)
	if err != nil {
		return err
	}

	ext := rasterExt(m.Filename)
	if ext == "" {
		return c.JSON(http.StatusOK, okResp{map[string]any{
			"id": m.ID, "filename": m.Filename, "class": "n/a", "would_fix": false,
			"warnings": []string{}, "version": optimizer.ClassifierVersion,
		}})
	}

	raw, err := originalBytes(a.media, m.Filename)
	if err != nil {
		a.log.Printf("error reading media %d for classification: %v", m.ID, err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			a.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.media}", "error", err.Error()))
	}

	_, v, err := optimizer.ClassifyRaw(raw, ext)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("media.errorReadingFile", "error", err.Error()))
	}

	return c.JSON(http.StatusOK, okResp{map[string]any{
		"id":        m.ID,
		"filename":  m.Filename,
		"class":     v.Class,
		"would_fix": optimizer.Repairable(v.Class),
		"warnings":  v.Warnings,
		"version":   v.Version,
		"stored":    storedVerdict(m.Meta),
	}})
}

// ReprocessMedia re-classifies a media item FROM ITS ORIGINAL, applies the repair, and
// rewrites the object, the thumbnail and meta.
//
// Refuses with 409 when the row already carries a verdict at the current classifier
// version, unless force=true -- so the sweep is re-runnable without re-repairing.
func (a *App) ReprocessMedia(c echo.Context) error {
	id := getID(c)
	m, err := a.core.GetMedia(id, "", "", a.media)
	if err != nil {
		return err
	}

	force := strings.EqualFold(c.FormValue("force"), "true")

	meta, out, err := reprocessStored(a.media, mediaLike{
		ID: m.ID, Filename: m.Filename, ContentType: m.ContentType, Meta: m.Meta,
	}, force)
	switch {
	case errors.Is(err, ErrVerdictCurrent):
		return echo.NewHTTPError(http.StatusConflict,
			a.i18n.Ts("media.darkmodeAlreadyCurrent", "version", strconv.Itoa(optimizer.ClassifierVersion)))
	case errors.Is(err, errNotRaster):
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("media.unsupportedFileType", "type", strings.TrimPrefix(strings.ToLower(fileExt(m.Filename)), ".")))
	case err != nil:
		a.log.Printf("error reprocessing media %d: %v", m.ID, err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			a.i18n.Ts("media.errorUploading", "error", err.Error()))
	}

	if err := a.core.UpdateMediaMeta(m.ID, meta); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// storedVerdict pulls meta.darkmode off a media row, or nil when it carries none.
func storedVerdict(meta models.JSON) map[string]any {
	if meta == nil {
		return nil
	}
	v, ok := meta[darkmodeMetaKey].(map[string]any)
	if !ok {
		return nil
	}
	return v
}

func fileExt(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i:]
	}
	return ""
}

var errNotRaster = errors.New("media item is not a raster image")

// rasterExt returns the classifier's extension for a filename, or "" when the file is not
// one of the raster formats the optimizer decodes.
func rasterExt(name string) string {
	ext := strings.TrimPrefix(strings.ToLower(fileExt(name)), ".")
	if !inArray(ext, imageExts) {
		return ""
	}
	return ext
}
