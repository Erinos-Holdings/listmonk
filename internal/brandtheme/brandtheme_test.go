package brandtheme

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

// The upstream contract these fixtures encode was observed live 2026-08-12: a known slug
// returns {page: {theme: {...}}}; an UNKNOWN slug returns HTTP 200 with a page that has no
// theme (no 404); theme values are flat strings today but may grow non-string entries.
func TestFetchBrandTheme(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// locale/market drive the upstream CMS snapshot selection and fallback — a dropped or
		// typo'd param changes live behavior while path-only assertions stay green.
		if got := r.URL.Query().Get("locale"); got != "en-US" {
			t.Errorf("locale = %q, want en-US", got)
		}
		if got := r.URL.Query().Get("market"); got != "US" {
			t.Errorf("market = %q, want US", got)
		}

		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/catalog/brand/liyora/page":
			w.Write([]byte(`{"page":{"version":2,"theme":{"fg":"#F2EFEA","accent":"#C8D6EB","bg":"#43273B","fontHeading":"Lato"}}}`))
		case "/api/catalog/brand/mixed/page":
			// A future theme that grew a nested value: strings survive, the rest is dropped.
			w.Write([]byte(`{"page":{"theme":{"bg":"#FFFFFF","palette":["#111111","#222222"],"weights":{"heading":700}}}}`))
		case "/api/catalog/brand/unknown/page":
			// Upstream fails soft: 200 with an empty page, not a 404.
			w.Write([]byte(`{"page":{}}`))
		case "/api/catalog/brand/broken/page":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	orig := BaseURL
	BaseURL = srv.URL
	defer func() { BaseURL = orig }()

	t.Run("known slug returns theme", func(t *testing.T) {
		got, err := Fetch("liyora")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.Found {
			t.Fatal("expected Found=true")
		}
		want := map[string]string{"fg": "#F2EFEA", "accent": "#C8D6EB", "bg": "#43273B", "fontHeading": "Lato"}
		if len(got.Theme) != len(want) {
			t.Fatalf("theme = %v, want %v", got.Theme, want)
		}
		for k, v := range want {
			if got.Theme[k] != v {
				t.Errorf("theme[%q] = %q, want %q", k, got.Theme[k], v)
			}
		}
	})

	t.Run("non-string theme values are dropped, not fatal", func(t *testing.T) {
		got, err := Fetch("mixed")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.Found {
			t.Fatal("expected Found=true")
		}
		if len(got.Theme) != 1 || got.Theme["bg"] != "#FFFFFF" {
			t.Fatalf("theme = %v, want only bg", got.Theme)
		}
	})

	t.Run("unknown slug is found=false, not an error", func(t *testing.T) {
		got, err := Fetch("unknown")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Found || len(got.Theme) != 0 {
			t.Fatalf("expected empty not-found response, got %+v", got)
		}
	})

	t.Run("upstream failure is an error", func(t *testing.T) {
		if _, err := Fetch("broken"); err == nil {
			t.Fatal("expected error on HTTP 500")
		}
	})
}

// The pinned themes never touch the catalog, so all that can rot is the data itself:
// every entry must be servable (Found=true) and carry only well-formed hex colors, and
// `curated` must stay pinned — cmd/campaigns_brand.go's defaultBrandSlug derives it for
// every unmapped campaign.
func TestPinned(t *testing.T) {
	reHex := regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)

	if _, ok := Pinned["curated"]; !ok {
		t.Fatal("curated must remain pinned (defaultBrandSlug derives it)")
	}

	for slug, resp := range Pinned {
		if !resp.Found {
			t.Errorf("pinned %q must have Found=true", slug)
		}
		if len(resp.Theme) == 0 {
			t.Errorf("pinned %q has an empty theme", slug)
		}
		for role, v := range resp.Theme {
			if !reHex.MatchString(v) {
				t.Errorf("pinned %q role %q: %q is not a hex color", slug, role, v)
			}
		}
	}
}
