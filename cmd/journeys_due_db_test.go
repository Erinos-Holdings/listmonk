package main

// Fork (smart sending look-ahead) -- integrations listmonk/SMART-SENDING-LOOKAHEAD-SPEC L9/L10
// against a real database, through the handlers and the same a.auth.Perm wrapper handlers.go
// registers (route registration is journeys_due_test.go). Shares newLinkHarness (LISTMONK_TEST_PG
// opt-in; plain go test skips).
//   L9   PUT /api/journeys/due replaces exactly (source, expected_at), leaves the other source and the
//        other times, deletes rows older than 7 days, is idempotent, and refuses each bad body with
//        nothing written; GET reports the switch and per-feed counts and no ids; the permissions.
//   L10  the switch row survives a Settings save (core.UpdateSettings) and get-settings still
//        unmarshals.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/auth"
)

type jdRow struct {
	Source string    `db:"source"`
	List   int       `db:"list_id"`
	Sub    int       `db:"subscriber_id"`
	At     time.Time `db:"expected_at"`
}

func jdRows(t *testing.T, h *linkHarness) string {
	t.Helper()
	var rows []jdRow
	if err := h.db.Select(&rows, `SELECT source, list_id, subscriber_id, expected_at FROM journey_due ORDER BY source, expected_at, list_id, subscriber_id`); err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s@%s:%d/%d", r.Source, r.At.UTC().Format("01-02T15"), r.List, r.Sub))
	}
	return strings.Join(out, " ")
}

func jdBody(source string, at time.Time, rows ...[2]int) string {
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		parts = append(parts, fmt.Sprintf("[%d,%d]", r[0], r[1]))
	}
	return fmt.Sprintf(`{"source":%q,"expected_at":%q,"rows":[%s]}`, source, at.Format(time.RFC3339), strings.Join(parts, ","))
}

