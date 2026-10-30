package main

// Fork (integrations LIFECYCLE-SERIES-SPEC I12/I13) -- the pure half of the join-trigger shape
// check, no database: validateJoinTrigger accepts {kind:"post_purchase", days} with an integer
// 1-365 and refuses every other key and value exactly as it does for no_purchase, and the ONE
// step-kind helper names both step kinds and nothing else. The DB-backed cases (stored form, the
// 400 leaves the list unchanged) are in lists_trigger_db_test.go.

import "testing"

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
