package models

// Fork (campaign mode, integrations CAMPAIGN-MODE-SPEC D5/D6) -- the top-level rows of the grouped
// Campaigns list (GET /api/campaigns?grouped=true) and the grouped Analytics picker
// (GET /api/analytics/campaigns?grouped=true). The grouping rule is internal/core/campaign_groups.go.

import (
	"encoding/json"

	"github.com/jmoiron/sqlx/types"
	"github.com/lib/pq"
	null "gopkg.in/volatiletech/null.v6"
)

// Campaign group kinds.
const (
	CampaignGroupKindSeries   = "series"
	CampaignGroupKindSet      = "set"
	CampaignGroupKindCampaign = "campaign"
)

// CampaignGroupSet is one set of a series: its name (the newest member's), its Step when every
// member agrees, and its member ids (newest first).
type CampaignGroupSet struct {
	Name string   `json:"name"`
	Step null.Int `json:"step"`
	IDs  []int    `json:"ids"`
}

// CampaignGroupBase is the grouped statement's row: kind (series | set | campaign), the group brand,
// the Code as stored on the newest member, the Step when every member agrees, the newest member's
// name, the member ids (newest first) and, for a series, its sets.
type CampaignGroupBase struct {
	Kind  string         `db:"kind" json:"kind"`
	Brand null.String    `db:"brand" json:"brand"`
	Code  null.String    `db:"code" json:"code"`
	Step  null.Int       `db:"step" json:"step"`
	Name  string         `db:"name" json:"name"`
	IDs   pq.Int64Array  `db:"ids" json:"ids"`
	Total int            `db:"total" json:"-"`
	Raw   types.JSONText `db:"sets" json:"-"`

	Sets []CampaignGroupSet `db:"-" json:"sets,omitempty"`
}

// ParseSets decodes the series' sets (NULL for any other kind).
func (g *CampaignGroupBase) ParseSets() error {
	if len(g.Raw) == 0 || string(g.Raw) == "null" {
		g.Sets = nil
		return nil
	}
	return json.Unmarshal(g.Raw, &g.Sets)
}

// CampaignGroup is a top-level row of the grouped Campaigns list; campaigns are its member rows,
// each exactly the ungrouped list's row, newest first.
type CampaignGroup struct {
	CampaignGroupBase
	Campaigns Campaigns `db:"-" json:"campaigns"`
}

// AnalyticsCampaignGroup is a top-level row of the grouped Analytics picker; campaigns are its
// members' picker rows, newest first, count their number.
type AnalyticsCampaignGroup struct {
	CampaignGroupBase
	Count     int                 `db:"-" json:"count"`
	Campaigns []AnalyticsCampaign `db:"-" json:"campaigns"`
}
