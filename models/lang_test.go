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
