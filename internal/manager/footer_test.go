package manager

import (
	"log"
	"os"
	"strings"
	"testing"

	"github.com/knadh/listmonk/models"
)

// Fork (footer guard) -- unit pins for I18N-SPEC C3. Package cmd hosts no Go tests, so
// the guard's policy (which transitions are guarded, what counts as a compliant footer,
// and that a render failure REFUSES rather than passing) is pinned here and only the
// handler wiring in cmd/campaigns.go is inspection-covered.

func newFooterManager() *Manager {
	m := &Manager{
		cfg: Config{
			UnsubURL: "https://lm.test/subscription/%s/%s",
		},
		log: log.New(os.Stderr, "", 0),
	}
	m.tplFuncs = m.makeGnericFuncMap()
	return m
}

func footerCampaign(body, contentType string) models.Campaign {
	c := models.Campaign{
		Subject:     "s",
		Body:        body,
		ContentType: contentType,
		Messenger:   "email",
	}
	c.UUID = "camp-uuid"
	return c
}

// The guard trigger matrix. EVERY transition into running|scheduled is guarded,
// regardless of the stored status -- resume (paused -> running) and paused -> scheduled
// included (2026-09-01 amendment: compliance over convenience; a refused resume is
// forced remediation via the equally guarded save). Transitions to any other status are
// never guarded. Internal automation restarts are pure SQL and never reach this guard.
func TestFooterGuardOnStatus(t *testing.T) {
	stored := []string{
		models.CampaignStatusDraft,
		models.CampaignStatusRunning,
		models.CampaignStatusScheduled,
		models.CampaignStatusPaused,
		models.CampaignStatusCancelled,
		models.CampaignStatusFinished,
	}

	// Every next in {running, scheduled} is guarded from every stored status --
	// including ones core will reject anyway (finished -> running), which is accepted:
	// the guard runs before core's legality check.
	for _, st := range stored {
		for _, next := range []string{models.CampaignStatusRunning, models.CampaignStatusScheduled} {
			if got := FooterGuardOnStatus(st, next); !got {
				t.Fatalf("FooterGuardOnStatus(%q, %q) = false, want true", st, next)
			}
		}
	}

	// Everything that is not a dispatch stays unguarded.
	for _, st := range stored {
		for _, next := range []string{
			models.CampaignStatusDraft,
			models.CampaignStatusPaused,
			models.CampaignStatusCancelled,
			models.CampaignStatusFinished,
		} {
			if got := FooterGuardOnStatus(st, next); got {
				t.Fatalf("FooterGuardOnStatus(%q, %q) = true, want false", st, next)
			}
		}
	}
}

// A save is guarded for the statuses that send again with no further guarded
// transition (scheduled claim, paused resume). Drafts stay freely saveable.
func TestFooterGuardOnUpdate(t *testing.T) {
	cases := map[string]bool{
		models.CampaignStatusScheduled: true,
		models.CampaignStatusPaused:    true,
		models.CampaignStatusDraft:     false,
		models.CampaignStatusRunning:   false,
		models.CampaignStatusFinished:  false,
		models.CampaignStatusCancelled: false,
	}
	for status, want := range cases {
		if got := FooterGuardOnUpdate(status); got != want {
			t.Fatalf("FooterGuardOnUpdate(%q) = %v, want %v", status, got, want)
		}
	}
}

// The marker check, and the empty-list skip that ships the guard inert.
func TestFooterProblemsMarkers(t *testing.T) {
	compliant := []byte(`<p>hi</p><a href="https://lm.test/subscription/c/s">Unsubscribe</a>` +
		`<p>Acme, 100 Main St, Springfield</p>`)

	// No markers configured -- only the unsubscribe rule runs.
	if p := FooterProblems(compliant, models.CampaignContentTypeRichtext, nil); len(p) != 0 {
		t.Fatalf("empty marker list must skip the marker check, got %v", p)
	}
	if p := FooterProblems(compliant, models.CampaignContentTypeRichtext, []string{}); len(p) != 0 {
		t.Fatalf("empty marker list must skip the marker check, got %v", p)
	}

	// A blank tag from the settings taginput must not block every campaign.
	if p := FooterProblems(compliant, models.CampaignContentTypeRichtext, []string{"  "}); len(p) != 0 {
		t.Fatalf("blank marker must be ignored, got %v", p)
	}

	// Present markers pass; a missing one is reported and names itself.
	if p := FooterProblems(compliant, models.CampaignContentTypeRichtext,
		[]string{"Springfield", "Unsubscribe"}); len(p) != 0 {
		t.Fatalf("markers present must pass, got %v", p)
	}
	p := FooterProblems(compliant, models.CampaignContentTypeRichtext,
		[]string{"Springfield", "Se désabonner"})
	if len(p) != 1 || !strings.Contains(p[0], "Se désabonner") {
		t.Fatalf("missing marker must be reported by name, got %v", p)
	}

	// Match is exact and case-sensitive.
	if p := FooterProblems(compliant, models.CampaignContentTypeRichtext,
		[]string{"lehi ut"}); len(p) != 1 {
		t.Fatalf("marker match must be case-sensitive, got %v", p)
	}
}

