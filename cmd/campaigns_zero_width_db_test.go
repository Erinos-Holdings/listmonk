package main

import (
	"strings"
	"testing"
)

// Fork (CAMPAIGN-52-HARDENING T7, I9) -- the handler half: a stored campaign read the way
// UpdateCampaign / UpdateCampaignStatus read it (core.GetCampaign) warns on a zero-width
// subject / preheader and stays silent on a clean one, whatever the body carries (the
// preheader filler injects U+200C into every rendered body by design). Shares
// newLinkHarness (LISTMONK_TEST_PG opt-in).
func TestZeroWidthWarningsHandler(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db

	newCampaign := func(subject, attribs string) int {
		var id int
		db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, content_type, messenger, template_id, attribs) VALUES (gen_random_uuid(), 'C', $1, 'f@x', '<p>&zwnj;body&#8203;</p>', 'richtext', 'email', (SELECT id FROM templates LIMIT 1), $2::jsonb) RETURNING id`, subject, attribs)
		return id
	}
	read := func(id int) []string {
		c, err := h.app.core.GetCampaign(id, "", "")
		if err != nil {
			t.Fatalf("GetCampaign %d: %v", id, err)
		}
		return h.app.zeroWidthWarnings(&c)
	}

	// Campaign 52's shape: ZWSP around the emoji, an English preheader.
	dirty := newCampaign("Riscatta adesso \u200b ✨ \u200b - Il tuo set", `{"lang":"it","preheader":"Curated Loyalty Rewards"}`)
	w := read(dirty)
	if len(w) != 1 || !strings.Contains(w[0], "subject") {
		t.Fatalf("dirty subject: want one warning naming the subject, got %q", w)
	}

	both := newCampaign("x\u200dy", `{"preheader":"\ufeffIl tuo set"}`)
	if w := read(both); len(w) != 2 || !strings.Contains(w[0], "subject") || !strings.Contains(w[1], "preheader") {
		t.Fatalf("both fields: got %q", w)
	}

	// Clean fields, body full of zero-width entities → silent.
	clean := newCampaign("Riscatta adesso uno sconto del 50% su un set esclusivo ✨", `{"preheader":"Il tuo set ricompensa è pronto per essere riscattato."}`)
	if w := read(clean); len(w) != 0 {
		t.Fatalf("clean: want no warnings, got %q", w)
	}
}
