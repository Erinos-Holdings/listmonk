package main

// Fork (click tracking) -- CLICK-TRACKING-SPEC T3-DB (I4, I5, I6, I9): one redirect through
// LinkRedirect writes exactly one link_clicks row and leaves links.url unchanged; a dynamic
// link resolves for a real subscriber, falls back to the brand site otherwise, and carries UTM
// parameters for a storefront host. Opt-in like the migrations harness: LISTMONK_TEST_PG=<dsn>.
// package main's runtime init is skipped for test binaries (isTestBinary), so the App is
// assembled here from the same constructors the app uses.

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/goyesql/v2"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/internal/migrations"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
)

const linkTestDB = "listmonk_linkredirect_test"

type stubRenderer struct{}

func (stubRenderer) Render(w io.Writer, name string, data any, c echo.Context) error {
	_, err := w.Write([]byte("rendered:" + name))
	return err
}

type linkHarness struct {
	t   *testing.T
	db  *sqlx.DB
	app *App
}

func newLinkHarness(t *testing.T) *linkHarness {
	dsn := os.Getenv("LISTMONK_TEST_PG")
	if dsn == "" {
		t.Skip("LISTMONK_TEST_PG not set")
	}

	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	admin.MustExec("DROP DATABASE IF EXISTS " + linkTestDB)
	admin.MustExec("CREATE DATABASE " + linkTestDB)
	admin.Close()

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + linkTestDB
	db, err := sqlx.Connect("postgres", u.String())
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		if admin, err := sqlx.Connect("postgres", dsn); err == nil {
			admin.Exec("DROP DATABASE IF EXISTS " + linkTestDB)
			admin.Close()
		}
	})

	root := ".."
	schema, err := os.ReadFile(filepath.Join(root, "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	db.MustExec(string(schema))
	if err := migrations.V6_2_6(db, nil, nil, log.New(os.Stderr, "", 0)); err != nil {
		t.Fatal(err)
	}

	files, _ := filepath.Glob(filepath.Join(root, "queries", "*.sql"))
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

	// The app's own globals: individual tracking on, UTM on, the default fallback.
	ko.Set("privacy.individual_tracking", true)
	ko.Set("app.link_fallback_url", "https://curatedfor.you")
	ko.Set("app.utm_enable", true)
	ko.Set("app.utm_hosts", []string{"curatedfor.you"})
	ko.Set("app.utm_params.utm_source", "listmonk")
	ko.Set("app.utm_params.utm_medium", "email")
	ko.Set("app.utm_params.utm_campaign", "{{ .Campaign.Name }}")
	ko.Set("app.utm_params.utm_content", "{{ .Campaign.ID }}")
	// D7 (a): sending-identity domains enter the UTM host union through the SMTP blocks'
	// from_addresses, not through list tags -- configure one so acme.test is a storefront host.
	if err := ko.Load(confmap.Provider(map[string]any{
		"smtp": []any{map[string]any{"enabled": true, "from_addresses": []any{"hello@acme.test"}}},
	}, "."), nil); err != nil {
		t.Fatal(err)
	}
	if m := ko.StringMap("app.utm_params"); len(m) != 4 {
		t.Fatalf("utm_params not loaded into koanf: %v", m)
	}
	if len(configuredFromAddresses()) != 1 {
		t.Fatalf("smtp from_addresses not loaded: %v", configuredFromAddresses())
	}

	// The app prepares queries on an Unsafe() handle (initDB): rows carry columns the
	// models do not (max_subscriber_id on campaigns).
	db = db.Unsafe()
	q := prepareQueries(qMap, db, ko)

	b, err := os.ReadFile(filepath.Join(root, "i18n", "en.json"))
	if err != nil {
		t.Fatal(err)
	}
	i, err := i18n.New(b)
	if err != nil {
		t.Fatal(err)
	}

	lg := log.New(os.Stderr, "", 0)
	co := core.New(&core.Opt{Queries: q, DB: db, I18n: i, Log: lg}, &core.Hooks{})

	cfg := &Config{}
	cfg.Privacy.IndividualTracking = true

	app := &App{
		cfg:           cfg,
		core:          co,
		i18n:          i,
		log:           lg,
		linkFallbacks: newLinkFallbacks(co, i, ko, lg),
	}
	return &linkHarness{t: t, db: db, app: app}
}

func (h *linkHarness) redirect(campUUID, subUUID, linkUUID string) (int, string, string) {
	e := echo.New()
	e.Renderer = stubRenderer{}
	req := httptest.NewRequest(http.MethodGet, "/link/"+campUUID+"/"+subUUID+"/"+linkUUID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("campUUID", "subUUID", "linkUUID")
	c.SetParamValues(campUUID, subUUID, linkUUID)
	if err := h.app.LinkRedirect(c); err != nil {
		h.t.Fatalf("LinkRedirect: %v", err)
	}
	return rec.Code, rec.Header().Get("Location"), rec.Body.String()
}

func (h *linkHarness) clicks(linkUUID string) int {
	var n int
	h.db.Get(&n, `SELECT COUNT(*) FROM link_clicks lc JOIN links l ON l.id = lc.link_id WHERE l.uuid = $1`, linkUUID)
	return n
}

func (h *linkHarness) linkURL(linkUUID string) string {
	var u string
	h.db.Get(&u, `SELECT url FROM links WHERE uuid = $1`, linkUUID)
	return u
}

func TestLinkRedirectDynamic(t *testing.T) {
	h := newLinkHarness(t)
	db := h.db

	// Brand list (acme: from-domain fallback), an unmapped list, subscribers with and
	// without the attribute, one campaign per list, and the links.
	var acmeList, plainList int
	db.Get(&acmeList, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Acme', 'public', 'single', '{brand:acme,"from:Acme <hello@acme.test>"}') RETURNING id`)
	db.Get(&plainList, `INSERT INTO lists (uuid, name, type, optin, tags) VALUES (gen_random_uuid(), 'Plain', 'public', 'single', '{}') RETURNING id`)

	var withUUID, withoutUUID, jsUUID string
	db.Get(&withUUID, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 'with@x', 'W', '{"site":"https://shop.acme.test/creator-1"}') RETURNING uuid`)
	db.Get(&withoutUUID, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 'without@x', 'W', '{}') RETURNING uuid`)
	db.Get(&jsUUID, `INSERT INTO subscribers (uuid, email, name, attribs) VALUES (gen_random_uuid(), 'js@x', 'J', '{"site":"javascript:alert(1)"}') RETURNING uuid`)

	var acmeCamp, plainCamp, acmeCampID int
	var acmeUUID, plainUUID string
	db.QueryRow(`INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id) VALUES (gen_random_uuid(), 'Acme Rewards & More', 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id, uuid`).Scan(&acmeCamp, &acmeUUID)
	db.QueryRow(`INSERT INTO campaigns (uuid, name, subject, from_email, body, messenger, template_id) VALUES (gen_random_uuid(), 'Plain', 's', 'f@x', 'b', 'email', (SELECT id FROM templates LIMIT 1)) RETURNING id, uuid`).Scan(&plainCamp, &plainUUID)
	acmeCampID = acmeCamp
	db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'Acme')`, acmeCamp, acmeList)
	db.MustExec(`INSERT INTO campaign_lists (campaign_id, list_id, list_name) VALUES ($1, $2, 'Plain')`, plainCamp, plainList)

	const expr = `{{ or .Subscriber.Attribs.site "" }}`
	var dynLink, staticLink, extLink string
	db.Get(&dynLink, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), $1) RETURNING uuid`, expr)
	db.Get(&staticLink, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://curatedfor.you/p?x=1') RETURNING uuid`)
	db.Get(&extLink, `INSERT INTO links (uuid, url) VALUES (gen_random_uuid(), 'https://instagram.com/acme') RETURNING uuid`)

	utm := func(name string, id int) string {
		return "utm_source=listmonk&utm_medium=email&utm_campaign=" + url.QueryEscape(name) + "&utm_content=" + url.QueryEscape(itoa(id))
	}

	// I4: dynamic link + real subscriber → the resolved storefront, UTM-tagged (acme.test is
	// in the union via the from: address; shop. is a subdomain). Exactly one click row.
	code, loc, _ := h.redirect(acmeUUID, withUUID, dynLink)
	if code != http.StatusTemporaryRedirect {
		t.Fatalf("code %d", code)
	}
	if want := "https://shop.acme.test/creator-1?" + utm("Acme Rewards & More", acmeCampID); loc != want {
		t.Fatalf("resolved location\n got  %s\n want %s", loc, want)
	}
	if n := h.clicks(dynLink); n != 1 {
		t.Fatalf("link_clicks rows after one redirect = %d", n)
	}
	// I9: the stored URL is the unexpanded expression, untouched.
	if h.linkURL(dynLink) != expr {
		t.Fatalf("links.url mutated: %q", h.linkURL(dynLink))
	}

	// I6: missing attribute → the brand fallback (https://acme.test), UTM applied to it too;
	// the click still counts.
	_, loc, _ = h.redirect(acmeUUID, withoutUUID, dynLink)
	if want := "https://acme.test?" + utm("Acme Rewards & More", acmeCampID); loc != want {
		t.Fatalf("fallback location\n got  %s\n want %s", loc, want)
	}
	if n := h.clicks(dynLink); n != 2 {
		t.Fatalf("click not recorded on the fallback path: %d", n)
	}

	// I6: javascript: → fallback.
	_, loc, _ = h.redirect(acmeUUID, jsUUID, dynLink)
	if !strings.HasPrefix(loc, "https://acme.test?") {
		t.Fatalf("javascript destination not refused: %s", loc)
	}

	// I5: dummy subscriber UUID → fallback, and no click row.
	before := h.clicks(dynLink)
	_, loc, _ = h.redirect(acmeUUID, dummyUUID, dynLink)
	if !strings.HasPrefix(loc, "https://acme.test?") || h.clicks(dynLink) != before {
		t.Fatalf("dummy: loc=%s clicks=%d", loc, h.clicks(dynLink))
	}

	// I5: unknown subscriber UUID (deleted subscriber) → the click records with a NULL
	// subscriber (upstream behaviour) and the redirect falls back; never a crash.
	code, loc, _ = h.redirect(acmeUUID, "11111111-1111-1111-1111-111111111111", dynLink)
	if code != http.StatusTemporaryRedirect || !strings.HasPrefix(loc, "https://acme.test?") {
		t.Fatalf("unknown subscriber: code=%d loc=%q", code, loc)
	}
	var body string

	// I7b/I5: unmapped campaign → app.link_fallback_url; with it blank → the error page.
	_, loc, _ = h.redirect(plainUUID, withoutUUID, dynLink)
	if want := "https://curatedfor.you?" + utm("Plain", plainCamp); loc != want {
		t.Fatalf("unmapped fallback\n got  %s\n want %s", loc, want)
	}
	ko.Set("app.link_fallback_url", "")
	h.app.linkFallbacks.invalidate()
	code, _, body = h.redirect(plainUUID, withoutUUID, dynLink)
	if code != http.StatusNotFound || body != "rendered:"+tplMessage {
		t.Fatalf("blank setting: code=%d body=%q", code, body)
	}
	ko.Set("app.link_fallback_url", "https://curatedfor.you")
	h.app.linkFallbacks.invalidate()

	// I8: static storefront link tagged; external host untouched; existing utm respected.
	_, loc, _ = h.redirect(acmeUUID, withUUID, staticLink)
	if want := "https://curatedfor.you/p?x=1&" + utm("Acme Rewards & More", acmeCampID); loc != want {
		t.Fatalf("static UTM\n got  %s\n want %s", loc, want)
	}
	_, loc, _ = h.redirect(acmeUUID, withUUID, extLink)
	if loc != "https://instagram.com/acme" {
		t.Fatalf("external host tagged: %s", loc)
	}
	ko.Set("app.utm_enable", false)
	_, loc, _ = h.redirect(acmeUUID, withUUID, staticLink)
	if loc != "https://curatedfor.you/p?x=1" {
		t.Fatalf("utm_enable=false still tagged: %s", loc)
	}
	ko.Set("app.utm_enable", true)

	// I7b: a site: tag wins over the from-domain; the union picks the site host up.
	db.MustExec(`UPDATE lists SET tags = '{brand:acme,"from:Acme <hello@acme.test>","site:https://store.example"}' WHERE id = $1`, acmeList)
	h.app.linkFallbacks.invalidate()
	_, loc, _ = h.redirect(acmeUUID, withoutUUID, dynLink)
	if want := "https://store.example?" + utm("Acme Rewards & More", acmeCampID); loc != want {
		t.Fatalf("site: tag fallback\n got  %s\n want %s", loc, want)
	}

	// I5: individual tracking OFF → every dynamic click goes to the fallback (D2 hazard).
	h.app.cfg.Privacy.IndividualTracking = false
	_, loc, _ = h.redirect(acmeUUID, withUUID, dynLink)
	if !strings.HasPrefix(loc, "https://store.example?") {
		t.Fatalf("individual tracking off must fall back: %s", loc)
	}
	h.app.cfg.Privacy.IndividualTracking = true

	// Static link stored URL untouched throughout.
	if h.linkURL(staticLink) != "https://curatedfor.you/p?x=1" {
		t.Fatal("static links.url mutated")
	}
}

func itoa(i int) string { return strconv.Itoa(i) }
