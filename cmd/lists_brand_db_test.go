package main

// Fork (brand picker) -- integrations BRAND-PICKER-SPEC I1-I4 and I14 through the real router
// (bpFixture, brands_db_test.go): a list's reserved tags are exactly the projection of its brand,
// a typed reserved tag is refused, an absent brand keeps the projection on update, an unknown slug
// writes nothing, and GET /api/lists rows carry brand. LISTMONK_TEST_PG opt-in.

import (
	"encoding/json"
	"net/http"
	"testing"
)

type listResp struct {
	ID   int      `json:"id"`
	Name string   `json:"name"`
	Tags []string `json:"tags"`

	Brand string `json:"brand"`
}

func (f *bpFixture) listsFixture() {
	f.brandRow("acme", "Acme <hello@acme.test>", "https://shop.acme.test")
	f.brandRow("beta", "Beta <hello@beta.test>", "")
}

// apiList reads one list through GET /api/lists/:id and finds it in GET /api/lists (both shapes
// carry brand -- I14).
func (f *bpFixture) apiList(id int) (listResp, listResp) {
	f.h.t.Helper()
	var one listResp
	json.Unmarshal(f.ok("GET list", f.admin.json(http.MethodGet, "/api/lists/"+itoa(id), nil)), &one)
	var page struct {
		Results []listResp `json:"results"`
	}
	json.Unmarshal(f.ok("GET lists", f.admin.json(http.MethodGet, "/api/lists?per_page=all", nil)), &page)
	var minimal []listResp
	json.Unmarshal(f.ok("GET lists minimal", f.admin.json(http.MethodGet, "/api/lists?minimal=true", nil)), &struct {
		Results *[]listResp `json:"results"`
	}{&minimal})
	var inPage, inMin listResp
	for _, r := range page.Results {
		if r.ID == id {
			inPage = r
		}
	}
	for _, r := range minimal {
		if r.ID == id {
			inMin = r
		}
	}
	if inPage.Brand != one.Brand || inMin.Brand != one.Brand {
		f.h.t.Fatalf("list %d brand differs: GET one %q, query %q, minimal %q", id, one.Brand, inPage.Brand, inMin.Brand)
	}
	return one, inPage
}

// I1 + I14 -- create with a slug / with "", update slug -> slug and slug -> "": the reserved tags
// are exactly the projection (site iff the row has one), free tags kept, and brand reads back.
func TestListBrandProjection(t *testing.T) {
	f := newBPFixture(t)
	f.listsFixture()

	var l listResp
	json.Unmarshal(f.ok("create acme", f.admin.json(http.MethodPost, "/api/lists",
		map[string]any{"name": "Acme list", "type": "public", "optin": "single", "tags": []string{"holiday"}, "brand": "acme"})), &l)
	if got := f.tags(l.ID); got != "brand:acme|from:Acme <hello@acme.test>|holiday|site:https://shop.acme.test" {
		t.Fatalf("created with acme: %q", got)
	}
	if one, _ := f.apiList(l.ID); one.Brand != "acme" {
		t.Fatalf("brand read back %q", one.Brand)
	}

	var u listResp
	json.Unmarshal(f.ok("create untagged", f.admin.json(http.MethodPost, "/api/lists",
		map[string]any{"name": "Untagged list", "tags": []string{"internal"}, "brand": ""})), &u)
	if got := f.tags(u.ID); got != "internal" {
		t.Fatalf("created with \"\": %q", got)
	}
	if one, _ := f.apiList(u.ID); one.Brand != "" {
		t.Fatalf("untagged brand read back %q", one.Brand)
	}

	// slug -> slug: the old projection (site included) is replaced, free tags kept.
	f.ok("acme -> beta", f.admin.json(http.MethodPut, "/api/lists/"+itoa(l.ID),
		map[string]any{"name": "Acme list", "tags": []string{"holiday"}, "brand": "beta"}))
	if got := f.tags(l.ID); got != "brand:beta|from:Beta <hello@beta.test>|holiday" {
		t.Fatalf("acme -> beta: %q", got)
	}
	if one, _ := f.apiList(l.ID); one.Brand != "beta" {
		t.Fatalf("brand after acme -> beta %q", one.Brand)
	}

	// slug -> "": no reserved tag remains.
	f.ok("beta -> none", f.admin.json(http.MethodPut, "/api/lists/"+itoa(l.ID),
		map[string]any{"name": "Acme list", "tags": []string{"holiday"}, "brand": ""}))
	if got := f.tags(l.ID); got != "holiday" {
		t.Fatalf("beta -> \"\": %q", got)
	}
	if one, _ := f.apiList(l.ID); one.Brand != "" {
		t.Fatalf("brand after beta -> \"\" %q", one.Brand)
	}

	// "" -> slug.
	f.ok("none -> acme", f.admin.json(http.MethodPut, "/api/lists/"+itoa(u.ID),
		map[string]any{"name": "Untagged list", "tags": []string{"internal"}, "brand": "acme"}))
	if got := f.tags(u.ID); got != "brand:acme|from:Acme <hello@acme.test>|internal|site:https://shop.acme.test" {
		t.Fatalf("\"\" -> acme: %q", got)
	}
}

