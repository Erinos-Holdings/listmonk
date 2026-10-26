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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
		// Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC R4) -- the Location panel's two.
		"misc.sql": {"get-dashboard-counts-scoped", "get-dashboard-charts-scoped",
			"get-dashboard-countries", "get-dashboard-countries-scoped",
			// Fork (brand matrix, integrations BRAND-MATRIX-SPEC D3).
			"get-dashboard-brand-matrix",
			// Fork (integrations DASHBOARDS-POLISH3-SPEC D5).
			"get-dashboard-skipped-langs"},
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
			// Fork (dashboard counts, integrations DASHBOARD-COUNTS-SPEC D4) -- the counts query is
			// interpolated at boot and marked "-- raw: true" like get-campaign-stats; that one tag
			// is expected, any other is a colon-bearing comment line.
			tags := map[string]string{}
			for k, v := range got.Tags {
				tags[k] = v
			}
			// Fork (dashboard sends, integrations DASHBOARD-SENDS-SPEC I6) -- the charts query is
			// interpolated at boot the same way (the clicks mode).
			if (n == "get-dashboard-counts-scoped" || n == "get-dashboard-charts-scoped" ||
				n == "get-dashboard-brand-matrix" || n == "get-dashboard-skipped-langs") && tags["raw"] == "true" {
				delete(tags, "raw")
			}
			if len(tags) != 0 {
				t.Errorf("%s: query %q carries extra tags %v (a colon-bearing comment line?)", file, n, got.Tags)
			}
		}
	}
}

// Fork (shop-link attribution, integrations SHOP-LINK-ATTRIBUTION-SPEC I6) -- the shop-link counts
// query is interpolated on boot through fmt.Sprintf (cmd/init.go), so a bare % anywhere in its text
// (a LIKE pattern) would corrupt the prepared SQL. Parse it as the app does, interpolate it with both
// counted expressions, and assert no %! verb error and no leftover placeholder; its only tag is raw.
func TestShopLinkCountsQueryInterpolates(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "queries", "campaigns.sql"))
	if err != nil {
		t.Fatal(err)
	}
	q, err := goyesql.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := q["get-campaign-shop-link-counts"]
	if !ok || !strings.Contains(got.Query, "SELECT") {
		t.Fatal("get-campaign-shop-link-counts missing or empty after parse")
	}
	if len(got.Tags) != 1 || got.Tags["raw"] != "true" {
		t.Errorf("get-campaign-shop-link-counts tags %v, want only raw=true (a colon-bearing comment line?)", got.Tags)
	}
	if n := strings.Count(got.Query, "%"); n != 1 {
		t.Errorf("query text carries %d percent signs, want exactly the one placeholder", n)
	}
	for _, sel := range []string{"*", "DISTINCT subscriber_id"} {
		s := fmt.Sprintf(got.Query, sel)
		if strings.Contains(s, "%!") || strings.Contains(s, "%s") {
			t.Errorf("interpolated with %q: verb error or leftover placeholder\n%s", sel, s)
		}
		if !strings.Contains(s, "COUNT("+sel+")") {
			t.Errorf("interpolated with %q: counted expression not in place\n%s", sel, s)
		}
	}
}

// Fork (dashboard sends, integrations DASHBOARD-SENDS-SPEC I6) -- the charts query is interpolated
// on boot with the clicks mode (TRUE/FALSE, cmd/init.go), so like the shop-link counts its text may
// carry no bare % beyond the placeholder, and its only tag is raw.
func TestDashboardChartsQueryInterpolates(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "queries", "misc.sql"))
	if err != nil {
		t.Fatal(err)
	}
	q, err := goyesql.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := q["get-dashboard-charts"]; ok {
		t.Error("get-dashboard-charts (the mat_dashboard_charts read) is retired and must not be shipped")
	}
	got, ok := q["get-dashboard-charts-scoped"]
	if !ok || !strings.Contains(got.Query, "SELECT") {
		t.Fatal("get-dashboard-charts-scoped missing or empty after parse")
	}
	if len(got.Tags) != 1 || got.Tags["raw"] != "true" {
		t.Errorf("get-dashboard-charts-scoped tags %v, want only raw=true (a colon-bearing comment line?)", got.Tags)
	}
	if n := strings.Count(got.Query, "%"); n != strings.Count(got.Query, "%[1]s") {
		t.Errorf("query text carries %d percent signs beyond the %%[1]s placeholders", n)
	}
	if strings.Contains(got.Query, "mat_dashboard_charts") {
		t.Error("the charts query reads mat_dashboard_charts")
	}
	// Revision 2 (R5) -- the business-day zone is ONE literal in the statement, and it must equal
	// the integrations repo's SES_DAILY_ZONE (lib/ses-health.ts), which buckets the Sends series.
	// No test spans the repos; each side pins its own literal.
	if n := strings.Count(got.Query, "'America/Denver'"); n != 1 {
		t.Errorf("the charts query carries the 'America/Denver' literal %d times, want exactly 1", n)
	}
	if strings.Contains(got.Query, "'UTC'") {
		t.Error("the charts query still buckets in UTC")
	}
	for _, mode := range []string{"TRUE", "FALSE"} {
		s := fmt.Sprintf(got.Query, mode)
		if strings.Contains(s, "%!") || strings.Contains(s, "%[") {
			t.Errorf("interpolated with %q: verb error or leftover placeholder\n%s", mode, s)
		}
	}
}

