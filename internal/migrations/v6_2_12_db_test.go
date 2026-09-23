package migrations

// Fork (brand health) -- integrations BRAND-HEALTH-SPEC F1 (migration), F2 (brands queries),
// F3 (the query-lists health join) and the query-level half of F4 (all-or-nothing), against a
// real database. Same opt-in harness as evergreen_db_test.go (LISTMONK_TEST_PG).

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
)

func bhDoc(brand, day, status string, def bool) map[string]any {
	return map[string]any{"v": 1, "brand": brand, "domain": brand + ".example", "day": day,
		"status": status, "default": def, "note": nil, "inputs": map[string]any{}}
}

func bhUpsert(t *testing.T, h *evergreenHarness, docs ...map[string]any) error {
	t.Helper()
	b, _ := json.Marshal(docs)
	_, err := h.db.Exec(h.qs["upsert-brand-health"].Query, string(b))
	return err
}

func bhDay(t *testing.T, h *evergreenHarness, ago int) string {
	t.Helper()
	var d string
	if err := h.db.Get(&d, `SELECT (CURRENT_DATE - $1::INT)::TEXT`, ago); err != nil {
		t.Fatal(err)
	}
	return d
}

func bhList(t *testing.T, h *evergreenHarness, name string, tags ...string) int {
	t.Helper()
	var id int
	if err := h.db.Get(&id, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), $1, 'private', 'single', $2) RETURNING id`,
		name, pq.StringArray(tags)); err != nil {
		t.Fatal(err)
	}
	return id
}

func rolePerms(t *testing.T, h *evergreenHarness, id int) []string {
	t.Helper()
	var p pq.StringArray
	if err := h.db.Get(&p, `SELECT permissions FROM roles WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	out := []string(p)
	sort.Strings(out)
	return out
}

func has(perms []string, p string) bool {
	for _, x := range perms {
		if x == p {
			return true
		}
	}
	return false
}

