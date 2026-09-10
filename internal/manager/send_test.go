package manager

// Fork (send retry, integrations SEND-RETRY-SPEC §4) -- the manager half of I1, I3, I7, I8
// and the D5 render-failure record, driven through the extracted per-message step
// (sendCampaignMessage) and pipe.cleanup(). Where a test needs the real retry it uses the
// real email messenger against the smtppool fork's in-process smtptest listener, so the
// path from the manager down to the SMTP pool is the deployed one.

import (
	"bytes"
	"errors"
	"html/template"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/knadh/listmonk/internal/messenger/email"
	"github.com/knadh/listmonk/models"
	"github.com/knadh/smtppool/v2"
	"github.com/knadh/smtppool/v2/smtptest"
	"github.com/paulbellamy/ratecounter"
)

// sendStore extends fakeStore with the campaign-completion calls cleanup() makes and
// the D5 record, so a pipe can be driven to completion.
type sendStore struct {
	fakeStore
	mu       sync.Mutex
	toSend   int
	sent     int
	statuses []string
	failures []models.SendFailure
	subs     []models.Subscriber
}

func (s *sendStore) UpdateCampaignCounts(campID, toSend, sent, lastSubID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if toSend != 0 {
		s.toSend = toSend
	}
	s.sent += sent
	return nil
}

func (s *sendStore) GetCampaign(campID int) (*models.Campaign, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := &models.Campaign{Name: "c", Status: models.CampaignStatusRunning}
	c.ToSend, c.Sent = s.toSend, s.sent
	c.ID = campID
	return c, nil
}

func (s *sendStore) UpdateCampaignStatus(campID int, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses = append(s.statuses, status)
	return nil
}

func (s *sendStore) RecordSendFailure(f models.SendFailure) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, f)
	return nil
}

func (s *sendStore) CountSendFailures(campID int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, f := range s.failures {
		if f.CampaignID == campID {
			n++
		}
	}
	return n, nil
}

func (s *sendStore) NextSubscribers(campID, limit int) ([]models.Subscriber, error) {
	return s.subs, nil
}

// scriptedMessenger fails the calls whose (0-based) ordinal is in fail.
type scriptedMessenger struct {
	mu    sync.Mutex
	calls int
	fail  map[int]bool
}

func (f *scriptedMessenger) Name() string { return "email" }
func (f *scriptedMessenger) Flush() error { return nil }
func (f *scriptedMessenger) Close() error { return nil }
func (f *scriptedMessenger) Push(m models.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.calls
	f.calls++
	if f.fail[n] {
		return errors.New("451 4.4.2 Timeout waiting for data from client.")
	}
	return nil
}

type sendHarness struct {
	m     *Manager
	fs    *sendStore
	logs  *bytes.Buffer
	notif []map[string]any
}

func newSendHarness(t *testing.T, msgr Messenger) *sendHarness {
	t.Helper()
	fs := &sendStore{}
	h := &sendHarness{fs: fs, logs: &bytes.Buffer{}}
	h.m = newTestManager(&fs.fakeStore)
	h.m.store = fs
	h.m.log = log.New(h.logs, "", 0)
	h.m.messengers = map[string]Messenger{msgr.Name(): msgr}
	h.m.pipes = map[int]*pipe{}
	h.m.fnNotify = func(subject string, data any) error {
		h.notif = append(h.notif, data.(map[string]any))
		return nil
	}
	return h
}

func (h *sendHarness) pipeFor(c *models.Campaign) *pipe {
	return &pipe{camp: c, m: h.m, wg: &sync.WaitGroup{}, rate: ratecounter.NewRateCounter(time.Minute)}
}

func (h *sendHarness) send(p *pipe, subID int) {
	s := models.Subscriber{Email: "r@example.com"}
	s.ID = subID
	s.UUID = "00000000-0000-0000-0000-00000000000" + string(rune('0'+subID%10))
	p.wg.Add(1)
	h.m.sendCampaignMessage(CampaignMessage{
		Campaign:   p.camp,
		Subscriber: s,
		from:       "s@example.com",
		to:         s.Email,
		subject:    "hi",
		body:       []byte("body"),
		pipe:       p,
	})
}

func regularCamp(id int) *models.Campaign {
	c := &models.Campaign{Name: "c", Messenger: "email", ContentType: models.CampaignContentTypeHTML}
	c.ID = id
	c.UUID = "11111111-1111-1111-1111-111111111111"
	return c
}

