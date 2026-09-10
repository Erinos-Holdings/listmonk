package smtppool

// Erinos fork — SEND-RETRY-SPEC §4. These tests drive the real Pool against the
// in-process smtptest listener; nothing here mocks the pool. Each test names the
// spec invariant it pins.

import (
	"errors"
	"io"
	"net"
	"net/textproto"
	"testing"
	"time"

	"github.com/knadh/smtppool/v2/smtptest"
)

const stale451 = "451 4.4.2 Timeout waiting for data from client."

func newTestPool(t *testing.T, srv *smtptest.Server, attempts int) *Pool {
	t.Helper()
	p, err := New(Opt{
		Host:              srv.Host(),
		Port:              srv.Port(),
		MaxConns:          2,
		MaxMessageRetries: attempts,
		MsgRetryDelay:     5 * time.Millisecond,
		PoolWaitTimeout:   2 * time.Second,
		SSL:               SSLNone,
	})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	return p
}

func testEmail() Email {
	return Email{
		From:    "sender@example.com",
		To:      []string{"recipient@example.com"},
		Subject: "retry",
		Text:    []byte("body"),
	}
}

// I5 — the classifier over the §3.1 error inventory.
func TestCanRetryClassifier(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"451 stale connection", &textproto.Error{Code: 451, Msg: "4.4.2 Timeout"}, true},
		{"421 closing", &textproto.Error{Code: 421, Msg: "Too many concurrent SMTP connections"}, true},
		{"454 throttling", &textproto.Error{Code: 454, Msg: "Throttling failure"}, true},
		{"4yz wrapped", errorsWrap(&textproto.Error{Code: 450, Msg: "x"}), true},
		{"550 permanent", &textproto.Error{Code: 550, Msg: "5.1.1 no such user"}, false},
		{"535 auth", &textproto.Error{Code: 535, Msg: "Authentication Credentials Invalid"}, false},
		{"554 rejected", &textproto.Error{Code: 554, Msg: "Message rejected"}, false},
		{"net.Error timeout", &net.DNSError{IsTimeout: true}, true},
		{"*net.OpError", &net.OpError{Op: "dial", Err: errors.New("refused")}, true},
		{"io.EOF", io.EOF, true},
		{"wrapped io.EOF", errorsWrap(io.EOF), true},
		{"ErrPoolClosed", ErrPoolClosed, false},
		{"ErrMaybeDelivered", ErrMaybeDelivered, false},
		{"ErrMaybeDelivered wrapping a 4yz", errorsWrap(errorsJoin(ErrMaybeDelivered, &textproto.Error{Code: 451, Msg: "x"})), false},
		{"ErrPoolWaitTimeout", ErrPoolWaitTimeout, true},
		{"plain string error", errors.New("something else"), false},
	}
	for _, c := range cases {
		if got := canRetry(c.err); got != c.want {
			t.Errorf("%s: canRetry = %v, want %v", c.name, got, c.want)
		}
	}

	// Address-parse failures are permanent: conn.send returns retry=false for them
	// before any SMTP traffic (D11).
	if _, err := combineEmails([]string{"not an address"}); err == nil {
		t.Fatal("combineEmails must reject a malformed address")
	}
	bad := Email{From: "nope", To: []string{"a@b.com"}}
	if _, err := bad.parseSender(); err == nil {
		t.Fatal("parseSender must reject a malformed sender")
	}
}

type wrapped struct{ err error }

func (w wrapped) Error() string  { return "wrapped: " + w.err.Error() }
func (w wrapped) Unwrap() error  { return w.err }
func errorsWrap(err error) error { return wrapped{err} }

type joined struct{ a, b error }

func (j joined) Error() string    { return j.a.Error() + ": " + j.b.Error() }
func (j joined) Unwrap() []error  { return []error{j.a, j.b} }
func errorsJoin(a, b error) error { return joined{a, b} }

