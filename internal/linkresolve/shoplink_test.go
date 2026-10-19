package linkresolve

// Fork (shop-link rate) -- integrations SHOP-LINK-RATE-SPEC I3: ShopLinkBrands reads the brand the
// way the click query's SQL does, and the click query carries ShopLinkBrandPattern verbatim (F11).

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// shopLinkBrandCases is the expression table: the parent spec's SQL fixtures (both grammars, a
// second brand) plus the or-wrapped and ?ref= forms, hyphen/digit slugs and a non-shop-link
// dynamic expression.
var shopLinkBrandCases = []struct {
	expr  string
	brand string // "" = no brand
}{
	{`{{ .Subscriber.Attribs.shop_links.ruze.url }}`, "ruze"},
	{`{{ index .Subscriber.Attribs.shop_links "ruze" }}`, "ruze"},
	{`{{ .Subscriber.Attribs.shop_links.thirstygirl.url }}`, "thirstygirl"},
	{`{{ or .Subscriber.Attribs.shop_links.thirstygirl.url "https://thirstygirl.test" }}`, "thirstygirl"},
	{`https://ruze.test/?ref={{ .Subscriber.Attribs.shop_links.ruze.slug }}`, "ruze"},
	{`{{ (index .Subscriber.Attribs.shop_links "brand-2_x").url }}`, "brand-2_x"},
	{`{{ .Subscriber.Attribs.shop_links.b9.url }}`, "b9"},
	{`{{ .Subscriber.Attribs.rewards_site_url }}`, ""},
	{`https://x.test/?e={{ .Subscriber.Email }}`, ""},
}

func TestShopLinkBrandsPerExpression(t *testing.T) {
	for _, c := range shopLinkBrandCases {
		got := ShopLinkBrands([]string{c.expr})
		var want []string
		if c.brand != "" {
			want = []string{c.brand}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ShopLinkBrands(%q) = %v, want %v", c.expr, got, want)
		}
	}
}

func TestShopLinkBrandsDedupesInOrder(t *testing.T) {
	exprs := []string{
		`{{ .Subscriber.Attribs.rewards_site_url }}`,
		`{{ .Subscriber.Attribs.shop_links.thirstygirl.url }}`,
		`{{ index .Subscriber.Attribs.shop_links "ruze" }}`,
		`{{ .Subscriber.Attribs.shop_links.thirstygirl.slug }}`,
		`{{ .Subscriber.Attribs.shop_links.ruze.url }}`,
	}
	if got, want := ShopLinkBrands(exprs), []string{"thirstygirl", "ruze"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ShopLinkBrands = %v, want %v", got, want)
	}
	if got := ShopLinkBrands(nil); got != nil {
		t.Fatalf("ShopLinkBrands(nil) = %v, want nil", got)
	}
}

// TestShopLinkBrandPatternInClickQuery is the F11 agreement pin: the click query's SQL text holds
// the same pattern literal, so the snapshot's brands and the clicks' brands cannot drift apart.
func TestShopLinkBrandPatternInClickQuery(t *testing.T) {
	regexp.MustCompile(ShopLinkBrandPattern) // RE2 accepts the POSIX class inside the bracket.

	b, err := os.ReadFile(filepath.Join("..", "..", "queries", "campaigns.sql"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	start := strings.Index(src, "-- name: get-campaign-shop-link-counts\n")
	if start < 0 {
		t.Fatal("get-campaign-shop-link-counts not found in queries/campaigns.sql")
	}
	body := src[start+1:]
	if end := strings.Index(body, "\n-- name: "); end >= 0 {
		body = body[:end]
	}
	if !strings.Contains(body, "'"+ShopLinkBrandPattern+"'") {
		t.Fatalf("get-campaign-shop-link-counts does not contain ShopLinkBrandPattern %q verbatim", ShopLinkBrandPattern)
	}
}
