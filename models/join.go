package models

// Fork (join link, integrations listmonk/JOIN-LINK-SPEC D4). JoinPlan is one row of the
// get-join-plan query: join_plan's outcome for (subscriber, campaign) plus what the page shows.
// Outcome is one of the JoinOutcome* values; only JoinOutcomeJoin ever leads to a write.
type JoinPlan struct {
	Outcome   string `db:"outcome"`
	Brand     string `db:"brand"`
	ListID    int    `db:"list_id"`
	Lang      string `db:"lang"`
	Email     string `db:"email"`
	SubLang   string `db:"sub_lang"`
	FromEmail string `db:"from_email"`
}

// The join_plan outcomes (D4's table, first match wins).
const (
	JoinOutcomeNoCampaign = "no_campaign"
	JoinOutcomeNoBrand    = "no_brand"
	JoinOutcomeNoMaster   = "no_master"
	JoinOutcomeRefused    = "refused"
	JoinOutcomeAlready    = "already"
	JoinOutcomeCannot     = "cannot"
	JoinOutcomeJoin       = "join"
)

// JoinResult is the join-brand query's row: whether the master row was inserted, and how many
// win-back rows were deleted with it.
type JoinResult struct {
	Joined  bool `db:"joined"`
	Removed int  `db:"removed"`
}