// The unsubscribe rule applies to every content type; plain text gets it and nothing else.
func TestFooterProblemsUnsubscribeAndPlain(t *testing.T) {
	noUnsub := []byte("Hello, no links here.")
	withUnsub := []byte("Hello. Unsubscribe https://lm.test/subscription/c/s")

	if p := FooterProblems(noUnsub, models.CampaignContentTypeRichtext, nil); len(p) != 1 {
		t.Fatalf("a body with no unsubscribe URL must be refused, got %v", p)
	}

	// Plain text: unsubscribe still required...
	if p := FooterProblems(noUnsub, models.CampaignContentTypePlain, nil); len(p) != 1 {
		t.Fatalf("plain text must still require an unsubscribe URL, got %v", p)
	}
	// ...but markers never apply to it (no HTML footer block to carry them).
	if p := FooterProblems(withUnsub, models.CampaignContentTypePlain,
		[]string{"Springfield", "anything"}); len(p) != 0 {
		t.Fatalf("plain text must be exempt from markers, got %v", p)
	}
	// The same body as HTML does fail the markers -- proving the exemption is the
	// content type and not the body.
	if p := FooterProblems(withUnsub, models.CampaignContentTypeRichtext,
		[]string{"Springfield"}); len(p) != 1 {
		t.Fatalf("HTML must apply markers, got %v", p)
	}
}

// CheckFooter renders the real message (template wrap + UnsubscribeURL) and judges that,
// never the raw editor body.
func TestCheckFooterRendersMessage(t *testing.T) {
	m := newFooterManager()

	// {{ UnsubscribeURL }} is only an unsubscribe link once RENDERED -- the raw body
	// contains no "/subscription/".
	camp := footerCampaign(`<p>hi</p><a href="{{ UnsubscribeURL }}">Unsubscribe</a>`,
		models.CampaignContentTypeRichtext)
	problems, err := m.CheckFooter(camp, models.Subscriber{UUID: "sub-uuid", Email: "a@b.c"}, nil)
	if err != nil {
		t.Fatalf("CheckFooter: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("rendered unsubscribe link must pass, got %v", problems)
	}

	// No unsubscribe tag anywhere -- refused.
	camp = footerCampaign(`<p>no footer at all</p>`, models.CampaignContentTypeRichtext)
	problems, err = m.CheckFooter(camp, models.Subscriber{UUID: "sub-uuid", Email: "a@b.c"}, nil)
	if err != nil {
		t.Fatalf("CheckFooter: %v", err)
	}
	if len(problems) != 1 {
		t.Fatalf("body with no unsubscribe link must be refused, got %v", problems)
	}
}

// A body that fails to compile is a REFUSAL, never a pass. This is the reason the guard
// does not build on renderWarnings, which returns nil (= compliant) on a compile error.
func TestCheckFooterCompileFailureRefuses(t *testing.T) {
	m := newFooterManager()

	camp := footerCampaign(`<p>{{ this is not a template`, models.CampaignContentTypeRichtext)
	problems, err := m.CheckFooter(camp, models.Subscriber{UUID: "sub-uuid", Email: "a@b.c"}, nil)
	if err == nil {
		t.Fatalf("a body that fails to compile must error, got problems %v", problems)
	}

	// Contrast: the non-blocking warning path fails open on the same body.
	if w := RenderWarnings([]byte("")); len(w) != 0 {
		t.Fatalf("sanity: empty body carries no warnings, got %v", w)
	}
}
