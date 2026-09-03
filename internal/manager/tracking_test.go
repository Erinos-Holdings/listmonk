package manager

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/knadh/listmonk/models"
)

// Fork (visual tracking) -- render-level pins for TRACKING-SPEC §4. A real Manager
// (the warnings/preheader harness pattern, no DB): TemplateFuncs' TrackView/TrackLink
// against a fake link store, campaigns compiled by CompileTemplate and rendered by
// CampaignMessage.render().

// fakeLinkStore records CreateLink registrations.
type fakeLinkStore struct {
	Store
	created []string
}

func (f *fakeLinkStore) CreateLink(url string) (string, error) {
	f.created = append(f.created, url)
	return fmt.Sprintf("lnk-%d", len(f.created)), nil
}

func newTrackingManager(fs *fakeLinkStore) *Manager {
	m := &Manager{
		cfg: Config{
			IndividualTracking: true,
			ViewTrackURL:       "https://lm.test/campaign/%s/%s/px.png",
			LinkTrackURL:       "https://lm.test/link/%s/%s/%s",
		},
		store: fs,
		log:   log.New(os.Stderr, "", 0),
		links: map[string]string{},
	}
	m.tplFuncs = m.makeGnericFuncMap()
	return m
}

func visualCampaign(body string) *models.Campaign {
	c := &models.Campaign{
		Subject:     "s",
		Body:        body,
		ContentType: models.CampaignContentTypeVisual,
		Messenger:   "email",
	}
	c.UUID = "camp-uuid"
	return c
}

func renderCampaign(t *testing.T, m *Manager, c *models.Campaign, sub models.Subscriber) string {
	t.Helper()
	if err := c.CompileTemplate(m.TemplateFuncs(c)); err != nil {
		t.Fatalf("CompileTemplate: %v", err)
	}
	msg := CampaignMessage{Campaign: c, Subscriber: sub}
	if err := msg.render(); err != nil {
		t.Fatalf("render: %v", err)
	}
	return string(msg.body)
}

var testSub = models.Subscriber{UUID: "sub-uuid", Email: "a@b.c"}

