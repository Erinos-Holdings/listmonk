package main

// Fork (smart sending look-ahead) -- integrations listmonk/SMART-SENDING-LOOKAHEAD-SPEC L9 (the
// handler-permission and body-shape unit) and D7 (the switch is not a settings-struct field). No
// database; the stored behaviour is journeys_due_db_test.go.

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/models"
)

func TestJourneyDueRoutes(t *testing.T) {
	b, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, re := range []string{
		`g\.PUT\("/api/journeys/due", pm\(a\.PutJourneyDue, "subscribers:manage"\)\)`,
		`g\.GET\("/api/journeys/due", pm\(a\.GetJourneyDue, "subscribers:get_all"\)\)`,
	} {
		if !regexp.MustCompile(re).Match(b) {
			t.Fatalf("handlers.go does not register %s", re)
		}
	}
}

func TestValidateJourneyDue(t *testing.T) {
	now := time.Date(2026, 11, 9, 15, 0, 0, 0, time.UTC)
	at := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	rows := func(r ...[]int64) *[][]int64 { return &r }

	// Valid: both sources, an empty list (removes the feed), the 48-hour edge.
	for name, req := range map[string]journeyDueReq{
		"series":     {Source: "series", ExpectedAt: at(24 * time.Hour), Rows: rows([]int64{3, 7}, []int64{3, 8})},
		"sunset":     {Source: "sunset", ExpectedAt: at(24 * time.Hour), Rows: rows()},
		"edge 48 h":  {Source: "series", ExpectedAt: at(48 * time.Hour), Rows: rows()},
		"1 s ahead":  {Source: "series", ExpectedAt: at(time.Second), Rows: rows()},
		"int32 edge": {Source: "series", ExpectedAt: at(time.Hour), Rows: rows([]int64{2147483647, 2147483647})},
	} {
		if _, _, _, err := validateJourneyDue(req, now); err != nil {
			t.Errorf("%s refused: %v", name, err)
		}
	}
	got, lists, subs, err := validateJourneyDue(journeyDueReq{Source: "series", ExpectedAt: "2026-11-10T08:00:00-07:00", Rows: rows([]int64{3, 7}, []int64{4, 8})}, now)
	if err != nil || !got.Equal(now.Add(24*time.Hour)) || got.Location() != time.UTC || fmt.Sprint(lists) != "[3 4]" || fmt.Sprint(subs) != "[7 8]" {
		t.Fatalf("parsed %v %v %v %v", got, lists, subs, err)
	}

	big := make([][]int64, journeyDueMaxRows+1)
	for i := range big {
		big[i] = []int64{1, int64(i + 1)}
	}
	for name, req := range map[string]journeyDueReq{
		"unknown source":  {Source: "welcome", ExpectedAt: at(time.Hour), Rows: rows()},
		"empty source":    {ExpectedAt: at(time.Hour), Rows: rows()},
		"bad timestamp":   {Source: "series", ExpectedAt: "tomorrow", Rows: rows()},
		"now":             {Source: "series", ExpectedAt: at(0), Rows: rows()},
		"past":            {Source: "series", ExpectedAt: at(-time.Hour), Rows: rows()},
		"over 48 h":       {Source: "series", ExpectedAt: at(48*time.Hour + time.Second), Rows: rows()},
		"no rows key":     {Source: "series", ExpectedAt: at(time.Hour)},
		"zero list id":    {Source: "series", ExpectedAt: at(time.Hour), Rows: rows([]int64{0, 7})},
		"negative sub id": {Source: "series", ExpectedAt: at(time.Hour), Rows: rows([]int64{3, -7})},
		"over INT":        {Source: "series", ExpectedAt: at(time.Hour), Rows: rows([]int64{2147483648, 7})},
		"one-element row": {Source: "series", ExpectedAt: at(time.Hour), Rows: rows([]int64{3})},
		"three-id row":    {Source: "series", ExpectedAt: at(time.Hour), Rows: rows([]int64{3, 7, 9})},
		"over 500,000":    {Source: "series", ExpectedAt: at(time.Hour), Rows: &big},
	} {
		if _, _, _, err := validateJourneyDue(req, now); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// D7 -- the switch key is deliberately NOT a field of the Go settings struct, so a Settings-page
// save (update-settings writes the struct's keys) can never overwrite it.
func TestLookaheadSwitchNotInSettingsStruct(t *testing.T) {
	ty := reflect.TypeOf(models.Settings{})
	for i := 0; i < ty.NumField(); i++ {
		if tag := strings.Split(ty.Field(i).Tag.Get("json"), ",")[0]; tag == "app.smart_sending_lookahead" {
			t.Fatalf("models.Settings.%s carries the look-ahead switch", ty.Field(i).Name)
		}
	}
}
