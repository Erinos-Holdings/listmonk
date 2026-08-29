package core

// Fork (erinos multi-language campaigns) -- see queries/campaigns.sql (next-campaigns,
// next-campaign-subscribers) and queries/evergreen.sql for the send-time predicates.

import "github.com/knadh/listmonk/models"

// LangLockedChange reports whether an update would change a STARTED campaign's language.
// Locked once started_at is set because the last_subscriber_id checkpoint window was
// computed for the old population (FR sent to ids <= N, switch to EN, resume -> EN ids <= N
// are skipped while the UI shows a full count). Clone to change language.
//
// Its own check rather than an extension of EvergreenLockedChange, which returns false
// for every non-evergreen campaign. prevLang is the stored value captured BEFORE the
// handler clears cm.Attribs for binding; a request that omits attribs entirely (nil)
// keeps the stored value (COALESCE in update-campaign) and is never a change.
func LangLockedChange(cm models.Campaign, prevLang string, attribs models.JSON) bool {
	if !cm.StartedAt.Valid || attribs == nil {
		return false
	}
	next, _ := attribs["lang"].(string)
	return next != prevLang
}
