package linkresolve

// CLICK-TRACKING-SPEC T3/T4/T6: the pure resolver, fallback derivation, UTM rules and the
// Start-time expression analysis, with no DB or App.

import (
	"reflect"
	"strings"
	"testing"
)

type fakeSub struct {
	UUID    string
	Email   string
	Name    string
	Attribs map[string]any
}

func sub(attribs map[string]any) *fakeSub {
	return &fakeSub{UUID: "u", Email: "a@b.c", Name: "Ann Lee", Attribs: attribs}
}

// I4/I6/I7 -- Resolve: success incl. the `or` idiom and Go builtins; empty/<no value>/
// javascript:/relative → unresolvable; sprig/TrackLink/Safe → parse failure.
func TestResolve(t *testing.T) {
	site := map[string]any{"site": "https://shop.example/creator-1", "path": "/x"}
	cases := []struct {
		name string
		expr string
		sub  any
		want string
		kind Kind
	}{
		{"attrib resolves", `{{ .Subscriber.Attribs.site }}`, sub(site), "https://shop.example/creator-1", 0},
		{"or idiom uses the attrib when present", `{{ or .Subscriber.Attribs.site "https://curatedfor.you" }}`, sub(site), "https://shop.example/creator-1", 0},
		{"or idiom falls to the literal when missing", `{{ or .Subscriber.Attribs.site "https://curatedfor.you" }}`, sub(map[string]any{}), "https://curatedfor.you", 0},
		{"printf builtin", `{{ printf "https://x.test/%s" .Subscriber.UUID }}`, sub(nil), "https://x.test/u", 0},
		{"urlquery builtin", `https://x.test/?n={{ urlquery .Subscriber.Name }}`, sub(nil), "https://x.test/?n=Ann+Lee", 0},
		{"index builtin", `{{ index .Subscriber.Attribs "site" }}`, sub(site), "https://shop.example/creator-1", 0},
		{"surrounding whitespace trimmed", `  {{ .Subscriber.Attribs.site }}  `, sub(site), "https://shop.example/creator-1", 0},
		{"missing attrib renders <no value> → empty", `{{ .Subscriber.Attribs.site }}`, sub(map[string]any{}), "", KindEmpty},
		{"empty attrib → empty", `{{ .Subscriber.Attribs.site }}`, sub(map[string]any{"site": ""}), "", KindEmpty},
		{"nil attribs map → empty", `{{ .Subscriber.Attribs.site }}`, sub(nil), "", KindEmpty},
		{"javascript: → invalid", `{{ .Subscriber.Attribs.site }}`, sub(map[string]any{"site": "javascript:alert(1)"}), "", KindInvalid},
		{"relative path → invalid", `{{ .Subscriber.Attribs.path }}`, sub(site), "", KindInvalid},
		{"bare word → invalid", `{{ .Subscriber.Name }}`, sub(nil), "", KindInvalid},
		{"mailto → invalid", `mailto:{{ .Subscriber.Email }}`, sub(nil), "", KindInvalid},
		{"sprig lower is not defined → parse", `{{ .Subscriber.Attribs.site | lower }}`, sub(site), "", KindParse},
		{"TrackLink cannot recurse → parse", `{{ TrackLink "https://x.test" . }}`, sub(nil), "", KindParse},
		{"Safe not available → parse", `{{ Safe "<b>x</b>" }}`, sub(nil), "", KindParse},
		{"Campaign context renders empty → empty", `{{ .Campaign.Name }}`, sub(nil), "", KindEmpty},
		{"unterminated action → parse", `{{ .Subscriber.Attribs.site `, sub(site), "", KindParse},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Resolve(c.expr, c.sub)
			if c.kind == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != c.want {
					t.Fatalf("got %q want %q", got, c.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected %s error, got %q", c.kind, got)
			}
			e, ok := err.(*Error)
			if !ok || e.Kind != c.kind {
				t.Fatalf("expected kind %s, got %v", c.kind, err)
			}
			if got != "" {
				t.Fatalf("destination must be empty on error, got %q", got)
			}
		})
	}
}

// I3-adjacent: a static URL is never a dynamic one.
func TestIsDynamic(t *testing.T) {
	if IsDynamic("https://x.test/{a}") || !IsDynamic("{{ .Subscriber.UUID }}") {
		t.Fatal("IsDynamic misclassifies")
	}
}

