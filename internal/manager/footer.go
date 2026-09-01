package manager

import (
	"fmt"
	"strings"

	"github.com/knadh/listmonk/models"
)

// Fork (footer guard) -- I18N-SPEC C3. The first BLOCKING send-quality rule in this
// fork: every other check in warnings.go warns and lets the send through. It exists
// because a campaign whose footer lost its unsubscribe link (or its language-correct
// legal copy) is a compliance incident that cannot be recalled once sent, and nothing
// in listmonk enforced a footer at all.
//
// Everything here is pure or render-only so it can be unit tested: package cmd hosts
// no Go tests (its init() reads config.toml), so the handler wiring is inspection
// covered and the policy lives here.

// unsubMarker is the path every {{ UnsubscribeURL }} / {{ ManageURL }} rendering
// contains (cfg.UnsubURL is "<root>/subscription/%s/%s"). Its presence in the rendered
// body is what "the unsubscribe link resolves" means for the guard -- the guard does
// not fetch the URL, it checks that one was rendered.
const unsubMarker = "/subscription/"

// FooterGuardOnStatus reports whether a campaign status change must be footer-guarded.
// EVERY transition into running|scheduled is -- resume (paused -> running) and
// paused -> scheduled included. Compliance beats convenience: no button may put a
// non-compliant campaign on the wire, and a refused resume is forced remediation, not a
// dead end (edit the footer in -- that save is guarded by FooterGuardOnUpdate -- then
// resume).
//
// stored is deliberately ignored, so the guard also fires on transitions core will
// reject anyway (e.g. finished -> running). Since the guard runs before core's legality
// check, such a transition on a non-compliant campaign 400s with the footer error rather
// than the transition error -- accepted: both are 400s, the UI never offers illegal
// transitions, and mirroring core's legality table here is worse than the mislabeled
// error.
//
// This guard is HTTP-handler-only. Evergreen ticking, claims and internal restarts are
// pure SQL inside the manager and MUST stay unguarded -- blocking them would be a silent
// send-path outage rather than a UI refusal.
func FooterGuardOnStatus(stored, next string) bool {
	_ = stored
	return next == models.CampaignStatusRunning || next == models.CampaignStatusScheduled
}

// FooterGuardOnUpdate reports whether saving a campaign in the given stored status must
// be footer-guarded. A scheduled or paused campaign sends again with no further guarded
// transition -- the claim and the resume are pure SQL -- so "schedule a compliant
// campaign, delete the footer, save" (and the documented pause / edit / resume automation
// editing workflow) would otherwise walk straight past the guard. A draft save is never
// guarded: drafts are work in progress and must stay saveable.
func FooterGuardOnUpdate(stored string) bool {
	return stored == models.CampaignStatusScheduled || stored == models.CampaignStatusPaused
}

// FooterProblems inspects a fully rendered per-subscriber message body (the same output
// preview and the send loop emit) and returns the reasons it fails the footer rules.
// Empty result = compliant.
//
//   - Every content type must render a working unsubscribe URL.
//   - markers are matched as exact, case-sensitive substrings of the rendered output.
//     An EMPTY marker list skips the marker check entirely -- that is how the guard
//     ships inert. Blank entries are ignored so a stray tag in the settings taginput
//     cannot block every campaign.
//   - Plain-text campaigns are exempt from the markers (they carry no HTML footer
//     block); the unsubscribe rule still applies to them.
func FooterProblems(rendered []byte, contentType string, markers []string) []string {
	var out []string

	body := string(rendered)
	if !strings.Contains(body, unsubMarker) {
		out = append(out, fmt.Sprintf("the rendered message contains no unsubscribe link (no %q URL) -- add {{ UnsubscribeURL }} to the campaign or its template footer", unsubMarker))
	}

	if contentType == models.CampaignContentTypePlain {
		return out
	}

	for _, m := range markers {
		if strings.TrimSpace(m) == "" {
			continue
		}
		if !strings.Contains(body, m) {
			out = append(out, fmt.Sprintf("the rendered message is missing required footer content %q", m))
		}
	}

	return out
}

// CheckFooter renders camp against the given dummy subscriber and applies FooterProblems
// to the result. Unlike RenderWarnings' caller (renderWarnings, which returns nil on a
// render failure and therefore fails OPEN), a compile or render failure here is an ERROR
// and the caller must refuse the send -- a body that cannot be rendered cannot be shown
// to carry a footer.
//
// camp is taken by value so the caller's copy keeps its real UUID and uncompiled state;
// the campaign UUID is swapped for the dummy subscriber's the way preview does, so any
// {{ TrackView }}/{{ TrackLink }} in the body registers nothing against the real campaign.
func (m *Manager) CheckFooter(camp models.Campaign, sub models.Subscriber, markers []string) ([]string, error) {
	camp.UUID = sub.UUID

	if err := camp.CompileTemplate(m.TemplateFuncs(&camp)); err != nil {
		return nil, fmt.Errorf("error compiling campaign: %v", err)
	}

	msg, err := m.NewCampaignMessage(&camp, sub)
	if err != nil {
		return nil, fmt.Errorf("error rendering campaign: %v", err)
	}

	return FooterProblems(msg.Body(), camp.ContentType, markers), nil
}