// §4.1 -- the pixel renders exactly once, the live-tag guard suppresses a second one,
// and the bare string "TrackView" in prose or a URL does not trip the guard.
func TestVisualPixelExactlyOnce(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"plain visual document", `<html><body><p>hello</p></body></html>`},
		{"body already carrying a live tag", `<html><body>{{ TrackView }}</body></html>`},
		{"live tag without spaces", `<html><body>{{TrackView}}</body></html>`},
		{"TrackView in prose", `<html><body>our TrackView feature</body></html>`},
		{"TrackView in a URL", `<html><body><a href="https://x.test/TrackView/docs">d</a></body></html>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTrackingManager(&fakeLinkStore{})
			out := renderCampaign(t, m, visualCampaign(c.body), testSub)
			if n := strings.Count(out, "/px.png"); n != 1 {
				t.Fatalf("expected exactly one pixel, got %d in:\n%s", n, out)
			}
			if !strings.Contains(out, "https://lm.test/campaign/camp-uuid/sub-uuid/px.png") {
				t.Fatalf("pixel URL missing campaign/subscriber UUIDs:\n%s", out)
			}
		})
	}
}

// IMAGE-WIDTH-SPEC I8 -- the pixel declares its size (width="1" height="1") so the
// render-warning size lint stays quiet on every visual campaign and Outlook draws it
// at 1×1 instead of the file's stored size.
func TestVisualPixelDeclaresSize(t *testing.T) {
	m := newTrackingManager(&fakeLinkStore{})
	out := renderCampaign(t, m, visualCampaign(`<html><body>x</body></html>`), testSub)
	want := `<img src="https://lm.test/campaign/camp-uuid/sub-uuid/px.png" alt="" width="1" height="1" />`
	if !strings.Contains(out, want) {
		t.Fatalf("pixel missing width=\"1\" height=\"1\":\n%s", out)
	}
	if w := RenderWarnings([]byte(out)); len(w) != 0 {
		t.Fatalf("rendered visual body with only the pixel must produce no warnings, got %v", w)
	}
}

// §3a -- tail placement: the base pixel lands after the document's closing </html>.
func TestVisualPixelAfterHTMLClose(t *testing.T) {
	m := newTrackingManager(&fakeLinkStore{})
	out := renderCampaign(t, m, visualCampaign(`<html><body>x</body></html>`), testSub)
	px := strings.Index(out, "/px.png")
	closeTag := strings.Index(out, "</html>")
	if px == -1 || closeTag == -1 || px < closeTag {
		t.Fatalf("pixel not after </html>:\n%s", out)
	}
}

// §4.2 -- plain hrefs become /link/ URLs; shorthand and template-expr hrefs keep
// their existing behaviour; a backslash URL stays plain; a matched URL containing
// &amp; lands in the links row decoded.
func TestVisualHrefRewrite(t *testing.T) {
	fs := &fakeLinkStore{}
	m := newTrackingManager(fs)
	body := `<html><body>` +
		`<a href="https://ex.test/a">A</a>` +
		`<a href="https://ex.test/b?x=1&amp;y=2">B</a>` +
		`<a href="https://ex.test/c@TrackLink">C</a>` +
		`<a href="{{ UnsubscribeURL }}">U</a>` +
		`<a href="https://ex.test/d\evil">D</a>` +
		"<a href=\"https://ex.test/e\nf\">E</a>" +
		`</body></html>`
	out := renderCampaign(t, m, visualCampaign(body), testSub)

	// The two rewritten links plus the shorthand one — three tracked URLs.
	if n := strings.Count(out, "https://lm.test/link/"); n != 3 {
		t.Fatalf("expected 3 tracked links, got %d in:\n%s", n, out)
	}
	for _, raw := range []string{`href="https://ex.test/a"`, `href="https://ex.test/b`} {
		if strings.Contains(out, raw) {
			t.Fatalf("raw href %s survived the rewrite:\n%s", raw, out)
		}
	}
	// Tracked URLs carry the real campaign and subscriber UUIDs.
	if !strings.Contains(out, "/camp-uuid/sub-uuid") {
		t.Fatalf("tracked link missing campaign/subscriber UUIDs:\n%s", out)
	}
	// The backslash URL is left plain and verbatim.
	if !strings.Contains(out, `href="https://ex.test/d\evil"`) {
		t.Fatalf("backslash URL not left plain:\n%s", out)
	}
	// The LF URL is left plain and the campaign still compiles and renders
	// (review finding 1: pre-fix this body was a compile error — a dead campaign).
	if !strings.Contains(out, "href=\"https://ex.test/e\nf\"") {
		t.Fatalf("LF URL not left plain:\n%s", out)
	}
	// links rows: &amp; decoded (finding 5d); the shorthand URL registered without
	// its @TrackLink suffix; the backslash URL never registered.
	got := strings.Join(fs.created, "\n")
	for _, want := range []string{"https://ex.test/a", "https://ex.test/b?x=1&y=2", "https://ex.test/c"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected links row %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "amp;") || strings.Contains(got, `\evil`) {
		t.Fatalf("bad links row registered:\n%s", got)
	}
}

