package models

import (
	"time"

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
}

// BrandDisplayName returns the display-name part of a `Display Name <address>` From, or the
// bare address when the From has none.
func BrandDisplayName(from string) string {
	if m := ReFromAddress.FindStringSubmatch(from); len(m) == 5 && m[2] != "" {
		return m[2]
	}
	return BareAddress(from)
}
