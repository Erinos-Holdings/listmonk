package core

import (
	"strings"

	"github.com/knadh/listmonk/models"
)

// Brand-mapping tag prefixes, owned by models (models/list_tags.go) and shared with
// cmd/campaigns_brand.go and the import presets.
const (
	brandTagPrefix = models.BrandTagPrefix
	fromTagPrefix  = models.FromTagPrefix
)

// normalizeListTags is normalizeTags with one fork exception: `brand:`/`from:` brand-mapping
// tags are stored VERBATIM, trimmed of outer whitespace only.
//
// WHY: upstream's normalizeTags rewrites every whitespace run to `-`, and the brand mapping
// deliberately stores a full From header in a tag — `from:Liyora <hello@liyorahair.com>` — whose
// display name contains a space. Left to the normaliser, ANY create or update of a tagged list
// (renaming it in the UI is enough) silently corrupts the tag to `from:Liyora-<hello@…>`: every
// existing campaign on the list then fails the From-mismatch check, and a new campaign derives
// the hyphenated display name into real mail. Likewise `brand:Thirsty Girl` would be rewritten
// to the VALID but wrong slug `Thirsty-Girl` instead of being refused.
//
// Verbatim storage is safe only because cmd/lists_brand.go validates these tags on the same
// save: an invalid slug or a malformed From is refused at tag-edit time rather than stored.
// Non-mapping tags keep upstream's normalisation untouched.
func normalizeListTags(tags []string) []string {
	var out []string

	for _, t := range tags {
		trimmed := strings.TrimSpace(t)

		// Prefix-adjacent whitespace is canonicalised away (`brand: liyora` -> `brand:liyora`):
		// the Go readers trim it after TrimPrefix but the editor's derivation slices the prefix
		// off without trimming, so a stored space here would make the editor refuse campaigns
		// the API accepts. Whitespace INSIDE the value -- the display name's space -- is kept;
		// it is the whole reason these tags bypass normalizeTags.
		// One trim rule for all three reserved prefixes (brand:, from:, site:), shared with
		// the preset INSERT path via models.TrimListTag so both writers store one shape.
		if strings.HasPrefix(trimmed, brandTagPrefix) || strings.HasPrefix(trimmed, fromTagPrefix) || strings.HasPrefix(trimmed, models.SiteTagPrefix) {
			out = append(out, models.TrimListTag(trimmed))
			continue
		}

		out = append(out, normalizeTags([]string{t})...)
	}

	return out
}
