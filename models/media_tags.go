package models

// Fork (media tags, MEDIA-TAGS-SPEC D2). The one server-side rule for a media tag: a bare
// lowercase slug. Every write path (the upload's `tags` field, PUT /api/media/:id/tags) and
// the listing's `tag` filter go through NormalizeMediaTags, strictly — an invalid tag is a
// 400, never silently dropped. (The lenient variant that drops invalid entries lives only
// where context is DERIVED rather than typed: the frontend's mediaTags.js and the one-off
// backfill script.)

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// MediaTagMaxLen matches the column type, VARCHAR(100)[].
const MediaTagMaxLen = 100

// ReMediaTag is a media tag after trimming and lower-folding. No `brand:` prefix: media has no
// From derivation to feed.
var ReMediaTag = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// MediaTagInvalidError names the first tag that failed the rule. Key is the i18n key the
// handlers translate with {tag}.
type MediaTagInvalidError struct {
	Tag string
}

// MediaTagInvalidKey is the i18n key for MediaTagInvalidError.
const MediaTagInvalidKey = "media.tagInvalid"

func (e *MediaTagInvalidError) Error() string {
	return fmt.Sprintf("invalid media tag %q", e.Tag)
}

// NormalizeMediaTags trims, lower-folds, drops empties, dedupes and sorts. Any tag failing
// ReMediaTag or longer than MediaTagMaxLen is rejected with a *MediaTagInvalidError. nil and
// an empty slice both yield a non-nil empty slice, so the JSON is `[]` and the bind is '{}'.
func NormalizeMediaTags(in []string) ([]string, error) {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))

	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if len(t) > MediaTagMaxLen || !ReMediaTag.MatchString(t) {
			return nil, &MediaTagInvalidError{Tag: t}
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}

	sort.Strings(out)
	return out, nil
}