// I1 + I4 — a 451 on MAIL FROM from a connection the server then closes is
// retried, the message is delivered exactly once, and the retry does NOT reuse the
// failed connection: exactly two connections are opened.
func TestRetryOn4yzDeliversOnFreshConnection(t *testing.T) {
	srv, err := smtptest.New(func(ev smtptest.Event) smtptest.Action {
		if ev.Conn == 1 && ev.Cmd == "MAIL" {
			return smtptest.Action{Reply: stale451, Close: true}
		}
		return smtptest.Action{}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	p := newTestPool(t, srv, 2)
	defer p.Close()

	if err := p.Send(testEmail()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := len(srv.Messages()); got != 1 {
		t.Fatalf("delivered %d messages, want exactly 1", got)
	}
	if got := srv.Mails(); got != 2 {
		t.Fatalf("attempts (MAIL commands) = %d, want 2", got)
	}
	if got := srv.Conns(); got != 2 {
		t.Fatalf("connections opened = %d, want exactly 2 (the failed one must not be reused)", got)
	}
}

// D2 — a HEALTHY connection that returns 4yz (server did not close; RSET succeeds)
// may be reused for the retry: one connection, two attempts, delivered.
func TestRetryOn4yzReusesHealthyConnection(t *testing.T) {
	first := true
	srv, err := smtptest.New(func(ev smtptest.Event) smtptest.Action {
		if ev.Cmd == "MAIL" && first {
			first = false
			return smtptest.Action{Reply: "454 4.7.0 Throttling failure: Maximum sending rate exceeded."}
		}
		return smtptest.Action{}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	p := newTestPool(t, srv, 2)
	defer p.Close()

	if err := p.Send(testEmail()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := len(srv.Messages()); got != 1 {
		t.Fatalf("delivered %d, want 1", got)
	}
	if got := srv.Mails(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	if got := srv.Conns(); got != 1 {
		t.Fatalf("connections = %d, want 1 (a healthy 4yz connection is reused after backoff)", got)
	}
}

// I2 — a 5yz reply is not retried: one connection, one attempt, the error surfaces.
func TestNoRetryOn5yz(t *testing.T) {
	srv, err := smtptest.New(func(ev smtptest.Event) smtptest.Action {
		if ev.Cmd == "MAIL" {
			return smtptest.Action{Reply: "550 5.1.0 Sender rejected"}
		}
		return smtptest.Action{}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	p := newTestPool(t, srv, 3)
	defer p.Close()

	err = p.Send(testEmail())
	var tpErr *textproto.Error
	if !errors.As(err, &tpErr) || tpErr.Code != 550 {
		t.Fatalf("Send error = %v, want the 550", err)
	}
	if got := srv.Mails(); got != 1 {
		t.Fatalf("attempts = %d, want exactly 1", got)
	}
	if got := srv.Conns(); got != 1 {
		t.Fatalf("connections = %d, want 1", got)
	}
	if got := len(srv.Messages()); got != 0 {
		t.Fatalf("delivered %d, want 0", got)
	}
}

// I3 (pool half) — a message that fails 4yz on every attempt is tried exactly
// MaxMessageRetries times (attempts, not retries) and then surfaces the last error.
func TestExhaustedAfterMaxAttempts(t *testing.T) {
	srv, err := smtptest.New(func(ev smtptest.Event) smtptest.Action {
		if ev.Cmd == "MAIL" {
			return smtptest.Action{Reply: stale451, Close: true}
		}
		return smtptest.Action{}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	const attempts = 3
	p := newTestPool(t, srv, attempts)
	defer p.Close()

	err = p.Send(testEmail())
	var tpErr *textproto.Error
	if !errors.As(err, &tpErr) || tpErr.Code != 451 {
		t.Fatalf("Send error = %v, want the final 451", err)
	}
	if got := srv.Mails(); got != attempts {
		t.Fatalf("attempts = %d, want %d", got, attempts)
	}
	if got := len(srv.Messages()); got != 0 {
		t.Fatalf("delivered %d, want 0", got)
	}
}

// I6 / D10 — the server reads the whole message and drops the connection before
// acknowledging. The message MAY have been accepted, so smtppool must not retry:
// one attempt, an error, and the server holds exactly one copy.
func TestNoRetryAfterDataDrop(t *testing.T) {
	srv, err := smtptest.New(func(ev smtptest.Event) smtptest.Action {
		if ev.Cmd == "DATAEND" {
			return smtptest.Action{Drop: true}
		}
		return smtptest.Action{}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	p := newTestPool(t, srv, 3)
	defer p.Close()

	err = p.Send(testEmail())
	if err == nil {
		t.Fatal("Send must surface the post-DATA error")
	}
	if !errors.Is(err, ErrMaybeDelivered) {
		t.Fatalf("post-DATA error must carry ErrMaybeDelivered so callers can tell the duplicate-exposed case apart: %v", err)
	}
	if got := srv.Mails(); got != 1 {
		t.Fatalf("attempts = %d, want exactly 1 (no duplicate exposure)", got)
	}
	if got := len(srv.Messages()); got != 1 {
		t.Fatalf("server holds %d copies, want 1", got)
	}
}

// D10 (refined) — a 4yz REPLY to the end-of-data is an explicit rejection (RFC 5321
// §4.2.1), the shape of SES "454 Throttling failure", and is retried like any other 4yz:
// two attempts on the same healthy connection, Send succeeds, and the error is NOT
// ErrMaybeDelivered. The listener holds two payloads because it read the body twice;
// it rejected the first.
func TestRetryOn4yzReplyAfterData(t *testing.T) {
	first := true
	srv, err := smtptest.New(func(ev smtptest.Event) smtptest.Action {
		if ev.Cmd == "DATAEND" && first {
			first = false
			return smtptest.Action{Reply: "454 4.7.0 Throttling failure: Maximum sending rate exceeded."}
		}
		return smtptest.Action{}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	p := newTestPool(t, srv, 2)
	defer p.Close()

	if err := p.Send(testEmail()); err != nil {
		t.Fatalf("Send: %v (a 4yz reply after DATA must be retried)", err)
	}
	if got := srv.Mails(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	if got := len(srv.Messages()); got != 2 {
		t.Fatalf("payloads read = %d, want 2 (rejected, then accepted)", got)
	}
	if got := srv.Conns(); got != 1 {
		t.Fatalf("connections = %d, want 1 (healthy connection reused)", got)
	}
}

// D10 (refined) — exhausting on a post-DATA 4yz reply surfaces the reply itself, not
// ErrMaybeDelivered: the server said the action did not occur, so the caller must not
// treat the recipient as possibly reached. A 5yz reply there is not retried at all.
func TestPostDataReplyIsNeverMaybeDelivered(t *testing.T) {
	srv, err := smtptest.New(func(ev smtptest.Event) smtptest.Action {
		if ev.Cmd == "DATAEND" {
			return smtptest.Action{Reply: "454 4.7.0 Throttling failure: Maximum sending rate exceeded."}
		}
		return smtptest.Action{}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	p := newTestPool(t, srv, 2)
	defer p.Close()

	err = p.Send(testEmail())
	var tpErr *textproto.Error
	if !errors.As(err, &tpErr) || tpErr.Code != 454 {
		t.Fatalf("err = %v, want the 454", err)
	}
	if errors.Is(err, ErrMaybeDelivered) {
		t.Fatalf("a 4yz reply after DATA is an explicit rejection and must not be ErrMaybeDelivered: %v", err)
	}
	if got := srv.Mails(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}

	srv5, err := smtptest.New(func(ev smtptest.Event) smtptest.Action {
		if ev.Cmd == "DATAEND" {
			return smtptest.Action{Reply: "554 5.6.0 Message rejected"}
		}
		return smtptest.Action{}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv5.Close()
	p5 := newTestPool(t, srv5, 3)
	defer p5.Close()
	err = p5.Send(testEmail())
	if !errors.As(err, &tpErr) || tpErr.Code != 554 || errors.Is(err, ErrMaybeDelivered) {
		t.Fatalf("err = %v, want a bare 554", err)
	}
	if got := srv5.Mails(); got != 1 {
		t.Fatalf("5yz after DATA: attempts = %d, want 1", got)
	}
}

// D11 — a closed pool fails fast; it is never retried with backoff.
func TestPoolClosedNotRetried(t *testing.T) {
	srv, err := smtptest.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	p := newTestPool(t, srv, 3)
	p.opt.MsgRetryDelay = 500 * time.Millisecond
	p.Close()

	start := time.Now()
	err = p.Send(testEmail())
	if !errors.Is(err, ErrPoolClosed) {
		t.Fatalf("err = %v, want ErrPoolClosed", err)
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatal("a closed pool must not back off before failing")
	}
	if got := srv.Conns(); got != 0 {
		t.Fatalf("connections = %d, want 0", got)
	}
}

// D11 (review F4) -- a malformed recipient is permanent: Send returns without a retry
// delay and without a single MAIL command.
func TestMalformedAddressNotRetried(t *testing.T) {
	srv, err := smtptest.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	p := newTestPool(t, srv, 3)
	p.opt.MsgRetryDelay = 300 * time.Millisecond
	defer p.Close()

	e := testEmail()
	e.To = []string{"not an address"}
	start := time.Now()
	if err := p.Send(e); err == nil {
		t.Fatal("Send must reject a malformed recipient")
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatal("a permanent address error must not back off")
	}
	if got := srv.Mails(); got != 0 {
		t.Fatalf("MAIL commands = %d, want 0", got)
	}
}
