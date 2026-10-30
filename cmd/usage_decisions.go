package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/decisions"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// Fork (usage decisions, integrations USAGE-DECISIONS-SPEC D1-D6). Marketing's accept/reject record
// of the lifecycle-series items on Brands > Usage -- the one class of Usage-pane content the fork
// STORES itself (the rest is the integrations BrandHealth Lambda's, rendered verbatim). The item
// texts are internal/decisions/lifecycle.json; the rows are the append-only usage_decisions table.
//
//   GET  /api/usage/decisions       brands:get      every item, its current state and history, and
//                                                   can_decide (canDecide -- the POST's own gate)
//   POST /api/usage/decisions/:key  brands:decide   one decision, gated by canDecide in the handler
//
// After a write commits, a goroutine posts one notice to #integrations-notices
// (ERINOS_SLACK_NOTICES_WEBHOOK); a failed post is logged and never fails the decision.

const (
	// decisionNoteMax caps a reject note, in characters (runes).
	decisionNoteMax = 2000
	// decisionNoticeCut is the Slack notice's item-line cut, in runes, never bytes.
	decisionNoticeCut = 120
	// decisionNoticeTimeout bounds the Slack POST.
	decisionNoticeTimeout = 5 * time.Second
)

// canDecide is D3's ONE predicate: a user-type account (never an API token), whose role is not
// Super Admin on EITHER role field (a retrieved user carries its role in UserRole.ID with UserRoleID
// zeroed -- core.setupUserFields -- the isAnalyticsOnly precedent), holding brands:decide read
// directly from PermissionsMap (never HasPerm, whose Super Admin short-circuit would let it in).
func canDecide(u auth.User) bool {
	if u.Type != auth.UserTypeUser {
		return false
	}
	if u.UserRole.ID == auth.SuperAdminRoleID || u.UserRoleID == auth.SuperAdminRoleID {
		return false
	}
	_, ok := u.PermissionsMap[auth.PermBrandsDecide]
	return ok
}

// decideGate is the function both handlers call: the GET's can_decide and the POST's refusal are
// the same predicate (I6; TestCanDecide swaps it to prove both paths go through it).
var decideGate = canDecide

// usageDecisionStore is the table's whole surface: a read of every row and one append.
type usageDecisionStore interface {
	UsageDecisions() ([]decisions.Row, error)
	InsertUsageDecision(r decisions.Row) error
}

// dbDecisionStore is the production store over queries/brands.sql.
type dbDecisionStore struct {
	q *models.Queries
}

