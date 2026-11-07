package main

// Fork (smart sending) -- integrations listmonk/SMART-SENDING-SPEC I6: attribs.smart_sending is
// accepted absent / true / false only at the API. TestValidSmartSending is the pure rule
// (models.ValidSmartSending, which validateCampaignFields calls); TestSmartSendingValidation drives
// the UpdateCampaign handler (validateCampaignFields) against a real database -- LISTMONK_TEST_PG
// opt-in, newLinkHarness -- so "no" is a 400 naming the field and nothing is stored.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/knadh/listmonk/models"
)

func TestValidSmartSending(t *testing.T) {
	for name, tc := range map[string]struct {
		attribs models.JSON
		ok      bool
	}{
		"nil attribs": {nil, true},
		"absent":      {models.JSON{"lang": "en"}, true},
		"true":        {models.JSON{"smart_sending": true}, true},
		"false":       {models.JSON{"smart_sending": false}, true},
		"string no":   {models.JSON{"smart_sending": "no"}, false},
		"string true": {models.JSON{"smart_sending": "true"}, false},
		"number":      {models.JSON{"smart_sending": 0.0}, false},
		"null":        {models.JSON{"smart_sending": nil}, false},
	} {
		if got := models.ValidSmartSending(tc.attribs); got != tc.ok {
			t.Errorf("%s: %v, want %v", name, got, tc.ok)
		}
	}
	// The accessor: off only for the boolean false.
	for attribs, want := range map[string]bool{`{}`: true, `{"smart_sending": true}`: true, `{"smart_sending": false}`: false, `{"smart_sending": "n/a"}`: true} {
		var c models.Campaign
		if err := json.Unmarshal([]byte(attribs), &c.Attribs); err != nil {
			t.Fatal(err)
		}
		if got := c.SmartSending(); got != want {
			t.Errorf("%s: SmartSending() = %v, want %v", attribs, got, want)
		}
	}
}

func TestSmartSendingValidation(t *testing.T) {
	h := newLinkHarness(t)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'single') RETURNING id`)
	lists := fmt.Sprintf(`"lists": [%d]`, list)
	c := newLangCampaign(t, h, list, models.JSON{"lang": "en"})

	code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "attribs": {"lang": "en", "smart_sending": "no"}}`)
	if code != http.StatusBadRequest || msg != h.app.i18n.T("campaigns.invalidSmartSending") {
		t.Fatalf("smart_sending \"no\": %d %q, want 400 %q", code, msg, h.app.i18n.T("campaigns.invalidSmartSending"))
	}
	if h.app.i18n.T("campaigns.invalidSmartSending") == "campaigns.invalidSmartSending" {
		t.Fatal("campaigns.invalidSmartSending is missing from en.json")
	}
	for _, v := range []string{"false", "true"} {
		if code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "attribs": {"lang": "en", "smart_sending": `+v+`}}`); code != http.StatusOK {
			t.Fatalf("smart_sending %s: %d %s", v, code, msg)
		}
		got, err := h.app.core.GetCampaign(c.ID, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if b, ok := got.Attribs["smart_sending"].(bool); !ok || fmt.Sprint(b) != v {
			t.Fatalf("stored smart_sending %v, want %s", got.Attribs["smart_sending"], v)
		}
	}
}
