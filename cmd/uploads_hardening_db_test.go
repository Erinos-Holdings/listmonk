package main

// Fork (uploads hardening) -- integrations UPLOADS-HARDENING-SPEC K2 and K6 against a real
// database (LISTMONK_TEST_PG opt-in, see link_redirect_db_test.go; these do not run in CI).
// K2 drives UploadMedia with the dark-mode tests' recording fakeStore; K6 goes through the real
// router (twofaHarness, built by initHTTPHandlers), so the route wiring is the production one.

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"gopkg.in/volatiletech/null.v6"
)

// ---- K2: an upload's stored types never come from the client ------------------------------

// uploadAs posts one file to UploadMedia with the part declaring Content-Type: text/html, and
// returns the new media row.
func uploadAs(t *testing.T, a *App, filename string, body []byte) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	hdr.Set("Content-Type", "text/html")
	fw, err := w.CreatePart(hdr)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(body)
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/media", &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	rec := httptest.NewRecorder()
	if err := a.UploadMedia(echo.New().NewContext(req, rec)); err != nil {
		t.Fatalf("UploadMedia(%s): %v", filename, err)
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

func TestUploadIgnoresClientContentType(t *testing.T) {
	h := newLinkHarness(t)
	s := newFakeStore()
	h.app.media = s
	h.app.cfg.MediaUpload.Provider = "s3"
	h.app.cfg.MediaUpload.Extensions = []string{"jpg", "jpeg", "png", "gif", "webp", "woff2"}

	cases := []struct {
		upload string
		body   []byte
		key    string            // the stored filename
		want   map[string]string // every object of the upload and its type
	}{
		{"repaired.png", testFixture(t, "curated_logo3.png"), "repaired.png", map[string]string{
			"repaired.png": "image/png", "thumb_repaired.png": "image/png", "orig_repaired.png": "image/png"}},
		{"photo.jpg", testFixture(t, "shala_hero2.jpg"), "photo.jpg", map[string]string{
			"photo.jpg": "image/jpeg", "thumb_photo.jpg": "image/jpeg"}},
		// A still GIF that repairs is stored as PNG under a .png key; its original keeps the GIF.
		{"still.gif", stillGIF(t), "still.png", map[string]string{
			"still.png": "image/png", "thumb_still.png": "image/png", "orig_still.png": "image/gif"}},
		{"page.webp", []byte(htmlDoc), "page.webp", map[string]string{"page.webp": "image/webp"}},
		{"page.woff2", []byte(htmlDoc), "page.woff2", map[string]string{"page.woff2": "font/woff2"}},
	}

	for _, c := range cases {
		t.Run(c.upload, func(t *testing.T) {
			before := len(s.puts)
			row := uploadAs(t, h.app, c.upload, c.body)

			if row["filename"] != c.key {
				t.Fatalf("stored as %v, want %s", row["filename"], c.key)
			}
			puts := s.puts[before:]
			if len(puts) != len(c.want) {
				t.Fatalf("objects stored: %v, want %v", puts, c.want)
			}
			for k, ct := range c.want {
				if got, ok := s.types[k]; !ok || got != ct {
					t.Errorf("%s stored as %q (put=%v), want %q", k, got, ok, ct)
				}
			}
			if row["content_type"] != s.types[c.key] {
				t.Errorf("row content_type %v, main object %q", row["content_type"], s.types[c.key])
			}

			var dbType string
			h.db.Get(&dbType, `SELECT content_type FROM media WHERE filename = $1`, c.key)
			if dbType != s.types[c.key] {
				t.Errorf("media.content_type %q, main object %q", dbType, s.types[c.key])
			}
		})
	}
}

// ---- K6: every stored body answered as a page is sandboxed --------------------------------

func TestStoredBodiesAreSandboxed(t *testing.T) {
	h := newTwofaHarness(t)
	ensureReviewManager(h.linkHarness)
	h.app.cfg.EnablePublicArchive = true
	h.app.cfg.Permissions = map[string]struct{}{}
	for _, p := range permissionKeys(t) {
		h.app.cfg.Permissions[p] = struct{}{}
	}
	h.setSwitch(false) // rebuild the router: the archive routes register only when enabled

	// A reader: the preview routes are reads (campaigns:get_all, templates:get).
	var role int
	h.db.Get(&role, `INSERT INTO roles (type, name, permissions) VALUES ('user', 'Previewer', '{campaigns:get_all,campaigns:get,templates:get}') RETURNING id`)
	reader := h.user("reader", role)
	if _, err := cacheUsers(h.app.core, h.app.auth); err != nil {
		t.Fatal(err)
	}
	cl := h.sessionClient(reader)

	var listID, tplID int
	h.db.Get(&listID, `INSERT INTO lists (uuid, name, type) VALUES (gen_random_uuid(), 'Sandbox list', 'public') RETURNING id`)
	if err := h.db.Get(&tplID, `INSERT INTO templates (name, type, subject, body, is_default)
		VALUES ('Default', 'campaign', '', '<html><body><h1>Tpl</h1>{{ template "content" . }}</body></html>', true) RETURNING id`); err != nil {
		t.Fatal(err)
	}
	var subUUID string
	h.db.Get(&subUUID, `INSERT INTO subscribers (uuid, email, name, status) VALUES (gen_random_uuid(), 'reader@example.test', 'Reader', 'enabled') RETURNING uuid`)

	newCamp := func(name, ctype, body string) models.Campaign {
		t.Helper()
		c, err := h.app.core.CreateCampaign(models.Campaign{
			Type: models.CampaignTypeRegular, Name: name, Subject: name, FromEmail: "Acme <hello@acme.test>",
			Body: body, ContentType: ctype, Messenger: "email",
			Headers: models.Headers{}, TemplateID: null.IntFrom(tplID), ArchiveTemplateID: null.IntFrom(tplID), ArchiveMeta: json.RawMessage("{}"),
		}, []int{listID}, nil)
		if err != nil {
			t.Fatalf("CreateCampaign: %v", err)
		}
		return c
	}
	const mark = "SANDBOX-MARK-7c1"
	camp := newCamp("Sandboxed", models.CampaignContentTypeHTML, `<p>`+mark+`</p>`)
	plain := newCamp("Plain", models.CampaignContentTypePlain, "plain "+mark)
	h.db.MustExec(`UPDATE campaigns SET archive = true, archive_slug = 'sandboxed', status = 'finished' WHERE id = $1`, camp.ID)

	id := strconv.Itoa(camp.ID)
	form := func(v url.Values) string { return v.Encode() }
	htmlForm := form(url.Values{"content_type": {"html"}, "body": {`<p>posted ` + mark + `</p>`}})

	// The bodies the handlers render, computed without the router, for the exact-body checks.
	previewBody := func() string {
		c, err := h.app.core.GetCampaignForPreview(camp.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		c.UUID = dummySubscriber.UUID
		if err := c.CompileTemplate(h.app.manager.TemplateFuncs(&c)); err != nil {
			t.Fatal(err)
		}
		m, err := h.app.manager.NewCampaignMessage(&c, dummySubscriber)
		if err != nil {
			t.Fatal(err)
		}
		return string(m.Body())
	}
	viewBody := func() string {
		c, err := h.app.core.GetCampaign(0, camp.UUID, "")
		if err != nil {
			t.Fatal(err)
		}
		sub, err := h.app.core.GetSubscriber(0, subUUID, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := c.CompileTemplate(h.app.manager.TemplateFuncs(&c)); err != nil {
			t.Fatal(err)
		}
		m, err := h.app.manager.NewCampaignMessage(&c, sub)
		if err != nil {
			t.Fatal(err)
		}
		return string(m.Body())
	}
	tplBody := func() string {
		tpl, err := h.app.core.GetTemplate(tplID, false)
		if err != nil {
			t.Fatal(err)
		}
		b, err := h.app.previewTemplate(tpl)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	latestBody := func() string {
		camps, _, err := h.app.getCampaignArchives(0, 1, true)
		if err != nil || len(camps) != 1 {
			t.Fatalf("archives: %v %d", err, len(camps))
		}
		return camps[0].Content
	}

	cases := []struct {
		name, method, target, ctype, body string
		policy                            string
		exact                             func() string // nil: the body must carry the marker
	}{
		{"GET campaign preview", http.MethodGet, "/api/campaigns/" + id + "/preview", "", "", previewCSP, previewBody},
		{"POST campaign preview", http.MethodPost, "/api/campaigns/" + id + "/preview", echo.MIMEApplicationForm, htmlForm, previewCSP, nil},
		{"POST campaign text", http.MethodPost, "/api/campaigns/" + id + "/text", echo.MIMEApplicationForm, htmlForm, previewCSP, nil},
		{"POST campaign archive preview", http.MethodPost, "/api/campaigns/" + id + "/preview/archive", echo.MIMEApplicationForm,
			form(url.Values{"template_id": {strconv.Itoa(tplID)}, "archive_meta": {"{}"}}), previewCSP, nil},
		{"GET template preview", http.MethodGet, "/api/templates/" + strconv.Itoa(tplID) + "/preview", "", "", previewCSP, tplBody},
		{"POST template preview", http.MethodPost, "/api/templates/preview", echo.MIMEApplicationForm,
			form(url.Values{"template_type": {"campaign"}, "body": {`<div>` + mark + `</div>{{ template "content" . }}`}}), previewCSP, nil},
		{"view in browser", http.MethodGet, "/campaign/" + camp.UUID + "/" + subUUID, "", "", publicPageCSP, viewBody},
		{"archive page", http.MethodGet, "/archive/" + camp.UUID, "", "", publicPageCSP, nil},
		{"archive latest", http.MethodGet, "/archive/latest", "", "", publicPageCSP, latestBody},
	}

	wantPolicy := map[string]string{
		previewCSP:    "sandbox",
		publicPageCSP: "sandbox allow-popups allow-popups-to-escape-sandbox allow-forms",
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := cl.do(c.method, c.target, c.ctype, c.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get(echo.HeaderContentSecurityPolicy); got != wantPolicy[c.policy] {
				t.Fatalf("Content-Security-Policy = %q, want %q", got, wantPolicy[c.policy])
			}
			if got := rec.Header().Get(echo.HeaderContentType); !strings.HasPrefix(got, echo.MIMETextHTML) {
				t.Fatalf("Content-Type = %q", got)
			}
			body := rec.Body.String()
			if c.exact != nil {
				if want := c.exact(); body != want {
					t.Fatalf("body changed:\n got %q\nwant %q", body, want)
				}
			}
			if c.target != "/api/templates/"+strconv.Itoa(tplID)+"/preview" && !strings.Contains(body, mark) {
				t.Fatalf("the stored body is not in the answer: %q", body)
			}
		})
	}

	// The other branches carry no policy: the plain-text answer and the public error pages.
	t.Run("plain-text preview", func(t *testing.T) {
		rec := cl.do(http.MethodGet, "/api/campaigns/"+strconv.Itoa(plain.ID)+"/preview", "", "")
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get(echo.HeaderContentType), echo.MIMETextPlain) {
			t.Fatalf("status %d, Content-Type %q", rec.Code, rec.Header().Get(echo.HeaderContentType))
		}
		if got := rec.Header().Get(echo.HeaderContentSecurityPolicy); got != "" {
			t.Fatalf("plain-text answer carries Content-Security-Policy %q", got)
		}
	})
	t.Run("view in browser error page", func(t *testing.T) {
		rec := h.client().do(http.MethodGet, "/campaign/"+camp.UUID+"/00000000-0000-4000-8000-000000000000", "", "")
		if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "rendered:"+tplMessage) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get(echo.HeaderContentSecurityPolicy); got != "" {
			t.Fatalf("error page carries Content-Security-Policy %q", got)
		}
	})
}
