package main

// Fork (uploads hardening) -- integrations UPLOADS-HARDENING-SPEC K1, K3, K4, K5 and K7. Pure: no
// database, so these run in CI. K2 and K6 need a database and are in uploads_hardening_db_test.go.

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	iofs "io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/media/optimizer"
	"github.com/labstack/echo/v4"
)

const htmlDoc = "<!doctype html><html><body><p>hardening probe</p></body></html>"

// ---- K1: the extension map -------------------------------------------------------------

func TestMediaContentType(t *testing.T) {
	want := map[string]string{
		"jpg":   "image/jpeg",
		"jpeg":  "image/jpeg",
		"png":   "image/png",
		"gif":   "image/gif",
		"webp":  "image/webp",
		"svg":   "image/svg+xml",
		"woff2": "font/woff2",
	}
	for ext, ct := range want {
		if got := mediaContentType(ext); got != ct {
			t.Errorf("mediaContentType(%q) = %q, want %q", ext, got, ct)
		}
	}

	for _, ext := range []string{"html", "htm", "js", "xml", "", "xhtml", "mjs", "css", "txt", "pdf", "woff", "ttf", "svgz", "bmp", ".png"} {
		if got := mediaContentType(ext); got != "application/octet-stream" {
			t.Errorf("mediaContentType(%q) = %q, want application/octet-stream", ext, got)
		}
	}
}

// ---- K3: a reprocess never trusts the row's content_type --------------------------------

// stillGIF is curated_logo3.png (mono-on-white, repairable) re-encoded as a one-frame GIF.
func stillGIF(t *testing.T) []byte {
	t.Helper()
	img, err := optimizer.DecodeStill(testFixture(t, "curated_logo3.png"))
	if err != nil {
		t.Fatal(err)
	}
	pal := image.NewPaletted(img.Bounds(), palette.Plan9)
	draw.Draw(pal, img.Bounds(), img, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := gif.Encode(&buf, pal, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReprocessIgnoresRowContentType(t *testing.T) {
	cases := []struct {
		key      string
		raw      func(*testing.T) []byte
		wantOrig string
	}{
		{"logo.png", func(t *testing.T) []byte { return testFixture(t, "curated_logo3.png") }, "image/png"},
		{"logo.gif", stillGIF, "image/gif"},
	}

	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			s := newFakeStore()
			s.objects[c.key] = c.raw(t)

			_, out, err := reprocessStored(s, mediaLike{ID: 1, Filename: c.key, ContentType: "text/html"}, false)
			if err != nil {
				t.Fatal(err)
			}
			if out["fixed"] != true {
				t.Fatalf("the fixture did not repair: %v", out)
			}

			want := map[string]string{
				"orig_" + c.key:  c.wantOrig,  // the untouched bytes, typed by the filename
				c.key:            "image/png", // the repair is PNG, whatever the key says
				"thumb_" + c.key: "image/png",
			}
			for k, ct := range want {
				if got, ok := s.types[k]; !ok || got != ct {
					t.Errorf("%s stored as %q (put=%v), want %q", k, got, ok, ct)
				}
			}
			if !bytes.HasPrefix(s.objects[c.key], []byte("\x89PNG")) {
				t.Errorf("%s does not hold PNG bytes", c.key)
			}
		})
	}
}

// ---- K4 and K7: one write path, one HTML responder ---------------------------------------

