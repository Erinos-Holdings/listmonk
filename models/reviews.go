package models

import (
	"encoding/json"
	"errors"
	"time"

	null "gopkg.in/volatiletech/null.v6"
)

// Fork (campaign review, integrations CAMPAIGN-INSPECT-SPEC D5). The review Lambda computes the
// report; the fork stores it verbatim (JSONB) and reads only what the verdict rule needs
// (internal/review/verdict.go).

// Review statuses (campaign_reviews.status).
const (
	ReviewStatusRunning  = "running"
	ReviewStatusComplete = "complete"
	ReviewStatusFailed   = "failed"
	ReviewStatusStale    = "stale"
)

// Disposition actions (campaign_review_dispositions.action): Accept risk / Acknowledge, I'll fix
// it, Fix for me.
const (
	DispositionAccept = "accept"
	DispositionFixme  = "fixme"
	DispositionFixed  = "fixed"
)

// CampaignReview is one inspection of a campaign at one bundle hash.
type CampaignReview struct {
	ID          int64        `db:"id" json:"id"`
	CampaignID  int          `db:"campaign_id" json:"campaign_id"`
	BundleHash  string       `db:"bundle_hash" json:"bundle_hash"`
	JobID       string       `db:"job_id" json:"job_id"`
	Status      string       `db:"status" json:"status"`
	Progress    NullableJSON `db:"progress" json:"progress"`
	Report      NullableJSON `db:"report" json:"report"`
	Error       string       `db:"error" json:"error"`
	RequestedBy string       `db:"requested_by" json:"requested_by"`
	RequestedAt time.Time    `db:"requested_at" json:"requested_at"`
	UpdatedAt   time.Time    `db:"updated_at" json:"updated_at"`
}

// NullableJSON is a JSONB column that may be NULL: it scans NULL as empty and marshals it as JSON
// null, anything else verbatim (json.RawMessage cannot scan a NULL).
type NullableJSON json.RawMessage

// Scan implements sql.Scanner.
func (j *NullableJSON) Scan(src any) error {
	switch t := src.(type) {
	case nil:
		*j = nil
	case []byte:
		*j = append((*j)[0:0], t...)
	case string:
		*j = NullableJSON(t)
	default:
		return errors.New("NullableJSON: incompatible type")
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (j NullableJSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *NullableJSON) UnmarshalJSON(b []byte) error {
	*j = append((*j)[0:0], b...)
	return nil
}

// ReviewDisposition is one row of the append-only Accept-risk log.
type ReviewDisposition struct {
	ID         int64     `db:"id" json:"id"`
	CampaignID int       `db:"campaign_id" json:"campaign_id"`
	BundleHash string    `db:"bundle_hash" json:"bundle_hash"`
	ItemKey    string    `db:"item_key" json:"item_key"`
	RubricID   string    `db:"rubric_id" json:"rubric_id"`
	Action     string    `db:"action" json:"action"`
	Note       string    `db:"note" json:"note"`
	UserID     null.Int  `db:"user_id" json:"user_id"`
	Username   string    `db:"username" json:"username"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

// StructureVerification is a clean rendering-matrix record for one structure fingerprint.
type StructureVerification struct {
	Fingerprint string          `db:"fingerprint" json:"fingerprint"`
	VerifiedAt  time.Time       `db:"verified_at" json:"verified_at"`
	TestID      string          `db:"test_id" json:"test_id"`
	Components  json.RawMessage `db:"components" json:"components"`
	VerifiedBy  string          `db:"verified_by" json:"verified_by"`
}

// ReviewStat is one rubric id's disposition counts (the Accept-risk rate, parent spec §6).
type ReviewStat struct {
	RubricID  string    `db:"rubric_id" json:"rubric_id"`
	Accepts   int       `db:"accepts" json:"accepts"`
	Fixmes    int       `db:"fixmes" json:"fixmes"`
	Fixed     int       `db:"fixed" json:"fixed"`
	Campaigns int       `db:"campaigns" json:"campaigns"`
	Last      time.Time `db:"last" json:"last"`
}
