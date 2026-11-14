package main

// Fork (naming standard) -- integrations listmonk/NAMING-STANDARD-SPEC I9 (the cmd half), through
// LinkRedirect: with the setting reading {{ .Campaign.UTMCampaign }} a click carries
// <slug>_<Name>_<lang>; the slug is cached beside the fallback URL ONLY when the list mapping
// resolved (an unmapped read is retried on the next click, a mapped one is served from the cache);
// {{ .Campaign.Name }} keeps the bare name. Needs a database (newLinkHarness, LISTMONK_TEST_PG).

import (
	"net/url"
	"testing"
)

func TestUTMCampaignSlugCache(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db
	t.Cleanup(func() {
		ko.Set("app.utm_params.utm_campaign", "{{ .Campaign.Name }}")
		ko.Delete("app.utm_params.utm_term")
	})

	var acmeList, plainList int
	db.Get(&acmeList, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme', 'public', 'single', '{brand:acme,"from:Acme <hello@acme.test>"}') RETURNING id`)
	db.Get(&plainList, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Plain', 'public', 'single', '{}') RETURNING id`)
	var subUUID string
	db.Get(&subUUID, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 's@x', 'S', '{}') RETURNING uuid`)

	camp := func(name, lang string, listID int) string {
		var id int
		var uuid string
		db.QueryRow(`INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id, attribs)
			VALUES (gen_random_uuid(), $1, 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1), $2) RETURNING id, uuid`,
			name, `{"lang": "`+lang+`"}`).Scan(&id, &uuid)
		db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'x')`, id, listID)
		return uuid
	}
	acmeUUID := camp("No Nicotine", "en", acmeList)
	plainUUID := camp("Plain Thing", "fr", plainList)

	var link string
	db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://curatedfor.you/p') RETURNING uuid`)
	utmOf := func(campUUID, key string) string {
		_, loc, _ := h.redirect(campUUID, subUUID, link)
		u, err := url.Parse(loc)
		if err != nil {
			t.Fatalf("location %q: %v", loc, err)
		}
		return u.Query().Get(key)
	}

	// Dark (the live setting): {{ .Campaign.Name }} is the bare name.
	h.app.linkFallbacks.invalidate()
	if got := utmOf(acmeUUID, "utm_campaign"); got != "No Nicotine" {
		t.Fatalf("dark utm_campaign = %q", got)
	}

	ko.Set("app.utm_params.utm_campaign", "{{ .Campaign.UTMCampaign }}")
	ko.Set("app.utm_params.utm_term", "{{ .Campaign.Name }}")
	h.app.linkFallbacks.invalidate()

	// Mapped: slug_Name_lang; {{ .Campaign.Name }} unchanged beside it.
	if got := utmOf(acmeUUID, "utm_campaign"); got != "acme_No-Nicotine_en" {
		t.Fatalf("mapped utm_campaign = %q", got)
	}
	if got := utmOf(acmeUUID, "utm_term"); got != "No Nicotine" {
		t.Fatalf("utm_term (Name) = %q", got)
	}

	// Unmapped: the slug part is omitted, and the read is NOT trusted from the cache -- tagging the
	// list without invalidating the cache shows on the very next click.
	if got := utmOf(plainUUID, "utm_campaign"); got != "Plain-Thing_fr" {
		t.Fatalf("unmapped utm_campaign = %q", got)
	}
	db.MustExec(`UPDATE lists SET tags = '{brand:plain,"from:Plain <hello@acme.test>"}' WHERE id = $1`, plainList)
	if got := utmOf(plainUUID, "utm_campaign"); got != "plain_Plain-Thing_fr" {
		t.Fatalf("an unmapped read was cached: utm_campaign = %q", got)
	}

	// Mapped: served from the cache (a brand change inside the TTL is not re-read) ...
	db.MustExec(`UPDATE lists SET tags = '{brand:other,"from:Other <hello@acme.test>"}' WHERE id = $1`, acmeList)
	if got := utmOf(acmeUUID, "utm_campaign"); got != "acme_No-Nicotine_en" {
		t.Fatalf("a mapped slug was not cached: %q", got)
	}
	// ... until the cache is refreshed.
	h.app.linkFallbacks.invalidate()
	if got := utmOf(acmeUUID, "utm_campaign"); got != "other_No-Nicotine_en" {
		t.Fatalf("after invalidate: %q", got)
	}
}
