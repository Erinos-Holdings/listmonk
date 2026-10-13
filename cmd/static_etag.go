package main

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/labstack/echo/v4"
)

// Fork (emoji picker) -- integrations EMOJI-PICKER-SPEC D3. The admin static route answers HEAD
// and carries a content ETag.
//
// emoji-picker-element keeps its emoji data in IndexedDB and revalidates it with a HEAD request
// on the data URL: with an ETag it compares the tag and downloads nothing; with a 2xx and no
// ETag it downloads the whole file (~440 KB) on every picker creation; with a non-2xx (the 405
// a GET-only route gives) its lazy update rejects and the cached data is kept forever -- a data
// bump would then never reach a browser that already has the old file (its IndexedDB store is
// keyed by locale, not by URL). So the route answers HEAD, and both GET and HEAD carry an ETag
// derived from the file's bytes: stable across restarts, different for a changed file, and
// http.FileServer turns a matching If-None-Match into a 304 for every admin asset.
//
// The tag cache never invalidates: the embedded files are immutable for the life of the
// process (in dev mode, with files read from disk, restart after changing a static file).
type staticETagger struct {
	read func(path string) ([]byte, error)
	tags sync.Map // request path -> quoted ETag
}

func newStaticETagger(read func(path string) ([]byte, error)) *staticETagger {
	return &staticETagger{read: read}
}

// etag returns the quoted ETag for path, or "" when the file cannot be read (the wrapped file
// server then answers as it would have anyway).
func (s *staticETagger) etag(path string) string {
	if v, ok := s.tags.Load(path); ok {
		return v.(string)
	}
	b, err := s.read(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	tag := `"` + hex.EncodeToString(sum[:8]) + `"`
	s.tags.Store(path, tag)
	return tag
}

// wrap sets the ETag before handing the request to the file server, which honours it for
// If-None-Match (304) and answers HEAD with the headers alone.
func (s *staticETagger) wrap(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if tag := s.etag(c.Request().URL.Path); tag != "" {
			c.Response().Header().Set("ETag", tag)
		}
		return next(c)
	}
}
