package main

// Fork (emoji picker) -- integrations EMOJI-PICKER-SPEC I8: the admin static route answers HEAD,
// GET and HEAD carry the same content ETag, a matching If-None-Match is a 304, and a missing
// file carries no tag. Pure: no database, so this runs in CI.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/labstack/echo/v4"
)

func newStaticTestServer(t *testing.T) *echo.Echo {
	t.Helper()
	files := fstest.MapFS{
		"admin/static/emoji-data-1.8.0.json": {Data: []byte(`{"emoji":[]}`)},
		"admin/static/other.js":              {Data: []byte("console.log(1)")},
	}
	read := func(path string) ([]byte, error) {
		f, ok := files[strings.TrimPrefix(path, "/")]
		if !ok {
			return nil, errors.New("not found")
		}
		return f.Data, nil
	}
	srv := echo.New()
	static := newStaticETagger(read).wrap(echo.WrapHandler(http.FileServer(http.FS(files))))
	srv.GET("/admin/static/*", static)
	srv.HEAD("/admin/static/*", static)
	return srv
}

func do(srv *echo.Echo, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func TestAdminStaticETagAndHEAD(t *testing.T) {
	srv := newStaticTestServer(t)
	const path = "/admin/static/emoji-data-1.8.0.json"

	get := do(srv, http.MethodGet, path, nil)
	if get.Code != http.StatusOK || get.Body.String() != `{"emoji":[]}` {
		t.Fatalf("GET: want 200 with the body, got %d %q", get.Code, get.Body.String())
	}
	tag := get.Header().Get("ETag")
	if tag == "" || !strings.HasPrefix(tag, `"`) || !strings.HasSuffix(tag, `"`) {
		t.Fatalf("GET: want a quoted ETag, got %q", tag)
	}

	head := do(srv, http.MethodHead, path, nil)
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD: want 200, got %d (a GET-only route answers 405, and the picker then keeps stale data forever)", head.Code)
	}
	if head.Body.Len() != 0 {
		t.Fatalf("HEAD: want an empty body, got %d bytes", head.Body.Len())
	}
	if got := head.Header().Get("ETag"); got != tag {
		t.Fatalf("HEAD: want the GET's ETag %q, got %q", tag, got)
	}

	cached := do(srv, http.MethodGet, path, map[string]string{"If-None-Match": tag})
	if cached.Code != http.StatusNotModified {
		t.Fatalf("GET If-None-Match: want 304, got %d", cached.Code)
	}

	// A different file has a different tag; the tag is content-derived, not per process.
	other := do(srv, http.MethodGet, "/admin/static/other.js", nil)
	if got := other.Header().Get("ETag"); got == "" || got == tag {
		t.Fatalf("other.js: want its own ETag, got %q (data file's is %q)", got, tag)
	}
	if again := do(srv, http.MethodGet, path, nil).Header().Get("ETag"); again != tag {
		t.Fatalf("second GET: want the same ETag %q, got %q", tag, again)
	}

	missing := do(srv, http.MethodGet, "/admin/static/nope.json", nil)
	if missing.Code != http.StatusNotFound || missing.Header().Get("ETag") != "" {
		t.Fatalf("missing file: want 404 and no ETag, got %d %q", missing.Code, missing.Header().Get("ETag"))
	}
}

// The real server registers the HEAD route and the wrapped handler on the admin static path
// (initHTTPServer); a source pin, since initHTTPServer needs the whole app to run.
func TestAdminStaticRouteRegistersHEAD(t *testing.T) {
	src, err := os.ReadFile("init.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, want := range []string{
		`srv.GET("/admin/static/*", adminStatic)`,
		`srv.HEAD("/admin/static/*", adminStatic)`,
		`newStaticETagger(fs.Read)`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("cmd/init.go must contain %q (EMOJI-PICKER-SPEC D3: the admin static route answers HEAD with a content ETag)", want)
		}
	}
}
