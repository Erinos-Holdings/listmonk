package main

// Fork (approved brand logos) -- integrations LOGO-APPROVAL-SPEC against a real database through the
// real router (bpFixture, brands_db_test.go). LISTMONK_TEST_PG opt-in.
//   I1 (the database half) -- a session user holding brands:manage (and brands:get) but not
//       brands:logos is refused 403 and the row's lists do not change.
//   I2 -- the two lists stay disjoint and duplicate-free, in insertion order, under approve/reject/
//       remove sequences (approve then reject, reject then approve, approve twice, remove of an absent
//       url); the 400/404 refusals write nothing; the url is stored trimmed.
//   I3 -- only the update-brand-logos query writes the columns: UpdateBrand (PUT /api/brands/:slug),
//       CreateBrand's conflict path (POST of an existing slug) and PutBrandPersonas leave both lists
//       exactly as they were.
// Fixture names are acme.test / example.test only.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type logoBrand struct {
	Slug          string   `json:"slug"`
	LogosApproved []string `json:"logos_approved"`
	LogosRejected []string `json:"logos_rejected"`
	Personas      []string `json:"personas"`
}

// logos is a brand row's stored lists, each joined with |.
func (f *bpFixture) logos(slug string) (string, string) {
	f.h.t.Helper()
	var row struct {
		A string `db:"a"`
		R string `db:"r"`
	}
	if err := f.h.db.Get(&row, `SELECT ARRAY_TO_STRING(logos_approved, '|') AS a, ARRAY_TO_STRING(logos_rejected, '|') AS r FROM brands WHERE slug = $1`, slug); err != nil {
		f.h.t.Fatal(err)
	}
	return row.A, row.R
}

func logoBody(url, action string) map[string]any { return map[string]any{"url": url, "action": action} }

const (
	logoA = "https://media.example.test/uploads/a.png"
	logoB = "https://media.example.test/uploads/b.png"
	logoC = "https://media.example.test/uploads/c.png"
)

func TestBrandLogosPost(t *testing.T) {
	f := newBPFixture(t)
	f.brandRow("acme", "Acme <hello@acme.test>", "")
	url := "/api/brands/acme/logos"
	post := func(u, action string) logoBrand {
		t.Helper()
		var b logoBrand
		if err := json.Unmarshal(f.ok(action+" "+u, f.admin.json(http.MethodPost, url, logoBody(u, action))), &b); err != nil {
			t.Fatal(err)
		}
		// The response is the whole row and agrees with the store.
		a, r := f.logos("acme")
		if strings.Join(b.LogosApproved, "|") != a || strings.Join(b.LogosRejected, "|") != r || b.Slug != "acme" {
			t.Fatalf("%s %s: response %+v, stored approved=%q rejected=%q", action, u, b, a, r)
		}
		return b
	}
	want := func(what, approved, rejected string) {
		t.Helper()
		if a, r := f.logos("acme"); a != approved || r != rejected {
			t.Fatalf("%s: approved=%q rejected=%q, want %q / %q", what, a, r, approved, rejected)
		}
	}
	// disjoint + no duplicates, checked after every step.
	invariant := func() {
		t.Helper()
		a, r := f.logos("acme")
		seen := map[string]string{}
		for list, s := range map[string]string{"approved": a, "rejected": r} {
			if s == "" {
				continue
			}
			for _, u := range strings.Split(s, "|") {
				if prev, dup := seen[u]; dup {
					t.Fatalf("%q is in %s and %s", u, prev, list)
				}
				seen[u] = list
			}
		}
	}

	// A fresh row: both lists empty, JSON [] (never null).
	var rows []logoBrand
	json.Unmarshal(f.ok("GET brands", f.admin.json(http.MethodGet, "/api/brands", nil)), &rows)
	if len(rows) != 1 || rows[0].LogosApproved == nil || rows[0].LogosRejected == nil || len(rows[0].LogosApproved)+len(rows[0].LogosRejected) != 0 {
		t.Fatalf("GET /api/brands rows %+v", rows)
	}

	// I2: approve then reject.
	post(logoA, "approve")
	want("approve A", logoA, "")
	post(logoA, "reject")
	want("reject A", "", logoA)
	invariant()
	// reject then approve.
	post(logoB, "reject")
	want("reject B", "", logoA+"|"+logoB)
	post(logoB, "approve")
	want("approve B", logoB, logoA)
	invariant()
	// approve twice: no duplicate; insertion order kept.
	post(logoC, "approve")
	post(logoC, "approve")
	want("approve C twice", logoB+"|"+logoC, logoA)
	invariant()
	// remove of an absent url: 200, nothing changes.
	post("https://media.example.test/uploads/never.png", "remove")
	want("remove absent", logoB+"|"+logoC, logoA)
	// remove from each list.
	post(logoA, "remove")
	post(logoB, "remove")
	want("remove A and B", logoC, "")
	invariant()
	// The url is trimmed before it is stored.
	post("  "+logoA+"\n", "reject")
	want("trimmed", logoC, logoA)

	// Refusals write nothing.
	f.wantErr("empty url", f.admin.json(http.MethodPost, url, logoBody("   ", "approve")), http.StatusBadRequest, "brands.logoURLEmpty")
	f.wantErr("relative url", f.admin.json(http.MethodPost, url, logoBody("/uploads/a.png", "approve")), http.StatusBadRequest, "brands.logoURLInvalid")
	f.wantErr("ftp url", f.admin.json(http.MethodPost, url, logoBody("ftp://media.example.test/a.png", "approve")), http.StatusBadRequest, "brands.logoURLInvalid")
	f.wantErr("long url", f.admin.json(http.MethodPost, url, logoBody("https://media.example.test/"+strings.Repeat("a", 2048), "approve")), http.StatusBadRequest, "brands.logoURLTooLong", "max", "2048")
	f.wantErr("unknown action", f.admin.json(http.MethodPost, url, logoBody(logoB, "delete")), http.StatusBadRequest, "brands.logoActionInvalid", "action", "delete")
	f.wantErr("no action", f.admin.json(http.MethodPost, url, map[string]any{"url": logoB}), http.StatusBadRequest, "brands.logoActionInvalid", "action", "")
	f.wantErr("unknown slug", f.admin.json(http.MethodPost, "/api/brands/nope/logos", logoBody(logoB, "approve")), http.StatusNotFound, "")
	f.wantErr("health slug", f.admin.json(http.MethodPost, "/api/brands/health/logos", logoBody(logoB, "approve")), http.StatusNotFound, "")
	want("after refusals", logoC, logoA)

	// I1 (database half): brands:manage is not brands:logos -- 403, the row unchanged.
	manager, _ := f.brandRoleUser("manager", []string{"brands:get", "brands:manage", "brands:decide"})
	for _, action := range []string{"approve", "reject", "remove"} {
		if rec := manager.json(http.MethodPost, url, logoBody(logoC, action)); rec.Code != http.StatusForbidden {
			t.Fatalf("brands:manage-only %s: %d %s", action, rec.Code, rec.Body.String())
		}
	}
	want("after forbidden", logoC, logoA)
	// A holder of brands:logos alone (no brands:manage) may write.
	holder, _ := f.brandRoleUser("holder", []string{"brands:logos"})
	f.ok("holder approve", holder.json(http.MethodPost, url, logoBody(logoB, "approve")))
	want("holder", logoC+"|"+logoB, logoA)
}

