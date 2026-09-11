package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/knadh/listmonk/internal/media/optimizer"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

const (
	thumbPrefix   = "thumb_"
	thumbnailSize = 250
)

var (
	vectorExts = []string{"svg"}
	imageExts  = []string{"gif", "png", "jpg", "jpeg"}
)

// UploadMedia handles media file uploads.
func (a *App) UploadMedia(c echo.Context) error {
	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("media.invalidFile", "error", err.Error()))
	}

	// Read the file from the HTTP form.
	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			a.i18n.Ts("media.errorReadingFile", "error", err.Error()))
	}
	defer src.Close()

	var (
		// Naive check for content type and extension.
		ext         = strings.TrimPrefix(strings.ToLower(filepath.Ext(file.Filename)), ".")
		contentType = file.Header.Get("Content-Type")
	)

	// Validate file extension.
	if !inArray("*", a.cfg.MediaUpload.Extensions) {
		if ok := inArray(ext, a.cfg.MediaUpload.Extensions); !ok {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("media.unsupportedFileType", "type", ext))
		}
	}

	raw, err := io.ReadAll(src)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError,
			a.i18n.Ts("media.errorReadingFile", "error", err.Error()))
	}

	// Fork (dark-mode readiness) -- DARK-MODE-SPEC D4. Classify every raster upload and
	// repair the two classes that have a safe, unambiguous fix (a ring glyph's transparent
	// interior; a dark monochrome mark on an opaque white ground). This runs on the UPLOAD
	// ITSELF, before the optimizer: the optimizer may re-encode an opaque logo as a JPEG,
	// and the classifier treats JPEG input as photographic by construction, so classifying
	// the optimized bytes would let a mono-on-white logo slip past as a "photo". A repair
	// always encodes PNG (alpha), and is then optimized like any other upload. The untouched
	// upload bytes are kept under orig_<filename> whenever pixels changed.
	var (
		darkVerdict     optimizer.Verdict
		darkFixed       bool
		origRaw         []byte
		origContentType string
	)
	isImage := inArray(ext, imageExts)
	if isImage {
		out, outExt, v, fixed, err := classifyAndRepair(raw, ext)
		if err != nil {
			a.log.Printf("error classifying image for dark mode: %v", err)
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("media.errorReadingFile", "error", err.Error()))
		}
		darkVerdict, darkFixed = v, fixed
		if fixed {
			origRaw, origContentType = raw, contentType
			raw, ext, contentType = out, outExt, optimizer.RasterContentType(outExt)
		}
	}

	// Optimize raster images for e-mail delivery (downsize to
	// maxImageWidth, recompress, possibly convert format). The original
	// bytes are kept whenever optimization cannot produce a smaller file.
	var width, height int
	if isImage {
		opt, err := optimizer.Optimize(raw, ext)
		if errors.Is(err, optimizer.ErrAnimatedGIFTooLarge) {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("media.animatedGIFTooLarge",
				"size", fmt.Sprintf("%d KB", len(raw)/1024),
				"limit", fmt.Sprintf("%d KB", optimizer.MaxAnimatedGIFBytes/1024)))
		}
		if err != nil {
			a.log.Printf("error optimizing image: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				a.i18n.Ts("media.errorResizing", "error", err.Error()))
		}
		raw = opt.Data
		contentType = opt.ContentType
		width, height = opt.Width, opt.Height
		if opt.Ext != ext {
			ext = opt.Ext
		}
	}

	// Sanitize the filename and make its extension reflect the stored
	// format (an optimized PNG may have become a JPEG).
	fName := makeFilename(file.Filename)
	if e := strings.TrimPrefix(strings.ToLower(filepath.Ext(fName)), "."); e != ext {
		fName = strings.TrimSuffix(fName, filepath.Ext(fName)) + "." + ext
	}

	// If the filename already exists in the DB, make it unique by adding a random suffix.
	if _, err := a.core.GetMedia(0, "", fName, a.media); err == nil {
		suffix, err := generateRandomString(6)
		if err != nil {
			a.log.Printf("error generating random string: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("globals.messages.internalError"))
		}

		fName = appendSuffixToFilename(fName, suffix)
	}

	// This keeps track of whether the objects have to be deleted from the store if any of
	// the subsequent steps fail. One helper removes the whole set (file, thumb_, orig_) so the
	// cleanup path and the delete path cannot drift apart (runbook hazard 59).
	cleanUp := false
	defer func() {
		if cleanUp {
			deleteMediaObjects(a.media, fName)
		}
	}()

	// Fork (dark-mode readiness) -- keep the untouched upload BEFORE the repair is written
	// (DARK-MODE-SPEC D4), so at no point does a repaired object exist without its original.
	// A later reprocess always classifies and repairs from the original rather than from
	// repaired bytes. Never over an existing original (putOriginal enforces that).
	if darkFixed {
		if _, err := putOriginal(a.media, fName, origContentType, origRaw); err != nil {
			cleanUp = true
			a.log.Printf("error storing original: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				a.i18n.Ts("media.errorUploading", "error", err.Error()))
		}
	}

	// Upload the file to the media store.
	if _, err := a.media.Put(fName, contentType, bytes.NewReader(raw)); err != nil {
		cleanUp = true
		a.log.Printf("error uploading file: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			a.i18n.Ts("media.errorUploading", "error", err.Error()))
	}

	thumbfName := ""

	// Create thumbnail from the stored (optimized) bytes for non-vector formats.
	if isImage {
		thumbFile, err := makeThumbnail(raw, ext)
		if err != nil {
			cleanUp = true
			a.log.Printf("error resizing image: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				a.i18n.Ts("media.errorResizing", "error", err.Error()))
		}

		// Upload thumbnail.
		tf, err := a.media.Put(thumbPrefix+fName, contentType, thumbFile)
		if err != nil {
			cleanUp = true
			a.log.Printf("error saving thumbnail: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError,
				a.i18n.Ts("media.errorSavingThumbnail", "error", err.Error()))
		}
		thumbfName = tf
	}
	if inArray(ext, vectorExts) {
		thumbfName = fName
	}

	// Images have metadata.
	meta := models.JSON{}
	if isImage {
		meta = models.JSON{
			"width":  width,
			"height": height,
			// Fork (dark-mode readiness) -- DARK-MODE-SPEC D4.
			darkmodeMetaKey: verdictMeta(darkVerdict, darkFixed),
		}
		if darkFixed {
			meta[originalMetaKey] = true
		}
	}

	// Insert the media into the DB.
	m, err := a.core.InsertMedia(fName, thumbfName, contentType, meta, a.cfg.MediaUpload.Provider, a.media)
	if err != nil {
		cleanUp = true
		return err
	}

	return c.JSON(http.StatusOK, okResp{m})
}

// GetAllMedia handles retrieval of uploaded media.
func (a *App) GetAllMedia(c echo.Context) error {
	var (
		query = c.FormValue("query")

		pg = a.pg.NewFromURL(c.Request().URL.Query())
	)
	// Fetch the media items from the DB.
	res, total, err := a.core.QueryMedia(a.cfg.MediaUpload.Provider, a.media, query, pg.Offset, pg.Limit)
	if err != nil {
		return err
	}

	out := models.PageResults{
		Results: res,
		Total:   total,
		Page:    pg.Page,
		PerPage: pg.PerPage,
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// GetMedia handles retrieval of a media item by ID.
func (a *App) GetMedia(c echo.Context) error {
	// Fetch the media item from the DB.
	id := getID(c)
	out, err := a.core.GetMedia(id, "", "", a.media)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// DeleteMedia handles deletion of uploaded media.
func (a *App) DeleteMedia(c echo.Context) error {

	// Delete the media from the DB. The query returns the filename.
	id := getID(c)
	fname, err := a.core.DeleteMedia(id)
	if err != nil {
		return err
	}

	// Delete the files from the media store. Fork (dark-mode readiness) -- the retained
	// original is derived from the filename exactly as the thumbnail is, and goes with it.
	deleteMediaObjects(a.media, fname)

	return c.JSON(http.StatusOK, okResp{true})
}

// ServeS3Media serves media files stored in S3 when the public URL is a relative path.
func (a *App) ServeS3Media(c echo.Context) error {
	key := c.Param("filepath")
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing media file path")
	}

	b, err := a.media.GetBlob(key)
	if err != nil {
		a.log.Printf("error fetching media from s3 %s: %v", key, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "error fetching media")
	}

	return c.Stream(http.StatusOK, http.DetectContentType(b), bytes.NewReader(b))
}

// makeThumbnail renders a thumbnail from the stored image bytes, encoded
// in the stored image's own format so the thumbnail's bytes match its
// filename extension and content type.
func makeThumbnail(raw []byte, ext string) (*bytes.Reader, error) {
	img, err := imaging.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}

	var (
		thumb  = imaging.Resize(img, thumbnailSize, 0, imaging.Lanczos)
		format = imaging.PNG
		out    bytes.Buffer
	)
	switch ext {
	case "jpg", "jpeg":
		format = imaging.JPEG
	case "gif":
		format = imaging.GIF
	}
	if err := imaging.Encode(&out, thumb, format); err != nil {
		return nil, err
	}
	return bytes.NewReader(out.Bytes()), nil
}
