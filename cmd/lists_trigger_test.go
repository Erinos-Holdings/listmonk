package main

// Fork (integrations LIFECYCLE-SERIES-SPEC I12/I13) -- the pure half of the join-trigger shape
// check, no database: validateJoinTrigger accepts {kind:"post_purchase", days} with an integer
// 1-365 and refuses every other key and value exactly as it does for no_purchase, and the ONE
// step-kind helper names both step kinds and nothing else. The DB-backed cases (stored form, the
// 400 leaves the list unchanged) are in lists_trigger_db_test.go.

import (
	"strings"
	"testing"
)

func TestValidateJoinTriggerStepKinds(t *testing.T) {
	for _, kind := range []string{"no_purchase", "post_purchase"} {
		for in, want := range map[string]string{
			`{"kind":"` + kind + `","days":1}`:      `{"kind":"` + kind + `","days":1}`,
			`{"days":30,"kind":"` + kind + `"}`:     `{"kind":"` + kind + `","days":30}`,
			`{"kind":"` + kind + `","days":365}`:    `{"kind":"` + kind + `","days":365}`,
			`{ "days" : 14 , "kind":"` + kind + `"}`: `{"kind":"` + kind + `","days":14}`,
		} {
			got, err := validateJoinTrigger([]byte(in))
			if err != nil || string(got) != want {
				t.Fatalf("validateJoinTrigger(%s) = %s, %v; want %s", in, got, err, want)
			}
		}
		for name, in := range map[string]string{
			"days missing":  `{"kind":"` + kind + `"}`,
			"days null":     `{"kind":"` + kind + `","days":null}`,
			"days float":    `{"kind":"` + kind + `","days":5.5}`,
			"days 5.0":      `{"kind":"` + kind + `","days":5.0}`,
			"days exponent": `{"kind":"` + kind + `","days":5e0}`,
			"days string":   `{"kind":"` + kind + `","days":"5"}`,
			"days 0":        `{"kind":"` + kind + `","days":0}`,
			"days negative": `{"kind":"` + kind + `","days":-5}`,
			"days 366":      `{"kind":"` + kind + `","days":366}`,
			"extra key":     `{"kind":"` + kind + `","days":5,"skus":["a"]}`,
			"brand key":     `{"kind":"` + kind + `","days":5,"brand":"acme"}`,
		} {
			if got, err := validateJoinTrigger([]byte(in)); err == nil {
				t.Fatalf("%s %s: accepted as %s", kind, name, got)
			}
		}
	}
	if _, err := validateJoinTrigger([]byte(`{"kind":"post-purchase","days":5}`)); err == nil {
		t.Fatal("the registry spelling post-purchase is not a list trigger kind")
	}
}

// integrations SKU-JOURNEYS-SPEC I8 -- the two SKU journey kinds: skus normalized exactly as the
// skus kind, days a JSON integer 1-365, every other key refused.
func TestValidateJoinTriggerSkuJourneyKinds(t *testing.T) {
	for _, kind := range []string{"replenish", "first_sku"} {
		for in, want := range map[string]string{
			`{"kind":"` + kind + `","skus":[" JoyBelle-BrushOnGlue ","joybelle-brushonglue","B"],"days":90}`: `{"kind":"` + kind + `","skus":["joybelle-brushonglue","b"],"days":90}`,
			`{"days":1,"skus":["a"],"kind":"` + kind + `"}`:                                                  `{"kind":"` + kind + `","skus":["a"],"days":1}`,
			`{"kind":"` + kind + `","skus":["a"],"days":365}`:                                                `{"kind":"` + kind + `","skus":["a"],"days":365}`,
		} {
			got, err := validateJoinTrigger([]byte(in))
			if err != nil || string(got) != want {
				t.Fatalf("validateJoinTrigger(%s) = %s, %v; want %s", in, got, err, want)
			}
		}
		for name, in := range map[string]string{
			"days missing": `{"kind":"` + kind + `","skus":["a"]}`,
			"days 0":       `{"kind":"` + kind + `","skus":["a"],"days":0}`,
			"days 366":     `{"kind":"` + kind + `","skus":["a"],"days":366}`,
			"days float":   `{"kind":"` + kind + `","skus":["a"],"days":9.5}`,
			"days string":  `{"kind":"` + kind + `","skus":["a"],"days":"9"}`,
			"skus missing": `{"kind":"` + kind + `","days":9}`,
			"skus empty":   `{"kind":"` + kind + `","skus":[],"days":9}`,
			"skus blank":   `{"kind":"` + kind + `","skus":[" "],"days":9}`,
			"skus number":  `{"kind":"` + kind + `","skus":[1],"days":9}`,
			"sku too long": `{"kind":"` + kind + `","skus":["` + strings.Repeat("x", 101) + `"],"days":9}`,
			"extra key":    `{"kind":"` + kind + `","skus":["a"],"days":9,"mode":"replenish"}`,
		} {
			if got, err := validateJoinTrigger([]byte(in)); err == nil {
				t.Fatalf("%s %s: accepted as %s", kind, name, got)
			}
		}
	}
}

