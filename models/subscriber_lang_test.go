package models

import (
	"reflect"
	"testing"
)

// TestNormalizeSubscriberLang -- integrations LIST-GRID-SPEC I10. The primary subtag must EQUAL
// a CampaignLangs code: the last six cases are the ones a first-two-letters rule would accept.
func TestNormalizeSubscriberLang(t *testing.T) {
	cases := []struct {
		name string
		in   JSON
		ok   bool
		want JSON // the attribs afterwards
	}{
		{"nil attribs", nil, true, nil},
		{"absent", JSON{"city": "x"}, true, JSON{"city": "x"}},
		{"empty", JSON{"lang": "", "city": "x"}, true, JSON{"city": "x"}},
		{"blank", JSON{"lang": "  "}, true, JSON{}},
		{"json null", JSON{"lang": nil}, true, JSON{}},
		{"fr", JSON{"lang": "fr"}, true, JSON{"lang": "fr"}},
		{"FR", JSON{"lang": "FR"}, true, JSON{"lang": "fr"}},
		{"padded", JSON{"lang": " fr "}, true, JSON{"lang": "fr"}},
		{"fr-CA", JSON{"lang": "fr-CA"}, true, JSON{"lang": "fr"}},
		{"fr_CA", JSON{"lang": "fr_CA", "city": "x"}, true, JSON{"lang": "fr", "city": "x"}},

		{"pt", JSON{"lang": "pt"}, false, JSON{"lang": "pt"}},
		{"eng", JSON{"lang": "eng"}, false, JSON{"lang": "eng"}},
		{"estonian", JSON{"lang": "estonian"}, false, JSON{"lang": "estonian"}},
		{"italy", JSON{"lang": "italy"}, false, JSON{"lang": "italy"}},
		{"123", JSON{"lang": "123"}, false, JSON{"lang": "123"}},
		{"non-string number", JSON{"lang": float64(5)}, false, JSON{"lang": float64(5)}},
		{"non-string bool", JSON{"lang": true}, false, JSON{"lang": true}},
		{"non-string list", JSON{"lang": []any{"fr"}}, false, JSON{"lang": []any{"fr"}}},
		{"separator only", JSON{"lang": "-"}, false, JSON{"lang": "-"}},
	}
	for _, c := range cases {
		if ok := NormalizeSubscriberLang(c.in); ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
		}
		if !reflect.DeepEqual(c.in, c.want) {
			t.Errorf("%s: attribs = %v, want %v", c.name, c.in, c.want)
		}
	}

	// Every campaign language round-trips, in any case and with a region.
	for _, l := range CampaignLangs {
		for _, v := range []string{l, " " + l, l + "-XX", l + "_xx"} {
			if code, ok := SubscriberLangCode(v); !ok || code != l {
				t.Errorf("SubscriberLangCode(%q) = %q, %v", v, code, ok)
			}
		}
	}
}