// I3 -- the other brands writers never touch the lists.
func TestBrandLogosOtherWriters(t *testing.T) {
	f := newBPFixture(t)
	f.brandRow("acme", "Acme <hello@acme.test>", "")
	f.ok("approve", f.admin.json(http.MethodPost, "/api/brands/acme/logos", logoBody(logoA, "approve")))
	f.ok("approve", f.admin.json(http.MethodPost, "/api/brands/acme/logos", logoBody(logoB, "approve")))
	f.ok("reject", f.admin.json(http.MethodPost, "/api/brands/acme/logos", logoBody(logoC, "reject")))
	const wantA, wantR = logoA + "|" + logoB, logoC
	unchanged := func(what string) {
		t.Helper()
		if a, r := f.logos("acme"); a != wantA || r != wantR {
			t.Fatalf("%s changed the lists: approved=%q rejected=%q", what, a, r)
		}
	}

	// UpdateBrand: PUT /api/brands/:slug (From and site, re-projected).
	f.ok("PUT acme", f.admin.json(http.MethodPut, "/api/brands/acme", map[string]any{"from_email": "Acme Shop <hello@acme.test>", "site": "https://acme.test/s"}))
	unchanged("UpdateBrand")
	// CreateBrand's conflict path: POST of the existing slug is 409 and writes nothing.
	if rec := f.admin.json(http.MethodPost, "/api/brands", map[string]any{"slug": "acme", "from_email": "Acme <hello@acme.test>"}); rec.Code != http.StatusConflict {
		t.Fatalf("POST existing slug: %d %s", rec.Code, rec.Body.String())
	}
	unchanged("CreateBrand (conflict)")
	// PutBrandPersonas.
	var b logoBrand
	json.Unmarshal(f.ok("PUT personas", f.admin.json(http.MethodPut, "/api/brands/acme/personas", map[string]any{"personas": []string{"Nat at Acme Shop"}})), &b)
	unchanged("PutBrandPersonas")
	if strings.Join(b.LogosApproved, "|") != wantA || strings.Join(b.LogosRejected, "|") != wantR {
		t.Fatalf("PUT personas response lists %+v", b)
	}
	// A new brand created beside it starts with empty lists.
	f.ok("POST new", f.admin.json(http.MethodPost, "/api/brands", map[string]any{"slug": "acme2", "from_email": "Acme Two <hello@acme.test>"}))
	if a, r := f.logos("acme2"); a != "" || r != "" {
		t.Fatalf("a new brand's lists: %q / %q", a, r)
	}
	unchanged("CreateBrand (new row)")
}