// §4.3 -- non-visual campaigns render byte-identical to upstream: no pixel, no
// rewrite, exact output.
func TestNonVisualByteIdentical(t *testing.T) {
	m := newTrackingManager(&fakeLinkStore{})
	const anchor = `<p><a href="https://a.test/x?y=1&amp;z=2">x</a></p>`

	cases := []struct {
		name        string
		contentType string
		tplBody     string
		want        string
	}{
		{
			"richtext with template",
			models.CampaignContentTypeRichtext,
			`<html><body>{{ template "content" . }}</body></html>`,
			`<html><body>` + anchor + `</body></html>`,
		},
		{
			"richtext without template",
			models.CampaignContentTypeRichtext,
			"",
			anchor,
		},
		{
			"html with template",
			models.CampaignContentTypeHTML,
			`<html><body>{{ template "content" . }}</body></html>`,
			`<html><body>` + anchor + `</body></html>`,
		},
		{
			"html without template",
			models.CampaignContentTypeHTML,
			"",
			anchor,
		},
		{
			"plain with template",
			models.CampaignContentTypePlain,
			`<html><body>{{ template "content" . }}</body></html>`,
			`<html><body>` + anchor + `</body></html>`,
		},
		{
			"plain without template",
			models.CampaignContentTypePlain,
			"",
			anchor,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			camp := &models.Campaign{
				Subject:      "s",
				Body:         anchor,
				ContentType:  c.contentType,
				Messenger:    "email",
				TemplateBody: c.tplBody,
			}
			camp.UUID = "camp-uuid"
			out := renderCampaign(t, m, camp, testSub)
			if out != c.want {
				t.Fatalf("non-visual output changed:\n got  %s\n want %s", out, c.want)
			}
		})
	}

	// Markdown converts, so pin the absence of tracking artifacts and the verbatim href.
	for _, tplBody := range []string{`<html><body>{{ template "content" . }}</body></html>`, ""} {
		camp := &models.Campaign{
			Subject: "s", Body: "**b** [l](https://a.test/m)",
			ContentType: models.CampaignContentTypeMarkdown, Messenger: "email",
			TemplateBody: tplBody,
		}
		camp.UUID = "camp-uuid"
		out := renderCampaign(t, m, camp, testSub)
		if strings.Contains(out, "px.png") || strings.Contains(out, "/link/") {
			t.Fatalf("markdown campaign gained tracking artifacts (tpl=%q):\n%s", tplBody, out)
		}
		if !strings.Contains(out, `href="https://a.test/m"`) {
			t.Fatalf("markdown href not verbatim (tpl=%q):\n%s", tplBody, out)
		}
	}
}

// §4.4 -- a preview-style render (dummy campaign UUID) produces dummy-UUID pixel and
// link URLs, which the registration endpoints already exclude.
func TestPreviewDummyUUIDs(t *testing.T) {
	m := newTrackingManager(&fakeLinkStore{})
	camp := visualCampaign(`<html><body><a href="https://ex.test/a">A</a></body></html>`)
	camp.UUID = dummyUUID
	out := renderCampaign(t, m, camp, testSub)
	if !strings.Contains(out, "https://lm.test/campaign/"+dummyUUID+"/sub-uuid/px.png") {
		t.Fatalf("pixel URL missing dummy campaign UUID:\n%s", out)
	}
	if !strings.Contains(out, "/"+dummyUUID+"/sub-uuid") || !strings.Contains(out, "https://lm.test/link/") {
		t.Fatalf("link URL missing dummy campaign UUID:\n%s", out)
	}
}

// §4.5 -- a Safe-encoded Outlook body (real dev-campaign-22 markup) compiles and
// renders with the mso copy untracked and untouched; the sibling plain anchor is
// tracked. Byte-preservation of the payload itself is pinned in models.
func TestSafePayloadRender(t *testing.T) {
	const safeVML = `{{ Safe "\x3c!--[if\x20mso]\x3e\x3cv:roundrect\x20xmlns:v=\"urn:schemas-microsoft-com:vml\"\x20xmlns:w=\"urn:schemas-microsoft-com:office:word\"\x20href=\"https://listmonk.app\"\x20style=\"height:24pt;v-text-anchor:middle;width:202.5pt;\"\x20arcsize=\"50%\"\x20strokecolor=\"#FBF00B\"\x20fillcolor=\"#FBF00B\"\x3e\x3cw:anchorlock/\x3e\x3ccenter\x20style=\"color:#000000;font-family:Arial,\x20sans-serif;font-size:12pt;font-weight:bold;\"\x3eButton\x3c/center\x3e\x3c/v:roundrect\x3e\x3c![endif]--\x3e" }}`
	fs := &fakeLinkStore{}
	m := newTrackingManager(fs)
	body := `<html><body>` + safeVML + `<a href="https://plain.test/p">p</a></body></html>`
	out := renderCampaign(t, m, visualCampaign(body), testSub)

	// The decoded VML lands in the output with its destination href untracked.
	if !strings.Contains(out, `href="https://listmonk.app"`) {
		t.Fatalf("VML href missing or altered in render:\n%s", out)
	}
	if len(fs.created) != 1 || fs.created[0] != "https://plain.test/p" {
		t.Fatalf("expected only the plain anchor registered, got %v", fs.created)
	}
	if n := strings.Count(out, "/px.png"); n != 1 {
		t.Fatalf("expected one pixel, got %d", n)
	}
}

