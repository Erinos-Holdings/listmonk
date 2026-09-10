package smtppool

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"testing"
	"time"
)

const (
	smtpAddr = "localhost:1025"
	apiURL   = "http://localhost:8025"
)

var (
	reqTimeout = 3 * time.Second
)

// mailhogAvailable gates the upstream MailHog-backed tests. The erinos fork's own
// retry tests (retry_test.go) run against the in-process smtptest listener and
// need no external binary; without MailHog on PATH the upstream tests skip
// instead of failing the package.
var mailhogAvailable bool

func requireMailhog(t *testing.T) {
	t.Helper()
	if !mailhogAvailable {
		t.Skip("mailhog not available (set MAILHOG or put it on PATH)")
	}
}

func TestMain(m *testing.M) {
	// Start MailHog server.
	cmdPath := os.Getenv("MAILHOG")
	if cmdPath == "" {
		cmdPath = "mailhog"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := exec.CommandContext(ctx, cmdPath)
	if err := srv.Start(); err != nil {
		fmt.Printf("mailhog not started (%v); MailHog-backed tests will be skipped\n", err)
		os.Exit(m.Run())
	}
	mailhogAvailable = true

	// Wait for MailHog to be ready
	if !waitForServer(reqTimeout) {
		fmt.Println("mailhog start timed out")
		os.Exit(1)
	}

	code := m.Run()

	// Stop MailHog
	srv.Process.Kill()
	os.Exit(code)
}

func waitForServer(timeout time.Duration) bool {
	start := time.Now()
	for {
		conn, err := net.DialTimeout("tcp", smtpAddr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		if time.Since(start) > timeout {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func clearServer() {
	http.DefaultClient.Do(&http.Request{
		Method: "DELETE",
		URL:    mustParse(apiURL + "/api/v1/messages"),
	})
}

func mustParse(url string) *neturl.URL {
	u, _ := neturl.Parse(url)
	return u
}

func getMessageCount(t *testing.T) int {
	resp, err := http.Get(apiURL + "/api/v2/messages")
	if err != nil {
		t.Fatalf("error getting messages: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Count int
		Items []interface{}
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("error decoding response: %v", err)
	}
	return result.Count
}

func TestSendEmail(t *testing.T) {
	requireMailhog(t)
	clearServer()

	pool, err := New(Opt{
		Host:            "localhost",
		Port:            1025,
		MaxConns:        3,
		PoolWaitTimeout: 2 * time.Second,
		SSL:             SSLNone,
	})
	if err != nil {
		t.Fatalf("error creating pool: %v", err)
	}
	defer pool.Close()

	email := Email{
		From:    "sender@example.com",
		To:      []string{"recipient@example.com"},
		Subject: "Test Subject",
		Text:    []byte("Test Body"),
	}

	if err := pool.Send(email); err != nil {
		t.Fatalf("error sending email: %v", err)
	}

	// Verify email arrival.
	deadline := time.Now().Add(reqTimeout)
	for time.Now().Before(deadline) {
		if getMessageCount(t) > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Error("email not received by server")
}

func TestConnectionPooling(t *testing.T) {
	requireMailhog(t)
	clearServer()

	pool, err := New(Opt{
		Host:            "localhost",
		Port:            1025,
		MaxConns:        2,
		PoolWaitTimeout: 2 * time.Second,
		SSL:             SSLNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Send more emails than pool size
	for i := range 5 {
		go func() {
			email := Email{
				From:    fmt.Sprintf("sender%d@example.com", i),
				To:      []string{"recipient@example.com"},
				Subject: "Concurrent Test",
				Text:    []byte("Concurrent Body"),
			}
			pool.Send(email)
		}()
	}

	time.Sleep(2 * time.Second)
	if count := getMessageCount(t); count != 5 {
		t.Errorf("expected 5 messages, got %d", count)
	}
}

func TestPoolClose(t *testing.T) {
	requireMailhog(t)
	pool, err := New(Opt{
		Host:            "localhost",
		Port:            1025,
		MaxConns:        1,
		PoolWaitTimeout: 2 * time.Second,
		SSL:             SSLNone,
	})
	if err != nil {
		t.Fatal(err)
	}

	pool.Close()

	err = pool.Send(Email{
		From: "test@example.com",
		To:   []string{"recipient@example.com"},
	})
	if err == nil {
		t.Error("expected error when sending after pool closed")
	}
}

func TestSendInvalidEmail(t *testing.T) {
	requireMailhog(t)
	clearServer()

	pool, err := New(Opt{
		Host:            "localhost",
		Port:            1025,
		MaxConns:        1,
		PoolWaitTimeout: 2 * time.Second,
		SSL:             SSLNone,
	})
	if err != nil {
		t.Fatalf("error creating pool: %v", err)
	}
	defer pool.Close()

	// Test with invalid From address
	invalidFromEmail := Email{
		From:    "invalid-email-address",
		To:      []string{"recipient@example.com"},
		Subject: "Test Invalid From",
		Text:    []byte("Test Body"),
	}

	if err := pool.Send(invalidFromEmail); err == nil {
		t.Error("expected error when sending email with invalid From address")
	}

	// Test with invalid To address
	invalidToEmail := Email{
		From:    "sender@example.com",
		To:      []string{"invalid-recipient"},
		Subject: "Test Invalid To",
		Text:    []byte("Test Body"),
	}

	if err = pool.Send(invalidToEmail); err == nil {
		t.Error("expected error when sending email with invalid To address")
	}

	// Verify no emails were actually sent
	if count := getMessageCount(t); count != 0 {
		t.Errorf("expected 0 messages, got %d", count)
	}
}
