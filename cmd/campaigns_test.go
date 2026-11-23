package main

// Fork (smart sending) -- integrations listmonk/SMART-SENDING-SPEC I6: attribs.smart_sending is
// accepted absent / true / false only at the API. TestValidSmartSending is the pure rule
// (models.ValidSmartSending, which validateCampaignFields calls); TestSmartSendingValidation drives
// the UpdateCampaign handler (validateCampaignFields) against a real database -- LISTMONK_TEST_PG
// opt-in, newLinkHarness -- so "no" is a 400 naming the field and nothing is stored.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
	"github.com/knadh/paginator"
	"github.com/labstack/echo/v4"
)

func TestValidSmartSending(t *testing.T) {
	for name, tc := range map[string]struct {
		attribs models.JSON
		ok      bool
	}{
		"nil attribs": {nil, true},
		"absent":      {models.JSON{"lang": "en"}, true},
		"true":        {models.JSON{"smart_sending": true}, true},
		"false":       {models.JSON{"smart_sending": false}, true},
		"string no":   {models.JSON{"smart_sending": "no"}, false},
		"string true": {models.JSON{"smart_sending": "true"}, false},
		"number":      {models.JSON{"smart_sending": 0.0}, false},
		"null":        {models.JSON{"smart_sending": nil}, false},
	} {
		if got := models.ValidSmartSending(tc.attribs); got != tc.ok {
			t.Errorf("%s: %v, want %v", name, got, tc.ok)
		}
	}
	// The accessor: off only for the boolean false.
	for attribs, want := range map[string]bool{`{}`: true, `{"smart_sending": true}`: true, `{"smart_sending": false}`: false, `{"smart_sending": "n/a"}`: true} {
		var c models.Campaign
		if err := json.Unmarshal([]byte(attribs), &c.Attribs); err != nil {
			t.Fatal(err)
		}
		if got := c.SmartSending(); got != want {
			t.Errorf("%s: SmartSending() = %v, want %v", attribs, got, want)
		}
	}
}

