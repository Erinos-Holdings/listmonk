package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// Fork (TEST-SEND-WARNINGS-SPEC I1): the addresses a test send skipped -- each once, in the
// order typed, blanks ignored, a mixed-case stored email counting as sent.
func TestSkippedTestAddresses(t *testing.T) {
	subs := func(emails ...string) models.Subscribers {
		out := models.Subscribers{}
		for i, e := range emails {
			s := models.Subscriber{}
			s.ID = i + 1
			s.Email = e
			out = append(out, s)
		}
		return out
	}

	cases := []struct {
		name      string
		requested []string
		sent      models.Subscribers
		want      string
	}{
		{"none skipped", []string{"a@x", "b@x"}, subs("a@x", "b@x"), ""},
		{"all skipped", []string{"a@x", "b@x"}, subs(), "a@x,b@x"},
		{"partial", []string{"a@x", "b@x", "c@x"}, subs("b@x"), "a@x,c@x"},
		{"duplicates once", []string{"a@x", "b@x", "a@x"}, subs("b@x"), "a@x"},
		{"request order kept", []string{"z@x", "m@x", "a@x"}, subs(), "z@x,m@x,a@x"},
		{"blanks ignored", []string{"", "a@x", ""}, subs(), "a@x"},
		{"mixed-case stored email counts as sent", []string{"megan@x.com"}, subs("Megan@X.com"), ""},
		{"nil sent", []string{"a@x"}, nil, "a@x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.Join(skippedTestAddresses(c.requested, c.sent), ","); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

// testSendApp loads the shipped en.json so the assertions pin the real wording.
func testSendApp(t *testing.T) *App {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "i18n", "en.json"))
	if err != nil {
		t.Fatal(err)
	}
	i, err := i18n.New(b)
	if err != nil {
		t.Fatal(err)
	}
	return &App{i18n: i}
}

// Fork (TEST-SEND-WARNINGS-SPEC I2, I6): the warning sentence -- empty for nothing skipped, the
// partial and nothing-sent wordings, at most 10 names, plain text, braces never translated, and
// one reason regardless of why an address was skipped.
func TestTestSkipMessage(t *testing.T) {
	a := testSendApp(t)

	if got := a.testSkipMessage(nil, false); got != "" {
		t.Fatalf("nil: got %q", got)
	}
	if got := a.testSkipMessage([]string{}, true); got != "" {
		t.Fatalf("empty all-skipped: got %q", got)
	}

	const reason = "not a subscriber on a list you can access"

	partial := a.testSkipMessage([]string{"nobody@example.invalid", "b@x"}, false)
	if want := "Not sent to nobody@example.invalid, b@x — " + reason + "."; partial != want {
		t.Fatalf("partial:\n got %q\nwant %q", partial, want)
	}

	none := a.testSkipMessage([]string{"nobody@example.invalid"}, true)
	if want := "Nothing sent. nobody@example.invalid: " + reason + "."; none != want {
		t.Fatalf("all skipped:\n got %q\nwant %q", none, want)
	}

	var eleven []string
	for n := 1; n <= 11; n++ {
		eleven = append(eleven, fmt.Sprintf("u%d@x", n))
	}
	got := a.testSkipMessage(eleven, false)
	if !strings.Contains(got, "u10@x (+1 more)") || strings.Contains(got, "u11@x") {
		t.Fatalf("eleven: got %q", got)
	}

	// D4: the server never escapes; both toast paths escape in the browser.
	if got := a.testSkipMessage([]string{"<b>&amp;@x"}, false); !strings.Contains(got, "<b>&amp;@x") {
		t.Fatalf("html passthrough: got %q", got)
	}

	// §3.2: T + ReplaceAll, so a {key} inside an address is not translated.
	if got := a.testSkipMessage([]string{"{campaigns.testSent}@x"}, false); !strings.Contains(got, "{campaigns.testSent}@x") ||
		strings.Contains(got, "Test message sent") {
		t.Fatalf("brace passthrough: got %q", got)
	}

	// D2: no reason-specific wording -- the only reason is the shared one, in both wordings.
	for _, msg := range []string{partial, none} {
		if strings.Count(msg, reason) != 1 {
			t.Fatalf("reason not stated exactly once: %q", msg)
		}
		for _, leak := range []string{"permission", "does not exist", "unknown", "not found", "denied"} {
			if strings.Contains(strings.ToLower(msg), leak) {
				t.Fatalf("reason-specific wording %q in %q", leak, msg)
			}
		}
	}
}

// Fork (TEST-SEND-WARNINGS-SPEC I5): only a 403 drops a subscriber; any other hasSubPerm error
// fails the test send (D7).
func TestIsPermDenied(t *testing.T) {
	var typedNil *echo.HTTPError
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"403", echo.NewHTTPError(http.StatusForbidden, "denied"), true},
		{"wrapped 403", fmt.Errorf("check: %w", echo.NewHTTPError(http.StatusForbidden, "denied")), true},
		{"500", echo.NewHTTPError(http.StatusInternalServerError, "db"), false},
		{"plain error", errors.New("connection reset"), false},
		{"nil", nil, false},
		{"wrapped nil", fmt.Errorf("check: %w", nil), false},
		{"typed nil HTTPError", typedNil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isPermDenied(c.err); got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}
