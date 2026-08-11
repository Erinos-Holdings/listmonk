package manager

import (
	"bytes"
	"strings"
	"testing"

	"github.com/knadh/listmonk/models"
)

func TestInjectPreheaderAfterBodyTag(t *testing.T) {
	body := []byte(`<html><head></head><BODY class="body" style="margin:0">visible</BODY></html>`)
	out := injectPreheader(body, "hello preview")

	idx := bytes.Index(out, []byte("lm-preheader"))
	bodyIdx := bytes.Index(out, []byte(`style="margin:0">`))
	if idx == -1 || bodyIdx == -1 || idx < bodyIdx {
		t.Fatalf("preheader not injected after opening body tag: %s", out)
	}
	if !bytes.Contains(out, []byte(">hello preview")) {
		t.Fatalf("preheader text missing: %s", out)
	}
	if visIdx := bytes.Index(out, []byte("visible")); visIdx < idx {
		t.Fatalf("preheader injected after visible content: %s", out)
	}
}

func TestInjectPreheaderNoBodyTag(t *testing.T) {
	body := []byte(`<p>fragment only</p>`)
	out := injectPreheader(body, "preview")

	if !bytes.HasPrefix(out, []byte(`<div class="lm-preheader"`)) {
		t.Fatalf("preheader not prepended to tagless body: %s", out)
	}
	if !bytes.HasSuffix(out, body) {
		t.Fatalf("original body not preserved: %s", out)
	}
}

func TestInjectPreheaderEscapesHTML(t *testing.T) {
	out := injectPreheader([]byte(`<body>x</body>`), `Save 50% on "A & B" <today>`)

	if bytes.Contains(out, []byte("<today>")) {
		t.Fatalf("preheader text not escaped: %s", out)
	}
	for _, want := range []string{"&lt;today&gt;", "&amp;", "&#34;A"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("expected escaped sequence %q in: %s", want, out)
		}
	}
}

// TestRenderWithPreheader exercises the full path a real campaign takes: the preheader
// read from attribs, template expressions compiled by CompileTemplate, and the hidden div
// injected into the rendered body after the template's own <body> tag.
func TestRenderWithPreheader(t *testing.T) {
	camp := &models.Campaign{
		Name:         "August launch",
		Subject:      "subject",
		Body:         "<h1>Hello</h1>",
		ContentType:  models.CampaignContentTypeRichtext,
		Messenger:    "email",
		TemplateBody: `<html><body id="x">{{ template "content" . }}</body></html>`,
		Attribs:      models.JSON{"preheader": "Preview for {{ .Campaign.Name }}"},
	}
	if err := camp.CompileTemplate(nil); err != nil {
		t.Fatalf("CompileTemplate: %v", err)
	}

	msg := CampaignMessage{Campaign: camp, Subscriber: models.Subscriber{}}
	if err := msg.render(); err != nil {
		t.Fatalf("render: %v", err)
	}

	body := string(msg.body)
	div := strings.Index(body, "lm-preheader")
	tag := strings.Index(body, `<body id="x">`)
	if div == -1 || tag == -1 || div < tag {
		t.Fatalf("preheader not injected after body tag: %s", body)
	}
	if !strings.Contains(body, "Preview for August launch") {
		t.Fatalf("preheader template not rendered: %s", body)
	}

	// No preheader in attribs → body untouched.
	camp2 := &models.Campaign{
		Subject: "s", Body: "b", ContentType: models.CampaignContentTypeRichtext,
		Messenger:    "email",
		TemplateBody: `<html><body>{{ template "content" . }}</body></html>`,
	}
	if err := camp2.CompileTemplate(nil); err != nil {
		t.Fatalf("CompileTemplate: %v", err)
	}
	msg2 := CampaignMessage{Campaign: camp2, Subscriber: models.Subscriber{}}
	if err := msg2.render(); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(string(msg2.body), "lm-preheader") {
		t.Fatalf("preheader injected with no attribs value: %s", msg2.body)
	}
}

func TestInjectPreheaderFiller(t *testing.T) {
	out := injectPreheader([]byte(`<body>x</body>`), "short")
	if strings.Count(string(out), "&zwnj;") < 80 {
		t.Fatalf("filler run missing: %s", out)
	}
}