func (s dbDecisionStore) UsageDecisions() ([]decisions.Row, error) {
	var out []decisions.Row
	if err := s.q.GetUsageDecisions.Select(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s dbDecisionStore) InsertUsageDecision(r decisions.Row) error {
	var id int64
	return s.q.InsertUsageDecision.Get(&id, r.ItemKey, r.TextHash, r.Decision, r.Note, r.UserID, r.UserName)
}

// GetUsageDecisions returns every lifecycle item with its current state and history.
func (a *App) GetUsageDecisions(c echo.Context) error {
	rows, err := a.usageDecisions.UsageDecisions()
	if err != nil {
		a.log.Printf("decisions: read failed: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.Ts("globals.messages.errorFetching", "name", "decisions", "error", err.Error()))
	}
	u := auth.GetUser(c)
	return c.JSON(http.StatusOK, okResp{decisions.Build(a.decisionDoc, rows, decideGate(u))})
}

// PostUsageDecision appends one decision. Body: {hash, decision, note}. Every refusal writes
// nothing (D4): not a decider (403), an unknown key (404), a hash other than the current one (409,
// reload), a decision other than accept/reject, a reject with a blank note, a note over 2,000
// characters, or an accept carrying a note (400).
func (a *App) PostUsageDecision(c echo.Context) error {
	u := auth.GetUser(c)
	if !decideGate(u) {
		if u.Type != auth.UserTypeUser {
			return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("decisions.apiRefused"))
		}
		return echo.NewHTTPError(http.StatusForbidden, a.i18n.T("decisions.forbidden"))
	}

	it, ok := a.decisionDoc.Find(c.Param("key"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, a.i18n.T("decisions.unknown"))
	}

	var req struct {
		Hash     string  `json:"hash"`
		Decision string  `json:"decision"`
		Note     *string `json:"note"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "body"))
	}
	if req.Hash != decisions.Hash(it.Text) {
		return echo.NewHTTPError(http.StatusConflict, a.i18n.T("decisions.stale"))
	}

	raw := ""
	if req.Note != nil {
		raw = *req.Note
	}
	note := strings.TrimSpace(raw)
	var stored *string
	switch req.Decision {
	case decisions.Accept:
		if raw != "" {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("decisions.acceptNote"))
		}
	case decisions.Reject:
		if note == "" {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("decisions.noteRequired"))
		}
		if utf8.RuneCountInString(note) > decisionNoteMax {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("decisions.noteTooLong"))
		}
		stored = &note
	default:
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("decisions.badDecision"))
	}

	name := u.Name
	if strings.TrimSpace(name) == "" {
		name = u.Username
	}
	row := decisions.Row{ItemKey: it.Key, TextHash: decisions.Hash(it.Text), Decision: req.Decision, Note: stored, UserID: u.ID, UserName: name}
	if err := a.usageDecisions.InsertUsageDecision(row); err != nil {
		a.log.Printf("decisions: insert failed: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.Ts("globals.messages.errorCreating", "name", "decision", "error", err.Error()))
	}

	// Committed: the notice never fails or rolls back the decision.
	if a.decisionNotice != nil {
		a.decisionNotice.Notify(name, req.Decision, it, note)
	}

	rows, err := a.usageDecisions.UsageDecisions()
	if err != nil {
		a.log.Printf("decisions: read failed: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.Ts("globals.messages.errorFetching", "name", "decisions", "error", err.Error()))
	}
	return c.JSON(http.StatusOK, okResp{decisions.ItemOf(it, rows)})
}

// decisionNotifier posts the D6 Slack notice. A disabled notifier (unset or non-https URL) posts
// nothing.
type decisionNotifier struct {
	url    string
	root   string
	client *http.Client
	log    *log.Logger
	// wg tracks in-flight posts (tests wait on it).
	wg sync.WaitGroup
}

// newDecisionNotifier is read once at boot from ERINOS_SLACK_NOTICES_WEBHOOK. Unset or not https://
// logs one line and disables the posts.
func newDecisionNotifier(raw, rootURL string, lo *log.Logger) *decisionNotifier {
	n := &decisionNotifier{root: strings.TrimSuffix(rootURL, "/"), client: &http.Client{Timeout: decisionNoticeTimeout}, log: lo}
	switch {
	case raw == "":
		lo.Println("decisions: ERINOS_SLACK_NOTICES_WEBHOOK is not set; usage-decision notices are off")
	case !strings.HasPrefix(raw, "https://"):
		lo.Println("decisions: ERINOS_SLACK_NOTICES_WEBHOOK is not an https:// URL; usage-decision notices are off")
	default:
		n.url = raw
	}
	return n
}

// slackEscape applies Slack's three control-character escapes, so a note or a name cannot mention
// <!channel> or forge a link.
func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// cutRunes cuts s to at most n runes (never inside a multibyte character).
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// decisionNoticeText is D6's text: `Lifecycle decision: <name> <accepted|rejected> <#n | Both
// series> "<first line of the item, cut at 120 runes>"`, then `\nNote: <note>` on a reject, then
// `\n<root>/admin/brands`. The name, item line and note are escaped (the cut first, so an entity is
// never split).
func decisionNoticeText(name, decision string, it decisions.Item, note, root string) string {
	verb := "accepted"
	if decision == decisions.Reject {
		verb = "rejected"
	}
	where := "Both series"
	if it.N != nil {
		where = fmt.Sprintf("#%d", *it.N)
	}
	first := strings.SplitN(it.Text, "\n", 2)[0]
	var b strings.Builder
	fmt.Fprintf(&b, "Lifecycle decision: %s %s %s \"%s\"", slackEscape(name), verb, where, slackEscape(cutRunes(first, decisionNoticeCut)))
	if decision == decisions.Reject {
		b.WriteString("\nNote: " + slackEscape(note))
	}
	b.WriteString("\n" + root + "/admin/brands")
	return b.String()
}

// Notify posts the notice in a goroutine; a disabled notifier does nothing.
func (n *decisionNotifier) Notify(name, decision string, it decisions.Item, note string) {
	if n == nil || n.url == "" {
		return
	}
	body, err := json.Marshal(map[string]string{"text": decisionNoticeText(name, decision, it, note, n.root)})
	if err != nil {
		n.log.Printf("decisions: slack notice failed: %v", err)
		return
	}
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		if err := n.post(body); err != nil {
			n.log.Printf("decisions: slack notice failed: %v", err)
		}
	}()
}

func (n *decisionNotifier) post(body []byte) error {
	resp, err := n.client.Post(n.url, "application/json", bytes.NewReader(body))
	if err != nil {
		// A *url.Error names the URL, and the webhook URL IS the secret: log only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			return ue.Err
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
