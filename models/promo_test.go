package models

// Fork (promo series) -- integrations listmonk/PROMO-SERIES-SPEC T8 (I8): the three fields' rule
// (models/promo.go), no database. The Code shares NameProblem's cases (testdata/name-rule.json, the
// campaign rows without the CATALOG exemption, judged trimmed); Step; tags (count, length, comma,
// control, * placement, the 3-character prefix floor, a duplicate refused as non-normal); the Code
// requirement; every non-normal form refused (Stage 2 S7).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromoCodeSharesNameProblemCases(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "name-rule.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Cases []nameRuleCase `json:"cases"`
	}
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range fx.Cases {
		if c.Kind != NameKindCampaign {
			continue
		}
		code := strings.TrimSpace(c.Name)
		want := c.Want
		// No CATALOG exemption: an exempt name is judged by the length rule like any other.
		if IsExemptName(NameKindCampaign, code) && want == "" && len([]rune(code)) > NameMaxLen {
			want = "naming.nameTooLong"
		}
		if got := PromoCodeProblem(code); got != want {
			t.Errorf("code %q: %q want %q", code, got, want)
		}
		n++
	}
	if n < 10 {
		t.Fatalf("only %d campaign cases", n)
	}
	if PromoCodeProblem("CATALOG 01-button-outlook-word-columns-long") != "naming.nameTooLong" {
		t.Fatal("the CATALOG exemption must not apply to a Code")
	}
}

func TestValidatePromo(t *testing.T) {
	tags := func(v ...any) []any { return v }
	many := func(n int) []any {
		out := []any{}
		for i := 0; i < n; i++ {
			out = append(out, "tag-"+strings.Repeat("x", i+1))
		}
		return out
	}
	ok := map[string]JSON{
		"none":                {},
		"code only":           {PromoCodeAttrib: "BFCM-D3"},
		"code + step 1":       {PromoCodeAttrib: "BFCM-D3", PromoStepAttrib: 1.0},
		"code + step 3":       {PromoCodeAttrib: "BFCM-D3", PromoStepAttrib: 3.0},
		"code + tags":         {PromoCodeAttrib: "C", PromoTagsAttrib: tags("BFCM-Deal-3", "BFCM-BMSM-3")},
		"a * prefix of 3":     {PromoCodeAttrib: "C", PromoTagsAttrib: tags("abc*")},
		"64-character tag":    {PromoCodeAttrib: "C", PromoTagsAttrib: tags(strings.Repeat("x", 64))},
		"20 tags":             {PromoCodeAttrib: "C", PromoTagsAttrib: many(20)},
		"30-character code":   {PromoCodeAttrib: strings.Repeat("Á", 30)},
		"other attribs kept":  {"lang": "en", PromoCodeAttrib: "C", PromoStepAttrib: 2.0, PromoTagsAttrib: tags("x-1")},
		"% and _ are literal": {PromoCodeAttrib: "C", PromoTagsAttrib: tags("50%_off", "a_b%*")},
	}
	for name, a := range ok {
		if got := ValidatePromo(a); got != "" {
			t.Errorf("%s: refused %q", name, got)
		}
	}
	bad := map[string]struct {
		a    JSON
		want string
	}{
		"code not a string":      {JSON{PromoCodeAttrib: 5.0}, "campaigns.invalidPromoCode"},
		"code null":              {JSON{PromoCodeAttrib: nil}, "campaigns.invalidPromoCode"},
		"code empty":             {JSON{PromoCodeAttrib: ""}, "campaigns.invalidPromoCode"},
		"code untrimmed":         {JSON{PromoCodeAttrib: " BFCM "}, "campaigns.invalidPromoCode"},
		"code 31 characters":     {JSON{PromoCodeAttrib: strings.Repeat("x", 31)}, "campaigns.invalidPromoCode"},
		"code control":           {JSON{PromoCodeAttrib: "a\tb"}, "campaigns.invalidPromoCode"},
		"step 0":                 {JSON{PromoCodeAttrib: "C", PromoStepAttrib: 0.0}, "campaigns.invalidPromoStep"},
		"step 4":                 {JSON{PromoCodeAttrib: "C", PromoStepAttrib: 4.0}, "campaigns.invalidPromoStep"},
		"step 2.5":               {JSON{PromoCodeAttrib: "C", PromoStepAttrib: 2.5}, "campaigns.invalidPromoStep"},
		"step string":            {JSON{PromoCodeAttrib: "C", PromoStepAttrib: "2"}, "campaigns.invalidPromoStep"},
		"step bool":              {JSON{PromoCodeAttrib: "C", PromoStepAttrib: true}, "campaigns.invalidPromoStep"},
		"step null":              {JSON{PromoCodeAttrib: "C", PromoStepAttrib: nil}, "campaigns.invalidPromoStep"},
		"tags not an array":      {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: "BFCM*"}, "campaigns.invalidPromoTags"},
		"tags empty array":       {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: tags()}, "campaigns.invalidPromoTags"},
		"tags 21":                {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: many(21)}, "campaigns.invalidPromoTags"},
		"tag 65 characters":      {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: tags(strings.Repeat("x", 65))}, "campaigns.invalidPromoTags"},
		"tag empty":              {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: tags("")}, "campaigns.invalidPromoTags"},
		"tag comma":              {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: tags("a,b")}, "campaigns.invalidPromoTags"},
		"tag control":            {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: tags("a\nb")}, "campaigns.invalidPromoTags"},
		"tag * not last":         {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: tags("a*bc")}, "campaigns.invalidPromoTags"},
		"tag two *":              {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: tags("abc**")}, "campaigns.invalidPromoTags"},
		"tag * prefix of 2":      {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: tags("ab*")}, "campaigns.invalidPromoTags"},
		"tag bare *":             {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: tags("*")}, "campaigns.invalidPromoTags"},
		"tag untrimmed":          {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: tags(" BFCM*")}, "campaigns.invalidPromoTags"},
		"tag duplicate (case)":   {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: tags("BFCM-1", "bfcm-1")}, "campaigns.invalidPromoTags"},
		"tag not a string":       {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: tags(5.0)}, "campaigns.invalidPromoTags"},
		"tags null":              {JSON{PromoCodeAttrib: "C", PromoTagsAttrib: nil}, "campaigns.invalidPromoTags"},
		"step without a code":    {JSON{PromoStepAttrib: 2.0}, "campaigns.promoNeedsCode"},
		"tags without a code":    {JSON{PromoTagsAttrib: tags("BFCM*")}, "campaigns.promoNeedsCode"},
		"both without a code":    {JSON{PromoStepAttrib: 2.0, PromoTagsAttrib: tags("BFCM*")}, "campaigns.promoNeedsCode"},
	}
	for name, c := range bad {
		if got := ValidatePromo(c.a); got != c.want {
			t.Errorf("%s: %q want %q", name, got, c.want)
		}
	}
}

