package migrations

// Regression test for the 2026-08-20 outage: goyesql treats ANY "-- text: value"
// comment line in queries/*.sql as a tag, and a duplicate tag inside one named
// query aborts listmonk at boot ("error parsing SQL queries") — a crash-loop no
// psql-based harness or `go build` can catch, because only the app runs this
// parser. This test runs the real parser over every shipped query file so a
// colon-bearing comment fails CI instead of production. Lives in this package
// only because it is fork-owned and already exists; the test has no migration
// coupling.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knadh/goyesql/v2"
)

func TestShippedQueryFilesParse(t *testing.T) {
	// Walk up from the package dir to the repo root (go test runs in the package dir).
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(root, "queries", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no query files found under %s: %v", root, err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: goyesql parse panicked: %v", filepath.Base(f), r)
				}
			}()
			q := goyesql.MustParseBytes(b)
			if len(q) == 0 {
				t.Errorf("%s: parsed zero queries", filepath.Base(f))
			}
		}()
	}
}

// Fork (brand analytics, integrations BRAND-ANALYTICS-SPEC I9) -- the picker and scoped-Dashboard
// queries are present under their names after the real parse (a stray "-- key: value" comment
// would either abort the parse above or split the query here).
func TestBrandAnalyticsQueriesParse(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for file, names := range map[string][]string{
		"campaigns.sql": {"query-analytics-campaigns"},
		"misc.sql":      {"get-dashboard-counts-scoped", "get-dashboard-charts-scoped"},
	} {
		b, err := os.ReadFile(filepath.Join(root, "queries", file))
		if err != nil {
			t.Fatal(err)
		}
		q, err := goyesql.ParseBytes(b)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, n := range names {
			got, ok := q[n]
			if !ok || !strings.Contains(got.Query, "SELECT") {
				t.Errorf("%s: query %q missing or empty after parse", file, n)
				continue
			}
			if len(got.Tags) != 0 {
				t.Errorf("%s: query %q carries extra tags %v (a colon-bearing comment line?)", file, n, got.Tags)
			}
		}
	}
}
