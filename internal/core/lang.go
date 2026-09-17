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

// KeepCampaignLang (fork, integrations LIST-GRID-SPEC D11) makes a REGULAR campaign's language
// un-clearable: an update whose attribs arrive without a language gets the stored one back.
// models.NormalizeLang is deliberately unchanged -- its "" => delete-the-key branch serves
// opt-in campaigns, which stay language-less and may clear a hand-set language -- so by the
// time this runs an "All"/"" post has already become an absent key.
//
// It must run BEFORE LangLockedChange: the campaign form posts attribs without the lang key
// (it deletes an empty one), and on a STARTED campaign an absent key would read as a change to
// "" and turn every unrelated save -- a preheader edit -- into a lang-lock 400.
//
// attribs nil = the request sent none; update-campaign's COALESCE keeps the stored value.
// prevLang "" = a campaign that has no language to keep (legacy, written past core); it stays
// language-less and LangRequiredForStatus stops it from starting.
func KeepCampaignLang(campType, prevLang string, attribs models.JSON) models.JSON {
	if campType == models.CampaignTypeOptin || attribs == nil || prevLang == "" {
		return attribs
	}
	if s, ok := attribs["lang"].(string); ok && s != "" {
		return attribs
	}
	attribs["lang"] = prevLang
	return attribs
}

// LangRequiredForStatus (fork, LIST-GRID-SPEC D11) reports whether moving cm to status must be
// refused because it is a REGULAR campaign with no language. A language-less regular campaign
// reaches every reader of every language -- English to the fr/es/de/it readers -- so it can
// enter neither running nor scheduled. Scheduling is guarded too because the scheduler starts a
// scheduled campaign with no further handler. Opt-in campaigns are exempt: they are the double
// opt-in confirmation and must reach every language.
func LangRequiredForStatus(cm models.Campaign, status string) bool {
	if cm.Type == models.CampaignTypeOptin || cm.Lang() != "" {
		return false
	}
	return status == models.CampaignStatusRunning || status == models.CampaignStatusScheduled
}
