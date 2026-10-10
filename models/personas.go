package models

// Fork (persona From, integrations PERSONA-FROM-SPEC D2/D3). A persona is an approved
// display-name variant on a brand's existing From address -- `Natasha at Thirsty Girl
// <hello@thirstygirlhydration.com>` -- stored as the display name alone on the brands row
// (brands.personas) and chosen per campaign. The address never changes.
//
// This file is the ONE rule body (CAMPAIGN-52-HARDENING D7): the personas API, the brand-row
// update and the campaign From validation all call it, and the campaign editor has no mirror --
// it shows the server's message. The functions return i18n KEYS with their {param} args, like
// BrandProblem.

import (
	"net/mail"
	"strconv"
	"strings"
)

const (
	// PersonaMaxLen caps one persona display name.
	PersonaMaxLen = 60
	// PersonasMax caps a brand's persona set.
	PersonasMax = 10
)

// PersonaBlockingStatuses are the campaign statuses whose From blocks a persona's removal:
// "editable or running" -- canEditCampaign's statuses plus running. Finished and cancelled are
// terminal and never block.
var PersonaBlockingStatuses = []string{
	CampaignStatusDraft,
	CampaignStatusScheduled,
	CampaignStatusPaused,
	CampaignStatusRunning,
}

// PersonaCampaign is one campaign carrying a persona From (GET /api/brands/:slug/personas).
type PersonaCampaign struct {
	ID     int    `db:"id" json:"id"`
	Name   string `db:"name" json:"name"`
	Status string `db:"status" json:"status"`
}

// PersonaUse is one persona with the campaigns that block its removal.
type PersonaUse struct {
	Name      string            `json:"name"`
	Campaigns []PersonaCampaign `json:"campaigns"`
}

// PersonaFrom is the canonical campaign From for a persona: `<persona> <bare address>`. The ONE
// composition rule -- the campaign validation's acceptance and the in-use query both use it.
func PersonaFrom(persona, brandFrom string) string {
	return persona + " <" + BareAddress(brandFrom) + ">"
}

// PersonaFroms is PersonaFrom over a set, in order.
func PersonaFroms(personas []string, brandFrom string) []string {
	out := make([]string, 0, len(personas))
	for _, p := range personas {
		out = append(out, PersonaFrom(p, brandFrom))
	}
	return out
}

// IsPersonaFrom reports whether from is the canonical From of one of the brand's personas: the
// same bare address as the brand From AND byte-equal to PersonaFrom(p, brandFrom) for some p.
// Both conditions are stated so the address invariant is asserted, not only implied.
func IsPersonaFrom(from, brandFrom string, personas []string) bool {
	if BareAddress(from) != BareAddress(brandFrom) {
		return false
	}
	for _, p := range personas {
		if from == PersonaFrom(p, brandFrom) {
			return true
		}
	}
	return false
}

// PersonaProblem validates one persona display name against the brand's From, on the value AS
// GIVEN (the API trims outer whitespace first). It returns the i18n key of the first problem
// with its args, or "" when the persona is acceptable.
func PersonaProblem(persona, brandFrom string) (string, []string) {
	if strings.TrimSpace(persona) == "" {
		return "brands.personaEmpty", nil
	}
	if len(persona) > PersonaMaxLen {
		return "brands.personaTooLong", []string{"persona", persona, "max", strconv.Itoa(PersonaMaxLen)}
	}

	// Printable ASCII only: no control character (CR/LF/TAB/DEL -- header injection) and no
	// non-ASCII (net/mail would RFC 2047-encode it, which some clients show raw).
	for _, r := range persona {
		if r < 0x20 || r > 0x7e {
			return "brands.personaInvalid", []string{"persona", persona}
		}
	}

	// THE WIRE RULE. The From header is not from_email verbatim: smtppool.formatAddress
	// re-serialises it through mail.ParseAddress(...).String(), and a value net/mail cannot parse
	// fails every send of the campaign. So the persona must survive that parser unchanged, which
	// refuses by the parser's own rules every RFC 5322 special that needs quoting (so `at`, never
	// `@`), parentheses (a comment, silently dropped from the name) and repeated or outer spaces
	// (collapsed, so the stored name and the wire name would differ).
	bare := BareAddress(brandFrom)
	a, err := mail.ParseAddress(PersonaFrom(persona, brandFrom))
	if err != nil || a.Name != persona || a.Address != bare {
		return "brands.personaInvalid", []string{"persona", persona}
	}

	// Case-sensitive on purpose: the brand name appears exactly as the brand From spells it.
	name := BrandDisplayName(brandFrom)
	if !strings.Contains(persona, name) {
		return "brands.personaNoBrandName", []string{"persona", persona, "brand", name}
	}
	// The brand From is already the first choice; the display name alone would be it twice.
	if persona == name {
		return "brands.personaIsBrand", []string{"persona", persona}
	}

	return "", nil
}

// PersonasProblem validates a brand's whole persona set: every element by PersonaProblem, then
// at most PersonasMax, then unique ignoring case.
func PersonasProblem(personas []string, brandFrom string) (string, []string) {
	for _, p := range personas {
		if key, args := PersonaProblem(p, brandFrom); key != "" {
			return key, args
		}
	}
	if len(personas) > PersonasMax {
		return "brands.personasTooMany", []string{"max", strconv.Itoa(PersonasMax)}
	}
	seen := map[string]string{}
	for _, p := range personas {
		k := strings.ToLower(p)
		if first, ok := seen[k]; ok {
			return "brands.personaDuplicate", []string{"first", first, "second", p}
		}
		seen[k] = p
	}
	return "", nil
}
