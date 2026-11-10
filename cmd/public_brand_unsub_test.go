package main

// Fork (brand unsubscribe) -- integrations listmonk/BRAND-UNSUBSCRIBE-SPEC invariant A15 (D11):
// the public unsubscribe confirmation is GENERIC when the brand expansion applied, because the
// write reached every marketing list of the brand and naming one list of many would misdescribe
// it, and keeps today's list names when it did not (an untagged-list campaign -- D5). Shares
// newBrandFixture / newLinkHarness (LISTMONK_TEST_PG opt-in).

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// unsub drives SubscriptionPrefs the way the footer link does (GET-shaped POST, req.Manage false)
// and returns the rendered template name and the lists it named.
func (f *brandFixture) unsub(campUUID, subUUID string) (string, []string) {
	e := echo.New()
	rend := &capturingRenderer{}
	e.Renderer = rend
	req := httptest.NewRequest(http.MethodPost, "/subscription/"+campUUID+"/"+subUUID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("campUUID", "subUUID")
	c.SetParamValues(campUUID, subUUID)
	if err := f.app.SubscriptionPrefs(c); err != nil {
		f.t.Fatalf("SubscriptionPrefs: %v", err)
	}
	out, ok := rend.data.(unsubbedTpl)
	if !ok {
		f.t.Fatalf("rendered %s with %T", rend.name, rend.data)
	}
	return rend.name, out.Lists
}

func TestBrandUnsubscribe_ConfirmationPage(t *testing.T) {
	f := newBrandFixture(t)

	// The expansion applied (the campaign resolves a brand) ⇒ no list names. The campaign is on
	// the brand's PUBLIC newsletter list on purpose: get-campaign-list-names hides private
	// lists, so a campaign on a step list would render no names whatever this branch did.
	brandCamp := f.camp("acme day 5", f.news)
	id, uu := f.sub("page-brand@x", f.news, f.step)
	name, lists := f.unsub(brandCamp, uu)
	if name != "unsubscribed" {
		t.Fatalf("rendered %q", name)
	}
	if len(lists) != 0 {
		t.Fatalf("a brand unsubscribe named lists: %v", lists)
	}
	// The unsubscribe itself still happened -- the generic page is not a silent failure.
	if got := f.statusOf(id, f.news); got != "unsubscribed" {
		t.Fatalf("newsletter row %s, want unsubscribed", got)
	}

	// No brand resolved (an untagged-list campaign, D5) ⇒ today's list names, unchanged.
	plainCamp := f.camp("no brand blast", f.untagged)
	_, uu2 := f.sub("page-plain@x", f.untagged, f.news)
	name2, lists2 := f.unsub(plainCamp, uu2)
	if name2 != "unsubscribed" {
		t.Fatalf("rendered %q", name2)
	}
	if len(lists2) != 1 || lists2[0] != "no brand" {
		t.Fatalf("list names = %v, want [\"no brand\"]", lists2)
	}
}
