package main

import (
	"testing"
	"time"
)

// Fork -- a scheduled send_at is stored on the minute regardless of the seconds the writer
// supplied; an instant already on the minute is unchanged, and the zone is preserved.
func TestTruncateSendAt(t *testing.T) {
	mt, _ := time.LoadLocation("America/Denver")
	cases := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{"seconds dropped", time.Date(2026, 9, 23, 15, 0, 37, 0, time.UTC), time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)},
		{"sub-seconds dropped", time.Date(2026, 9, 23, 15, 0, 0, 999_000_000, time.UTC), time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)},
		{"already on the minute", time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC), time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)},
		{"59s never rounds up", time.Date(2026, 9, 23, 15, 0, 59, 0, time.UTC), time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)},
		{"zone preserved", time.Date(2026, 12, 16, 9, 11, 42, 0, mt), time.Date(2026, 12, 16, 9, 11, 0, 0, mt)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncateSendAt(tc.in); !got.Equal(tc.want) || got.Location().String() != tc.want.Location().String() {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
