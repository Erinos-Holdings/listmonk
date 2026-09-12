package migrations

// Fork (media tags, MEDIA-TAGS-SPEC I2/I3/I4/I5-insert/I6) — DB-backed cases for media.tags,
// run through the REAL goyesql-parsed media queries against a scratch database built from
// schema.sql (V6_2_9 run twice on top via newEvergreenHarness's shared migration ladder).
// Opt-in: set LISTMONK_TEST_PG, e.g. the dev suite's
//
//	LISTMONK_TEST_PG='postgres://listmonk-dev:listmonk-dev@localhost:5432/listmonk-dev?sslmode=disable' go test ./internal/migrations/ -run MediaTags -v

import (
	"reflect"
	"sort"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

const mediaTagsProvider = "filesystem"

func prepMediaQuery(t *testing.T, h *evergreenHarness, name string) *sqlx.Stmt {
	t.Helper()
	q, ok := h.qs[name]
	if !ok {
		t.Fatalf("query %s not found", name)
	}
	st, err := h.db.Preparex(q.Query)
	if err != nil {
		t.Fatalf("prepare %s: %v", name, err)
	}
	return st
}

// insertMedia inserts through the real insert-media statement (I5's DB half).
func insertMedia(t *testing.T, h *evergreenHarness, ins *sqlx.Stmt, filename string, tags []string) int {
	t.Helper()
	var uu string
	if err := h.db.Get(&uu, `SELECT gen_random_uuid()::TEXT`); err != nil {
		t.Fatal(err)
	}
	var id int
	if err := ins.Get(&id, uu,
		filename, "thumb_"+filename, "image/png", mediaTagsProvider, `{"width": 10}`, pq.StringArray(tags)); err != nil {
		t.Fatalf("insert-media %s: %v", filename, err)
	}
	return id
}

// queryMediaNames runs query-media and returns the filenames it selected, sorted.
func queryMediaNames(t *testing.T, st *sqlx.Stmt, search string, tags []string, untagged bool) []string {
	t.Helper()
	pattern := "%" + search + "%"
	// limit 0 is what per_page=all binds; the LIMIT CASE must turn it into "no limit".
	rows, err := st.Queryx(pattern, mediaTagsProvider, 0, 0, pq.StringArray(tags), untagged)
	if err != nil {
		t.Fatalf("query-media: %v", err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		m := map[string]any{}
		if err := rows.MapScan(m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m["filename"].(string))
	}
	sort.Strings(out)
	return out
}

func TestMediaTags(t *testing.T) {
	h := newEvergreenHarness(t)

	var (
		ins      = prepMediaQuery(t, h, "insert-media")
		query    = prepMediaQuery(t, h, "query-media")
		setMeta  = prepMediaQuery(t, h, "update-media-meta")
		setTags  = prepMediaQuery(t, h, "update-media-tags")
		tagCount = prepMediaQuery(t, h, "get-media-tags")
	)

	// Four rows: {a}, {a,b}, {b}, {}.
	idA := insertMedia(t, h, ins, "a.png", []string{"a"})
	insertMedia(t, h, ins, "ab.png", []string{"a", "b"})
	insertMedia(t, h, ins, "b.png", []string{"b"})
	insertMedia(t, h, ins, "none.png", []string{})

	// ---- I5 (DB half): insert-media stores the tag set it is given. ----
	var stored pq.StringArray
	if err := h.db.Get(&stored, `SELECT tags FROM media WHERE id=$1`, idA); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual([]string(stored), []string{"a"}) {
		t.Fatalf("I5 insert stored %v, want [a]", stored)
	}

	// ---- I3: tags defaults to {} and is NOT NULL (V6_2_9 ran twice in the harness). ----
	var defaulted pq.StringArray
	if err := h.db.Get(&defaulted, `INSERT INTO media (uuid, filename, thumb, provider)
		VALUES (gen_random_uuid(), 'default.png', '', 'other') RETURNING tags`); err != nil {
		t.Fatalf("I3 insert without tags: %v", err)
	}
	if defaulted == nil || len(defaulted) != 0 {
		t.Fatalf("I3 default tags = %#v, want {}", defaulted)
	}
	if _, err := h.db.Exec(`INSERT INTO media (uuid, filename, thumb, provider, tags)
		VALUES (gen_random_uuid(), 'null.png', '', 'other', NULL)`); err == nil {
		t.Fatal("I3 tags accepted NULL; want NOT NULL")
	}
	var notNull string
	if err := h.db.Get(&notNull, `SELECT is_nullable FROM information_schema.columns
		WHERE table_name='media' AND column_name='tags'`); err != nil || notNull != "NO" {
		t.Fatalf("I3 is_nullable = %q (%v), want NO", notNull, err)
	}

	// ---- I2: filter semantics. ----
	cases := []struct {
		name     string
		search   string
		tags     []string
		untagged bool
		want     []string
	}{
		{"no filter returns every row", "", nil, false, []string{"a.png", "ab.png", "b.png", "none.png"}},
		{"one tag", "", []string{"a"}, false, []string{"a.png", "ab.png"}},
		{"two tags OR, not AND", "", []string{"a", "b"}, false, []string{"a.png", "ab.png", "b.png"}},
		{"unknown tag", "", []string{"zzz"}, false, []string{}},
		{"untagged alone", "", nil, true, []string{"none.png"}},
		{"untagged ORs in", "", []string{"b"}, true, []string{"ab.png", "b.png", "none.png"}},
		{"search ANDs with no filter", "ab", nil, false, []string{"ab.png"}},
		{"search ANDs with a tag", "b.png", []string{"a"}, false, []string{"ab.png"}},
		{"search ANDs with untagged", "on", nil, true, []string{"none.png"}},
		{"search with untagged, no untagged match", "a", nil, true, []string{}},
		{"search excludes untagged row", "b", nil, true, []string{}},
	}
	for _, c := range cases {
		t.Run("I2 "+c.name, func(t *testing.T) {
			got := queryMediaNames(t, query, c.search, c.tags, c.untagged)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}

	// ---- I2 (pagination): a positive limit still limits; total counts the whole match. ----
	{
		rows, err := query.Queryx("%%", mediaTagsProvider, 0, 1, pq.StringArray{"a", "b"}, false)
		if err != nil {
			t.Fatal(err)
		}
		n, total := 0, int64(0)
		for rows.Next() {
			m := map[string]any{}
			if err := rows.MapScan(m); err != nil {
				t.Fatal(err)
			}
			n++
			total = m["total"].(int64)
		}
		rows.Close()
		if n != 1 || total != 3 {
			t.Fatalf("limit 1: got %d rows total %d, want 1 row total 3", n, total)
		}
	}

	// ---- I6: get-media-tags returns each distinct tag once with its count. ----
	{
		type tc struct {
			Tag   string `db:"tag"`
			Count int    `db:"count"`
		}
		var got []tc
		if err := tagCount.Select(&got, mediaTagsProvider); err != nil {
			t.Fatalf("get-media-tags: %v", err)
		}
		want := []tc{{"a", 2}, {"b", 2}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("I6 got %v want %v", got, want)
		}
	}

	// ---- I4: the reprocess meta merge leaves tags alone; a tag set leaves meta alone. ----
	if _, err := setMeta.Exec(idA, `{"darkmode": {"class": "ok", "version": 3}}`); err != nil {
		t.Fatalf("update-media-meta: %v", err)
	}
	var afterMeta struct {
		Tags pq.StringArray `db:"tags"`
		Meta string         `db:"meta"`
	}
	if err := h.db.Get(&afterMeta, `SELECT tags, meta::TEXT AS meta FROM media WHERE id=$1`, idA); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual([]string(afterMeta.Tags), []string{"a"}) {
		t.Fatalf("I4 tags after meta merge = %v, want [a]", afterMeta.Tags)
	}

	rows, err := setTags.Queryx(idA, pq.StringArray{"curated", "loyalty-rewards"})
	if err != nil {
		t.Fatalf("update-media-tags: %v", err)
	}
	if !rows.Next() {
		t.Fatal("update-media-tags returned no row")
	}
	ret := map[string]any{}
	if err := rows.MapScan(ret); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if ret["filename"] != "a.png" {
		t.Fatalf("update-media-tags RETURNING filename = %v", ret["filename"])
	}

	var afterTags struct {
		Tags pq.StringArray `db:"tags"`
		Meta string         `db:"meta"`
	}
	if err := h.db.Get(&afterTags, `SELECT tags, meta::TEXT AS meta FROM media WHERE id=$1`, idA); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual([]string(afterTags.Tags), []string{"curated", "loyalty-rewards"}) {
		t.Fatalf("I4 tags after update-media-tags = %v", afterTags.Tags)
	}
	if afterTags.Meta != afterMeta.Meta {
		t.Fatalf("I4 meta changed by update-media-tags: %s -> %s", afterMeta.Meta, afterTags.Meta)
	}

	// update-media-tags on an unknown id selects nothing (the core maps that to a 404).
	var missing int
	if err := setTags.Get(&missing, -1, pq.StringArray{}); err == nil {
		t.Fatal("update-media-tags on an unknown id returned a row")
	}
}