// integrations SKU-JOURNEYS-SPEC I8 (S3) -- the guard set is the four sweep-filled kinds; the lock
// set (isStandardTriggerKind, read by the lock and the hide rule) keeps first_order and the two series steps.
func TestIsSweepFilledKind(t *testing.T) {
	for kind, want := range map[string]bool{
		"no_purchase": true, "post_purchase": true, "replenish": true, "first_sku": true,
		"first_order": false, "skus": false, "": false,
	} {
		if got := isSweepFilledKind(kind); got != want {
			t.Fatalf("isSweepFilledKind(%q) = %v, want %v", kind, got, want)
		}
	}
	for kind, want := range map[string]bool{"first_order": true, "no_purchase": true, "post_purchase": true, "replenish": false, "first_sku": false, "skus": false} {
		if got := isStandardTriggerKind(kind); got != want {
			t.Fatalf("isStandardTriggerKind(%q) = %v, want %v", kind, got, want)
		}
	}
}

// skuPreviewDoc is a well-formed SKU-step activation preview (integrations SKU-JOURNEYS-SPEC D8).
const skuPreviewDoc = `{"as_of":"2026-11-24T15:05:00Z","next_run_at":"2026-11-25T15:10:00Z","joins":12,` +
	`"by_age":[{"from_days":90,"to_days":120,"joins":7},{"from_days":120,"to_days":150,"joins":5}],` +
	`"by_lang":{"en":11,"fr":1},"skips":{"outside-window":0,"not-due":40,"already-sent":0,"no-automation-in-language":2,"consent":9}}`

// integrations SKU-JOURNEYS-SPEC I17 -- the preview shape check refuses unknown keys and types.
func TestValidateTriggerSyncPreview(t *testing.T) {
	live := func(p string) string {
		return `{"synced_at":"2026-11-24T15:05:00Z","state":"live","slug":"acme-sku-step-9","preview":` + p + `}`
	}
	if err := validateTriggerSync([]byte(live(skuPreviewDoc))); err != nil {
		t.Fatalf("a well-formed preview: %v", err)
	}
	if err := validateTriggerSync([]byte(live(`{"as_of":"2026-11-24T15:05:00Z","next_run_at":"2026-11-25T15:10:00Z","joins":0,"by_age":[],"by_lang":{},"skips":{}}`))); err != nil {
		t.Fatalf("an empty preview: %v", err)
	}
	for name, p := range map[string]string{
		"not an object":     `[]`,
		"unknown key":       strings.Replace(skuPreviewDoc, `"joins":12,`, `"joins":12,"emails":[],`, 1),
		"missing as_of":     strings.Replace(skuPreviewDoc, `"as_of":"2026-11-24T15:05:00Z",`, ``, 1),
		"bad next_run_at":   strings.Replace(skuPreviewDoc, `"2026-11-25T15:10:00Z"`, `"tomorrow"`, 1),
		"joins string":      strings.Replace(skuPreviewDoc, `"joins":12,`, `"joins":"12",`, 1),
		"joins negative":    strings.Replace(skuPreviewDoc, `"joins":12,`, `"joins":-1,`, 1),
		"joins fraction":    strings.Replace(skuPreviewDoc, `"joins":12,`, `"joins":1.5,`, 1),
		"by_age object":     strings.Replace(skuPreviewDoc, `"by_age":[`, `"by_age":{"x":[`, 1),
		"bucket extra key":  strings.Replace(skuPreviewDoc, `{"from_days":90,`, `{"email":"a@example.test","from_days":90,`, 1),
		"bucket missing":    strings.Replace(skuPreviewDoc, `,"to_days":120`, ``, 1),
		"by_lang value str": strings.Replace(skuPreviewDoc, `"fr":1`, `"fr":"a@example.test"`, 1),
		"skips not object":  strings.Replace(skuPreviewDoc, `"skips":{`, `"skips":[{`, 1),
	} {
		if err := validateTriggerSync([]byte(live(p))); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
	if err := validateTriggerSync([]byte(`{"synced_at":"2026-11-24T15:05:00Z","state":"invalid","error":"e","preview":` + skuPreviewDoc + `}`)); err == nil {
		t.Fatal("a preview on an invalid write-back was accepted")
	}
}

func TestIsStepTriggerKind(t *testing.T) {
	for kind, want := range map[string]bool{
		"no_purchase": true, "post_purchase": true,
		"first_order": false, "skus": false, "": false, "post-purchase": false,
	} {
		if got := isStepTriggerKind(kind); got != want {
			t.Fatalf("isStepTriggerKind(%q) = %v, want %v", kind, got, want)
		}
	}
}
