package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestCheckLinkExpressionsHandler pins the handler half of CLICK-TRACKING-SPEC I14
// (implementation review F3): source-body extraction through the §3.1 transform, the 400
// refusal naming an unparseable expression, the non-.Subscriber reference warning, and the
// coverage warning counted against the campaign's targeted subscribers. Shares
// newLinkHarness (LISTMONK_TEST_PG opt-in).
func TestCheckLinkExpressionsHandler(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db

	var list int
	db.Get(&list, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme', 'public', 'single', '{brand:acme,"from:Acme <hello@acme.test>"}') RETURNING id`)
	for _, row := range []struct{ email, attribs string }{
		{"with@x", `{"site":"https://shop.acme.test/c1"}`},
		{"without@x", `{}`},
		{"blank@x", `{"site":""}`},
	} {
		var id int
		db.Get(&id, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), $1, 'S', $2::jsonb) RETURNING id`, row.email, row.attribs)
		db.MustExec(`INSERT INTO subscriber_lists (subscriber_id, list_id, status) VALUES ($1, $2, 'confirmed')`, id, list)
	}

	newCampaign := func(body string) int {
		var id int
		db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, content_type, messenger, template_id) VALUES (gen_random_uuid(), 'C', 's', 'f@x', $1, 'visual', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id`, body)
		db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'Acme')`, id, list)
		return id
	}

	// (a) An expression the D4 resolver cannot parse (sprig `lower`) refuses Start with a 400
	// that names the expression.
	bad := newCampaign(`<a href="{{ .Subscriber.Attribs.site | lower }}">x</a>`)
	_, err := h.app.checkLinkExpressions(bad)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("unparseable expression: want 400 HTTPError, got %v", err)
	}
	if msg, _ := he.Message.(string); !strings.Contains(msg, ".Subscriber.Attribs.site | lower") {
		t.Fatalf("400 message does not name the expression: %v", he.Message)
	}

	// (b)+(c) A parseable expression referencing .Campaign warns once; the attrib key referenced
	// in two buttons is counted once, against the three targeted subscribers (2 lack/blank).
	good := newCampaign(`<a href="{{ or .Subscriber.Attribs.site .Campaign.Name }}">x</a>` +
		`<a href="{{ .Subscriber.Attribs.site }}">y</a>`)
	warnings, err := h.app.checkLinkExpressions(good)
	if err != nil {
		t.Fatalf("valid expressions: %v", err)
	}
	var nonSub, coverage int
	for _, w := range warnings {
		switch {
		case strings.Contains(w, ".Campaign.Name"):
			nonSub++
		case strings.Contains(w, "'site'") && strings.Contains(w, "2") && strings.Contains(w, "3") && strings.Contains(w, "https://acme.test"):
			coverage++
		default:
			t.Fatalf("unexpected warning: %q", w)
		}
	}
	if nonSub != 1 || coverage != 1 {
		t.Fatalf("want 1 non-subscriber warning and 1 coverage warning (key de-duplicated), got %q", warnings)
	}

	// Zero missing → silent.
	db.MustExec(`UPDATE subscribers SET attribs = '{"site":"https://shop.acme.test/c2"}'::jsonb WHERE email IN ('without@x','blank@x')`)
	quiet := newCampaign(`<a href="{{ .Subscriber.Attribs.site }}">y</a>`)
	if w, err := h.app.checkLinkExpressions(quiet); err != nil || len(w) != 0 {
		t.Fatalf("full coverage: want no warnings, got %q err=%v", w, err)
	}
}
