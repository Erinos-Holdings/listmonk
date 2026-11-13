package manager

import (
	"log"
	"os"
	"strings"
	"testing"

	"github.com/knadh/listmonk/models"
)

// Fork (join link) -- integrations JOIN-LINK-SPEC I9 (the URL) and I11 (the footer guard).

func newJoinManager(individualTracking bool) *Manager {
	m := &Manager{
		cfg: Config{
			IndividualTracking: individualTracking,
			UnsubURL:           "https://lm.test/subscription/%s/%s",
			OptinURL:           "https://lm.test/subscription/optin/%s?%s",
			JoinURL:            "https://lm.test/subscription/join/%s/%s",
		},
		log:   log.New(os.Stderr, "", 0),
		links: map[string]string{},
	}
	m.tplFuncs = m.makeGnericFuncMap()
	return m
}

// TestJoinURLFunc -- I9: {{ JoinURL }} (the shorthand) and {{ JoinURL . }} render
// <root>/subscription/join/<campaign uuid>/<subscriber uuid>, with the subscriber's REAL UUID
// whether or not individual tracking is on (the UUID pair is the capability), and no TrackLink.
func TestJoinURLFunc(t *testing.T) {
	for _, tracking := range []bool{true, false} {
		m := newJoinManager(tracking)
		for _, body := range []string{
			`<a href="{{ JoinURL }}">Join</a>`,
			`<a href="{{ JoinURL . }}">Join</a>`,
		} {
			camp := footerCampaign(body, models.CampaignContentTypeRichtext)
			if err := camp.CompileTemplate(m.TemplateFuncs(&camp)); err != nil {
				t.Fatalf("compile %s: %v", body, err)
			}
			msg, err := m.NewCampaignMessage(&camp, models.Subscriber{UUID: "sub-uuid", Email: "a@b.c"})
			if err != nil {
				t.Fatalf("render %s: %v", body, err)
			}
			want := `href="https://lm.test/subscription/join/camp-uuid/sub-uuid"`
			if out := string(msg.Body()); !strings.Contains(out, want) {
				t.Fatalf("tracking=%v %s rendered %s, want %s", tracking, body, out, want)
			}
		}
	}

	// A visual body: the compile-time href rewrite leaves the Join link plain (no /link/).
	m := newJoinManager(true)
	camp := footerCampaign(`<html><body><a href="{{ JoinURL }}">Join</a></body></html>`, models.CampaignContentTypeVisual)
	if err := camp.CompileTemplate(m.TemplateFuncs(&camp)); err != nil {
		t.Fatal(err)
	}
	msg, err := m.NewCampaignMessage(&camp, models.Subscriber{UUID: "sub-uuid", Email: "a@b.c"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(msg.Body())
	if !strings.Contains(out, `href="https://lm.test/subscription/join/camp-uuid/sub-uuid"`) || strings.Contains(out, "/link/") {
		t.Fatalf("visual Join link tracked or missing: %s", out)
	}
}

// TestFooterGuard_JoinIsNotUnsubscribe -- I11 (D13): a rendered Join link alone never satisfies
// the unsubscribe-link check; beside a real unsubscribe link the body passes; OptinURL behaviour
// is unchanged (it still counts, as before).
func TestFooterGuard_JoinIsNotUnsubscribe(t *testing.T) {
	join := []byte(`<a href="https://lm.test/subscription/join/camp-uuid/sub-uuid">Join</a>`)
	if p := FooterProblems(join, models.CampaignContentTypeRichtext, nil); len(p) != 1 {
		t.Fatalf("a Join link alone must be refused, got %v", p)
	}
	two := []byte(`<a href="https://lm.test/subscription/join/c/s">Join</a><a href="https://lm.test/subscription/join/c/s">Again</a>`)
	if p := FooterProblems(two, models.CampaignContentTypeRichtext, nil); len(p) != 1 {
		t.Fatalf("two Join links must still be refused, got %v", p)
	}
	both := []byte(`<a href="https://lm.test/subscription/join/c/s">Join</a><a href="https://lm.test/subscription/c/s">Unsubscribe</a>`)
	if p := FooterProblems(both, models.CampaignContentTypeRichtext, nil); len(p) != 0 {
		t.Fatalf("Join + unsubscribe must pass, got %v", p)
	}
	optin := []byte(`<a href="https://lm.test/subscription/optin/s?">Confirm</a>`)
	if p := FooterProblems(optin, models.CampaignContentTypeRichtext, nil); len(p) != 0 {
		t.Fatalf("OptinURL behaviour changed, got %v", p)
	}

	// Through the real render: {{ JoinURL }} alone is refused, + {{ UnsubscribeURL }} passes.
	m := newJoinManager(true)
	camp := footerCampaign(`<a href="{{ JoinURL }}">Join</a>`, models.CampaignContentTypeRichtext)
	problems, err := m.CheckFooter(camp, models.Subscriber{UUID: "sub-uuid", Email: "a@b.c"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 {
		t.Fatalf("rendered Join link alone must be refused, got %v", problems)
	}
	camp = footerCampaign(`<a href="{{ JoinURL }}">Join</a><a href="{{ UnsubscribeURL }}">U</a>`, models.CampaignContentTypeRichtext)
	if problems, err = m.CheckFooter(camp, models.Subscriber{UUID: "sub-uuid", Email: "a@b.c"}, nil); err != nil || len(problems) != 0 {
		t.Fatalf("Join + UnsubscribeURL must pass, got %v %v", problems, err)
	}
}
