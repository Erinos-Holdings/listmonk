package main

// Fork (render canary) -- integrations INSPECT-SCOPE-SPEC I9, the server half: the per-key hashes
// are taken over the SEND-rendered output (a document with a link hashes differently from the
// builder's items, because TransformTrackLinks rewrites its href), are stable across restarts
// (two fresh managers over the same link store), follow the key grammar, and a missing canary
// file is a 503. No database: the manager's store is a fake whose CreateLink is deterministic,
// like the real upsert on links.url.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/manager"
	"github.com/knadh/stuffbin"
	"github.com/labstack/echo/v4"
)

// linkStore is the one Store method the preview path calls (TrackLink -> CreateLink).
type linkStore struct {
	manager.Store
}

func (linkStore) CreateLink(url string) (string, error) {
	h := sha256.Sum256([]byte(url))
	x := hex.EncodeToString(h[:16])
	return x[0:8] + "-" + x[8:12] + "-4" + x[13:16] + "-8" + x[17:20] + "-" + x[20:32], nil
}

func canaryApp() *App {
	m := manager.New(manager.Config{
		UnsubURL:           "https://lm.test/subscription/%s/%s",
		OptinURL:           "https://lm.test/subscription/optin/%s?%s",
		LinkTrackURL:       "https://lm.test/link/%s/%s/%s",
		ViewTrackURL:       "https://lm.test/campaign/%s/%s/px.png",
		MessageURL:         "https://lm.test/campaign/%s/%s",
		ArchiveURL:         "https://lm.test/archive",
		RootURL:            "https://lm.test",
		IndividualTracking: true,
	}, linkStore{}, nil, log.New(os.Stderr, "", 0))
	return &App{manager: m}
}

func canaryFile(t *testing.T) []byte {
	t.Helper()
	docs := map[string]any{
		"with-link": map[string]any{
			"doc":  map[string]any{"root": map[string]any{"type": "EmailLayout", "data": map[string]any{"childrenIds": []string{"b"}}}, "b": map[string]any{"type": "Button"}},
			"html": `<!doctype html><html><body><a href="https://canary.invalid/button">Go</a></body></html>`,
		},
		"text-only": map[string]any{
			"doc":  map[string]any{"root": map[string]any{"type": "EmailLayout", "data": map[string]any{"childrenIds": []string{"t"}}}, "t": map[string]any{"type": "Text"}},
			"html": `<!doctype html><html><body><p>Hello {{ .Subscriber.FirstName }}</p></body></html>`,
		},
	}
	// The builder items: the same grammar over the COMPILED html (what build-canary.cjs writes).
	sum := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	html := func(stem string) string { return docs[stem].(map[string]any)["html"].(string) }
	items := map[string]string{
		"Button":      sum("\n--with-link--\n" + html("with-link")),
		"Text":        sum("\n--text-only--\n" + html("text-only")),
		"EmailLayout": sum("\n--text-only--\n" + html("text-only") + "\n--with-link--\n" + html("with-link")),
		"Body":        sum("<p>body</p>"),
	}
	b, err := json.Marshal(map[string]any{"version": 1, "documents": docs, "body": "<p>body</p>", "items": items})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRenderCanaryServerHalf(t *testing.T) {
	raw := canaryFile(t)
	a := canaryApp()
	c1, err := buildRenderCanary(raw, a.canaryRender)
	if err != nil {
		t.Fatal(err)
	}

	keys := make([]string, 0, len(c1.Items))
	for k := range c1.Items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "Body,Button,EmailLayout,Text" {
		t.Fatalf("keys = %v", keys)
	}

	// The link's document went through TransformTrackLinks: the server hash differs from the
	// builder's for every key sharing that document, and the rendered href is a /link/ URL.
	for _, k := range []string{"Button", "EmailLayout"} {
		if c1.Items[k] == c1.BuilderItems[k] {
			t.Fatalf("%s: the server hash equals the builder hash -- the send-path transform is not reflected", k)
		}
	}
	out, err := a.canaryRender(`<a href="https://canary.invalid/button">Go</a>`, "visual")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "https://lm.test/link/") || !strings.Contains(out, canaryCampUUID) || !strings.Contains(out, canarySubUUID) || !strings.Contains(out, "px.png") {
		t.Fatalf("rendered = %s, want a tracked link and the view pixel with the fixed UUIDs", out)
	}

	// Stable across restarts: a fresh manager over the same link store hashes identically.
	c2, err := buildRenderCanary(raw, canaryApp().canaryRender)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := mustJSON(t, c1), mustJSON(t, c2); a != b {
		t.Fatalf("two startups disagree:\n%s\n%s", a, b)
	}
	if len(c1.Documents) != 2 || c1.Version != 1 {
		t.Fatalf("documents/version = %d/%d", len(c1.Documents), c1.Version)
	}

	// Grammar: every key = sha256 over the rendered docs containing the type, in stem order.
	render := func(stem string) string {
		var f renderCanaryFile
		json.Unmarshal(raw, &f)
		s, _ := a.canaryRender(f.Documents[stem].HTML, "visual")
		return s
	}
	sum := sha256.Sum256([]byte("\n--text-only--\n" + render("text-only") + "\n--with-link--\n" + render("with-link")))
	if c1.Items["EmailLayout"] != hex.EncodeToString(sum[:]) {
		t.Fatal("EmailLayout is not the stem-ordered hash of every document")
	}
}