// realEmailer wires the deployed email messenger to the fake listener with the
// deployed attempt count (max_msg_retries 2).
func realEmailer(t *testing.T, hook func(smtptest.Event) smtptest.Action) (*smtptest.Server, Messenger) {
	t.Helper()
	srv, err := smtptest.New(hook)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	e, err := email.New("email", email.Server{
		Name:    "test",
		TLSType: "none",
		Opt: smtppool.Opt{
			Host:              srv.Host(),
			Port:              srv.Port(),
			MaxConns:          1,
			MaxMessageRetries: 2,
			MsgRetryDelay:     time.Millisecond,
			PoolWaitTimeout:   2 * time.Second,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	return srv, e
}

const stale451 = "451 4.4.2 Timeout waiting for data from client."

// I1 / D8 / D4 -- a 4yz reply is retried inside the pool; the manager sees one success:
// sent == 1, no OnError, no record.
func TestSendStepRetriedMessageCountsOnce(t *testing.T) {
	srv, e := realEmailer(t, func(ev smtptest.Event) smtptest.Action {
		if ev.Conn == 1 && ev.Cmd == "MAIL" {
			return smtptest.Action{Reply: stale451, Close: true}
		}
		return smtptest.Action{}
	})
	h := newSendHarness(t, e)
	p := h.pipeFor(regularCamp(1))

	h.send(p, 7)

	if got := srv.Mails(); got != 2 {
		t.Fatalf("attempts = %d, want 2 (one retry)", got)
	}
	if got := len(srv.Messages()); got != 1 {
		t.Fatalf("delivered %d, want 1", got)
	}
	if p.sent.Load() != 1 {
		t.Fatalf("sent = %d, want exactly 1", p.sent.Load())
	}
	if p.errors.Load() != 0 {
		t.Fatalf("errors = %d, want 0 -- a retry absorbed by the pool must not count", p.errors.Load())
	}
	if len(h.fs.failures) != 0 {
		t.Fatalf("failures recorded = %v, want none", h.fs.failures)
	}
	if strings.Contains(h.logs.String(), "error sending message") {
		t.Fatalf("a successful retry must not log a send error:\n%s", h.logs.String())
	}
}

// I3 -- a message that exhausts max_msg_retries is recorded once and counts once.
func TestSendStepExhaustedMessageRecordedOnce(t *testing.T) {
	srv, e := realEmailer(t, func(ev smtptest.Event) smtptest.Action {
		if ev.Cmd == "MAIL" {
			return smtptest.Action{Reply: stale451, Close: true}
		}
		return smtptest.Action{}
	})
	h := newSendHarness(t, e)
	p := h.pipeFor(regularCamp(2))

	h.send(p, 22)

	if got := srv.Mails(); got != 2 {
		t.Fatalf("attempts = %d, want max_msg_retries (2)", got)
	}
	if p.sent.Load() != 0 {
		t.Fatalf("sent = %d, want 0", p.sent.Load())
	}
	if p.errors.Load() != 1 {
		t.Fatalf("errors = %d, want exactly 1 (D4: attempts do not count, exhaustion does)", p.errors.Load())
	}
	if len(h.fs.failures) != 1 {
		t.Fatalf("failures recorded = %d, want exactly 1", len(h.fs.failures))
	}
	f := h.fs.failures[0]
	if f.CampaignID != 2 || f.SubscriberID != 22 || f.Email != "r@example.com" || f.Stage != models.SendFailureStageSend || !strings.Contains(f.Error, "451") {
		t.Fatalf("record = %+v", f)
	}
	if p.failed.Load() != 1 {
		t.Fatalf("pipe.failed = %d, want 1", p.failed.Load())
	}
	if !strings.Contains(h.logs.String(), "error sending message in campaign c: subscriber 22: ") {
		t.Fatalf("the existing error line (the SendErrors alarm's pattern) must survive:\n%s", h.logs.String())
	}
}

// I7 -- a campaign that finishes with sent < to_send emits the shortfall exactly once:
// one log line, and the same reason on the single completion notification.
func TestCleanupShortfallEmittedOnce(t *testing.T) {
	msgr := &scriptedMessenger{fail: map[int]bool{1: true, 3: true}}
	h := newSendHarness(t, msgr)
	h.fs.toSend = 5
	p := h.pipeFor(regularCamp(3))
	h.m.pipes[3] = p

	for i := 1; i <= 5; i++ {
		h.send(p, i)
	}
	if p.sent.Load() != 3 || p.errors.Load() != 2 {
		t.Fatalf("sent/errors = %d/%d, want 3/2", p.sent.Load(), p.errors.Load())
	}
	p.cleanup()

	if got := strings.Count(h.logs.String(), "campaign shortfall"); got != 1 {
		t.Fatalf("shortfall logged %d times, want exactly once:\n%s", got, h.logs.String())
	}
	if len(h.notif) != 1 {
		t.Fatalf("notifications = %d, want 1", len(h.notif))
	}
	reason, _ := h.notif[0]["Reason"].(string)
	if !strings.Contains(reason, "sent 3 of 5") || !strings.Contains(reason, "2 recorded in campaign_send_failures") {
		t.Fatalf("notification reason = %q", reason)
	}
	if h.notif[0]["Status"] != models.CampaignStatusFinished {
		t.Fatalf("status = %v, want finished", h.notif[0]["Status"])
	}
	if len(h.fs.failures) != 2 {
		t.Fatalf("records = %d, want 2", len(h.fs.failures))
	}
	if _, still := h.m.pipes[3]; still {
		t.Fatal("cleanup must release the pipe")
	}

	// Control: a full send finishes quietly with an empty reason.
	h2 := newSendHarness(t, &scriptedMessenger{})
	h2.fs.toSend = 2
	p2 := h2.pipeFor(regularCamp(4))
	h2.send(p2, 1)
	h2.send(p2, 2)
	p2.cleanup()
	if strings.Contains(h2.logs.String(), "campaign shortfall") {
		t.Fatalf("no shortfall expected:\n%s", h2.logs.String())
	}
	if len(h2.notif) != 1 || h2.notif[0]["Reason"] != "" {
		t.Fatalf("notif = %+v", h2.notif)
	}
}

// D6 caveat -- a shortfall with nothing recorded (to_send grew from a list edit) names
// that in the reason rather than implying a loss.
func TestCleanupShortfallWithoutRecordsNamesListEdit(t *testing.T) {
	h := newSendHarness(t, &scriptedMessenger{})
	h.fs.toSend = 3
	p := h.pipeFor(regularCamp(5))
	h.send(p, 1)
	h.send(p, 2)
	p.cleanup()
	reason, _ := h.notif[0]["Reason"].(string)
	if !strings.Contains(reason, "sent 2 of 3") || !strings.Contains(reason, "none recorded") {
		t.Fatalf("reason = %q", reason)
	}
}

// I8 -- an evergreen message retried to success marks its claim sent and resets the
// streak (the retried attempt does not increment it); one that exhausts its attempts is
// recorded and NOT marked sent, and counts one toward the streak.
func TestEvergreenRetryAndExhaustion(t *testing.T) {
	first := true
	srv, e := realEmailer(t, func(ev smtptest.Event) smtptest.Action {
		if ev.Cmd == "MAIL" && first {
			first = false
			return smtptest.Action{Reply: stale451, Close: true}
		}
		return smtptest.Action{}
	})
	h := newSendHarness(t, e)
	c := evergreenCamp(9)
	c.Messenger = "email"
	c.ContentType = models.CampaignContentTypeHTML
	p := h.pipeFor(c)
	h.m.evergreenErrors[c.ID] = 2 // a prior streak

	h.send(p, 31) // 451 then success
	if got := srv.Mails(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	if h.fs.marked == nil || len(h.fs.marked) != 1 || h.fs.marked[0] != 31 {
		t.Fatalf("marked = %v, want [31]", h.fs.marked)
	}
	if h.m.evergreenErrors[c.ID] != 0 {
		t.Fatalf("streak = %d, want 0 (reset by the eventual success, not bumped by the retry)", h.m.evergreenErrors[c.ID])
	}
	if p.sent.Load() != 1 || len(h.fs.failures) != 0 {
		t.Fatalf("sent=%d failures=%v", p.sent.Load(), h.fs.failures)
	}

	// Now exhaust: every MAIL fails from here on.
	srv2, e2 := realEmailer(t, func(ev smtptest.Event) smtptest.Action {
		if ev.Cmd == "MAIL" {
			return smtptest.Action{Reply: stale451, Close: true}
		}
		return smtptest.Action{}
	})
	h.m.messengers["email"] = e2
	h.send(p, 32)
	if got := srv2.Mails(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	if len(h.fs.marked) != 1 {
		t.Fatalf("marked = %v -- an exhausted evergreen send must NOT be marked sent", h.fs.marked)
	}
	if len(h.fs.failures) != 1 || h.fs.failures[0].SubscriberID != 32 || h.fs.failures[0].CampaignID != 9 {
		t.Fatalf("failures = %+v, want the one exhausted record", h.fs.failures)
	}
	if h.m.evergreenErrors[c.ID] != 1 {
		t.Fatalf("streak = %d, want 1", h.m.evergreenErrors[c.ID])
	}
	if p.sent.Load() != 1 {
		t.Fatalf("sent = %d, want still 1", p.sent.Load())
	}
}

// D5 (review M1) -- a recipient whose message fails to render is recorded with stage
// "render" and is not counted toward MaxSendErrors.
func TestRenderFailureRecorded(t *testing.T) {
	h := newSendHarness(t, &scriptedMessenger{})
	h.m.cfg.BatchSize = 10
	c := regularCamp(6)
	c.Tpl = template.Must(template.New(models.BaseTpl).Parse(`{{ .Subscriber.NoSuchField }}`))
	p := h.pipeFor(c)

	sub := models.Subscriber{Email: "broken@example.com"}
	sub.ID = 41
	h.fs.subs = []models.Subscriber{sub}

	if _, err := p.NextSubscribers(); err != nil {
		t.Fatal(err)
	}
	if len(h.fs.failures) != 1 {
		t.Fatalf("failures = %+v, want 1", h.fs.failures)
	}
	f := h.fs.failures[0]
	if f.Stage != models.SendFailureStageRender || f.SubscriberID != 41 || f.Email != "broken@example.com" || f.CampaignID != 6 {
		t.Fatalf("record = %+v", f)
	}
	if p.errors.Load() != 0 {
		t.Fatalf("errors = %d, want 0 (render failures are recorded, not counted)", p.errors.Load())
	}
	if p.failed.Load() != 1 {
		t.Fatalf("failed = %d, want 1", p.failed.Load())
	}
}

// D10 / review F1 -- an error raised after the message data was handed over MAY be a
// delivered message. For an evergreen the claim is consumed (no hourly re-welcome, no
// duplicate); for any campaign the record carries the send-unconfirmed stage and the error
// still counts once. The path is the deployed one: email messenger, pool, socket.
func TestPostDataErrorConsumesClaimAndIsRecordedUnconfirmed(t *testing.T) {
	srv, e := realEmailer(t, func(ev smtptest.Event) smtptest.Action {
		if ev.Cmd == "DATAEND" {
			return smtptest.Action{Drop: true}
		}
		return smtptest.Action{}
	})
	h := newSendHarness(t, e)
	c := evergreenCamp(12)
	c.Messenger = "email"
	c.ContentType = models.CampaignContentTypeHTML
	p := h.pipeFor(c)
	h.m.evergreenErrors[c.ID] = 1

	h.send(p, 51)

	if got := srv.Mails(); got != 1 {
		t.Fatalf("attempts = %d, want exactly 1 (never retried)", got)
	}
	if len(h.fs.marked) != 1 || h.fs.marked[0] != 51 {
		t.Fatalf("marked = %v, want [51] -- the claim must be consumed, or the subscriber is re-welcomed in an hour", h.fs.marked)
	}
	if len(h.fs.failures) != 1 || h.fs.failures[0].Stage != models.SendFailureStageSendUnconfirmed {
		t.Fatalf("failures = %+v, want one send-unconfirmed record", h.fs.failures)
	}
	if p.sent.Load() != 0 {
		t.Fatalf("sent = %d, want 0 (unconfirmed is not sent)", p.sent.Load())
	}
	// An evergreen counts errors on the manager's streak, not the pipe.
	if h.m.evergreenErrors[c.ID] != 2 {
		t.Fatalf("streak = %d, want 2 (an unconfirmed send counts once and is not a success)", h.m.evergreenErrors[c.ID])
	}
}

// Review F7 -- the shortfall line reports the campaign's recorded total, not this pipe's.
func TestCleanupShortfallCountsAcrossPipes(t *testing.T) {
	h := newSendHarness(t, &scriptedMessenger{fail: map[int]bool{0: true}})
	h.fs.toSend = 4
	h.fs.failures = append(h.fs.failures, models.SendFailure{CampaignID: 7, SubscriberID: 99, Stage: models.SendFailureStageSend})
	h.fs.sent = 1 // an earlier pipe's success
	p := h.pipeFor(regularCamp(7))
	h.send(p, 1) // fails
	h.send(p, 2)
	p.cleanup()
	reason, _ := h.notif[0]["Reason"].(string)
	if !strings.Contains(reason, "sent 2 of 4") || !strings.Contains(reason, "2 recorded") {
		t.Fatalf("reason = %q", reason)
	}
}
