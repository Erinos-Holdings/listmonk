package models

import "testing"

// Fork (join link, integrations JOIN-LINK-SPEC I9) -- the shorthand rewrite: a bare
// `{{ JoinURL }}` (any inner spacing) becomes `{{ JoinURL . }}` so it receives the message, like
// the other URL functions; one already carrying its argument is untouched.
func TestJoinURLShorthand(t *testing.T) {
	rewrite := func(s string) string {
		for _, r := range regTplFuncs {
			s = r.apply(s)
		}
		return s
	}
	for in, want := range map[string]string{
		`<a href="{{ JoinURL }}">j</a>`:   `<a href="{{ JoinURL . }}">j</a>`,
		`<a href="{{JoinURL}}">j</a>`:     `<a href="{{ JoinURL . }}">j</a>`,
		`<a href="{{  JoinURL  }}">j</a>`: `<a href="{{ JoinURL . }}">j</a>`,
		`<a href="{{ JoinURL . }}">j</a>`: `<a href="{{ JoinURL . }}">j</a>`,
	} {
		if got := rewrite(in); got != want {
			t.Fatalf("rewrite(%s) = %s, want %s", in, got, want)
		}
	}
	// The reserved-function list (I10's other half): a JoinURL action is never wrapped.
	if !regReservedTplFunc.MatchString(`{{ JoinURL }}`) || !regReservedTplFunc.MatchString(`{{JoinURL . }}`) {
		t.Fatalf("regReservedTplFunc does not name JoinURL")
	}
}