// Fork (brand matrix, integrations BRAND-MATRIX-SPEC I2/I3) -- the matrix statement is interpolated
// on boot with the charts statement's clicks mode. I3 (fork half): its business-day zone is ONE
// literal and the SAME literal as the charts statement's (the integrations SES_DAILY_ZONE side is a
// cross-repo prose pin). I2 (fork half): after interpolation the block predicate is the verbatim
// output of the integrations lib/bounce-rules.ts classPredicate("hard","block") -- its '5.7.%'
// pattern survives fmt.Sprintf only because the file writes it '5.7.%%'.
func TestDashboardBrandMatrixQueryInterpolates(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "queries", "misc.sql"))
	if err != nil {
		t.Fatal(err)
	}
	q, err := goyesql.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := q["get-dashboard-brand-matrix"]
	if !ok || !strings.Contains(got.Query, "SELECT") {
		t.Fatal("get-dashboard-brand-matrix missing or empty after parse")
	}
	if len(got.Tags) != 1 || got.Tags["raw"] != "true" {
		t.Errorf("get-dashboard-brand-matrix tags %v, want only raw=true (a colon-bearing comment line?)", got.Tags)
	}
	// Every % is a placeholder or the escaped LIKE pattern's %%.
	if n := strings.Count(got.Query, "%"); n != strings.Count(got.Query, "%[1]s")+2*strings.Count(got.Query, "%%") {
		t.Errorf("query text carries %d percent signs beyond the %%[1]s placeholders and %%%% escapes", n)
	}

	// I3 -- the zone literal, once, and equal to the charts statement's.
	zoneRe := regexp.MustCompile(`'([A-Za-z_]+/[A-Za-z_]+)'::TEXT AS tz`)
	charts, ok := q["get-dashboard-charts-scoped"]
	if !ok {
		t.Fatal("get-dashboard-charts-scoped missing")
	}
	mz, cz := zoneRe.FindAllStringSubmatch(got.Query, -1), zoneRe.FindAllStringSubmatch(charts.Query, -1)
	if len(mz) != 1 || len(cz) != 1 || mz[0][1] != cz[0][1] {
		t.Errorf("zone literals: matrix %v, charts %v -- want exactly one each, equal", mz, cz)
	}
	if strings.Contains(got.Query, "'UTC'") {
		t.Error("the matrix query buckets in UTC")
	}

	// DASHBOARDS-POLISH3-SPEC I5 -- the skipped-language statement shares the window: its zone
	// literal is ONE and the SAME as the charts and matrix statements'. It is not interpolated, so
	// it carries no percent sign at all, and its only tag is raw.
	sk, ok := q["get-dashboard-skipped-langs"]
	if !ok || !strings.Contains(sk.Query, "SELECT") {
		t.Fatal("get-dashboard-skipped-langs missing or empty after parse")
	}
	if len(sk.Tags) != 1 || sk.Tags["raw"] != "true" {
		t.Errorf("get-dashboard-skipped-langs tags %v, want only raw=true (a colon-bearing comment line?)", sk.Tags)
	}
	sz := zoneRe.FindAllStringSubmatch(sk.Query, -1)
	if len(sz) != 1 || len(cz) != 1 || len(mz) != 1 || sz[0][1] != cz[0][1] || sz[0][1] != mz[0][1] {
		t.Errorf("zone literals: skipped %v, charts %v, matrix %v -- want exactly one each, equal", sz, cz, mz)
	}
	if strings.Contains(sk.Query, "'UTC'") {
		t.Error("the skipped-language query buckets in UTC")
	}
	if strings.Contains(sk.Query, "%") {
		t.Error("the skipped-language query carries a percent sign")
	}
	// The one segment and language definitions, never a hand-rolled status join (I5).
	for _, fn := range []string{"subscription_segment(", "subscriber_lang("} {
		if !strings.Contains(sk.Query, fn) {
			t.Errorf("the skipped-language query does not use %s", fn)
		}
	}

	// I2 -- the classPredicate("hard","block") output, verbatim, in the prepared SQL.
	const blockPred = `(COALESCE(b.meta->'bounce'->'bouncedRecipients'->0->>'status', '') LIKE '5.7.%') AND NOT (COALESCE(b.meta->'bounce'->>'bounceSubType', '') = 'Suppressed' OR COALESCE(b.meta->'bounce'->>'bounceSubType', '') = 'OnAccountSuppressionList')`
	for _, mode := range []string{"TRUE", "FALSE"} {
		s := fmt.Sprintf(got.Query, mode)
		if strings.Contains(s, "%!") || strings.Contains(s, "%[") || strings.Contains(s, "%%") {
			t.Errorf("interpolated with %q: verb error or leftover placeholder\n%s", mode, s)
		}
		if strings.Count(s, blockPred) != 1 {
			t.Errorf("interpolated with %q: the block predicate is not the classPredicate output exactly once", mode)
		}
	}
}

