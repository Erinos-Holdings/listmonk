package migrations

// Fork (editor polish, EDITOR-POLISH-SPEC I4a) — DB-backed case for templates.brand, run
// through the REAL goyesql-parsed create-template / update-template statements against a
// scratch database built from schema.sql (V6_2_8 run twice on top, proving idempotency, via
// newEvergreenHarness's shared migration ladder). Opt-in: set LISTMONK_TEST_PG, e.g. the dev
// suite's
//
//	LISTMONK_TEST_PG='postgres://listmonk-dev:listmonk-dev@localhost:5432/listmonk-dev?sslmode=disable' go test ./internal/migrations/ -run TemplateBrand -v

import (
	"database/sql"
	"testing"
)

func TestTemplateBrandPersistence(t *testing.T) {
	h := newEvergreenHarness(t)

	create, ok := h.qs["create-template"]
	if !ok {
		t.Fatal("create-template query not found")
	}
	update, ok := h.qs["update-template"]
	if !ok {
		t.Fatal("update-template query not found")
	}

	// Insert with brand='liyora'.
	var id int
	if err := h.db.Get(&id, create.Query,
		"brand-test", "campaign_visual", "", []byte("{}"), sql.NullString{String: "{}", Valid: true}, "liyora",
	); err != nil {
		t.Fatalf("create-template: %v", err)
	}

	var brand string
	if err := h.db.Get(&brand, `SELECT brand FROM templates WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if brand != "liyora" {
		t.Fatalf("brand after create = %q, want %q", brand, "liyora")
	}

	// Update to '' (clearing the dropdown) must persist as '', not silently keep 'liyora' --
	// brand=$6 is unconditional in the query, unlike name/subject/body/body_source's
	// CASE-WHEN-not-empty guards.
	if _, err := h.db.Exec(update.Query, id, "", "", "", sql.NullString{}, ""); err != nil {
		t.Fatalf("update-template: %v", err)
	}
	if err := h.db.Get(&brand, `SELECT brand FROM templates WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if brand != "" {
		t.Fatalf("brand after clearing update = %q, want empty", brand)
	}
}
