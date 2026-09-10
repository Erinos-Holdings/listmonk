# smtppool — erinos fork (in-tree nested module)

This directory is `github.com/knadh/smtppool/v2` **v2.1.2** copied verbatim and patched;
listmonk's root `go.mod` points at it with a `replace` directive. It is a nested Go module
(own `go.mod`), so `go test ./internal/...` from the repo root does NOT run its tests —
run them from here (`cd internal/smtppool && go test ./...`); CI does the same.

Why in-tree rather than an org fork or `go mod vendor`: the patch is a few dozen lines that
rebase with the listmonk fork branch and are reviewed in the same diff; an org repo would
add a second CI pipeline and a pseudo-version bump per change, and vendoring would pull
every other dependency into the tree for one file. See integrations
`listmonk/SEND-RETRY-SPEC.md` §9.

Fork deltas against v2.1.2 (all in `pool.go`, tests in `retry_test.go`):

- `canRetry` treats an SMTP **4yz** reply (`*textproto.Error`) as retriable and **5yz** as
  permanent (SEND-RETRY-SPEC D1); `ErrPoolClosed` is never retried (D11); the pool wait
  timeout is a sentinel (`ErrPoolWaitTimeout`) and retriable.
- `combineEmails` / `parseSender` failures are permanent (were marked retriable).
- The DATA writer is closed exactly once: the deferred `w.Close()` no longer re-fires after
  an end-of-data error (textproto's DotWriter re-sends the terminator on a second Close and
  the stray reply poisons the next `RSET`).
- After the DATA terminator, an SMTP **reply** (4yz/5yz) is classified like any other reply
  (a 454 throttle there is retried); a **non-reply** error (write failure, EOF, timeout) is
  wrapped in `ErrMaybeDelivered` and never retried (D10 — duplicate safety).
- `smtptest/` — an in-process fake SMTP listener used by the retry tests and by
  listmonk's manager tests.
- `smtptest/sesim/` — a standalone SES-like listener for release rehearsals (SEND-RETRY-SPEC
  G2/G3): `-idle D` closes idle sessions with the SES 451, `-first451 N` rejects the first MAIL
  on the first N connections, `GET /stats` counts connections, MAILs, accepted and per-recipient
  deliveries. `go run ./smtptest/sesim -idle 8s` from this directory.
- `pool_test.go`'s MailHog-backed tests skip when MailHog is absent instead of failing.
- Dial address built with `net.JoinHostPort` (vet: IPv6).

Upgrading: diff the new upstream tag against v2.1.2, re-apply the deltas above, re-run
the tests here. Upstream: https://github.com/knadh/smtppool