// I7b -- brand fallback derivation table.
func TestFallback(t *testing.T) {
	const setting = "https://curatedfor.you"
	cases := []struct {
		name    string
		b       Brand
		setting string
		want    string
	}{
		{"from-domain, bare address", Brand{Mapped: true, Slug: "thirstygirl", FromAddress: "hello@thirstygirlhydration.com"}, setting, "https://thirstygirlhydration.com"},
		{"from-domain is lower-cased", Brand{Mapped: true, Slug: "x", FromAddress: "Hello@Example.COM"}, setting, "https://example.com"},
		{"site tag wins over the from domain", Brand{Mapped: true, Slug: "x", FromAddress: "hello@x.com", Site: "https://shop.x.com/store"}, setting, "https://shop.x.com/store"},
		{"invalid site tag is ignored", Brand{Mapped: true, Slug: "x", FromAddress: "hello@x.com", Site: "shop.x.com"}, setting, "https://x.com"},
		{"unmapped → setting", Brand{}, setting, setting},
		{"default curated slug → setting", Brand{Mapped: true, Slug: DefaultBrandSlug, FromAddress: "hello@curatedfor.you"}, setting, setting},
		{"mapping error → setting", Brand{Err: true, Mapped: true, Slug: "x", FromAddress: "hello@x.com"}, setting, setting},
		{"mapped but no usable address → setting", Brand{Mapped: true, Slug: "x", FromAddress: "nonsense"}, setting, setting},
		{"blank setting + no brand → empty (error page)", Brand{}, "", ""},
		{"non-http setting is treated as blank", Brand{}, "ftp://x", ""},
		{"blank setting but a brand → brand site", Brand{Mapped: true, Slug: "x", FromAddress: "hello@x.com"}, "", "https://x.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Fallback(c.b, c.setting); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

// I7a -- the absolute http(s) rule shared by the setting and the site: tag.
func TestIsAbsoluteHTTP(t *testing.T) {
	for _, ok := range []string{"https://x.test", "http://x.test/p?q=1", "HTTPS://X.TEST"} {
		if !IsAbsoluteHTTP(ok) {
			t.Fatalf("%q should be accepted", ok)
		}
	}
	for _, bad := range []string{"", "x.test", "/relative", "javascript:alert(1)", "mailto:a@b.c", "https://", "ftp://x.test", "<no value>"} {
		if IsAbsoluteHTTP(bad) {
			t.Fatalf("%q should be refused", bad)
		}
	}
}

// I8 -- UTM: host union (subdomains included), no-override rule, encoding, disabled flag,
// static/dynamic/fallback destinations alike.
func TestApplyUTM(t *testing.T) {
	hosts := HostUnion(
		[]string{"hello@thirstygirlhydration.com", "Curated <hello@curatedfor.you>", ""},
		[]string{"https://shop.example.com/store"},
		[]string{"curatedby.you", " ", "https://extra.test/x"},
	)
	want := []string{"curatedby.you", "curatedfor.you", "extra.test", "shop.example.com", "thirstygirlhydration.com"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("HostUnion = %v want %v", hosts, want)
	}
	// Fallback derivation takes the BARE address (cmd's bareAddress is the parser); a
	// display-name form there is not an address and yields no domain.
	if got := Fallback(Brand{Mapped: true, Slug: "x", FromAddress: "Curated <hello@curatedfor.you>"}, ""); got != "https://curatedfor.you" {
		t.Fatalf("display-name form tolerated defensively, got %q", got)
	}

	params := map[string]string{"utm_source": "listmonk", "utm_medium": "email", "utm_campaign": "Loyalty Rewards & More", "utm_content": "52"}
	cases := []struct {
		name string
		dest string
		want string
	}{
		{"storefront host tagged", "https://curatedfor.you/", "https://curatedfor.you/?utm_source=listmonk&utm_medium=email&utm_campaign=Loyalty+Rewards+%26+More&utm_content=52"},
		{"subdomain tagged", "https://shop.curatedfor.you/p?x=1", "https://shop.curatedfor.you/p?x=1&utm_source=listmonk&utm_medium=email&utm_campaign=Loyalty+Rewards+%26+More&utm_content=52"},
		{"host case-insensitive", "https://CuratedFor.You/", "https://CuratedFor.You/?utm_source=listmonk&utm_medium=email&utm_campaign=Loyalty+Rewards+%26+More&utm_content=52"},
		{"fragment preserved", "https://curatedfor.you/#top", "https://curatedfor.you/?utm_source=listmonk&utm_medium=email&utm_campaign=Loyalty+Rewards+%26+More&utm_content=52#top"},
		{"site: tag host tagged", "https://shop.example.com/store/1", "https://shop.example.com/store/1?utm_source=listmonk&utm_medium=email&utm_campaign=Loyalty+Rewards+%26+More&utm_content=52"},
		{"external host untouched", "https://instagram.com/x", "https://instagram.com/x"},
		{"lookalike suffix untouched", "https://notcuratedfor.you/", "https://notcuratedfor.you/"},
		{"existing utm_ key → untouched", "https://curatedfor.you/?utm_source=manual", "https://curatedfor.you/?utm_source=manual"},
		{"existing UTM_ key any case → untouched", "https://curatedfor.you/?UTM_term=x", "https://curatedfor.you/?UTM_term=x"},
		{"relative dest untouched", "/local", "/local"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ApplyUTM(c.dest, hosts, params); got != c.want {
				t.Fatalf("got  %s\nwant %s", got, c.want)
			}
		})
	}

	// Disabled = the caller passes no params (cmd checks app.utm_enable) → unchanged.
	if got := ApplyUTM("https://curatedfor.you/", hosts, nil); got != "https://curatedfor.you/" {
		t.Fatalf("no params must be a no-op, got %s", got)
	}
	// Empty union → unchanged.
	if got := ApplyUTM("https://curatedfor.you/", nil, params); got != "https://curatedfor.you/" {
		t.Fatalf("empty host union must be a no-op, got %s", got)
	}
}

// D8 -- the parameter templates render campaign name/id; a bad template is dropped, not
// emitted raw; validation names the offending key.
func TestUTMParams(t *testing.T) {
	tpls := map[string]string{
		"utm_source":   "listmonk",
		"utm_campaign": "{{ .Campaign.Name }}",
		"utm_content":  "{{ .Campaign.ID }}",
		"utm_broken":   "{{ .Campaign.Name | lower }}",
	}
	got := RenderUTMParams(tpls, CampaignInfo{ID: 52, Name: "Loyalty Rewards"})
	want := map[string]string{"utm_source": "listmonk", "utm_campaign": "Loyalty Rewards", "utm_content": "52"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RenderUTMParams = %v want %v", got, want)
	}
	if k, err := ValidateUTMParams(tpls); err == nil || k != "utm_broken" {
		t.Fatalf("ValidateUTMParams should name utm_broken, got %q %v", k, err)
	}
	if _, err := ValidateUTMParams(want); err != nil {
		t.Fatalf("valid params refused: %v", err)
	}
	if k, err := ValidateUTMParams(map[string]string{" ": "x"}); err == nil || k != " " {
		t.Fatal("empty key must be refused")
	}
}

// I14 -- Expressions extracts only DYNAMIC TrackLink arguments from a transformed body, with
// Go string escapes undone, de-duplicated.
func TestExpressions(t *testing.T) {
	body := `<a href="{{ TrackLink "https://static.test/p" . }}">s</a>` +
		`<a href="{{ TrackLink "{{ .Subscriber.Attribs.site }}" . }}">d1</a>` +
		`{{ Safe "\x3c!--[if\x20mso]\x3e…href=\"" }}{{ TrackLink "{{ .Subscriber.Attribs.site }}" . }}{{ Safe "\"…" }}` +
		`<a href="{{ TrackLink "{{ or .Subscriber.Attribs.site \"https://curatedfor.you\" }}" . }}">d2</a>` +
		`<a href="{{ TrackLink "https://x.test/{{ .Subscriber.UUID }}" }}">shorthand-normalised-later</a>`
	got := Expressions(body)
	want := []string{
		`{{ .Subscriber.Attribs.site }}`,
		`{{ or .Subscriber.Attribs.site "https://curatedfor.you" }}`,
		`https://x.test/{{ .Subscriber.UUID }}`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Expressions = %#v want %#v", got, want)
	}
	if UnescapeGoString(`a\"b\\c\n`) != `a"b\c\n` {
		t.Fatal("UnescapeGoString")
	}
	if EscapeGoString(`a"b`) != `a\"b` {
		t.Fatal("EscapeGoString")
	}
}

// I14 -- Check: parse refusal names the failure; non-.Subscriber references are reported;
// attrib keys are collected from both field and index forms; a clean expression is silent.
func TestCheck(t *testing.T) {
	others, keys, err := Check(`{{ or .Subscriber.Attribs.site .Subscriber.Attribs.home }}`)
	if err != nil || len(others) != 0 || !reflect.DeepEqual(keys, []string{"home", "site"}) {
		t.Fatalf("clean expr: others=%v keys=%v err=%v", others, keys, err)
	}

	others, keys, err = Check(`{{ .Campaign.Name }}/{{ index .Subscriber.Attribs "loyalty_rewards_creator_site_url" }}/{{ $.Subscriber.Attribs.x }}/{{ .Other.Thing }}`)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !reflect.DeepEqual(others, []string{".Campaign.Name", ".Other.Thing"}) {
		t.Fatalf("others = %v", others)
	}
	if !reflect.DeepEqual(keys, []string{"loyalty_rewards_creator_site_url", "x"}) {
		t.Fatalf("keys = %v", keys)
	}

	// Non-attrib subscriber fields are neither "other" nor keys.
	others, keys, err = Check(`https://x.test/{{ .Subscriber.UUID }}?e={{ urlquery .Subscriber.Email }}`)
	if err != nil || len(others) != 0 || len(keys) != 0 {
		t.Fatalf("subscriber fields: others=%v keys=%v err=%v", others, keys, err)
	}

	// Parse failures name the function.
	if _, _, err := Check(`{{ .Subscriber.Attribs.site | lower }}`); err == nil || !strings.Contains(err.Error(), `"lower"`) {
		t.Fatalf("expected a parse error naming lower, got %v", err)
	}
	if _, _, err := Check(`{{ TrackLink "x" . }}`); err == nil {
		t.Fatal("TrackLink must not parse")
	}
	// if/with blocks are walked too.
	_, keys, err = Check(`{{ if .Subscriber.Attribs.a }}{{ .Subscriber.Attribs.b }}{{ else }}{{ .Subscriber.Attribs.c }}{{ end }}`)
	if err != nil || !reflect.DeepEqual(keys, []string{"a", "b", "c"}) {
		t.Fatalf("if/else keys = %v err=%v", keys, err)
	}
}
