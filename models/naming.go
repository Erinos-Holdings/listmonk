package models

// Fork (naming standard, integrations listmonk/NAMING-STANDARD-SPEC D3/D6 §4.3). The server's name
// rule for campaigns, lists and templates -- applied on create, on the name-only rename endpoints
// and on an update whose name CHANGES (an unchanged name is never re-validated, so an existing
// longer name stays valid until it is renamed). Underscores and Marketing step codes are guidance
// only (the form's hint), never refused here.
//
// THE TS MIRROR is integrations lib/naming.ts::nameProblem. Both are pinned to ONE fixture,
// models/testdata/name-rule.json, read by models/naming_test.go and by the integrations test
// (tests/lib/naming.test.ts, from the fork checkout) -- change them together.
//
// Returns an i18n KEY with its {param} args, like BrandProblem.

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// NameMaxLen is the standard's limit: the input field and the server enforce 30 (the
	// guidance is 28 or fewer). For a branded list it applies to the part after the brand prefix.
	NameMaxLen = 30

	// CatalogCampaignPrefix marks a render-catalog draft (`CATALOG <stem>`, integrations
	// lib/campaign-review/catalog-sync.ts): code-named and found by name, exempt from the rule.
	CatalogCampaignPrefix = "CATALOG "

	NameKindCampaign = "campaign"
	NameKindList     = "list"
	NameKindTemplate = "template"
)

// IsExemptName reports whether a name is code-named for its kind (a `CATALOG ` campaign, an
// `Official_` template): exempt from the length rule and never renamed by the endpoints.
func IsExemptName(kind, name string) bool {
	switch kind {
	case NameKindCampaign:
		return strings.HasPrefix(name, CatalogCampaignPrefix)
	case NameKindTemplate:
		return IsOfficialTemplate(name)
	}
	return false
}

// ListNameRemainder is the part of a list name the 30-character limit applies to: when the list
// has a brand and the name starts with EXACTLY `<SLUG-UPPERCASED> ` (a private list) or
// `<brandDisplay> ` (a public list), the remainder; otherwise the whole name. Exact match only --
// never a heuristic (D6).
func ListNameRemainder(name, brandSlug, brandDisplay, listType string) string {
	if brandSlug == "" {
		return name
	}
	prefix := ""
	switch listType {
	case ListTypePublic:
		prefix = brandDisplay
	default:
		prefix = strings.ToUpper(brandSlug)
	}
	if prefix != "" && strings.HasPrefix(name, prefix+" ") {
		return strings.TrimPrefix(name, prefix+" ")
	}
	return name
}

// NameProblem validates a name of the given kind. The name is judged trimmed (the callers store
// the trimmed name): non-empty, no control character, and at most NameMaxLen runes -- a campaign
// unless it starts with `CATALOG `, a template unless it starts with `Official_`, a list on the
// part after its brand prefix (ListNameRemainder). "" = valid.
func NameProblem(kind, name, brandSlug, brandDisplay, listType string) (string, []string) {
	n := strings.TrimSpace(name)
	if n == "" {
		return "naming.nameEmpty", nil
	}
	for _, r := range n {
		if unicode.IsControl(r) {
			return "naming.nameControl", []string{"name", n}
		}
	}
	measured := n
	switch kind {
	case NameKindCampaign, NameKindTemplate:
		if IsExemptName(kind, n) {
			return "", nil
		}
	case NameKindList:
		measured = ListNameRemainder(n, brandSlug, brandDisplay, listType)
	}
	if utf8.RuneCountInString(measured) > NameMaxLen {
		return "naming.nameTooLong", []string{"name", n, "max", strconv.Itoa(NameMaxLen)}
	}
	return "", nil
}
