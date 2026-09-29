package main

// Fork (location stats) -- integrations LOCATION-STATS-SPEC I2, I3, I5 and I6 against a real
// database, through the handlers: the pixel and the click redirect store the CloudFront-Viewer-Country
// code (NULL when absent or invalid) under every privacy switch without changing what they return;
// the per-country aggregation counts in both the unique and the total preparation; type=countries
// sits behind the same permission checks as the other analytics types. Shares newLinkHarness
// (LISTMONK_TEST_PG opt-in; see link_redirect_db_test.go).

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/knadh/goyesql/v2"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

type trackRow struct {
	Sub     sql.NullInt64  `db:"subscriber_id"`
	Country sql.NullString `db:"country"`
}

// locationFixture is one campaign on one list, one subscriber and one static external link (no
// UTM tagging, so the redirect destination is the stored URL verbatim).
type locationFixture struct {
	listID, campID, subID int
	campUUID, subUUID     string
	linkUUID, linkURL     string
}

func newLocationFixture(t *testing.T, h *linkHarness) locationFixture {
	t.Helper()
	var f locationFixture
	f.linkURL = "https://instagram.com/acme?x=1"
	h.db.Get(&f.listID, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Loc', 'public', 'single', '{}') RETURNING id`)
	if err := h.db.QueryRow(`INSERT INTO subscribers (uuid, email, name) VALUES (gen_random_uuid(), 'loc@x', 'L') RETURNING id, uuid`).Scan(&f.subID, &f.subUUID); err != nil {
		t.Fatal(err)
	}
	if err := h.db.QueryRow(`INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id) VALUES (gen_random_uuid(), 'Loc', 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id, uuid`).Scan(&f.campID, &f.campUUID); err != nil {
		t.Fatal(err)
	}
	h.db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'Loc')`, f.campID, f.listID)
	h.db.Get(&f.linkUUID, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), $1) RETURNING uuid`, f.linkURL)
	return f
}

