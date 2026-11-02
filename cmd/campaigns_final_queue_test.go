package main

import "testing"

// Fork (final-queue guard) -- integrations FINAL-QUEUE-GUARD-SPEC K1: isFinalQueueTag matches
// exactly reengage:<digits>:3 and reengage:<digits>:4.
func TestIsFinalQueueTag(t *testing.T) {
	for tag, want := range map[string]bool{
		"reengage:16:3":  true,
		"reengage:16:4":  true,
		"reengage:1:3":   true,
		"reengage:16:1":  false,
		"reengage:16:2":  false,
		"reengage:16:34": false,
		"reengage:16:5":  false,
		"reengage::3":    false,
		"reengage:x:3":   false,
		"reengage:16:3 ": false,
		"xreengage:16:3": false,
		"repermission:3": false,
		"brand:x":        false,
		"":               false,
	} {
		if got := isFinalQueueTag(tag); got != want {
			t.Errorf("isFinalQueueTag(%q) = %v, want %v", tag, got, want)
		}
	}
}

// D2 -- the list-update predicate: only a typed reengage: tag the stored list lacks is returned.
func TestNewReengageTag(t *testing.T) {
	stored := []string{"reengage:23:4", "x"}
	for _, c := range []struct {
		typed []string
		want  string
	}{
		{nil, ""},
		{[]string{"x", "y"}, ""},
		{[]string{"reengage:23:4"}, ""},
		{[]string{" reengage:23:4 "}, ""},
		{[]string{"reengage:23:4", "reengage:23:1"}, "reengage:23:1"},
		{[]string{" reengage:5:3"}, "reengage:5:3"},
	} {
		if got := newReengageTag(c.typed, stored); got != c.want {
			t.Errorf("newReengageTag(%q) = %q, want %q", c.typed, got, c.want)
		}
	}
}