func TestJourneyDueEndpoint(t *testing.T) {
	h := newLinkHarness(t)
	h.app.auth = &auth.Auth{}
	writer := perms("listmonk_sync", auth.PermSubscribersManage)
	reader := perms("listmonk_agent", auth.PermSubscribersGetAll)
	put := func(u auth.User, body string) (int, string) {
		return convCall(t, u, http.MethodPut, "/api/journeys/due", body, h.app.auth.Perm(h.app.PutJourneyDue, "subscribers:manage"))
	}
	get := func(u auth.User) (int, string) {
		return convCall(t, u, http.MethodGet, "/api/journeys/due", "", h.app.auth.Perm(h.app.GetJourneyDue, "subscribers:get_all"))
	}

	// Hour-truncated times keep the row strings readable; all inside (now, now + 48 h].
	base := time.Now().UTC().Truncate(time.Hour)
	t1, t2 := base.Add(24*time.Hour), base.Add(26*time.Hour)

	// A row older than 7 days (written by hand -- the API refuses a past expected_at).
	h.db.MustExec(`INSERT INTO journey_due (source, list_id, subscriber_id, expected_at) VALUES ('sunset', 9, 9, NOW() - INTERVAL '8 days')`)
	// A row 6 days old: inside the retention, kept.
	h.db.MustExec(`INSERT INTO journey_due (source, list_id, subscriber_id, expected_at) VALUES ('sunset', 8, 8, $1)`, base.Add(-6*24*time.Hour))
	old6 := fmt.Sprintf("sunset@%s:8/8", base.Add(-6*24*time.Hour).Format("01-02T15"))

	if code, body := put(writer, jdBody("series", t1, [2]int{3, 7}, [2]int{3, 8}, [2]int{3, 8})); code != http.StatusOK || !strings.Contains(body, `"rows":2`) {
		t.Fatalf("series t1: %d %s", code, body)
	}
	if code, body := put(writer, jdBody("series", t2, [2]int{4, 1})); code != http.StatusOK {
		t.Fatalf("series t2: %d %s", code, body)
	}
	if code, body := put(writer, jdBody("sunset", t1, [2]int{5, 7})); code != http.StatusOK {
		t.Fatalf("sunset t1: %d %s", code, body)
	}
	f := func(s string, at time.Time, l, sub int) string {
		return fmt.Sprintf("%s@%s:%d/%d", s, at.Format("01-02T15"), l, sub)
	}
	want := strings.Join([]string{f("series", t1, 3, 7), f("series", t1, 3, 8), f("series", t2, 4, 1), old6, f("sunset", t1, 5, 7)}, " ")
	if got := jdRows(t, h); got != want {
		t.Fatalf("rows\n got %s\nwant %s", got, want)
	}

	// Replacing (series, t1) touches nothing else; a retry converges.
	for i := 0; i < 2; i++ {
		if code, body := put(writer, jdBody("series", t1, [2]int{6, 2})); code != http.StatusOK {
			t.Fatalf("replace run %d: %d %s", i+1, code, body)
		}
	}
	want = strings.Join([]string{f("series", t1, 6, 2), f("series", t2, 4, 1), old6, f("sunset", t1, 5, 7)}, " ")
	if got := jdRows(t, h); got != want {
		t.Fatalf("after the replace\n got %s\nwant %s", got, want)
	}

	// Every refusal is a 400 with nothing written.
	for name, body := range map[string]string{
		"unknown source": jdBody("welcome", t1, [2]int{1, 1}),
		"past":           jdBody("series", base.Add(-time.Hour), [2]int{1, 1}),
		"over 48 h":      jdBody("series", time.Now().Add(49*time.Hour), [2]int{1, 1}),
		"bad timestamp":  `{"source":"series","expected_at":"tomorrow","rows":[]}`,
		"no rows":        fmt.Sprintf(`{"source":"series","expected_at":%q}`, t1.Format(time.RFC3339)),
		"zero id":        jdBody("series", t1, [2]int{0, 1}),
		"negative id":    jdBody("series", t1, [2]int{1, -1}),
		"short row":      fmt.Sprintf(`{"source":"series","expected_at":%q,"rows":[[1]]}`, t1.Format(time.RFC3339)),
		"string id":      fmt.Sprintf(`{"source":"series","expected_at":%q,"rows":[["1",2]]}`, t1.Format(time.RFC3339)),
		"not JSON":       `rows`,
	} {
		if code, body := put(writer, body); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s, want 400", name, code, body)
		}
		if got := jdRows(t, h); got != want {
			t.Fatalf("%s wrote\n got %s\nwant %s", name, got, want)
		}
	}
	// Over 500,000 rows.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`{"source":"series","expected_at":%q,"rows":[`, t1.Format(time.RFC3339)))
	for i := 0; i <= journeyDueMaxRows; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("[1,%d]", i+1))
	}
	sb.WriteString("]}")
	if code, _ := put(writer, sb.String()); code != http.StatusBadRequest {
		t.Fatalf("500,001 rows: %d, want 400", code)
	}

	// An empty PUT removes the feed.
	if code, body := put(writer, jdBody("series", t2)); code != http.StatusOK || !strings.Contains(body, `"rows":0`) {
		t.Fatalf("empty: %d %s", code, body)
	}
	want = strings.Join([]string{f("series", t1, 6, 2), old6, f("sunset", t1, 5, 7)}, " ")
	if got := jdRows(t, h); got != want {
		t.Fatalf("after the empty PUT\n got %s\nwant %s", got, want)
	}

	// The GET: the switch and the per-feed counts, no subscriber ids.
	readFeeds := func() (bool, []map[string]any, string) {
		code, body := get(reader)
		if code != http.StatusOK {
			t.Fatalf("GET: %d %s", code, body)
		}
		var resp struct {
			Data struct {
				Lookahead bool             `json:"lookahead"`
				Feeds     []map[string]any `json:"feeds"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("GET body %s: %v", body, err)
		}
		return resp.Data.Lookahead, resp.Data.Feeds, body
	}
	on, feeds, body := readFeeds()
	if on || len(feeds) != 3 {
		t.Fatalf("GET lookahead %v feeds %v", on, feeds)
	}
	for _, fd := range feeds {
		for k := range fd {
			if k != "source" && k != "expected_at" && k != "rows" && k != "created_at" {
				t.Fatalf("a feed carries %q: %s", k, body)
			}
		}
	}
	if strings.Contains(body, "subscriber") || strings.Contains(body, "list_id") {
		t.Fatalf("the GET names ids: %s", body)
	}
	h.db.MustExec(`UPDATE settings SET value = 'true' WHERE key = 'app.smart_sending_lookahead'`)
	if on, _, _ := readFeeds(); !on {
		t.Fatal("GET lookahead false with the switch true")
	}
	h.db.MustExec(`UPDATE settings SET value = '"true"' WHERE key = 'app.smart_sending_lookahead'`)
	if on, _, _ := readFeeds(); on {
		t.Fatal(`GET lookahead true with the switch the string "true" (the function reads it as dark)`)
	}
	h.db.MustExec(`DELETE FROM settings WHERE key = 'app.smart_sending_lookahead'`)
	if on, _, _ := readFeeds(); on {
		t.Fatal("GET lookahead true with the switch row absent")
	}

	// Permissions through the wrapper: the PUT needs subscribers:manage, the GET subscribers:get_all.
	if code, _ := put(reader, jdBody("series", t1)); code != http.StatusForbidden {
		t.Fatalf("PUT as a reader: %d, want 403", code)
	}
	if code, _ := get(perms("analytics", auth.PermCampaignsGetAnalytics)); code != http.StatusForbidden {
		t.Fatalf("GET without subscribers:get_all: %d, want 403", code)
	}
}

func TestLookaheadSwitchSurvivesSettingsSave(t *testing.T) {
	h := newLinkHarness(t)
	h.db.MustExec(`UPDATE settings SET value = 'true' WHERE key = 'app.smart_sending_lookahead'`)

	s, err := h.app.core.GetSettings()
	if err != nil {
		t.Fatalf("get-settings with the switch row: %v", err)
	}
	if err := h.app.core.UpdateSettings(s); err != nil {
		t.Fatalf("settings save: %v", err)
	}
	var v string
	h.db.Get(&v, `SELECT value::text FROM settings WHERE key = 'app.smart_sending_lookahead'`)
	if v != "true" {
		t.Fatalf("a Settings save changed the switch to %s", v)
	}
	if _, err := h.app.core.GetSettings(); err != nil {
		t.Fatalf("get-settings after the save: %v", err)
	}
}
