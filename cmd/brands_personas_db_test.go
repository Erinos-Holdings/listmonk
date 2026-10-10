package main

// Fork (persona From) -- integrations PERSONA-FROM-SPEC I5, I6 and I8 against a real database
// through the real router (bpFixture, brands_db_test.go): the personas API's permission, its
// validation, the status-based delete block, and the brand-row update's re-validation.
// LISTMONK_TEST_PG opt-in. Fixture names are acme.test / example.test only.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/models"
)

type pfBrand struct {
	Slug        string   `json:"slug"`
	FromEmail   string   `json:"from_email"`
	DisplayName string   `json:"display_name"`
	Address     string   `json:"address"`
	Personas    []string `json:"personas"`
}

// sqlCampaign inserts a campaign directly with the given From and status.
func (f *bpFixture) sqlCampaign(name, from, status string) int {
	f.h.t.Helper()
	var id int
	if err := f.h.db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, status)
		VALUES (gen_random_uuid(), $1, 's', $2, 'b', 'email', $3::campaign_status) RETURNING id`, name, from, status); err != nil {
		f.h.t.Fatal(err)
	}
	return id
}

func (f *bpFixture) personas(slug string) string {
	f.h.t.Helper()
	var out string
	if err := f.h.db.Get(&out, `SELECT ARRAY_TO_STRING(personas, '|') FROM brands WHERE slug = $1`, slug); err != nil {
		f.h.t.Fatal(err)
	}
	return out
}

// putPersonas is the PUT body for a persona set.
func putPersonas(personas ...string) map[string]any {
	if personas == nil {
		personas = []string{}
	}
	return map[string]any{"personas": personas}
}

// I5 -- PUT /api/brands/:slug/personas.
func TestBrandPersonasPut(t *testing.T) {
	f := newBPFixture(t)
	f.brandRow("acme", pfBrandFrom, "")
	mine := f.sqlList("Acme list", "brand:acme", "from:"+pfBrandFrom)
	put := putPersonas
	url := "/api/brands/acme/personas"

	// 403 without lists:manage_all, even holding every campaign permission.
	camper := f.perListUser("camper", []string{"campaigns:get_all", "campaigns:manage_all", "campaigns:get", "campaigns:manage"},
		[]string{auth.PermListGet, auth.PermListManage}, mine)
	if rec := camper.json(http.MethodPut, url, put("Nat at Acme")); rec.Code != http.StatusForbidden {
		t.Fatalf("campaigns:manage_all-only PUT: %d %s", rec.Code, rec.Body.String())
	}
	if f.personas("acme") != "" {
		t.Fatal("a forbidden PUT wrote")
	}

	// 404 for an unknown slug.
	f.wantErr("unknown slug", f.admin.json(http.MethodPut, "/api/brands/nope/personas", put("Nat at Acme")), http.StatusNotFound, "")

	// 400 for each rule, by key; nothing written.
	eleven := make([]string, 0, 11)
	for i := 0; i < 11; i++ {
		eleven = append(eleven, fmt.Sprintf("P%d at Acme", i))
	}
	long := strings.Repeat("N", 60) + " Acme"
	for _, c := range []struct {
		name     string
		personas []string
		key      string
		args     []string
	}{
		{"empty", []string{"Nat at Acme", "   "}, "brands.personaEmpty", nil},
		{"too long", []string{long}, "brands.personaTooLong", []string{"persona", long, "max", "60"}},
		{"at sign", []string{"Nat @ Acme"}, "brands.personaInvalid", []string{"persona", "Nat @ Acme"}},
		{"non-ASCII", []string{"Natá at Acme"}, "brands.personaInvalid", []string{"persona", "Natá at Acme"}},
		{"no brand name", []string{"Nat"}, "brands.personaNoBrandName", []string{"persona", "Nat", "brand", "Acme"}},
		{"case-only brand name", []string{"Nat at acme"}, "brands.personaNoBrandName", []string{"persona", "Nat at acme", "brand", "Acme"}},
		{"the brand name alone", []string{"Acme"}, "brands.personaIsBrand", []string{"persona", "Acme"}},
		{"too many", eleven, "brands.personasTooMany", []string{"max", "10"}},
		{"duplicate", []string{"Nat at Acme", "nat at Acme"}, "brands.personaDuplicate", []string{"first", "Nat at Acme", "second", "nat at Acme"}},
	} {
		f.wantErr(c.name, f.admin.json(http.MethodPut, url, put(c.personas...)), http.StatusBadRequest, c.key, c.args...)
	}
	// A body without the personas array is not "remove them all".
	f.wantErr("no personas key", f.admin.json(http.MethodPut, url, map[string]any{}), http.StatusBadRequest, "")
	if f.personas("acme") != "" {
		t.Fatalf("refused PUTs wrote %q", f.personas("acme"))
	}

	// 200: trimmed, in request order; the response is the brand row.
	var b pfBrand
	json.Unmarshal(f.ok("PUT set", f.admin.json(http.MethodPut, url, put("  Nat at Acme ", "Jo at Acme", "Al at Acme"))), &b)
	if strings.Join(b.Personas, "|") != "Nat at Acme|Jo at Acme|Al at Acme" || b.Slug != "acme" || b.Address != "hello@acme.test" || b.DisplayName != "Acme" || b.FromEmail != pfBrandFrom {
		t.Fatalf("PUT response %+v", b)
	}
	if got := f.personas("acme"); got != "Nat at Acme|Jo at Acme|Al at Acme" {
		t.Fatalf("stored %q", got)
	}
	// The list's projection is untouched: personas are not projected.
	if got := f.tags(mine); got != "brand:acme|from:"+pfBrandFrom {
		t.Fatalf("list tags changed: %q", got)
	}

	// The delete block: a draft, scheduled, paused or running carrier refuses the removal, naming
	// the persona and the campaign; nothing is written.
	carrier := f.sqlCampaign("Natasha's send", pfNat, models.CampaignStatusDraft)
	for _, status := range []string{models.CampaignStatusDraft, models.CampaignStatusScheduled, models.CampaignStatusPaused, models.CampaignStatusRunning} {
		f.h.db.MustExec(`UPDATE campaigns SET status = $2::campaign_status WHERE id = $1`, carrier, status)
		f.wantErr("removal while "+status, f.admin.json(http.MethodPut, url, put("Jo at Acme", "Al at Acme")), http.StatusConflict,
			"brands.personaInUse", "persona", "Nat at Acme", "campaigns", fmt.Sprintf("%q (%s)", itoa(carrier)+" Natasha's send", status))
		// Emptying the set is refused the same way.
		if rec := f.admin.json(http.MethodPut, url, put()); rec.Code != http.StatusConflict {
			t.Fatalf("empty set while %s: %d", status, rec.Code)
		}
		if got := f.personas("acme"); got != "Nat at Acme|Jo at Acme|Al at Acme" {
			t.Fatalf("a 409 wrote: %q", got)
		}
		// A set that KEEPS the carried persona is no removal: reorder and drop another.
		f.ok("keep while "+status, f.admin.json(http.MethodPut, url, put("Al at Acme", "Nat at Acme", "Jo at Acme")))
		f.ok("restore", f.admin.json(http.MethodPut, url, put("Nat at Acme", "Jo at Acme", "Al at Acme")))
	}
	// Two carriers are both named, by id.
	second := f.sqlCampaign("Second", pfNat, models.CampaignStatusPaused)
	f.wantErr("two carriers", f.admin.json(http.MethodPut, url, put("Jo at Acme")), http.StatusConflict,
		"brands.personaInUse", "persona", "Nat at Acme", "campaigns",
		fmt.Sprintf("%q (running), %q (paused)", itoa(carrier)+" Natasha's send", itoa(second)+" Second"))
	f.h.db.MustExec(`DELETE FROM campaigns WHERE id = $1`, second)

	// Only a finished (or a cancelled) carrier: the removal goes through.
	for _, status := range []string{models.CampaignStatusFinished, models.CampaignStatusCancelled} {
		f.h.db.MustExec(`UPDATE campaigns SET status = $2::campaign_status WHERE id = $1`, carrier, status)
		json.Unmarshal(f.ok("removal with a "+status+" carrier", f.admin.json(http.MethodPut, url, put("Jo at Acme", "Al at Acme"))), &b)
		if got := f.personas("acme"); got != "Jo at Acme|Al at Acme" || strings.Join(b.Personas, "|") != got {
			t.Fatalf("%s carrier: stored %q, response %v", status, got, b.Personas)
		}
		f.ok("re-add", f.admin.json(http.MethodPut, url, put("Nat at Acme", "Jo at Acme", "Al at Acme")))
	}
	// The finished campaign keeps its snapshotted From.
	if n := f.count(`SELECT COUNT(*) FROM campaigns WHERE id = $1 AND from_email = $2`, carrier, pfNat); n != 1 {
		t.Fatal("a persona write changed a campaign's From")
	}

	// The empty set is a valid set.
	json.Unmarshal(f.ok("empty", f.admin.json(http.MethodPut, url, put())), &b)
	if f.personas("acme") != "" || b.Personas == nil || len(b.Personas) != 0 {
		t.Fatalf("empty set: stored %q response %v", f.personas("acme"), b.Personas)
	}
}

// I6 -- GET /api/brands/:slug/personas needs lists:manage_all and lists exactly the blocking
// carriers per persona; GET /api/brands rows carry personas and address.
func TestBrandPersonasGet(t *testing.T) {
	f := newBPFixture(t)
	f.brandRow("acme", pfBrandFrom, "")
	f.brandRow("beta", "hello@beta.test", "")
	mine := f.sqlList("Acme list", "brand:acme", "from:"+pfBrandFrom)
	f.setPersonas("acme", "Nat at Acme", "Jo at Acme", "Al at Acme")

	draft := f.sqlCampaign("Draft", pfNat, models.CampaignStatusDraft)
	running := f.sqlCampaign("Running", pfNat, models.CampaignStatusRunning)
	f.sqlCampaign("Finished", pfNat, models.CampaignStatusFinished)
	f.sqlCampaign("Cancelled", pfNat, models.CampaignStatusCancelled)
	paused := f.sqlCampaign("Paused", pfJo, models.CampaignStatusPaused)
	scheduled := f.sqlCampaign("Scheduled", pfJo, models.CampaignStatusScheduled)
	f.sqlCampaign("Brand From", pfBrandFrom, models.CampaignStatusDraft)
	f.sqlCampaign("Other address", "Nat at Acme <hello@other.test>", models.CampaignStatusDraft)
	f.sqlCampaign("Not canonical", `"Nat at Acme" <hello@acme.test>`, models.CampaignStatusDraft)

	camper := f.perListUser("camper", []string{"campaigns:get_all", "campaigns:manage_all"},
		[]string{auth.PermListGet, auth.PermListManage}, mine)
	if rec := camper.json(http.MethodGet, "/api/brands/acme/personas", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("GET without lists:manage_all: %d %s", rec.Code, rec.Body.String())
	}

	raw := f.ok("GET personas", f.admin.json(http.MethodGet, "/api/brands/acme/personas", nil))
	var uses []models.PersonaUse
	if err := json.Unmarshal(raw, &uses); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range uses {
		var cs []string
		for _, c := range u.Campaigns {
			cs = append(cs, fmt.Sprintf("%d %s %s", c.ID, c.Name, c.Status))
		}
		got = append(got, u.Name+"="+strings.Join(cs, ","))
	}
	want := []string{
		fmt.Sprintf("Nat at Acme=%d Draft draft,%d Running running", draft, running),
		fmt.Sprintf("Jo at Acme=%d Paused paused,%d Scheduled scheduled", paused, scheduled),
		"Al at Acme=",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("GET personas:\n got  %v\n want %v", got, want)
	}
	// No `from` field, and an unused persona's campaigns is [] rather than null.
	if strings.Contains(string(raw), `"from"`) || !strings.Contains(string(raw), `"campaigns":[]`) {
		t.Fatalf("GET personas shape: %s", raw)
	}

	f.wantErr("unknown slug", f.admin.json(http.MethodGet, "/api/brands/nope/personas", nil), http.StatusNotFound, "")
	// A brand with none: an empty array.
	if raw := f.ok("GET beta", f.admin.json(http.MethodGet, "/api/brands/beta/personas", nil)); strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("GET beta personas: %s", raw)
	}

	// GET /api/brands rows carry personas and address -- for every logged-in user.
	for name, cl := range map[string]*twofaClient{"admin": f.admin, "campaign user": camper} {
		raw := f.ok(name+" GET /api/brands", cl.json(http.MethodGet, "/api/brands", nil))
		var rows []pfBrand
		if err := json.Unmarshal(raw, &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0].Slug != "acme" || strings.Join(rows[0].Personas, "|") != "Nat at Acme|Jo at Acme|Al at Acme" ||
			rows[0].Address != "hello@acme.test" || rows[1].Address != "hello@beta.test" || rows[1].Personas == nil || len(rows[1].Personas) != 0 {
			t.Fatalf("%s: rows %+v", name, rows)
		}
		if !strings.Contains(string(raw), `"personas":[]`) {
			t.Fatalf("%s: an empty set is not []: %s", name, raw)
		}
	}
}

// I8 -- PUT /api/brands/:slug re-validates the stored personas against the new From.
func TestBrandUpdateRevalidatesPersonas(t *testing.T) {
	f := newBPFixture(t)
	f.brandRow("acme", pfBrandFrom, "")
	list := f.sqlList("Acme list", "brand:acme", "from:"+pfBrandFrom)
	f.setPersonas("acme", "Jo from the Acme team", "Nat at Acme")
	state := func() string {
		var s string
		f.h.db.Get(&s, `SELECT from_email || '=' || COALESCE(site, '-') || '=' || ARRAY_TO_STRING(personas, '|') FROM brands WHERE slug = 'acme'`)
		return s + "=" + f.tags(list)
	}
	before := state()

	// The new display name is no longer contained in a stored persona: refused, naming the first
	// failing one; the row and its lists are as they were.
	f.wantErr("display name change", f.admin.json(http.MethodPut, "/api/brands/acme", map[string]any{"from_email": "Acme Shop <hello@acme.test>"}),
		http.StatusBadRequest, "brands.personasInvalidForFrom", "persona", "Jo from the Acme team", "from", "Acme Shop <hello@acme.test>")
	f.wantErr("partly contained", f.admin.json(http.MethodPut, "/api/brands/acme", map[string]any{"from_email": "Acme team <hello@acme.test>"}),
		http.StatusBadRequest, "brands.personasInvalidForFrom", "persona", "Nat at Acme", "from", "Acme team <hello@acme.test>")
	f.wantErr("bare From", f.admin.json(http.MethodPut, "/api/brands/acme", map[string]any{"from_email": "hello@acme.test"}),
		http.StatusBadRequest, "brands.personasInvalidForFrom", "persona", "Jo from the Acme team", "from", "hello@acme.test")
	if got := state(); got != before {
		t.Fatalf("a refused update wrote:\n before %s\n after  %s", before, got)
	}

	// A site-only change on a row with personas succeeds and keeps the set.
	var b pfBrand
	json.Unmarshal(f.ok("site only", f.admin.json(http.MethodPut, "/api/brands/acme",
		map[string]any{"from_email": pfBrandFrom, "site": "https://shop.acme.test"})), &b)
	if strings.Join(b.Personas, "|") != "Jo from the Acme team|Nat at Acme" {
		t.Fatalf("site-only change: personas %v", b.Personas)
	}
	if got := state(); got != pfBrandFrom+"=https://shop.acme.test=Jo from the Acme team|Nat at Acme=brand:acme|from:"+pfBrandFrom+"|site:https://shop.acme.test" {
		t.Fatalf("site-only change: %s", got)
	}

	// With the set emptied the From changes freely.
	f.setPersonas("acme")
	f.ok("after emptying", f.admin.json(http.MethodPut, "/api/brands/acme", map[string]any{"from_email": "Acme Shop <hello@acme.test>"}))
}