// selectorsIn parses every non-test Go file under the roots (skipping the skip dirs and testdata)
// and returns "file:line in func" for each selector named sel. With args < 0 every selector
// counts -- a call or a method value alike, since there is no type information to tell a store's
// Put from another type's. With args >= 0 only calls with that many arguments count: echo's
// c.HTML(code, body) takes two, which tells it from a template.HTML(s) conversion.
func selectorsIn(t *testing.T, sel string, args int, skip []string, roots ...string) []string {
	t.Helper()
	var out []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d iofs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				for _, s := range skip {
					if filepath.Clean(p) == filepath.Clean(s) {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, p, nil, 0)
			if err != nil {
				return err
			}
			for _, decl := range f.Decls {
				fn := "<package scope>"
				if fd, ok := decl.(*ast.FuncDecl); ok {
					fn = fd.Name.Name
				}
				ast.Inspect(decl, func(n ast.Node) bool {
					var se *ast.SelectorExpr
					if args >= 0 {
						ce, ok := n.(*ast.CallExpr)
						if !ok || len(ce.Args) != args {
							return true
						}
						if se, ok = ce.Fun.(*ast.SelectorExpr); !ok {
							return true
						}
					} else {
						var ok bool
						if se, ok = n.(*ast.SelectorExpr); !ok {
							return true
						}
					}
					if se.Sel.Name == sel {
						out = append(out, fset.Position(se.Pos()).String()+" in "+fn)
					}
					return true
				})
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestPutMediaIsOnlyPut(t *testing.T) {
	providers := filepath.Join("..", "internal", "media", "providers")
	got := selectorsIn(t, "Put", -1, []string{providers}, ".", filepath.Join("..", "internal"))
	if len(got) != 1 || !strings.HasSuffix(got[0], " in putMedia") {
		t.Fatalf("want exactly one store Put, inside putMedia; found %d:\n%s\n"+
			"every media object must be written through putMedia (integrations UPLOADS-HARDENING-SPEC D1)",
			len(got), strings.Join(got, "\n"))
	}
}

func TestSandboxedHTMLIsOnlyHTML(t *testing.T) {
	got := selectorsIn(t, "HTML", 2, nil, ".")
	if len(got) != 1 || !strings.HasSuffix(got[0], " in sandboxedHTML") {
		t.Fatalf("want exactly one c.HTML call in cmd/, inside sandboxedHTML; found %d:\n%s\n"+
			"a stored body answered as a page must go through sandboxedHTML (integrations UPLOADS-HARDENING-SPEC D4)",
			len(got), strings.Join(got, "\n"))
	}

	// The other way to answer an HTML page. Its one use is the admin UI's own index.html; a
	// second one is a new page that must be looked at.
	blob := selectorsIn(t, "HTMLBlob", 2, nil, ".")
	if len(blob) != 1 || !strings.HasSuffix(blob[0], " in AdminPage") {
		t.Fatalf("want exactly one c.HTMLBlob call in cmd/, inside AdminPage; found %d:\n%s",
			len(blob), strings.Join(blob, "\n"))
	}
}

// ---- K5: the app's own /uploads route ------------------------------------------------------

func TestServeS3MediaTypeAndHeaders(t *testing.T) {
	png := testFixture(t, "curated_logo3.png")
	cases := []struct {
		key  string
		body []byte
		want string
	}{
		{"x.woff2", []byte(htmlDoc), "font/woff2"},
		{"x.webp", []byte(htmlDoc), "image/webp"},
		{"x.html", []byte(htmlDoc), "application/octet-stream"},
		{"x.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), "image/svg+xml"},
		{"x.gif", png, "image/png"}, // a reprocess repair under a .gif key: truthful
		{"x.png", png, "image/png"},
	}

	// A parameter with a query or fragment is refused: the S3 provider would fetch the key before
	// it and the type would come from what follows it.
	for _, key := range []string{"x.woff2?a.svg", "x.woff2#a.svg"} {
		t.Run(key, func(t *testing.T) {
			s := newFakeStore()
			s.objects["x.woff2"] = []byte(htmlDoc)
			s.objects[key] = []byte(htmlDoc)
			a := &App{media: s}

			e := echo.New()
			rec := httptest.NewRecorder()
			ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/uploads/x", nil), rec)
			ctx.SetParamNames("filepath")
			ctx.SetParamValues(key)
			err := a.ServeS3Media(ctx)
			if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
				t.Fatalf("ServeS3Media(%q) = %v (status %d), want a 400", key, err, rec.Code)
			}
		})
	}

	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			if got := servedMediaType(c.key, c.body); got != c.want {
				t.Fatalf("servedMediaType(%q) = %q, want %q", c.key, got, c.want)
			}

			s := newFakeStore()
			s.objects[c.key] = c.body
			a := &App{media: s}

			e := echo.New()
			rec := httptest.NewRecorder()
			ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/uploads/"+c.key, nil), rec)
			ctx.SetParamNames("filepath")
			ctx.SetParamValues(c.key)
			if err := a.ServeS3Media(ctx); err != nil {
				t.Fatal(err)
			}

			h := rec.Header()
			if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), c.body) {
				t.Fatalf("status %d, body changed=%v", rec.Code, !bytes.Equal(rec.Body.Bytes(), c.body))
			}
			if got := h.Get(echo.HeaderContentType); got != c.want {
				t.Fatalf("Content-Type = %q, want %q", got, c.want)
			}
			if got := h.Get(echo.HeaderXContentTypeOptions); got != "nosniff" {
				t.Fatalf("X-Content-Type-Options = %q", got)
			}
			if got := h.Get(echo.HeaderContentSecurityPolicy); got != "default-src 'none'; style-src 'unsafe-inline'; sandbox" {
				t.Fatalf("Content-Security-Policy = %q", got)
			}
		})
	}
}