func TestSmartSendingValidation(t *testing.T) {
	h := newLinkHarness(t)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'single') RETURNING id`)
	lists := fmt.Sprintf(`"lists": [%d]`, list)
	c := newLangCampaign(t, h, list, models.JSON{"lang": "en"})

	code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "attribs": {"lang": "en", "smart_sending": "no"}}`)
	if code != http.StatusBadRequest || msg != h.app.i18n.T("campaigns.invalidSmartSending") {
		t.Fatalf("smart_sending \"no\": %d %q, want 400 %q", code, msg, h.app.i18n.T("campaigns.invalidSmartSending"))
	}
	if h.app.i18n.T("campaigns.invalidSmartSending") == "campaigns.invalidSmartSending" {
		t.Fatal("campaigns.invalidSmartSending is missing from en.json")
	}
	for _, v := range []string{"false", "true"} {
		if code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "attribs": {"lang": "en", "smart_sending": `+v+`}}`); code != http.StatusOK {
			t.Fatalf("smart_sending %s: %d %s", v, code, msg)
		}
		got, err := h.app.core.GetCampaign(c.ID, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if b, ok := got.Attribs["smart_sending"].(bool); !ok || fmt.Sprint(b) != v {
			t.Fatalf("stored smart_sending %v, want %s", got.Attribs["smart_sending"], v)
		}
	}
}

// Fork (60-day opens) -- integrations listmonk/SIXTY-DAY-OPENS-SPEC T9 (I10, I13): attribs.silent_skip
// is accepted absent / true / false only at the API; the accessor is on only for the boolean true.
func TestValidSilentSkip(t *testing.T) {
	for name, tc := range map[string]struct {
		attribs models.JSON
		ok      bool
	}{
		"nil attribs": {nil, true},
		"absent":      {models.JSON{"lang": "en"}, true},
		"true":        {models.JSON{"silent_skip": true}, true},
		"false":       {models.JSON{"silent_skip": false}, true},
		"string yes":  {models.JSON{"silent_skip": "yes"}, false},
		"string true": {models.JSON{"silent_skip": "true"}, false},
		"number":      {models.JSON{"silent_skip": 1.0}, false},
		"null":        {models.JSON{"silent_skip": nil}, false},
	} {
		if got := models.ValidSilentSkip(tc.attribs); got != tc.ok {
			t.Errorf("%s: %v, want %v", name, got, tc.ok)
		}
	}
	for attribs, want := range map[string]bool{`{}`: false, `{"silent_skip": true}`: true, `{"silent_skip": false}`: false, `{"silent_skip": "yes"}`: false} {
		var c models.Campaign
		if err := json.Unmarshal([]byte(attribs), &c.Attribs); err != nil {
			t.Fatal(err)
		}
		if got := c.SilentSkip(); got != want {
			t.Errorf("%s: SilentSkip() = %v, want %v", attribs, got, want)
		}
	}
}

// Fork (promo series) -- integrations listmonk/PROMO-SERIES-SPEC T8 (I8; the cmd half): through the
// UpdateCampaign handler (validateCampaignFields), a malformed Code, Step or tag list and a Step or
// tags without a Code are a 400 with nothing stored, a valid set is stored verbatim (never rewritten,
// and no silent_skip default from Step); the served top-level fields and their defaults.
func TestPromoFieldsValidationAndServed(t *testing.T) {
	h := newLinkHarness(t)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'single') RETURNING id`)
	lists := fmt.Sprintf(`"lists": [%d]`, list)
	c := newLangCampaign(t, h, list, models.JSON{"lang": "en"})

	served := func() map[string]any {
		t.Helper()
		got, err := h.app.core.GetCampaign(c.ID, "", "")
		if err != nil {
			t.Fatal(err)
		}
		h.app.core.FillAudience(&got)
		b, _ := json.Marshal(got)
		var m map[string]any
		json.Unmarshal(b, &m)
		return m
	}
	// Defaults: Code "", Step null, tags [].
	if m := served(); m["promo_code"] != "" || m["promo_step"] != nil || fmt.Sprint(m["promo_tags"]) != "[]" {
		t.Fatalf("defaults code %#v step %#v tags %#v", m["promo_code"], m["promo_step"], m["promo_tags"])
	}

	for name, tc := range map[string]struct {
		attribs string
		key     string
	}{
		"untrimmed code":   {`"promo_code": " BFCM "`, "campaigns.invalidPromoCode"},
		"31-char code":     {`"promo_code": "1234567890123456789012345678901"`, "campaigns.invalidPromoCode"},
		"string step":      {`"promo_code": "C", "promo_step": "2"`, "campaigns.invalidPromoStep"},
		"step 4":           {`"promo_code": "C", "promo_step": 4`, "campaigns.invalidPromoStep"},
		"empty tags":       {`"promo_code": "C", "promo_tags": []`, "campaigns.invalidPromoTags"},
		"duplicate tag":    {`"promo_code": "C", "promo_tags": ["a-1", "A-1"]`, "campaigns.invalidPromoTags"},
		"short prefix":     {`"promo_code": "C", "promo_tags": ["ab*"]`, "campaigns.invalidPromoTags"},
		"step without code": {`"promo_step": 2`, "campaigns.promoNeedsCode"},
		"tags without code": {`"promo_tags": ["BFCM*"]`, "campaigns.promoNeedsCode"},
	} {
		code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "attribs": {"lang": "en", `+tc.attribs+`}}`)
		if code != http.StatusBadRequest || msg != h.app.i18n.T(tc.key) {
			t.Fatalf("%s: %d %q, want 400 %q", name, code, msg, h.app.i18n.T(tc.key))
		}
		if h.app.i18n.T(tc.key) == tc.key {
			t.Fatalf("%s is missing from en.json", tc.key)
		}
		if got, _ := h.app.core.GetCampaign(c.ID, "", ""); got.Attribs["promo_code"] != nil || got.Attribs["promo_step"] != nil || got.Attribs["promo_tags"] != nil {
			t.Fatalf("%s: a refused value was stored: %v", name, got.Attribs)
		}
	}

	if code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "attribs": {"lang": "en", "promo_code": "BFCM-D3", "promo_step": 2, "promo_tags": ["BFCM-Deal-3", "BFCM*"]}}`); code != http.StatusOK {
		t.Fatalf("valid set: %d %s", code, msg)
	}
	got, _ := h.app.core.GetCampaign(c.ID, "", "")
	if got.Attribs["promo_code"] != "BFCM-D3" || got.Attribs["promo_step"] != 2.0 || fmt.Sprint(got.Attribs["promo_tags"]) != "[BFCM-Deal-3 BFCM*]" {
		t.Fatalf("stored %v", got.Attribs)
	}
	if _, present := got.Attribs["silent_skip"]; present {
		t.Fatalf("the server applied a silent_skip default from Step: %v", got.Attribs)
	}
	if m := served(); m["promo_code"] != "BFCM-D3" || m["promo_step"] != 2.0 || fmt.Sprint(m["promo_tags"]) != "[BFCM-Deal-3 BFCM*]" || m["silent_skip"] != false {
		t.Fatalf("served code %#v step %#v tags %#v silent %#v", m["promo_code"], m["promo_step"], m["promo_tags"], m["silent_skip"])
	}
}

