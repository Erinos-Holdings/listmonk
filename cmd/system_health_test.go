package main

// Fork (system health) -- integrations SES-HEALTH-SPEC F3, the validation half. The query-level
// all-or-nothing case lives in internal/migrations/v6_2_13_db_test.go.

import (
	"encoding/json"
	"strings"
	"testing"
)

func systemRow(mut func(m map[string]any)) json.RawMessage {
	m := map[string]any{"v": 1, "kind": "ses", "day": "2026-09-23", "status": "ok",
		"enforcement": map[string]any{"status": "ok"}, "brands": []any{}}
	if mut != nil {
		mut(m)
	}
	b, _ := json.Marshal(m)
	return b
}

func TestValidateSystemHealthRows(t *testing.T) {
	good := systemRow(nil)
	if err := validateSystemHealthRows([]json.RawMessage{good}); err != nil {
		t.Fatalf("good row refused: %v", err)
	}
	for _, s := range []string{"ok", "warn", "issues", "unknown"} {
		s := s
		if err := validateSystemHealthRows([]json.RawMessage{systemRow(func(m map[string]any) { m["status"] = s })}); err != nil {
			t.Fatalf("status %s refused: %v", s, err)
		}
	}
	for _, k := range []string{"ses", "snds", "dmarc_digest", "a-1", strings.Repeat("k", 40)} {
		k := k
		if err := validateSystemHealthRows([]json.RawMessage{systemRow(func(m map[string]any) { m["kind"] = k })}); err != nil {
			t.Fatalf("kind %q refused: %v", k, err)
		}
	}

	bad := map[string]func(m map[string]any){
		"bad status":        func(m map[string]any) { m["status"] = "red" },
		"status not string": func(m map[string]any) { m["status"] = 1 },
		"status missing":    func(m map[string]any) { delete(m, "status") },
		"non-ISO day":       func(m map[string]any) { m["day"] = "23/09/2026" },
		"impossible day":    func(m map[string]any) { m["day"] = "2026-02-30" },
		"datetime as day":   func(m map[string]any) { m["day"] = "2026-09-23T15:30:00Z" },
		"day missing":       func(m map[string]any) { delete(m, "day") },
		"kind missing":      func(m map[string]any) { delete(m, "kind") },
		"kind empty":        func(m map[string]any) { m["kind"] = "" },
		"kind upper":        func(m map[string]any) { m["kind"] = "SES" },
		"kind space":        func(m map[string]any) { m["kind"] = "s es" },
		"kind slash":        func(m map[string]any) { m["kind"] = "ses/x" },
		"kind too long":     func(m map[string]any) { m["kind"] = strings.Repeat("k", 41) },
		"kind not string":   func(m map[string]any) { m["kind"] = 7 },
		"v 2":               func(m map[string]any) { m["v"] = 2 },
		"v missing":         func(m map[string]any) { delete(m, "v") },
		"v string":          func(m map[string]any) { m["v"] = "1" },
	}
	for name, mut := range bad {
		// A bad row anywhere refuses the whole batch.
		if err := validateSystemHealthRows([]json.RawMessage{good, systemRow(mut)}); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}

	if err := validateSystemHealthRows(nil); err == nil {
		t.Fatal("empty batch accepted")
	}
	if err := validateSystemHealthRows([]json.RawMessage{json.RawMessage(`[1]`)}); err == nil {
		t.Fatal("non-object row accepted")
	}
	if err := validateSystemHealthRows([]json.RawMessage{good, good}); err == nil {
		t.Fatal("duplicate (kind, day) accepted")
	}
	other := systemRow(func(m map[string]any) { m["day"] = "2026-09-22" })
	snds := systemRow(func(m map[string]any) { m["kind"] = "snds" })
	if err := validateSystemHealthRows([]json.RawMessage{good, other, snds}); err != nil {
		t.Fatalf("two days of one kind + another kind refused: %v", err)
	}
	many := make([]json.RawMessage, brandHealthMaxRows+1)
	for i := range many {
		many[i] = good
	}
	if err := validateSystemHealthRows(many); err == nil {
		t.Fatal("oversized batch accepted")
	}
}

func TestSystemHealthKindParam(t *testing.T) {
	for _, k := range []string{"ses", "x_1-y"} {
		if !reSystemHealthKind.MatchString(k) {
			t.Fatalf("kind %q refused", k)
		}
	}
	for _, k := range []string{"", "SES", "../x", "a b", strings.Repeat("k", 41)} {
		if reSystemHealthKind.MatchString(k) {
			t.Fatalf("kind %q accepted", k)
		}
	}
}
