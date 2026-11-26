package models

import (
	"net/url"
	"strings"
	"time"

	"github.com/lib/pq"
	null "gopkg.in/volatiletech/null.v6"
)

// Brand is one row of the brands table (fork, BRAND-PICKER-SPEC D1): a brand's sending identity.
// Lists reference it by slug; their brand:/from:/site: tags are its projection (BrandProjection).
type Brand struct {
	Slug      string      `db:"slug" json:"slug"`
	FromEmail string      `db:"from_email" json:"from_email"`
	Site      null.String `db:"site" json:"site"`
	CreatedAt time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt time.Time   `db:"updated_at" json:"updated_at"`

	// DisplayName is the From's display-name part, or the bare address when it has none
	// (computed, never stored) -- the "brand name" the successor PERSONA-FROM spec means.
	DisplayName string `db:"-" json:"display_name"`

	// Fork (persona From, integrations PERSONA-FROM-SPEC D1). Personas is the brand's approved
	// persona display names (models/personas.go), in stored order; `{}` until a human adds one.
	// Address is the From's bare address (computed, never stored): a persona From is
	// `<persona> <address>`.
	Personas pq.StringArray `db:"personas" json:"personas"`
	Address  string         `db:"-" json:"address"`

	// Fork (approved brand logos, integrations LOGO-APPROVAL-SPEC D3). The files approved and
	// rejected as the brand's logo for the campaign review's D3.5, in insertion order; `{}` until a
	// brands:logos holder decides. Disjoint and duplicate-free, every entry a LogoURLProblem-clean
	// URL. Their ONE writer is the update-brand-logos query (POST /api/brands/:slug/logos).
	LogosApproved pq.StringArray `db:"logos_approved" json:"logos_approved"`
	LogosRejected pq.StringArray `db:"logos_rejected" json:"logos_rejected"`
}

// Fork (approved brand logos, LOGO-APPROVAL-SPEC D3/D5) -- the logo actions of
// POST /api/brands/:slug/logos.
const (
	BrandLogoApprove = "approve"
	BrandLogoReject  = "reject"
	BrandLogoRemove  = "remove"
)

// BrandLogoURLMax is the longest logo URL a brand's lists may hold, in bytes.
const BrandLogoURLMax = 2048

// LogoURLProblem is the ONE rule on an approved or rejected logo entry (LOGO-APPROVAL-SPEC D3): a
// non-empty, trimmed, absolute http or https URL with a host, at most BrandLogoURLMax bytes. It
// returns the i18n key of the first problem, or "" when the URL is acceptable. The caller trims
// before calling (an untrimmed value is refused, never silently fixed here).
func LogoURLProblem(u string) string {
	if u == "" {
		return "brands.logoURLEmpty"
	}
	if strings.TrimSpace(u) != u {
		return "brands.logoURLInvalid"
	}
	if len(u) > BrandLogoURLMax {
		return "brands.logoURLTooLong"
	}
	p, err := url.Parse(u)
	if err != nil || !p.IsAbs() || (p.Scheme != "http" && p.Scheme != "https") || p.Hostname() == "" {
		return "brands.logoURLInvalid"
	}
	return ""
}

// BrandDisplayName returns the display-name part of a `Display Name <address>` From, or the
// bare address when the From has none.
func BrandDisplayName(from string) string {
	if m := ReFromAddress.FindStringSubmatch(from); len(m) == 5 && m[2] != "" {
		return m[2]
	}
	return BareAddress(from)
}