// §4.6 -- CompileTemplate must not mutate stored fields: the evergreen prepared-cache
// hash must be identical before and after, or reuseEvergreen re-prepares every tick.
func TestCompileLeavesStoredFieldsUnmutated(t *testing.T) {
	m := newTrackingManager(&fakeLinkStore{})
	body := `<html><body><a href="https://ex.test/a">A</a></body></html>`
	camp := visualCampaign(body)
	hashBefore := evergreenContentHash(camp)

	if err := camp.CompileTemplate(m.TemplateFuncs(camp)); err != nil {
		t.Fatalf("CompileTemplate: %v", err)
	}
	if camp.Body != body {
		t.Fatalf("c.Body mutated by compile:\n%s", camp.Body)
	}
	if camp.TemplateBody != "" {
		t.Fatalf("c.TemplateBody mutated by compile:\n%s", camp.TemplateBody)
	}
	if h := evergreenContentHash(camp); h != hashBefore {
		t.Fatalf("evergreenContentHash changed across compile: %s != %s", h, hashBefore)
	}
}

// §4.7 -- compiling the same campaign twice renders identical output: one pixel, no
// double-wrapped links.
func TestCompileTwiceIdempotent(t *testing.T) {
	m := newTrackingManager(&fakeLinkStore{})
	camp := visualCampaign(`<html><body><a href="https://ex.test/a">A</a></body></html>`)

	out1 := renderCampaign(t, m, camp, testSub)
	out2 := renderCampaign(t, m, camp, testSub)
	if out1 != out2 {
		t.Fatalf("compile-twice output differs:\n1: %s\n2: %s", out1, out2)
	}
	if n := strings.Count(out2, "/px.png"); n != 1 {
		t.Fatalf("expected one pixel after recompile, got %d", n)
	}
	if n := strings.Count(out2, "https://lm.test/link/"); n != 1 {
		t.Fatalf("expected one tracked link after recompile, got %d", n)
	}
}

// §4.8 (manager half) -- the archive path renders with a subscriber unmarshalled from
// archive_meta, which carries no uuid; compileArchiveCampaigns fills the empty UUID
// with the dummy. This pins the contract that fill relies on: a dummy-UUID subscriber
// yields routable dummy-UUID link and pixel URLs (which LinkRedirect and
// RegisterCampaignView exclude from registration). The cmd halves themselves
// (compileArchiveCampaigns, LinkRedirect) cannot host Go tests — package cmd's init()
// reads config.toml — so their wiring is covered by review and the prod verify.
func TestArchiveDummySubscriberRender(t *testing.T) {
	// The SPA's archive_meta shape: no uuid key.
	var sub models.Subscriber
	if err := json.Unmarshal([]byte(`{"email":"archive@example.com","name":"Archive Reader"}`), &sub); err != nil {
		t.Fatalf("unmarshal archive_meta: %v", err)
	}
	if sub.UUID != "" {
		t.Fatalf("fixture error: archive_meta unexpectedly carries a uuid: %q", sub.UUID)
	}
	// compileArchiveCampaigns' fill.
	sub.UUID = dummyUUID

	m := newTrackingManager(&fakeLinkStore{})
	out := renderCampaign(t, m, visualCampaign(`<html><body><a href="https://ex.test/a">A</a></body></html>`), sub)

	if !strings.Contains(out, "https://lm.test/link/lnk-1/camp-uuid/"+dummyUUID) {
		t.Fatalf("archive link URL missing dummy subscriber UUID:\n%s", out)
	}
	if !strings.Contains(out, "https://lm.test/campaign/camp-uuid/"+dummyUUID+"/px.png") {
		t.Fatalf("archive pixel URL missing dummy subscriber UUID:\n%s", out)
	}
	// No empty URL segment anywhere — the pre-fix failure mode.
	if strings.Contains(out, "/camp-uuid//") || strings.Contains(out, "/camp-uuid/\"") {
		t.Fatalf("empty subscriber UUID segment in URL:\n%s", out)
	}
}
