package linkresolve

import "regexp"

// ShopLinkBrandPattern is the ONE literal that names a shop link's brand: the first
// [A-Za-z0-9_-]+ token after shop_links, in both grammars the resolver accepts
// (`.shop_links.<brand>` and `index .Subscriber.Attribs.shop_links "<brand>"`).
//
// Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D3) -- the click query
// get-campaign-shop-link-counts (queries/campaigns.sql) carries this exact text in its SUBSTRING,
// so the brands the audience snapshot counts are the brands the clicks are bucketed by. RE2
// accepts the POSIX class inside the bracket as Postgres does. The agreement is pinned by
// shoplink_test.go; change both or neither.
const ShopLinkBrandPattern = `shop_links[."[:space:]]*([A-Za-z0-9_-]+)`

var reShopLinkBrand = regexp.MustCompile(ShopLinkBrandPattern)

// ShopLinkBrands returns the distinct brand tokens named by the shop-link expressions among exprs
// (dynamic tracked-link expressions, as Expressions returns them), first-seen order: the first
// token after shop_links in each. An expression without shop_links contributes nothing.
//
// Fork (shop-link rate, integrations SHOP-LINK-RATE-SPEC D3).
func ShopLinkBrands(exprs []string) []string {
	var (
		out  []string
		seen = map[string]struct{}{}
	)
	for _, e := range exprs {
		m := reShopLinkBrand.FindStringSubmatch(e)
		if m == nil {
			continue
		}
		if _, ok := seen[m[1]]; ok {
			continue
		}
		seen[m[1]] = struct{}{}
		out = append(out, m[1])
	}
	return out
}