func TestPromoTagProblemAndParam(t *testing.T) {
	for tag, want := range map[string]string{
		"BFCM-Deal-3": "", "BFCM*": "", "abc*": "", "ab*": "prefix", "*": "prefix", "a*b": "star",
		"a,b": "comma", "a\x7fb": "control", "": "length", strings.Repeat("é", 64): "", strings.Repeat("é", 65): "length",
	} {
		if got := PromoTagProblem(tag); got != want {
			t.Errorf("PromoTagProblem(%q) = %q want %q", tag, got, want)
		}
	}
	// The preview's comma-separated parameter: pieces trimmed, empties dropped, duplicates dropped
	// (first kept), a malformed piece refuses.
	got, ok := ParsePromoTagsParam(" BFCM-Deal-3, bfcm-deal-3 ,, BFCM* ")
	if !ok || strings.Join(got, "|") != "BFCM-Deal-3|BFCM*" {
		t.Fatalf("parsed %v %v", got, ok)
	}
	if got, ok := ParsePromoTagsParam(""); !ok || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, ok)
	}
	if _, ok := ParsePromoTagsParam("ok, ab*"); ok {
		t.Fatal("a malformed tag must refuse")
	}
}

func TestPromoServedFields(t *testing.T) {
	c := Campaign{Attribs: JSON{PromoCodeAttrib: "BFCM-D3", PromoStepAttrib: 2.0, PromoTagsAttrib: []any{"a-1", 7.0, "BFCM*"}}}
	if c.PromoCode() != "BFCM-D3" || c.PromoStep() != 2 || strings.Join(c.PromoTags(), "|") != "a-1|BFCM*" {
		t.Fatalf("served %q %d %v", c.PromoCode(), c.PromoStep(), c.PromoTags())
	}
	none := Campaign{Attribs: JSON{PromoStepAttrib: "2", PromoCodeAttrib: 5.0}}
	if none.PromoCode() != "" || none.PromoStep() != 0 || none.PromoTags() == nil || len(none.PromoTags()) != 0 {
		t.Fatalf("defaults %q %d %v", none.PromoCode(), none.PromoStep(), none.PromoTags())
	}
}