// I2 -- a write whose tags carry a reserved tag (any of the three prefixes, the whitespace form
// included) is refused 400 lists.reservedTag, with or without brand, on create and update, and
// nothing is written. repermission:<id> is not reserved.
func TestListReservedTagRefused(t *testing.T) {
	f := newBPFixture(t)
	f.listsFixture()
	id := f.sqlList("Existing", "keep", "brand:acme", "from:Acme <hello@acme.test>", "site:https://shop.acme.test")
	before := f.tags(id)
	lists := f.count(`SELECT COUNT(*) FROM lists`)

	for _, tag := range []string{"brand:x", " brand: x", "from:X <hello@acme.test>", "site:https://x.test", "  site:https://x.test "} {
		for _, brand := range []any{nil, "acme", ""} {
			body := map[string]any{"name": "New", "tags": []string{"ok", tag}}
			if brand != nil {
				body["brand"] = brand
			}
			f.wantErr("create "+tag, f.admin.json(http.MethodPost, "/api/lists", body), http.StatusBadRequest, "lists.reservedTag", "tag", trimmed(tag))
			body["name"] = "Existing"
			f.wantErr("update "+tag, f.admin.json(http.MethodPut, "/api/lists/"+itoa(id), body), http.StatusBadRequest, "lists.reservedTag", "tag", trimmed(tag))
		}
	}
	if n := f.count(`SELECT COUNT(*) FROM lists`); n != lists {
		t.Fatalf("a refused create wrote a list: %d -> %d", lists, n)
	}
	if got := f.tags(id); got != before {
		t.Fatalf("a refused update changed the tags: %q -> %q", before, got)
	}

	// The system-set repermission tag is a free tag.
	f.ok("repermission", f.admin.json(http.MethodPut, "/api/lists/"+itoa(id), map[string]any{"name": "Existing", "tags": []string{"repermission:12"}}))
	if got := f.tags(id); got != "brand:acme|from:Acme <hello@acme.test>|repermission:12|site:https://shop.acme.test" {
		t.Fatalf("repermission tag: %q", got)
	}
}

func trimmed(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}

// I3 -- brand absent (missing or null) on update keeps the list's current projection, whatever the
// free tags become; on create it yields no reserved tag.
func TestListBrandAbsentKeeps(t *testing.T) {
	f := newBPFixture(t)
	f.listsFixture()
	var l listResp
	json.Unmarshal(f.ok("create", f.admin.json(http.MethodPost, "/api/lists", map[string]any{"name": "L", "tags": []string{"a"}, "brand": "acme"})), &l)

	f.ok("missing", f.admin.json(http.MethodPut, "/api/lists/"+itoa(l.ID), map[string]any{"name": "L renamed", "tags": []string{"b", "c"}}))
	if got := f.tags(l.ID); got != "b|brand:acme|c|from:Acme <hello@acme.test>|site:https://shop.acme.test" {
		t.Fatalf("brand missing: %q", got)
	}
	f.ok("null", f.admin.json(http.MethodPut, "/api/lists/"+itoa(l.ID), map[string]any{"name": "L renamed", "tags": []string{}, "brand": nil}))
	if got := f.tags(l.ID); got != "brand:acme|from:Acme <hello@acme.test>|site:https://shop.acme.test" {
		t.Fatalf("brand null: %q", got)
	}

	var c listResp
	json.Unmarshal(f.ok("create absent", f.admin.json(http.MethodPost, "/api/lists", map[string]any{"name": "Plain", "tags": []string{"x"}})), &c)
	if got := f.tags(c.ID); got != "x" {
		t.Fatalf("create with brand absent: %q", got)
	}
}

// I4 -- an unknown slug is 400 lists.brandUnknown and nothing is written (create or update).
func TestListBrandUnknown(t *testing.T) {
	f := newBPFixture(t)
	f.listsFixture()
	id := f.sqlList("Existing", "keep", "brand:beta", "from:Beta <hello@beta.test>")
	before, lists := f.tags(id), f.count(`SELECT COUNT(*) FROM lists`)

	// Slugs compare as stored: ACME is not acme.
	for _, slug := range []string{"nope", "ACME"} {
		f.wantErr("create "+slug, f.admin.json(http.MethodPost, "/api/lists", map[string]any{"name": "New", "brand": slug}),
			http.StatusBadRequest, "lists.brandUnknown", "brand", slug)
		f.wantErr("update "+slug, f.admin.json(http.MethodPut, "/api/lists/"+itoa(id), map[string]any{"name": "Renamed", "tags": []string{}, "brand": slug}),
			http.StatusBadRequest, "lists.brandUnknown", "brand", slug)
	}
	if n := f.count(`SELECT COUNT(*) FROM lists`); n != lists {
		t.Fatalf("an unknown slug created a list")
	}
	if got := f.tags(id); got != before || f.count(`SELECT COUNT(*) FROM lists WHERE id = $1 AND name = 'Existing'`, id) != 1 {
		t.Fatalf("an unknown slug changed the list: %q", got)
	}
}
