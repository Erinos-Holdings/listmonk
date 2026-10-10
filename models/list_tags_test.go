package models

import (
	"errors"
	"strings"
	"testing"

	null "gopkg.in/volatiletech/null.v6"
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

	// BRAND-PICKER-SPEC I6 -- one implementation. Every row of the table that names exactly one
	// brand and one from (and at most one site) passes through BrandProblem DIRECTLY with the
	// same inputs and yields the same key and args: the value rules live only there. The other
	// rows are the structural rules a tag set alone can break (half-tagged, duplicates, a site
	// without the pair), which have no brands-row equivalent.
	direct := 0
	for _, c := range cases {
		var brands, froms, sites []string
		for _, tg := range c.tags {
			tg = TrimListTag(tg)
			switch {
			case strings.HasPrefix(tg, BrandTagPrefix):
				brands = append(brands, strings.TrimPrefix(tg, BrandTagPrefix))
			case strings.HasPrefix(tg, FromTagPrefix):
				froms = append(froms, strings.TrimPrefix(tg, FromTagPrefix))
			case strings.HasPrefix(tg, SiteTagPrefix):
				sites = append(sites, strings.TrimPrefix(tg, SiteTagPrefix))
			}
		}
		if len(brands) != 1 || len(froms) != 1 || len(sites) > 1 {
			continue
		}
		site := ""
		if len(sites) == 1 {
			site = sites[0]
		}
		direct++
		t.Run("direct/"+c.name, func(t *testing.T) {
			key, args := BrandProblem(brands[0], froms[0], site, c.allowed, sanitize)
			if key != c.want {
				t.Fatalf("BrandProblem key %q want %q (args %v)", key, c.want, args)
			}
			if c.arg != "" && (len(args) != 2 || args[1] != c.arg) {
				t.Fatalf("BrandProblem args %v want [_ %q]", args, c.arg)
			}
		})
	}
	if direct < 10 {
		t.Fatalf("only %d table rows exercised BrandProblem directly", direct)
	}
}

// BRAND-PICKER-SPEC I6 -- the two rules new in BrandProblem: a From or site that cannot fit a
// VARCHAR(100) list tag (with its prefix), and the reserved slug `health` (any case). Both apply
// through ListTagsProblem as well, since it delegates.
func TestBrandProblemNewRules(t *testing.T) {
	addr := "<hello@x.com>"
	fits := strings.Repeat("A", 100-len("from:")-len(addr)-1) + " " + addr // exactly 100 with from:
	long := strings.Repeat("A", 100-len("from:")-len(addr)) + " " + addr   // 101
	siteFits := "https://x.com/" + strings.Repeat("a", 100-len("site:")-len("https://x.com/"))
	siteLong := siteFits + "a"
	if len("from:"+fits) != 100 || len("from:"+long) != 101 || len("site:"+siteFits) != 100 {
		t.Fatal("fixture lengths are off")
	}

	cases := []struct {
		name, slug, from, site, want string
	}{
		{"from fits", "x", fits, "", ""},
		{"from too long", "x", long, "", "lists.brandFromTagTooLong"},
		{"site fits", "x", "hello@x.com", siteFits, ""},
		{"site too long", "x", "hello@x.com", siteLong, "lists.siteTagTooLong"},
		{"health reserved", "health", "hello@x.com", "", "lists.brandSlugReserved"},
		{"Health reserved", "Health", "hello@x.com", "", "lists.brandSlugReserved"},
		{"healthy allowed", "healthy", "hello@x.com", "", ""},
		{"empty from", "x", "", "", "lists.brandFromTagInvalid"},
		// Header injection (Stage 4 review finding 1): control characters and a bracketed display name.
		{"CRLF in display name", "x", "Curated\r\n<hello@x.com>", "", "lists.brandFromTagInvalid"},
		{"injected header", "x", "Curated\rX-Injected: y <hello@x.com>", "", "lists.brandFromTagInvalid"},
		{"LF before address", "x", "Curated\n<hello@x.com>", "", "lists.brandFromTagInvalid"},
		{"tab in display name", "x", "Cur\tated <hello@x.com>", "", "lists.brandFromTagInvalid"},
		{"DEL in display name", "x", "Curated\x7f <hello@x.com>", "", "lists.brandFromTagInvalid"},
		{"two angle pairs", "x", "<a@b.com> <c@d.com>", "", "lists.brandFromTagInvalid"},
		{"bracket in display name", "x", "Cur<ated <hello@x.com>", "", "lists.brandFromTagInvalid"},
		// The live roster's shapes must keep passing.
		{"live Curated", "curated", "Curated <hello@curatedfor.you>", "", ""},
		{"live Thirsty Girl", "thirstygirl", "Thirsty Girl <hello@thirstygirlhydration.com>", "", ""},
		{"live KNOWN FOR", "knownfor", "KNOWN FOR FRAGRANCE <hello@knownforfragrance.com>", "", ""},
		{"live BRIM beauty", "brimbeauty", "BRIM beauty <hello@brimbeauty.co>", "", ""},
		{"bare address", "x", "hello@x.com", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if key, args := BrandProblem(c.slug, c.from, c.site, nil, nil); key != c.want {
				t.Fatalf("BrandProblem key %q want %q (args %v)", key, c.want, args)
			}
		})
	}
	if key, _ := ListTagsProblem([]string{"brand:health", "from:hello@x.com"}, nil, nil); key != "lists.brandSlugReserved" {
		t.Fatalf("ListTagsProblem does not delegate the reserved slug: %q", key)
	}
	if key, _ := ListTagsProblem([]string{"brand:x", "from:" + long}, nil, nil); key != "lists.brandFromTagTooLong" {
		t.Fatalf("ListTagsProblem does not delegate the length rule: %q", key)
	}
}

// BRAND-PICKER-SPEC D2 -- the projection is byte-identical to the tags the readers parse, the
// reserved-tag test judges after TrimListTag, and the display name is the From's name part.
func TestBrandProjectionAndReserved(t *testing.T) {
	b := Brand{Slug: "curated", FromEmail: "Curated <hello@curatedfor.you>"}
	if got := strings.Join(BrandProjection(b), "|"); got != "brand:curated|from:Curated <hello@curatedfor.you>" {
		t.Fatalf("projection = %q", got)
	}
	b.Site = null.StringFrom("https://shop.x.com")
	if got := strings.Join(BrandProjection(b), "|"); got != "brand:curated|from:Curated <hello@curatedfor.you>|site:https://shop.x.com" {
		t.Fatalf("projection with site = %q", got)
	}
	for tg, want := range map[string]bool{"brand:x": true, " brand: x": true, "from:a@b": true, "site:https://x": true,
		"repermission:12": false, "holiday-2026": false, "brandx": false} {
		if IsReservedListTag(tg) != want {
			t.Errorf("IsReservedListTag(%q) != %v", tg, want)
		}
	}
	if BrandDisplayName("Curated <hello@curatedfor.you>") != "Curated" || BrandDisplayName("hello@x.com") != "hello@x.com" {
		t.Fatal("BrandDisplayName")
	}
	if !IsLockedListName("Render catalog (never send)") || IsLockedListName("Render catalog") {
		t.Fatal("IsLockedListName")
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
