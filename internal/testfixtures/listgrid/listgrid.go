// Package listgrid is the shared DB-test fixture of the fork's Lists-page grid (integrations
// LIST-GRID-SPEC I1, I2, I3, I5, I12). It is imported by tests only. Expect is deliberately an
// INDEPENDENT statement of the spec's D1/D2 tables in Go -- the tests compare the SQL functions,
// the view, the filter and the send query against it, never against each other alone.
package listgrid

import (
	_ "embed"
	"strings"

	"github.com/jmoiron/sqlx"
)

//go:embed fixture.sql
var fixtureSQL string

const (
	SingleList = "grid-single"
	DoubleList = "grid-double"
)

// Segments and Buckets in the spec's order.
var (
	Segments = []string{"active", "held", "unsubscribed", "pending", "blocked"}
	Buckets  = []string{"none", "en", "es", "fr", "de", "it", "other"}
)

// variantBucket is the storage bucket each fixture language variant must land in (spec I1).
var variantBucket = map[string]string{
	"absent": "none", "jsonnull": "none", "empty": "none",
	"space": "other", "spacefr": "other", "nonstring": "other", "pt": "other",
	"upperen": "en", "upperfr": "fr", "frca": "fr", "es": "es", "de": "de", "upperit": "it",
}

// Row is one fixture subscriber, decoded from its e-mail.
type Row struct {
	ID       int    `db:"id"`
	Email    string `db:"email"`
	Variant  string
	SStatus  string
	SLStatus string
	Hold     bool
}

// Load inserts the fixture and returns the ids of the single and double opt-in lists.
func Load(db *sqlx.DB) (single, double int, err error) {
	if _, err = db.Exec(fixtureSQL); err != nil {
		return 0, 0, err
	}
	if err = db.Get(&single, `SELECT id FROM lists WHERE name = $1`, SingleList); err != nil {
		return 0, 0, err
	}
	err = db.Get(&double, `SELECT id FROM lists WHERE name = $1`, DoubleList)
	return single, double, err
}

// Rows returns every fixture subscriber.
func Rows(db *sqlx.DB) ([]Row, error) {
	var out []Row
	if err := db.Select(&out, `SELECT id, email FROM subscribers WHERE email LIKE '%@grid.test' ORDER BY id`); err != nil {
		return nil, err
	}
	for i, r := range out {
		p := strings.Split(strings.TrimSuffix(r.Email, "@grid.test"), "-")
		out[i].Variant, out[i].SStatus, out[i].SLStatus, out[i].Hold = p[0], p[1], p[2], p[3] == "hold"
	}
	return out, nil
}

// Bucket is the row's expected storage language bucket (D2).
func (r Row) Bucket() string { return variantBucket[r.Variant] }

// SendLang is the row's expected send language -- none folds into en (D2).
func (r Row) SendLang() string {
	if b := r.Bucket(); b != "none" {
		return b
	}
	return "en"
}

// Segment is the row's expected segment on a list with the given opt-in (D1, first match wins).
func (r Row) Segment(doubleOptin bool) string {
	switch {
	case r.SStatus == "blocklisted":
		return "blocked"
	case r.SLStatus == "unsubscribed" && r.Hold:
		return "held"
	case r.SLStatus == "unsubscribed":
		return "unsubscribed"
	case r.SLStatus == "unconfirmed" && doubleOptin:
		return "pending"
	}
	return "active"
}

// LegacyStatus is the row's v6.2.10 subscriber_statuses key -- the raw subscription status, or
// held for an unsubscribed row with a hold on a subscriber who is NOT blocklisted (I5).
func (r Row) LegacyStatus() string {
	if r.SLStatus == "unsubscribed" && r.Hold && r.SStatus != "blocklisted" {
		return "held"
	}
	return r.SLStatus
}
