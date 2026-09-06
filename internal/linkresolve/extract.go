package linkresolve

import (
	"regexp"
	"strings"
)

// reTrackLinkCall matches a full `{{ TrackLink "<string>" . }}` call (the form the compile
// transform emits and regTplFuncs normalizes to) and captures the Go string literal body,
// backslash escapes included.
var reTrackLinkCall = regexp.MustCompile(`{{\s*TrackLink\s+"((?:[^"\\]|\\.)*)"\s*\.?\s*}}`)

// Expressions returns every DYNAMIC tracked-link expression in a compiled (transformed, not
// rendered) campaign body: the string argument of each `TrackLink` call that itself contains
// `{{`, with the Go string escapes undone. Run the models compile transform on the SOURCE body
// first — in a rendered body every TrackLink has already become a `/link/…` URL (spec §3.5,
// review F6). Duplicates are collapsed, first-seen order kept.
func Expressions(compiledBody string) []string {
	var (
		out  []string
		seen = map[string]struct{}{}
	)
	for _, m := range reTrackLinkCall.FindAllStringSubmatch(compiledBody, -1) {
		arg := UnescapeGoString(m[1])
		if !IsDynamic(arg) {
			continue
		}
		if _, ok := seen[arg]; ok {
			continue
		}
		seen[arg] = struct{}{}
		out = append(out, arg)
	}
	return out
}

// UnescapeGoString undoes the two escapes the compile transform applies inside a TrackLink
// string literal (`\"` and `\\`). Other escapes are left as typed.
func UnescapeGoString(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
			b.WriteByte(s[i+1])
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// EscapeGoString is the inverse: makes s safe inside a double-quoted Go template string
// literal. Callers must have already refused backslashes and control characters (the
// compile transform's skip rules) — this only escapes the quote.
func EscapeGoString(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
