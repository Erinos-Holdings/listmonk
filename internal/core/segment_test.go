package core

import (
	"os"
	"strings"
	"testing"

	"github.com/knadh/listmonk/models"
)

// Fork (integrations SUBSCRIBERS-FILTERS-2-SPEC I7) -- the no-language predicate has ONE
// definition: lang=none in subscriberFilterExp and the lang_none_total count
// (CountSubscribersLangNone) both read SubscriberLangNonePred, and no second literal of it is
// written in this package's source.
func TestSubscriberLangNonePredIsShared(t *testing.T) {
	c := &Core{}
	got, err := c.subscriberFilterExp("", nil, models.SubscriberFilter{Lang: models.SubscriberLangNone})
	if err != nil {
		t.Fatal(err)
	}
	if want := "(" + SubscriberLangNonePred + ")"; got != want {
		t.Fatalf("lang=none predicate %q, want %q", got, want)
	}

	for _, f := range []string{"segment.go", "subscribers.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		// The constant's own declaration is the one literal.
		want := 0
		if f == "segment.go" {
			want = 1
		}
		if n := strings.Count(src, "subscriber_lang(subscribers.attribs) = 'none'"); n != want {
			t.Errorf("%s: %d literal no-language predicates (want the constant only)", f, n)
		}
	}
	b, err := os.ReadFile("subscribers.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	i := strings.Index(body, "func (c *Core) CountSubscribersLangNone(")
	if i < 0 {
		t.Fatal("CountSubscribersLangNone not found")
	}
	if j := strings.Index(body[i:], "\n}\n"); j < 0 || !strings.Contains(body[i:i+j], "SubscriberLangNonePred") {
		t.Error("CountSubscribersLangNone does not narrow by SubscriberLangNonePred")
	}
}

// D3/I6 -- the lang sort is the fixed bucket expression plus the id tiebreak in the same
// direction; every other field is spliced as before.
func TestSubOrderClause(t *testing.T) {
	if got, want := subOrderClause("lang", "asc"), subLangOrder+" asc, subscribers.id asc"; got != want {
		t.Errorf("lang asc: %q, want %q", got, want)
	}
	if got, want := subOrderClause("lang", "desc"), subLangOrder+" desc, subscribers.id desc"; got != want {
		t.Errorf("lang desc: %q, want %q", got, want)
	}
	if got := subOrderClause("email", "asc"); got != "email asc" {
		t.Errorf("email asc: %q", got)
	}
	found := false
	for _, f := range subQuerySortFields {
		if f == "lang" {
			found = true
		}
	}
	if !found {
		t.Error("subQuerySortFields lacks lang")
	}
}