// TestBrandHealthMigration -- F1.
func TestBrandHealthMigration(t *testing.T) {
	h := newEvergreenHarness(t)
	lo := log.New(os.Stderr, "", 0)

	// Roles as a live install has them: Super Admin (id 1), a lists:get_all role, one without,
	// and a list role (its permissions are per-list and never lists:get_all).
	var admin, reader, other, listRole int
	h.db.Get(&admin, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Super Admin', '{lists:get_all,lists:manage_all}') RETURNING id`)
	h.db.Get(&reader, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Reader', '{lists:get_all,campaigns:get_all}') RETURNING id`)
	h.db.Get(&other, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Media only', '{media:get}') RETURNING id`)
	h.db.Get(&listRole, `INSERT INTO roles (type, parent_id, list_id, permissions) VALUES ('list', $1, $2, '{list:get}') RETURNING id`, other, h.listA)
	if admin != 1 {
		t.Fatalf("fixture: Super Admin must be role 1, got %d", admin)
	}

	var snap [][]string
	for i := 0; i < 2; i++ {
		if err := V6_2_12(h.db, nil, nil, lo); err != nil {
			t.Fatalf("V6_2_12 run %d: %v", i+1, err)
		}
		cur := [][]string{rolePerms(t, h, admin), rolePerms(t, h, reader), rolePerms(t, h, other), rolePerms(t, h, listRole)}
		if i == 1 && fmt.Sprint(cur) != fmt.Sprint(snap) {
			t.Fatalf("second run changed roles: %v -> %v", snap, cur)
		}
		snap = cur
	}

	a, r, o, l := snap[0], snap[1], snap[2], snap[3]
	if !has(a, "brands:get") || !has(a, "brands:manage") {
		t.Fatalf("Super Admin = %v, want brands:get and brands:manage", a)
	}
	if !has(r, "brands:get") || has(r, "brands:manage") {
		t.Fatalf("lists:get_all role = %v, want brands:get only", r)
	}
	if has(o, "brands:get") || has(o, "brands:manage") {
		t.Fatalf("role without lists:get_all = %v, must gain nothing", o)
	}
	if has(l, "brands:get") {
		t.Fatalf("list role = %v, must gain nothing", l)
	}
	// Idempotent -- no duplicate grant.
	n := 0
	for _, p := range a {
		if p == "brands:get" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("Super Admin carries brands:get %d times", n)
	}

	// The table's shape and index.
	var cols []string
	h.db.Select(&cols, `SELECT column_name || ' ' || data_type || ' ' || is_nullable FROM information_schema.columns
		WHERE table_name = 'brand_health' ORDER BY ordinal_position`)
	want := []string{"brand text NO", "day date NO", "status text NO", "is_default boolean NO", "doc jsonb NO",
		"computed_at timestamp with time zone NO"}
	if fmt.Sprint(cols) != fmt.Sprint(want) {
		t.Fatalf("brand_health columns = %v, want %v", cols, want)
	}
	var idx int
	h.db.Get(&idx, `SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'brand_health' AND indexname = 'idx_brand_health_brand_day'`)
	if idx != 1 {
		t.Fatal("idx_brand_health_brand_day missing")
	}

	// list_brand_tag -- the brand tag value, or NULL.
	for tags, want := range map[string]string{`{brand:shala,"from:Shala <a@b.example>"}`: "shala", `{foo,bar}`: "<nil>", `{}`: "<nil>"} {
		var got *string
		h.db.Get(&got, `SELECT list_brand_tag($1::VARCHAR(100)[])`, tags)
		s := "<nil>"
		if got != nil {
			s = *got
		}
		if s != want {
			t.Fatalf("list_brand_tag(%s) = %s, want %s", tags, s, want)
		}
	}
}

// TestBrandHealthQueries -- F2 and the query-level all-or-nothing half of F4.
func TestBrandHealthQueries(t *testing.T) {
	h := newEvergreenHarness(t)
	today, d5, d40 := bhDay(t, h, 0), bhDay(t, h, 5), bhDay(t, h, 40)

	if err := bhUpsert(t, h, bhDoc("alpha", today, "ok", false), bhDoc("alpha", d5, "issues", false),
		bhDoc("alpha", d40, "warn", false), bhDoc("curated", today, "unknown", true), bhDoc("curated", d5, "ok", true)); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// A second PUT for the same day replaces the row.
	if err := bhUpsert(t, h, bhDoc("alpha", today, "warn", false)); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM brand_health`)
	if n != 5 {
		t.Fatalf("rows = %d, want 5 (a same-day upsert replaces)", n)
	}
	var st string
	h.db.Get(&st, `SELECT status FROM brand_health WHERE brand = 'alpha' AND day = $1`, today)
	if st != "warn" {
		t.Fatalf("replaced row status = %s, want warn", st)
	}

	// All-or-nothing: a batch whose second row cannot be stored writes nothing.
	bad := bhDoc("beta", today, "ok", false)
	bad["day"] = "not-a-date"
	if err := bhUpsert(t, h, bhDoc("gamma", today, "ok", false), bad); err == nil {
		t.Fatal("a batch with an unstorable row must fail")
	}
	h.db.Get(&n, `SELECT COUNT(*) FROM brand_health WHERE brand IN ('gamma', 'beta')`)
	if n != 0 {
		t.Fatalf("a failed batch wrote %d rows", n)
	}

	// Lists -- tagged alpha, untagged (the default row's), tagged with a brand that has no row.
	la := bhList(t, h, "Alpha news", "brand:alpha", "from:Alpha <hi@alpha.example>")
	lu := bhList(t, h, "Untagged", "misc")
	bhList(t, h, "Orphan", "brand:nobody", "from:Nobody <hi@nobody.example>")

	latest := func(getAll bool, ids []int) map[string]map[string]any {
		var docs []string
		if err := h.db.Select(&docs, h.qs["get-brand-health-latest"].Query, getAll, pq.Array(ids)); err != nil {
			t.Fatalf("latest: %v", err)
		}
		out := map[string]map[string]any{}
		for _, d := range docs {
			m := map[string]any{}
			json.Unmarshal([]byte(d), &m)
			out[m["brand"].(string)] = m
		}
		return out
	}
	listIDs := func(m map[string]any) []int {
		var out []int
		for _, l := range m["lists"].([]any) {
			out = append(out, int(l.(map[string]any)["id"].(float64)))
		}
		sort.Ints(out)
		return out
	}

	got := latest(true, nil)
	if len(got) != 2 {
		t.Fatalf("latest returned %d brands, want exactly one row per brand (2)", len(got))
	}
	if got["alpha"]["day"] != today || got["alpha"]["status"] != "warn" || got["curated"]["day"] != today {
		t.Fatalf("latest must be the newest day per brand: %v", got)
	}
	if ids := listIDs(got["alpha"]); fmt.Sprint(ids) != fmt.Sprint([]int{la}) {
		t.Fatalf("alpha lists = %v, want [%d]", ids, la)
	}
	// The default row carries every untagged list (the harness's list A is untagged too).
	wantDefault := []int{h.listA, lu}
	sort.Ints(wantDefault)
	if ids := listIDs(got["curated"]); fmt.Sprint(ids) != fmt.Sprint(wantDefault) {
		t.Fatalf("default row lists = %v, want %v", ids, wantDefault)
	}
	// lists[] honours the caller's list permission.
	if ids := listIDs(latest(false, []int{lu})["curated"]); fmt.Sprint(ids) != fmt.Sprint([]int{lu}) {
		t.Fatalf("permitted-lists filter: %v", ids)
	}
	if l := latest(false, []int{lu})["alpha"]["lists"].([]any); len(l) != 0 {
		t.Fatalf("alpha lists without permission = %v, want []", l)
	}

	// History honours days, newest first.
	hist := func(days int) []string {
		var docs []string
		if err := h.db.Select(&docs, h.qs["get-brand-health-history"].Query, "alpha", days); err != nil {
			t.Fatalf("history: %v", err)
		}
		var out []string
		for _, d := range docs {
			m := map[string]any{}
			json.Unmarshal([]byte(d), &m)
			out = append(out, m["day"].(string))
		}
		return out
	}
	if g := hist(30); fmt.Sprint(g) != fmt.Sprint([]string{today, d5}) {
		t.Fatalf("history 30d = %v", g)
	}
	if g := hist(1); fmt.Sprint(g) != fmt.Sprint([]string{today}) {
		t.Fatalf("history 1d = %v", g)
	}
	if g := hist(400); len(g) != 3 {
		t.Fatalf("history 400d = %v", g)
	}
}

// TestListsHealthJoin -- F3.
func TestListsHealthJoin(t *testing.T) {
	h := newEvergreenHarness(t)
	today, d5 := bhDay(t, h, 0), bhDay(t, h, 5)

	lx := bhList(t, h, "X news", "brand:x", "from:X <hi@x.example>")
	lu := bhList(t, h, "Untagged", "misc")
	ly := bhList(t, h, "Y news", "brand:y", "from:Y <hi@y.example>")

	x := bhDoc("x", today, "issues", false)
	x["note"] = "a note"
	if err := bhUpsert(t, h, bhDoc("x", d5, "ok", false), x, bhDoc("curated", today, "unknown", true)); err != nil {
		t.Fatal(err)
	}
	h.db.MustExec(`REFRESH MATERIALIZED VIEW mat_list_subscriber_stats`)

	q := strings.ReplaceAll(h.qs["query-lists"].Query, "%order%", "id ASC")
	get := func(id int) map[string]any {
		var l models.List
		if err := h.db.Unsafe().Get(&l, q, id, "", "", "", "", "", pq.StringArray{}, true, pq.Array([]int{}), 0, 1); err != nil {
			t.Fatalf("query-lists %d: %v", id, err)
		}
		b, _ := json.Marshal(l)
		m := map[string]any{}
		json.Unmarshal(b, &m)
		return m
	}

	// A tagged list carries its brand's LATEST row.
	m := get(lx)
	hx, ok := m["health"].(map[string]any)
	if !ok || hx["brand"] != "x" || hx["status"] != "issues" || hx["as_of"] != today || hx["default"] != false || hx["note"] != "a note" {
		t.Fatalf("tagged list health = %v", m["health"])
	}
	if m["health_tag"] != "x" {
		t.Fatalf("tagged list health_tag = %v", m["health_tag"])
	}

	// An untagged list carries the default row.
	m = get(lu)
	hu, ok := m["health"].(map[string]any)
	if !ok || hu["brand"] != "curated" || hu["default"] != true {
		t.Fatalf("untagged list health = %v", m["health"])
	}
	if v, present := m["health_tag"]; !present || v != nil {
		t.Fatalf("untagged list health_tag = %v (present %v), want literal null", v, present)
	}

	// A tagged brand with no row -- health null, health_tag set.
	m = get(ly)
	if v, present := m["health"]; !present || v != nil {
		t.Fatalf("no-row list health = %v (present %v), want literal null", v, present)
	}
	if m["health_tag"] != "y" {
		t.Fatalf("no-row list health_tag = %v", m["health_tag"])
	}

	// An untagged list with no default row -- both literal null.
	h.db.MustExec(`DELETE FROM brand_health WHERE is_default`)
	m = get(lu)
	if v, present := m["health"]; !present || v != nil {
		t.Fatalf("untagged, no default: health = %v (present %v)", v, present)
	}
	if v, present := m["health_tag"]; !present || v != nil {
		t.Fatalf("untagged, no default: health_tag = %v (present %v)", v, present)
	}

	// minimal=true (get-lists) omits both fields.
	var ls []models.List
	if err := h.db.Unsafe().Select(&ls, h.qs["get-lists"].Query, "", "", "id", true, pq.Array([]int{})); err != nil {
		t.Fatalf("get-lists: %v", err)
	}
	for _, l := range ls {
		b, _ := json.Marshal(l)
		if strings.Contains(string(b), `"health"`) || strings.Contains(string(b), `"health_tag"`) {
			t.Fatalf("get-lists row carries health: %s", b)
		}
	}
}
