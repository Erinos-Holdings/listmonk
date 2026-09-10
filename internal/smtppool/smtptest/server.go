// Package smtptest is an in-process fake SMTP listener for exercising smtppool's
// retry and connection-reuse behaviour without a real mail server (erinos fork,
// SEND-RETRY-SPEC §4 — I1, I2, I4, I6 and the D2 reuse case). It speaks just enough
// of RFC 5321 for net/smtp: greeting, EHLO/HELO, MAIL, RCPT, DATA, RSET, NOOP, QUIT.
// A Hook scripts the reply to any command on any connection; everything else
// succeeds. The server counts connections and MAIL commands (one MAIL per delivery
// attempt), and keeps every DATA payload it read — including one it then dropped
// without acknowledging, which is exactly the duplicate-exposed case D10 names.
package smtptest

import (
	"net"
	"net/textproto"
	"strings"
	"sync"
)

// Event is a command the server received, offered to the Hook before replying.
type Event struct {
	// Conn is the 1-based ordinal of the connection the command arrived on.
	Conn int
	// Cmd is the SMTP verb ("MAIL", "RCPT", "DATA") or "DATAEND" once the message
	// body's terminating line has been read.
	Cmd string
	// Arg is the rest of the command line after the verb ("FROM:<a@b>").
	Arg string
}

// Action tells the server what to do instead of the default success reply.
// The zero Action means "reply as a healthy server would".
type Action struct {
	// Reply is a full reply line, e.g. "451 4.4.2 Timeout waiting for data from client".
	Reply string
	// Close closes the connection after writing Reply.
	Close bool
	// Drop closes the connection WITHOUT writing any reply.
	Drop bool
}

// Server is a fake SMTP listener on a loopback port.
type Server struct {
	// Hook scripts replies. Nil, or a Hook returning the zero Action, is a healthy server.
	Hook func(ev Event) Action

	ln net.Listener
	wg sync.WaitGroup

	mu       sync.Mutex
	conns    int
	mails    int
	messages [][]byte
}

// New starts a server on 127.0.0.1 on a free port.
func New(hook func(ev Event) Action) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{Hook: hook, ln: ln}
	s.wg.Add(1)
	go s.serve()
	return s, nil
}

// Host and Port are what to put in smtppool.Opt.
func (s *Server) Host() string { return s.ln.Addr().(*net.TCPAddr).IP.String() }
func (s *Server) Port() int    { return s.ln.Addr().(*net.TCPAddr).Port }

// Close stops listening and waits for the accept loop to exit. Open client
// connections are closed by their own handlers when the peer goes away.
func (s *Server) Close() {
	s.ln.Close()
	s.wg.Wait()
}

// Conns is the number of connections accepted so far.
func (s *Server) Conns() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns
}

// Mails is the number of MAIL commands received — one per delivery attempt.
func (s *Server) Mails() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mails
}

// Messages returns copies of every DATA payload read to its terminator, whether or
// not the server acknowledged it.
func (s *Server) Messages() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.messages))
	for i, m := range s.messages {
		out[i] = append([]byte(nil), m...)
	}
	return out
}

func (s *Server) serve() {
	defer s.wg.Done()
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns++
		n := s.conns
		s.mu.Unlock()
		go s.handle(c, n)
	}
}

func (s *Server) act(ev Event) Action {
	if s.Hook == nil {
		return Action{}
	}
	return s.Hook(ev)
}

func (s *Server) handle(c net.Conn, n int) {
	defer c.Close()
	tp := textproto.NewConn(c)

	if err := tp.PrintfLine("220 smtptest ESMTP"); err != nil {
		return
	}

	// reply writes the scripted or default line and reports whether the
	// connection should stay open.
	reply := func(ev Event, def string) bool {
		a := s.act(ev)
		if a.Drop {
			return false
		}
		line := def
		if a.Reply != "" {
			line = a.Reply
		}
		if err := tp.PrintfLine("%s", line); err != nil {
			return false
		}
		return !a.Close
	}

	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(line, " ")
		verb = strings.ToUpper(verb)

		switch verb {
		case "EHLO":
			if err := tp.PrintfLine("250-smtptest\r\n250 OK"); err != nil {
				return
			}
		case "HELO", "NOOP", "RSET":
			if err := tp.PrintfLine("250 OK"); err != nil {
				return
			}
		case "QUIT":
			_ = tp.PrintfLine("221 Bye")
			return
		case "MAIL":
			s.mu.Lock()
			s.mails++
			s.mu.Unlock()
			if !reply(Event{Conn: n, Cmd: "MAIL", Arg: arg}, "250 OK") {
				return
			}
		case "RCPT":
			if !reply(Event{Conn: n, Cmd: "RCPT", Arg: arg}, "250 OK") {
				return
			}
		case "DATA":
			if !reply(Event{Conn: n, Cmd: "DATA", Arg: arg}, "354 End data with <CR><LF>.<CR><LF>") {
				return
			}
			body, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.messages = append(s.messages, body)
			s.mu.Unlock()
			if !reply(Event{Conn: n, Cmd: "DATAEND"}, "250 OK queued") {
				return
			}
		default:
			if err := tp.PrintfLine("500 unrecognized command"); err != nil {
				return
			}
		}
	}
}
