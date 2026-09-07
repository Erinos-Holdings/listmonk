package models

import (
	"errors"
	"strings"
	"testing"
)

// Fork (CAMPAIGN-52-HARDENING D7) -- the pure list-tag validator shared by the list form and
// the import presets: every rule cmd/lists_brand.go used to hold, table-tested here once.
func TestListTagsProblem(t *testing.T) {
	sanitize := func(s string) (string, error) {
		if !strings.Contains(s, "@") || strings.ContainsAny(s, " <>") {
			return "", errors.New("invalid")
		}
		return s, nil
	}
	allowed := func(bare string) bool { return bare == "hello@x.com" }

	cases := []struct {
		name    string
		tags    []string
		allowed func(string) bool
		want    string
		arg     string
	}{
		{"no tags", nil, allowed, "", ""},
		{"free-form only", []string{"test", "internal"}, allowed, "", ""},
		{"valid pair", []string{"brand:x", "from:X <hello@x.com>"}, allowed, "", ""},
		{"valid pair, bare from", []string{"brand:x", "from:hello@x.com"}, allowed, "", ""},
		{"valid pair, whitespace", []string{" brand: x ", "from: X <hello@x.com> "}, allowed, "", ""},
		{"valid pair with site", []string{"brand:x", "from:hello@x.com", "site:https://shop.x.com"}, allowed, "", ""},
		{"no allowlist skips the address check", []string{"brand:x", "from:X <nobody@y.com>"}, nil, "", ""},
		{"half: brand only", []string{"brand:x"}, allowed, "lists.brandTagsHalfTagged", ""},
		{"half: from only", []string{"from:hello@x.com"}, allowed, "lists.brandTagsHalfTagged", ""},
		{"duplicate brand", []string{"brand:x", "brand:y", "from:hello@x.com"}, allowed, "lists.brandTagsDuplicate", ""},
		{"duplicate from", []string{"brand:x", "from:hello@x.com", "from:a@x.com"}, allowed, "lists.brandTagsDuplicate", ""},
		{"bad slug", []string{"brand:Thirsty Girl", "from:hello@x.com"}, allowed, "lists.brandTagInvalidSlug", "Thirsty Girl"},
		{"non-ASCII from", []string{"brand:x", "from:Liyorá <hello@x.com>"}, allowed, "lists.brandFromTagNotASCII", "Liyorá <hello@x.com>"},
		{"trailing junk", []string{"brand:x", "from:X <hello@x.com> JUNK"}, allowed, "lists.brandFromTagInvalid", "X <hello@x.com> JUNK"},
		{"bare unsanitizable", []string{"brand:x", "from:not an address"}, allowed, "lists.brandFromTagInvalid", "not an address"},
		{"unknown address", []string{"brand:x", "from:X <hello@y.com>"}, allowed, "lists.brandFromTagUnknownAddress", "X <hello@y.com>"},
		{"site without brand", []string{"site:https://shop.x.com"}, allowed, "lists.siteTagNeedsBrand", ""},
		{"site relative", []string{"brand:x", "from:hello@x.com", "site:/store"}, allowed, "lists.siteTagInvalid", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key, args := ListTagsProblem(c.tags, c.allowed, sanitize)
			if key != c.want {
				t.Fatalf("key %q want %q (args %v)", key, c.want, args)
			}
			if c.arg != "" && (len(args) != 2 || args[1] != c.arg) {
				t.Fatalf("args %v want [_ %q]", args, c.arg)
			}
		})
	}
}

func TestListTagHelpers(t *testing.T) {
	if got := TrimListTag(" brand: liyora "); got != "brand:liyora" {
		t.Fatalf("TrimListTag brand = %q", got)
	}
	if got := TrimListTag("from: Liyora <hello@x.com> "); got != "from:Liyora <hello@x.com>" {
		t.Fatalf("TrimListTag from = %q", got)
	}
	if got := TrimListTag("  plain  "); got != "plain" {
		t.Fatalf("TrimListTag plain = %q", got)
	}
	if got := BareAddress("Liyora <hello@x.com>"); got != "hello@x.com" {
		t.Fatalf("BareAddress = %q", got)
	}
	if got := BareAddress(" hello@x.com "); got != "hello@x.com" {
		t.Fatalf("BareAddress bare = %q", got)
	}
	if got := SiteTagOf([]string{"brand:x", " site: https://shop.x.com "}); got != "https://shop.x.com" {
		t.Fatalf("SiteTagOf = %q", got)
	}
}