func TestRenderCanaryUnavailable(t *testing.T) {
	// A missing file: initRenderCanary fails and the endpoint is 503.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := stuffbin.NewLocalFS("/", filepath.Join(dir, "other.txt")+":/other.txt")
	if err != nil {
		t.Fatal(err)
	}
	c, err := initRenderCanary(fs, canaryApp().canaryRender)
	if c != nil || err == nil {
		t.Fatal("a missing render-canary.json must be an error")
	}
	a := &App{renderCanaryErr: err}
	e := echo.New()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/campaigns/render-canary", nil), httptest.NewRecorder())
	he, ok := a.GetRenderCanary(ctx).(*echo.HTTPError)
	if !ok || he.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %v", he)
	}

	// Unparseable, or a render error, is an error too (never a partial canary).
	if _, err := buildRenderCanary([]byte("{"), canaryApp().canaryRender); err == nil {
		t.Fatal("unparseable file accepted")
	}
	if _, err := buildRenderCanary(canaryFile(t), func(string, string) (string, error) { return "", errors.New("boom") }); err == nil {
		t.Fatal("a render error must fail the canary")
	}

	// Available: 200 with the canary.
	rec := httptest.NewRecorder()
	ok2, _ := buildRenderCanary(canaryFile(t), canaryApp().canaryRender)
	b := &App{renderCanary: ok2}
	if err := b.GetRenderCanary(e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("available canary: %v %d", err, rec.Code)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The REAL corpus: every document renders through the send path. RENDER_CANARY_FILE names the
// file (CI: the dist file build-image.yml's Render canary step writes, before pack-bin -- and a
// named file that is missing FAILS); without it the local builder-half output is used when present.
func TestRenderCanaryRealCorpus(t *testing.T) {
	path := os.Getenv("RENDER_CANARY_FILE")
	named := path != ""
	if !named {
		path = filepath.Join("..", "frontend", "public", "static", "email-builder", "render-canary.json")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if named {
			t.Fatalf("RENDER_CANARY_FILE %s: %v", path, err)
		}
		t.Skip("render-canary.json not built (node frontend/email-builder/test/build-canary.cjs)")
	}
	c, err := buildRenderCanary(raw, canaryApp().canaryRender)
	if err != nil {
		t.Fatal(err)
	}
	var f renderCanaryFile
	json.Unmarshal(raw, &f)
	for k := range f.Items {
		if c.Items[k] == "" {
			t.Fatalf("server half has no hash for %s", k)
		}
	}
	if c.Items["Html"] == c.BuilderItems["Html"] {
		t.Fatal("the Html canary document carries a link: its server hash must differ from the builder's")
	}
}
