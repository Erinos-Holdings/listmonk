package main

// Fork (brand health) -- integrations BRAND-HEALTH-SPEC F4, the validation half. The
// all-or-nothing query-level case lives in internal/migrations/v6_2_12_db_test.go.

import (
	"encoding/json"
	"strings"
	"testing"
)

func healthRow(mut func(m map[string]any)) json.RawMessage {
	m := map[string]any{"v": 1, "brand": "shala", "domain": "shala.example", "day": "2026-09-23",
		"status": "warn", "default": false, "inputs": map[string]any{"verdict": map[string]any{"status": "warn"}}}
	if mut != nil {
		mut(m)
	}
	b, _ := json.Marshal(m)
	return b
}

func TestValidateBrandHealthRows(t *testing.T) {
	good := healthRow(nil)
	if err := validateBrandHealthRows([]json.RawMessage{good}); err != nil {
		t.Fatalf("good row refused: %v", err)
	}
	for _, s := range []string{"ok", "warn", "issues", "unknown"} {
		s := s
		if err := validateBrandHealthRows([]json.RawMessage{healthRow(func(m map[string]any) { m["status"] = s })}); err != nil {
			t.Fatalf("status %s refused: %v", s, err)
		}
	}

	bad := map[string]func(m map[string]any){
		"bad status":        func(m map[string]any) { m["status"] = "red" },
		"status not string": func(m map[string]any) { m["status"] = 1 },
		"non-ISO day":       func(m map[string]any) { m["day"] = "23/09/2026" },
		"impossible day":    func(m map[string]any) { m["day"] = "2026-02-30" },
		"datetime as day":   func(m map[string]any) { m["day"] = "2026-09-23T15:30:00Z" },
		"missing brand":     func(m map[string]any) { delete(m, "brand") },
		"empty brand":       func(m map[string]any) { m["brand"] = "" },
		"untrimmed brand":   func(m map[string]any) { m["brand"] = " shala" },
		"missing domain":    func(m map[string]any) { delete(m, "domain") },
		"v 2":               func(m map[string]any) { m["v"] = 2 },
		"v missing":         func(m map[string]any) { delete(m, "v") },
		"v string":          func(m map[string]any) { m["v"] = "1" },
		"default missing":   func(m map[string]any) { delete(m, "default") },
		"default not bool":  func(m map[string]any) { m["default"] = "true" },
		"default null":      func(m map[string]any) { m["default"] = nil },
		"brand too long":    func(m map[string]any) { m["brand"] = strings.Repeat("x", 201) },
	}
	for name, mut := range bad {
		// A bad row anywhere refuses the whole batch.
		if err := validateBrandHealthRows([]json.RawMessage{good, healthRow(mut)}); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}

	if err := validateBrandHealthRows(nil); err == nil {
		t.Fatal("empty batch accepted")
	}
	if err := validateBrandHealthRows([]json.RawMessage{json.RawMessage(`[1]`)}); err == nil {
		t.Fatal("non-object row accepted")
	}
	if err := validateBrandHealthRows([]json.RawMessage{good, good}); err == nil {
		t.Fatal("duplicate (brand, day) accepted")
	}
	other := healthRow(func(m map[string]any) { m["day"] = "2026-09-22" })
	if err := validateBrandHealthRows([]json.RawMessage{good, other}); err != nil {
		t.Fatalf("two days of one brand refused: %v", err)
	}
}

func TestParseBrandHealthDays(t *testing.T) {
	cases := map[string]int{"": 30, "1": 1, "90": 90, "400": 400, "5000": 400}
	for in, want := range cases {
		got, err := parseBrandHealthDays(in)
		if err != nil || got != want {
			t.Fatalf("days %q = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"0", "-3", "x", "1.5"} {
		if _, err := parseBrandHealthDays(in); err == nil {
			t.Fatalf("days %q accepted", in)
		}
	}
}
