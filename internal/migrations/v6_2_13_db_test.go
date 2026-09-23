package migrations

// Fork (system health) -- integrations SES-HEALTH-SPEC F1 (migration) and F2 (queries/system.sql)
// plus the query-level half of F3 (all-or-nothing), against a real database. Same opt-in harness
// as evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func shDoc(kind, day, status string) map[string]any {
	return map[string]any{"v": 1, "kind": kind, "day": day, "status": status, "brands": []any{}}
}

func shUpsert(t *testing.T, h *evergreenHarness, docs ...map[string]any) error {
	t.Helper()
	b, _ := json.Marshal(docs)
	_, err := h.db.Exec(h.qs["upsert-system-health"].Query, string(b))
	return err
}

// TestSystemHealthMigration -- F1.
func TestSystemHealthMigration(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	var admin, reader int
	h.db.Get(&admin, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Super Admin', '{lists:get_all,brands:get,brands:manage}') RETURNING id`)
	h.db.Get(&reader, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Reader', '{lists:get_all,brands:get}') RETURNING id`)
	before := [][]string{rolePerms(t, h, admin), rolePerms(t, h, reader)}

	// Idempotent and additive: twice on a schema that already has the table, and once on a
	// database where it was dropped (an upgrade from v6.2.12).
	for i := 0; i < 2; i++ {
		if err := V6_2_13(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_13 run %d: %v", i+1, err)
		}
	}
	h.db.MustExec(`DROP TABLE system_health`)
	if err := V6_2_13(h.db, nil, nil, lo); err != nil {
		t.Fatalf("V6_2_13 on a v6.2.12 database: %v", err)
	}

	// No role or permission change.
	after := [][]string{rolePerms(t, h, admin), rolePerms(t, h, reader)}
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatalf("V6_2_13 changed roles: %v -> %v", before, after)
	}

	var cols []string
	h.db.Select(&cols, `SELECT column_name || ' ' || data_type || ' ' || is_nullable FROM information_schema.columns
		WHERE table_name = 'system_health' ORDER BY ordinal_position`)
	want := []string{"kind text NO", "day date NO", "status text NO", "doc jsonb NO", "computed_at timestamp with time zone NO"}
	if fmt.Sprint(cols) != fmt.Sprint(want) {
		t.Fatalf("system_health columns = %v, want %v", cols, want)
	}
	var pk []string
	h.db.Select(&pk, `SELECT a.attname FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = 'system_health'::regclass AND i.indisprimary`)
	sort.Strings(pk)
	if fmt.Sprint(pk) != fmt.Sprint([]string{"day", "kind"}) {
		t.Fatalf("system_health primary key = %v, want (kind, day)", pk)
	}
	var idx int
	h.db.Get(&idx, `SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'system_health' AND indexname = 'idx_system_health_kind_day'`)
	if idx != 1 {
		t.Fatal("idx_system_health_kind_day missing")
	}

	// schema.sql mirrors the migration DDL exactly.
	schema, err := os.ReadFile(filepath.Join("..", "..", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), strings.TrimSpace(systemHealthDDL)) {
		t.Fatal("schema.sql does not carry the v6.2.13 system_health DDL verbatim")
	}
}

// TestSystemHealthQueries -- F2 and the query-level all-or-nothing half of F3.
func TestSystemHealthQueries(t *testing.T) {
	h := newEvergreenHarness(t)
	today, d1, d5, d40 := bhDay(t, h, 0), bhDay(t, h, 1), bhDay(t, h, 5), bhDay(t, h, 40)

	if err := shUpsert(t, h, shDoc("ses", today, "ok"), shDoc("ses", d1, "warn"), shDoc("ses", d5, "issues"),
		shDoc("ses", d40, "unknown"), shDoc("snds", today, "ok")); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// A same-day re-PUT replaces the row (status column and doc).
	re := shDoc("ses", today, "issues")
	re["marker"] = "second"
	if err := shUpsert(t, h, re); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM system_health`)
	if n != 5 {
		t.Fatalf("rows = %d, want 5 (a same-day upsert replaces)", n)
	}
	var st, marker string
	h.db.Get(&st, `SELECT status FROM system_health WHERE kind = 'ses' AND day = $1`, today)
	h.db.Get(&marker, `SELECT doc->>'marker' FROM system_health WHERE kind = 'ses' AND day = $1`, today)
	if st != "issues" || marker != "second" {
		t.Fatalf("replaced row status = %s marker = %s, want issues/second", st, marker)
	}

	// All-or-nothing: a batch whose second row cannot be stored writes nothing.
	bad := shDoc("gamma", today, "ok")
	bad["day"] = "not-a-date"
	if err := shUpsert(t, h, shDoc("beta", today, "ok"), bad); err == nil {
		t.Fatal("a batch with an unstorable row must fail")
	}
	h.db.Get(&n, `SELECT COUNT(*) FROM system_health WHERE kind IN ('beta', 'gamma')`)
	if n != 0 {
		t.Fatalf("a failed batch wrote %d rows", n)
	}

	hist := func(kind string, days int) []string {
		var docs []string
		if err := h.db.Select(&docs, h.qs["get-system-health-history"].Query, kind, days); err != nil {
			t.Fatalf("history: %v", err)
		}
		out := []string{}
		for _, d := range docs {
			m := map[string]any{}
			json.Unmarshal([]byte(d), &m)
			if m["kind"] != kind {
				t.Fatalf("history for %s returned a %v row", kind, m["kind"])
			}
			out = append(out, m["day"].(string))
		}
		return out
	}
	// History honours days, newest first, today included.
	if g := hist("ses", 30); fmt.Sprint(g) != fmt.Sprint([]string{today, d1, d5}) {
		t.Fatalf("history 30d = %v", g)
	}
	if g := hist("ses", 1); fmt.Sprint(g) != fmt.Sprint([]string{today}) {
		t.Fatalf("history 1d = %v", g)
	}
	if g := hist("ses", 2); fmt.Sprint(g) != fmt.Sprint([]string{today, d1}) {
		t.Fatalf("history 2d = %v", g)
	}
	if g := hist("ses", 400); len(g) != 4 {
		t.Fatalf("history 400d = %v", g)
	}
	// An unknown kind is an empty list, not an error.
	if g := hist("nothing", 30); len(g) != 0 {
		t.Fatalf("unknown kind = %v, want empty", g)
	}
}
