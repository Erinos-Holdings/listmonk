// Package brandtheme fetches a brand page's theme (bg/fg/accent colors, font names) from the
// public curatedfor.you catalog API, for the visual editor's brand color swatches.
//
// The catalog route is public (it is what the brand-page frontend itself fetches, no auth) but
// sends no Access-Control-Allow-Origin header, so the admin SPA cannot call it cross-origin.
// cmd/brand_theme.go proxies it same-origin through the backend and caches the result — the
// catalog API is not CDN-cached upstream. This package is the fetch/parse half, separated from
// the handler because the cmd package's init() hard-requires a config file and cannot host tests.
package brandtheme

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const urlFormat = "%s/api/catalog/brand/%s/page?locale=en-US&market=US"

// BaseURL is a var, not a const, so tests can point Fetch at an httptest stub.
var BaseURL = "https://curatedfor.you"

var client = &http.Client{Timeout: 8 * time.Second}

// Resp is the theme payload served to the admin SPA.
//
// The upstream API fails soft: an UNKNOWN slug returns 200 with a page that has no theme, not a
// 404. That surfaces here as Found=false, which is also what the default `curated` brand yields
// (it has no brand page). Callers must treat Found=false as "no brand swatches", never as an
// error — nothing validates that a list's `brand:` tag names a real catalog brand page.
type Resp struct {
	Found bool              `json:"found"`
	Theme map[string]string `json:"theme"`
}

// Fetch fetches a brand page from the catalog API and extracts its theme. The slug is
// interpolated into a URL path, so callers MUST validate it first (cmd uses reBrandSlug,
// [A-Za-z0-9_-] only) or this becomes an open path proxy into the catalog host.
func Fetch(slug string) (Resp, error) {
	out := Resp{Theme: map[string]string{}}

	resp, err := client.Get(fmt.Sprintf(urlFormat, BaseURL, slug))
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("catalog API returned HTTP %d", resp.StatusCode)
	}

	// Theme decodes as map[string]any and non-string values are dropped, so the endpoint keeps
	// working if the upstream theme object grows a nested value (e.g. a palette array) — the
	// swatch row only consumes flat string entries anyway.
	var body struct {
		Page struct {
			Theme map[string]any `json:"theme"`
		} `json:"page"`
	}

	// The brand page payload is a few KB; the cap only guards against a pathological response.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return out, err
	}

	for k, v := range body.Page.Theme {
		if s, ok := v.(string); ok {
			out.Theme[k] = s
		}
	}
	out.Found = len(out.Theme) > 0

	return out, nil
}