// Fork (campaign send series, integrations CAMPAIGN-SEND-SERIES-SPEC D5) -- the Campaign Analytics
// statements bucket in Mountain wall-clock: each carries its zone as ONE literal, the SAME literal as
// the Dashboard charts statement's (and so the integrations SES_DAILY_ZONE -- a cross-repo prose
// pin), converts with AT TIME ZONE (the naive-timestamp output contract), never buckets in UTC, and
// survives the boot-time interpolation of the %s table name where it has one.
func TestCampaignAnalyticsZoneAgreement(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	qs := goyesql.Queries{}
	for _, f := range []string{"campaigns.sql", "misc.sql"} {
		b, err := os.ReadFile(filepath.Join(root, "queries", f))
		if err != nil {
			t.Fatal(err)
		}
		q, err := goyesql.ParseBytes(b)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range q {
			qs[k] = v
		}
	}
	zoneRe := regexp.MustCompile(`'([A-Za-z_]+/[A-Za-z_]+)'::TEXT AS tz`)
	charts, ok := qs["get-dashboard-charts-scoped"]
	if !ok {
		t.Fatal("get-dashboard-charts-scoped missing")
	}
	cz := zoneRe.FindAllStringSubmatch(charts.Query, -1)
	if len(cz) != 1 {
		t.Fatalf("charts zone literals %v, want exactly one", cz)
	}
	for name, interpolated := range map[string]bool{
		"get-campaign-analytics-counts":        true,
		"get-campaign-analytics-unique-counts": true,
		"get-campaign-bounce-counts":           false,
		"get-campaign-send-counts":             false,
	} {
		q, ok := qs[name]
		if !ok || !strings.Contains(q.Query, "SELECT") {
			t.Errorf("%s missing or empty after parse", name)
			continue
		}
		// Only the raw tag (or none) -- a colon-bearing comment line would add one.
		for k := range q.Tags {
			if k != "raw" && k != "name" {
				t.Errorf("%s carries a stray tag %q (a colon-bearing comment line?)", name, k)
			}
		}
		z := zoneRe.FindAllStringSubmatch(q.Query, -1)
		if len(z) != 1 || z[0][1] != cz[0][1] {
			t.Errorf("%s zone literals %v, charts %v -- want exactly one each, equal", name, z, cz)
		}
		if strings.Count(q.Query, "'"+cz[0][1]+"'") != 1 {
			t.Errorf("%s names the zone more than once", name)
		}
		if !strings.Contains(q.Query, "AT TIME ZONE (SELECT tz FROM intval)") {
			t.Errorf("%s does not bucket through AT TIME ZONE", name)
		}
		if strings.Contains(q.Query, "'UTC'") {
			t.Errorf("%s buckets in UTC", name)
		}
		s := q.Query
		if interpolated {
			s = fmt.Sprintf(q.Query, "campaign_views")
		} else if strings.Contains(s, "%") {
			t.Errorf("%s carries a percent sign but is not interpolated", name)
		}
		if strings.Contains(s, "%!") || strings.Contains(s, "%s") {
			t.Errorf("%s: verb error or leftover placeholder after interpolation", name)
		}
	}
}