// T9 (DB-backed) -- the API refuses a non-boolean silent_skip with a 400 naming the field and stores
// nothing; true and false are stored; a new campaign is off (served silent_skip false).
func TestSilentSkipValidation(t *testing.T) {
	h := newLinkHarness(t)
	var list int
	h.db.Get(&list, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Acme', 'public', 'single') RETURNING id`)
	lists := fmt.Sprintf(`"lists": [%d]`, list)
	c := newLangCampaign(t, h, list, models.JSON{"lang": "en"})

	fresh, err := h.app.core.GetCampaign(c.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	h.app.core.FillAudience(&fresh)
	if _, present := fresh.Attribs["silent_skip"]; present || fresh.SilentSkipOn {
		t.Fatalf("a new campaign: attribs %v served %v, want off", fresh.Attribs, fresh.SilentSkipOn)
	}

	code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "attribs": {"lang": "en", "silent_skip": "yes"}}`)
	if code != http.StatusBadRequest || msg != h.app.i18n.T("campaigns.invalidSilentSkip") {
		t.Fatalf("silent_skip \"yes\": %d %q, want 400 %q", code, msg, h.app.i18n.T("campaigns.invalidSilentSkip"))
	}
	if h.app.i18n.T("campaigns.invalidSilentSkip") == "campaigns.invalidSilentSkip" {
		t.Fatal("campaigns.invalidSilentSkip is missing from en.json")
	}
	if got, _ := h.app.core.GetCampaign(c.ID, "", ""); got.Attribs["silent_skip"] != nil {
		t.Fatalf("a refused value was stored: %v", got.Attribs)
	}
	for _, v := range []string{"true", "false"} {
		if code, msg := putCampaign(t, h, c.ID, `{`+lists+`, "attribs": {"lang": "en", "silent_skip": `+v+`}}`); code != http.StatusOK {
			t.Fatalf("silent_skip %s: %d %s", v, code, msg)
		}
		got, err := h.app.core.GetCampaign(c.ID, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if b, ok := got.Attribs["silent_skip"].(bool); !ok || fmt.Sprint(b) != v {
			t.Fatalf("stored silent_skip %v, want %s", got.Attribs["silent_skip"], v)
		}
	}
}

// Fork (naming standard) -- integrations listmonk/NAMING-STANDARD-SPEC I12: the Campaigns page's
// lang filter (query-campaigns $12) over the SHARED case table frontend/src/testdata/lang-filter.json
// that naming.test.mjs reads for langMatches -- no-language campaigns are always included, and the
// SQL and the client predicate agree. LISTMONK_TEST_PG opt-in.
func TestCampaignLangFilterAgreesWithLangMatches(t *testing.T) {
	h := newLinkHarness(t)
	b, err := os.ReadFile(filepath.Join("..", "frontend", "src", "testdata", "lang-filter.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Cases []struct {
			Toggle string  `json:"toggle"`
			Lang   *string `json:"lang"`
			Match  bool    `json:"match"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	for i, c := range fx.Cases {
		h.db.MustExec(`DELETE FROM campaigns`)
		attribs := `{}`
		if c.Lang != nil {
			attribs = `{"lang": "` + *c.Lang + `"}`
		}
		var id int
		h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id, attribs)
			VALUES (gen_random_uuid(), 'C', 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1), $1) RETURNING id`, attribs)
		res, _, err := h.app.core.QueryCampaigns("", nil, nil, "", "", true, nil, nil, nil, nil, c.Toggle, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(res) == 1; got != c.Match {
			t.Errorf("case %d (toggle %q, lang %v): SQL match %v, table says %v", i, c.Toggle, c.Lang, got, c.Match)
		}
	}
	// Through the handler: ?lang=fr narrows, an unknown language is a 400 (the toggle never sends one).
	h.app.pg = paginator.New(paginator.Opt{DefaultPerPage: 20, MaxPerPage: 50, NumPageNums: 10,
		PageParam: "page", PerPageParam: "per_page", AllowAll: true})
	h.db.MustExec(`DELETE FROM campaigns`)
	for _, a := range []string{`{"lang": "fr"}`, `{"lang": "en"}`, `{}`} {
		h.db.MustExec(`INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id, attribs)
			VALUES (gen_random_uuid(), 'C', 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1), $1)`, a)
	}
	get := func(q string) (int, int) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/campaigns?"+q, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(auth.UserHTTPCtxKey, perms("x", auth.PermCampaignsGetAll))
		if err := h.app.GetCampaigns(c); err != nil {
			if he, ok := err.(*echo.HTTPError); ok {
				return he.Code, 0
			}
			t.Fatal(err)
		}
		var resp struct {
			Data struct {
				Results []json.RawMessage `json:"results"`
			} `json:"data"`
		}
		json.Unmarshal(rec.Body.Bytes(), &resp)
		return rec.Code, len(resp.Data.Results)
	}
	if code, n := get("lang=fr"); code != http.StatusOK || n != 2 {
		t.Fatalf("?lang=fr: %d, %d rows (want fr + no-language)", code, n)
	}
	if code, n := get(""); code != http.StatusOK || n != 3 {
		t.Fatalf("no lang: %d, %d rows", code, n)
	}
	if code, _ := get("lang=xx"); code != http.StatusBadRequest {
		t.Fatalf("?lang=xx: %d", code)
	}
}

// Fork (naming standard) -- integrations listmonk/NAMING-STANDARD-SPEC I13 (the SQL halves): the
// Campaigns page search (query-campaigns) and the analytics picker (query-analytics-campaigns)
// match the brand of the campaign's lists by slug and by display name (the brands row From's name
// part), beside name and subject. LISTMONK_TEST_PG opt-in.
func TestCampaignSearchMatchesBrand(t *testing.T) {
	h := newLinkHarness(t)
	h.db.MustExec(`INSERT INTO brands (slug, from_email) VALUES ('thirstygirl', 'Thirsty Girl <hello@tg.test>')`)
	var tg, plain int
	h.db.Get(&tg, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'TG', 'public', 'single', '{brand:thirstygirl,"from:Thirsty Girl <hello@tg.test>"}') RETURNING id`)
	h.db.Get(&plain, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Plain', 'public', 'single') RETURNING id`)
	mk := func(name string, list int) int {
		var id int
		h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id, started_at)
			VALUES (gen_random_uuid(), $1, 'Hello', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1), NOW()) RETURNING id`, name)
		h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'x')`, id, list)
		return id
	}
	tgCamp := mk("No-purchase D2", tg)
	mk("Unrelated", plain)

	for _, q := range []string{"thirstygirl", "THIRSTYGIRL", "Thirsty", "Thirsty Girl", "No-purchase"} {
		res, _, err := h.app.core.QueryCampaigns(q, nil, nil, "", "", true, nil, nil, nil, nil, "", 0, 0)
		if err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		if len(res) != 1 || res[0].ID != tgCamp {
			t.Errorf("Campaigns search %q: %d rows", q, len(res))
		}
		pick, err := h.app.core.QueryAnalyticsCampaigns(nil, q, true, nil, false, nil, 50, nil)
		if err != nil {
			t.Fatalf("picker %q: %v", q, err)
		}
		if len(pick) != 1 || pick[0].ID != tgCamp {
			t.Errorf("analytics picker %q: %d rows", q, len(pick))
		}
	}
	// No false positive: a brand word that is neither slug nor display name.
	if res, _, _ := h.app.core.QueryCampaigns("ruze", nil, nil, "", "", true, nil, nil, nil, nil, "", 0, 0); len(res) != 0 {
		t.Errorf("search ruze matched %d", len(res))
	}
}