// pixel drives RegisterCampaignView; country "-" sends no header at all.
func (h *linkHarness) pixel(campUUID, subUUID, country string) *httptest.ResponseRecorder {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaign/"+campUUID+"/"+subUUID+"/px.png", nil)
	if country != "-" {
		req.Header.Set("CloudFront-Viewer-Country", country)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("campUUID", "subUUID")
	c.SetParamValues(campUUID, subUUID)
	if err := h.app.RegisterCampaignView(c); err != nil {
		h.t.Fatalf("RegisterCampaignView: %v", err)
	}
	return rec
}

// click drives LinkRedirect; country "-" sends no header at all.
func (h *linkHarness) click(campUUID, subUUID, linkUUID, country string) (int, string) {
	e := echo.New()
	e.Renderer = stubRenderer{}
	req := httptest.NewRequest(http.MethodGet, "/link/"+linkUUID+"/"+campUUID+"/"+subUUID, nil)
	if country != "-" {
		req.Header.Set("CloudFront-Viewer-Country", country)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("linkUUID", "campUUID", "subUUID")
	c.SetParamValues(linkUUID, campUUID, subUUID)
	if err := h.app.LinkRedirect(c); err != nil {
		h.t.Fatalf("LinkRedirect: %v", err)
	}
	return rec.Code, rec.Header().Get("Location")
}

func (h *linkHarness) trackRows(table string, campID int) []trackRow {
	var out []trackRow
	if err := h.db.Select(&out, `SELECT subscriber_id, country FROM `+table+` WHERE campaign_id = $1 ORDER BY id`, campID); err != nil {
		h.t.Fatal(err)
	}
	return out
}

func (r trackRow) String() string {
	sub, c := "NULL", "NULL"
	if r.Sub.Valid {
		sub = fmt.Sprint(r.Sub.Int64)
	}
	if r.Country.Valid {
		c = r.Country.String
	}
	return sub + "/" + c
}

func rowsString(rows []trackRow) string {
	out := ""
	for i, r := range rows {
		if i > 0 {
			out += " "
		}
		out += r.String()
	}
	return out
}

// TestLocationPixel is I2.
func TestLocationPixel(t *testing.T) {
	h := newLinkHarness(t)
	f := newLocationFixture(t, h)
	sub := fmt.Sprint(f.subID)

	assertPixel := func(name string, rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusOK || rec.Header().Get(echo.HeaderContentType) != "image/png" || rec.Body.String() != string(pixelPNG) {
			t.Fatalf("%s: pixel not returned (code %d, type %q)", name, rec.Code, rec.Header().Get(echo.HeaderContentType))
		}
	}

	// Header present (lower-case, padded -- normalized), absent, invalid.
	assertPixel("present", h.pixel(f.campUUID, f.subUUID, " us "))
	assertPixel("absent", h.pixel(f.campUUID, f.subUUID, "-"))
	assertPixel("invalid", h.pixel(f.campUUID, f.subUUID, "USA"))
	if got, want := rowsString(h.trackRows("campaign_views", f.campID)), sub+"/US "+sub+"/NULL "+sub+"/NULL"; got != want {
		t.Fatalf("views after present/absent/invalid\n got  %s\n want %s", got, want)
	}

	// D5: individual tracking OFF -> the row is written with a NULL subscriber, country kept.
	h.app.cfg.Privacy.IndividualTracking = false
	assertPixel("individual off", h.pixel(f.campUUID, f.subUUID, "GB"))
	h.app.cfg.Privacy.IndividualTracking = true
	rows := h.trackRows("campaign_views", f.campID)
	if got := rows[len(rows)-1].String(); got != "NULL/GB" {
		t.Fatalf("individual tracking off: row %s, want NULL/GB", got)
	}

	// Dummy campaign or subscriber UUID (previews, archive) and tracking disabled -> no row.
	n := len(rows)
	assertPixel("dummy campaign", h.pixel(dummyUUID, f.subUUID, "FR"))
	assertPixel("dummy subscriber", h.pixel(f.campUUID, dummyUUID, "FR"))
	h.app.cfg.Privacy.DisableTracking = true
	assertPixel("tracking disabled", h.pixel(f.campUUID, f.subUUID, "FR"))
	h.app.cfg.Privacy.DisableTracking = false
	var total int
	h.db.Get(&total, `SELECT COUNT(*) FROM campaign_views`)
	if total != n {
		t.Fatalf("dummy/disabled hits recorded: %d rows, want %d", total, n)
	}
}

// TestLocationClick is I3.
func TestLocationClick(t *testing.T) {
	h := newLinkHarness(t)
	f := newLocationFixture(t, h)
	sub := fmt.Sprint(f.subID)

	assertRedirect := func(name string, code int, loc string) {
		t.Helper()
		if code != http.StatusTemporaryRedirect || loc != f.linkURL {
			t.Fatalf("%s: code %d location %q, want 307 %q", name, code, loc, f.linkURL)
		}
	}

	// The destination is the same with and without the header.
	code, loc := h.click(f.campUUID, f.subUUID, f.linkUUID, "de")
	assertRedirect("present", code, loc)
	code, loc = h.click(f.campUUID, f.subUUID, f.linkUUID, "-")
	assertRedirect("absent", code, loc)
	code, loc = h.click(f.campUUID, f.subUUID, f.linkUUID, "D3")
	assertRedirect("invalid", code, loc)
	if got, want := rowsString(h.trackRows("link_clicks", f.campID)), sub+"/DE "+sub+"/NULL "+sub+"/NULL"; got != want {
		t.Fatalf("clicks after present/absent/invalid\n got  %s\n want %s", got, want)
	}

	// D5: individual tracking OFF -> NULL subscriber, country kept.
	h.app.cfg.Privacy.IndividualTracking = false
	code, loc = h.click(f.campUUID, f.subUUID, f.linkUUID, "JP")
	assertRedirect("individual off", code, loc)
	h.app.cfg.Privacy.IndividualTracking = true
	rows := h.trackRows("link_clicks", f.campID)
	if got := rows[len(rows)-1].String(); got != "NULL/JP" {
		t.Fatalf("individual tracking off: row %s, want NULL/JP", got)
	}

	// Dummy UUIDs and tracking disabled -> no row, still redirects.
	n := len(rows)
	code, loc = h.click(dummyUUID, f.subUUID, f.linkUUID, "FR")
	assertRedirect("dummy campaign", code, loc)
	code, loc = h.click(f.campUUID, dummyUUID, f.linkUUID, "FR")
	assertRedirect("dummy subscriber", code, loc)
	h.app.cfg.Privacy.DisableTracking = true
	code, loc = h.click(f.campUUID, f.subUUID, f.linkUUID, "FR")
	assertRedirect("tracking disabled", code, loc)
	h.app.cfg.Privacy.DisableTracking = false
	var total int
	h.db.Get(&total, `SELECT COUNT(*) FROM link_clicks`)
	if total != n {
		t.Fatalf("dummy/disabled clicks recorded: %d rows, want %d", total, n)
	}
}

// loadQueryMap is a fresh parse of queries/*.sql: prepareQueries formats some queries in place,
// so each preparation needs its own map.
func loadQueryMap(t *testing.T) goyesql.Queries {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join("..", "queries", "*.sql"))
	qMap := goyesql.Queries{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		mp, err := goyesql.ParseBytes(b)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for k, v := range mp {
			qMap[k] = v
		}
	}
	return qMap
}

func countriesString(rows []models.CampaignAnalyticsCountry) string {
	sort.Slice(rows, func(i, j int) bool { return rows[i].Country < rows[j].Country })
	out := ""
	for _, r := range rows {
		out += fmt.Sprintf("[%s %d %d]", r.Country, r.Views, r.Clicks)
	}
	return out
}

// TestLocationAggregation is I5, in both preparations of the query.
func TestLocationAggregation(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db

	newCamp := func(name string) int {
		var id int
		db.Get(&id, `INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id) VALUES (gen_random_uuid(), $1, 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id`, name)
		return id
	}
	newSub := func(email string) int {
		var id int
		db.Get(&id, `INSERT INTO subscribers (uuid, email, name) VALUES (gen_random_uuid(), $1, 'S') RETURNING id`, email)
		return id
	}
	var link int
	db.Get(&link, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://x.test') RETURNING id`)

	a, b, other := newCamp("A"), newCamp("B"), newCamp("Other")
	s1, s2, s3 := newSub("s1@x"), newSub("s2@x"), newSub("s3@x")

	// sub 0 = NULL subscriber, country "" = NULL, ago = days before now.
	view := func(camp, sub int, country string, times, ago int) {
		for i := 0; i < times; i++ {
			db.MustExec(`INSERT INTO campaign_views (campaign_id, subscriber_id, country, created_at) VALUES ($1, NULLIF($2, 0), NULLIF($3, ''), NOW() - ($4 || ' days')::INTERVAL)`, camp, sub, country, fmt.Sprint(ago))
		}
	}
	click := func(camp, sub int, country string, times int) {
		for i := 0; i < times; i++ {
			db.MustExec(`INSERT INTO link_clicks (campaign_id, subscriber_id, link_id, country) VALUES ($1, NULLIF($2, 0), $3, NULLIF($4, ''))`, camp, sub, link, country)
		}
	}

	// Campaign A views: a repeat viewer, a second viewer, one viewer from two countries, several
	// NULL-subscriber rows (the composite-DISTINCT / DISTINCT ON trap), unknown country with and
	// without a subscriber, and one row outside the date window.
	view(a, s1, "US", 2, 0)
	view(a, s2, "US", 1, 0)
	view(a, s1, "GB", 1, 0)
	view(a, 0, "US", 3, 0)
	view(a, s3, "", 1, 0)
	view(a, 0, "", 2, 0)
	view(a, s1, "JP", 1, 30)
	// Campaign B: the same subscriber again (a distinct (subscriber, campaign) pair).
	view(b, s1, "US", 1, 0)
	// Outside the campaign filter.
	view(other, s1, "DE", 1, 0)
	click(other, s1, "DE", 1)
	// Clicks on A: FR is clicks-only, GB views-only.
	click(a, s1, "FR", 2)
	click(a, s2, "US", 1)

	from := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	ids := []int{a, b}

	// Unique (individual tracking ON -- the harness's preparation).
	got, err := h.app.core.GetCampaignAnalyticsCountries(ids, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if s, want := countriesString(got), "[ 1 0][FR 0 1][GB 1 0][US 3 1]"; s != want {
		t.Fatalf("unique counts\n got  %s\n want %s", s, want)
	}

	// Total (individual tracking OFF): a fresh preparation.
	ko.Set("privacy.individual_tracking", false)
	qTotal := prepareQueries(loadQueryMap(t), db, ko)
	ko.Set("privacy.individual_tracking", true)
	coTotal := core.New(&core.Opt{Queries: qTotal, DB: db, I18n: h.app.i18n, Log: log.New(os.Stderr, "", 0)}, &core.Hooks{})
	got, err = coTotal.GetCampaignAnalyticsCountries(ids, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if s, want := countriesString(got), "[ 3 0][FR 0 2][GB 1 0][US 7 1]"; s != want {
		t.Fatalf("total counts\n got  %s\n want %s", s, want)
	}

	// The date window alone: widened back, the JP row appears (both modes count it once).
	wide := time.Now().UTC().Add(-60 * 24 * time.Hour).Format(time.RFC3339)
	got, err = h.app.core.GetCampaignAnalyticsCountries([]int{a}, wide, to)
	if err != nil {
		t.Fatal(err)
	}
	if s, want := countriesString(got), "[ 1 0][FR 0 1][GB 1 0][JP 1 0][US 2 1]"; s != want {
		t.Fatalf("widened window, campaign A only\n got  %s\n want %s", s, want)
	}
}

// analyticsReq calls GetCampaignViewAnalytics as the given user.
func analyticsReq(t *testing.T, h *linkHarness, u auth.User, typ string, id int) (int, string) {
	t.Helper()
	q := url.Values{}
	q.Set("id", fmt.Sprint(id))
	q.Set("from", time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339))
	q.Set("to", time.Now().UTC().Add(48*time.Hour).Format(time.RFC3339))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/analytics/"+typ+"?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("type")
	c.SetParamValues(typ)
	c.Set(auth.UserHTTPCtxKey, u)
	if err := h.app.GetCampaignViewAnalytics(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, fmt.Sprint(he.Message)
		}
		t.Fatalf("handler: %v", err)
	}
	return rec.Code, rec.Body.String()
}

// TestLocationAnalyticsPermission is I6.
func TestLocationAnalyticsPermission(t *testing.T) {
	h := newLinkHarness(t)
	f := newLocationFixture(t, h)
	h.pixel(f.campUUID, f.subUUID, "US")

	var otherList int
	h.db.Get(&otherList, `INSERT INTO lists (uuid, name, type, optin) VALUES (gen_random_uuid(), 'Other', 'private', 'single') RETURNING id`)

	perms := map[string]struct{}{auth.PermCampaignsGetAnalytics: {}}
	outsider := auth.User{PermissionsMap: perms, GetListIDs: []int{otherList}, ListPermissionsMap: map[int]map[string]struct{}{otherList: {auth.PermListGet: {}}}}
	member := auth.User{PermissionsMap: perms, GetListIDs: []int{f.listID}, ListPermissionsMap: map[int]map[string]struct{}{f.listID: {auth.PermListGet: {}}}}

	// No access to the campaign's lists -> 403, exactly as for views.
	for _, typ := range []string{"views", "countries"} {
		if code, msg := analyticsReq(t, h, outsider, typ, f.campID); code != http.StatusForbidden {
			t.Fatalf("outsider %s: %d %s, want 403", typ, code, msg)
		}
	}

	// Access through the list -> 200 with the row.
	code, body := analyticsReq(t, h, member, "countries", f.campID)
	if code != http.StatusOK {
		t.Fatalf("member countries: %d %s", code, body)
	}
	var resp struct {
		Data []models.CampaignAnalyticsCountry `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("body %s: %v", body, err)
	}
	if s := countriesString(resp.Data); s != "[US 1 0]" {
		t.Fatalf("member countries = %s, want [US 1 0]", s)
	}

	// An unknown type is still 400.
	if code, msg := analyticsReq(t, h, member, "bogus", f.campID); code != http.StatusBadRequest {
		t.Fatalf("unknown type: %d %s, want 400", code, msg)
	}
}
