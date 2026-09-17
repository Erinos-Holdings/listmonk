package models

import "strings"

// Fork (integrations LIST-GRID-SPEC D13) -- the ONE normalizer of an incoming subscriber
// attribs.lang. It is the rule the import presets (subimporter.Preset.LangFor, re-pointed here)
// and the integrations repo's lib/language.ts::toLang already apply.

// SubscriberLangCode normalizes one language/locale value: trimmed, lower-cased, split on "-" or
// "_", and the PRIMARY SUBTAG must equal a CampaignLangs code -- FR, fr-CA and fr_CA are fr.
// An empty value is ("", true): no language. Anything else is ("", false).
//
// NOT first-two-letters. The send SQL's LEFT(lang, 2) is a tolerance for rows already stored,
// and as a WRITE rule it would store "estonian" as es, "italy" as it and "eng" as en forever.
func SubscriberLangCode(v string) (code string, ok bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "", true
	}
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) == 0 || !IsCampaignLang(parts[0]) {
		return "", false
	}
	return parts[0], true
}

// NormalizeSubscriberLang validates attribs.lang IN PLACE. Absent, JSON null or empty: the key
// is removed (the subscriber has no language; an en campaign reaches them) and ok is true. A
// value whose primary subtag is a CampaignLangs code is stored as that code, ok true. Anything
// else -- an unknown language, a non-string -- is left untouched and ok is false; the caller
// decides (the admin API refuses with a 400, a bulk import drops the key and counts the row).
//
// WHERE THIS RUNS matters as much as what it does (spec review H3): on the INCOMING value at the
// cmd admin/API edge and in the importer, never in core and never on the public subscription
// paths -- a row already holding a legacy value must stay editable, and a member of the public
// must never be refused over an admin's data defect. Attribs may be nil.
func NormalizeSubscriberLang(attribs JSON) (ok bool) {
	v, present := attribs["lang"]
	if !present {
		return true
	}
	if v == nil {
		delete(attribs, "lang")
		return true
	}
	s, isStr := v.(string)
	if !isStr {
		return false
	}
	code, ok := SubscriberLangCode(s)
	if !ok {
		return false
	}
	if code == "" {
		delete(attribs, "lang")
	} else {
		attribs["lang"] = code
	}
	return true
}
