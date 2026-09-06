package main

import "testing"

// Fork (click tracking) -- CLICK-TRACKING-SPEC T7 (I7a): the `site:` list tag is refused
// without the brand:/from: pair, when duplicated, and when not an absolute http(s) URL.
func TestSiteTagProblem(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want string
	}{
		{"no tags", nil, ""},
		{"brand pair only", []string{"brand:x", "from:X <hello@x.com>"}, ""},
		{"valid site beside the pair", []string{"brand:x", "from:X <hello@x.com>", "site:https://shop.x.com"}, ""},
		{"valid site, http", []string{"brand:x", "from:hello@x.com", "site:http://shop.x.com/store"}, ""},
		{"whitespace tolerated", []string{" brand:x ", "from:hello@x.com", " site: https://shop.x.com "}, ""},
		{"site alone", []string{"site:https://shop.x.com"}, "lists.siteTagNeedsBrand"},
		{"site with brand only", []string{"brand:x", "site:https://shop.x.com"}, "lists.siteTagNeedsBrand"},
		{"site with from only", []string{"from:hello@x.com", "site:https://shop.x.com"}, "lists.siteTagNeedsBrand"},
		{"duplicate site", []string{"brand:x", "from:hello@x.com", "site:https://a.x", "site:https://b.x"}, "lists.siteTagDuplicate"},
		{"relative site", []string{"brand:x", "from:hello@x.com", "site:/store"}, "lists.siteTagInvalid"},
		{"bare host site", []string{"brand:x", "from:hello@x.com", "site:shop.x.com"}, "lists.siteTagInvalid"},
		{"javascript site", []string{"brand:x", "from:hello@x.com", "site:javascript:alert(1)"}, "lists.siteTagInvalid"},
		{"empty site", []string{"brand:x", "from:hello@x.com", "site:"}, "lists.siteTagInvalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := siteTagProblem(c.tags); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}

	if got := siteTagOf([]string{"brand:x", " site: https://shop.x.com "}); got != "https://shop.x.com" {
		t.Fatalf("siteTagOf = %q", got)
	}
}
