package models

import "testing"

func TestNormalizeLang(t *testing.T) {
	if !NormalizeLang(nil) {
		t.Fatal("nil attribs must be ok")
	}
	a := JSON{"preheader": "x"}
	if !NormalizeLang(a) {
		t.Fatal("absent lang must be ok")
	}
	a = JSON{"lang": ""}
	if !NormalizeLang(a) {
		t.Fatal("empty lang must be ok")
	}
	if _, present := a["lang"]; present {
		t.Fatal("empty lang must delete the key")
	}
	for _, l := range CampaignLangs {
		if !NormalizeLang(JSON{"lang": l}) {
			t.Fatalf("%s must be ok", l)
		}
	}
	for _, bad := range []any{"EN", "pt", "fr-CA", " fr", 1, true, nil, []string{"fr"}} {
		if NormalizeLang(JSON{"lang": bad}) {
			t.Fatalf("%v must be rejected", bad)
		}
	}
	c := Campaign{Attribs: JSON{"lang": "de"}}
	if c.Lang() != "de" {
		t.Fatal("Lang() must read attribs.lang")
	}
	if (&Campaign{}).Lang() != "" {
		t.Fatal("no attribs -> everyone")
	}
}

// TestDefaultCampaignLang pins SHALA-CUTOVER-SPEC I5's pure half: the create-time default.
// The end-to-end create/clone/update behaviour is cmd/campaign_lang_default_db_test.go.
func TestDefaultCampaignLang(t *testing.T) {
	if got := DefaultCampaignLang(nil, CampaignTypeRegular); got["lang"] != "en" {
		t.Fatalf("nil attribs must yield lang en, got %v", got)
	}
	if got := DefaultCampaignLang(JSON{"preheader": "x"}, CampaignTypeRegular); got["lang"] != "en" || got["preheader"] != "x" {
		t.Fatalf("absent lang must be defaulted without disturbing other keys, got %v", got)
	}
	if got := DefaultCampaignLang(JSON{"lang": "fr"}, CampaignTypeRegular); got["lang"] != "fr" {
		t.Fatalf("an explicit language must be kept, got %v", got)
	}
	// An opt-in campaign is the confirmation mail for a double opt-in list -- defaulting it
	// would stop confirmations reaching non-English subscribers.
	if got := DefaultCampaignLang(nil, CampaignTypeOptin); got != nil {
		t.Fatalf("optin campaigns must not be defaulted, got %v", got)
	}
	if got := DefaultCampaignLang(JSON{}, CampaignTypeOptin); len(got) != 0 {
		t.Fatalf("optin campaigns must not be defaulted, got %v", got)
	}
	// The default must be a value NormalizeLang accepts, or every create would 400.
	if !NormalizeLang(JSON{"lang": CampaignLangDefault}) {
		t.Fatal("CampaignLangDefault must be a member of CampaignLangs")
	}
}

// TestSendLangCode -- the "+" convention's server twin: only the default language, whose send
// audience also takes in the no-language subscribers, carries the "+".
func TestSendLangCode(t *testing.T) {
	for in, want := range map[string]string{"en": "EN+", "EN": "EN+", "fr": "FR", "": "", "en-GB": "EN-GB"} {
		if got := SendLangCode(in); got != want {
			t.Fatalf("SendLangCode(%q) = %q, want %q", in, got, want)
		}
	}
}
