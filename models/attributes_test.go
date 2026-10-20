package models

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/knadh/listmonk/internal/linkresolve"
)

// Fork (attribute picker) -- integrations ATTRIBUTE-PICKER-SPEC I1b. The three `link`
// expressions the picker inserts (frontend/email-builder/src/attributes.js, brand `ruze`) --
// a copied literal list; the cross-repo agreement is I1 (attributes.test.cjs) ∧ I1b (here) ∧
// I1c (integrations tests/lib/campaign-review/attribute-roster.test.ts) on the same literals.
const (
	attrShopLink    = `{{ .Subscriber.Attribs.shop_links.ruze.url }}`
	attrUnsubscribe = `{{ UnsubscribeURL }}`
	attrMessage     = `{{ MessageURL }}`
)

// The shop link is a resolver expression: it parses with linkresolve.Check, reads exactly the
// attribs key `shop_links`, and references nothing outside `.Subscriber`. In a visual href it
// is wrapped into one dynamic TrackLink call and resolves per click.
func TestAttributePickerShopLinkResolves(t *testing.T) {
	others, keys, err := linkresolve.Check(attrShopLink)
	if err != nil {
		t.Fatalf("Check(%s): %v", attrShopLink, err)
	}
	if len(others) != 0 {
		t.Fatalf("non-Subscriber refs: %v", others)
	}
	if !reflect.DeepEqual(keys, []string{"shop_links"}) {
		t.Fatalf("attrib keys = %v, want [shop_links]", keys)
	}

	out := TransformTrackLinks(`<a href="`+attrShopLink+`">x</a>`, CampaignContentTypeVisual)
	want := `<a href="{{ TrackLink "` + attrShopLink + `" . }}">x</a>`
	if out != want {
		t.Fatalf("transform:\n got  %s\n want %s", out, want)
	}
	if got := linkresolve.Expressions(out); !reflect.DeepEqual(got, []string{attrShopLink}) {
		t.Fatalf("Expressions = %v, want [%s]", got, attrShopLink)
	}
}

// The two system actions are NOT resolver expressions (the resolver's FuncMap is empty, so it
// refuses them) and must survive the compile transform UNWRAPPED -- in a visual href and in the
// builder's VML href marker -- which is what keeps an unsubscribe Button working
// (regReservedTplFunc).
func TestAttributePickerSystemActionsUnwrapped(t *testing.T) {
	const marker = `<span data-lm-vml-href="%s"></span>`
	for _, expr := range []string{attrUnsubscribe, attrMessage} {
		t.Run(expr, func(t *testing.T) {
			if _, err := linkresolve.Parse(expr); err == nil {
				t.Fatalf("Parse(%s) succeeded; the resolver must refuse a system action", expr)
			}

			in := `<a href="` + expr + `">x</a>`
			out := TransformTrackLinks(in, CampaignContentTypeVisual)
			if out != in {
				t.Fatalf("href rewritten:\n got  %s\n want %s", out, in)
			}
			if got := linkresolve.Expressions(out); len(got) != 0 {
				t.Fatalf("Expressions = %v, want none", got)
			}

			out = TransformTrackLinks(fmt.Sprintf(marker, expr), CampaignContentTypeVisual)
			if out != expr {
				t.Fatalf("VML marker: got %s, want %s", out, expr)
			}
			if got := linkresolve.Expressions(out); len(got) != 0 {
				t.Fatalf("VML marker Expressions = %v, want none", got)
			}
		})
	}
}
