// sesim: a fake SMTP listener that behaves like SES for the SEND-RETRY-SPEC gates.
//
//	-idle D      after D of inactivity on an open session, write "451 4.4.2 Timeout waiting
//	             for data from client." and close it (the SES idle behaviour behind hazard 56)
//	-first451 N  the first N connections reply 451 + close on their first MAIL FROM (G2)
//	-addr        listen address (default 0.0.0.0:2525)
//	-stats       HTTP address for GET /stats (default 127.0.0.1:2580); GET /reset zeroes counters
//
// Counters: conns, mails (MAIL commands = delivery attempts), accepted (250 after end-of-data),
// idle451 (sessions closed by the idle rule), first451 (scripted MAIL rejections), and
// per-recipient accepted counts so "every recipient exactly once" is checkable.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	mu        sync.Mutex
	conns     int
	mails     int
	accepted  int
	idle451   int
	first451  int
	perRecip  = map[string]int{}
	idleDur   time.Duration
	first451N int
)

func reset() {
	mu.Lock()
	defer mu.Unlock()
	conns, mails, accepted, idle451, first451 = 0, 0, 0, 0, 0
	perRecip = map[string]int{}
}

func stats() map[string]any {
	mu.Lock()
	defer mu.Unlock()
	dups, uniq := 0, 0
	for _, n := range perRecip {
		uniq++
		if n > 1 {
			dups += n - 1
		}
	}
	return map[string]any{
		"conns": conns, "mails": mails, "accepted": accepted, "idle451": idle451, "first451": first451,
		"unique_recipients": uniq, "duplicate_deliveries": dups,
	}
}

func main() {
	addr := flag.String("addr", "0.0.0.0:2525", "smtp listen address")
	statsAddr := flag.String("stats", "127.0.0.1:2580", "stats http address")
	flag.DurationVar(&idleDur, "idle", 0, "idle timeout after which a session gets 451 + close (0 = never)")
	flag.IntVar(&first451N, "first451", 0, "first N connections get 451 + close on their first MAIL")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("sesim listening on %s (idle=%s first451=%d); stats on http://%s/stats", *addr, idleDur, first451N, *statsAddr)

	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(stats()) })
	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) { reset(); fmt.Fprintln(w, "ok") })
	go func() { log.Fatal(http.ListenAndServe(*statsAddr, nil)) }()

	for {
		c, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		mu.Lock()
		conns++
		n := conns
		mu.Unlock()
		go handle(c, n)
	}
}

func handle(c net.Conn, n int) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	say := func(s string) bool {
		w.WriteString(s + "\r\n")
		return w.Flush() == nil
	}
	if !say("220 sesim ESMTP") {
		return
	}
	firstMail := true
	var rcpts []string
	for {
		if idleDur > 0 {
			c.SetReadDeadline(time.Now().Add(idleDur))
		}
		line, err := r.ReadString('\n')
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				mu.Lock()
				idle451++
				mu.Unlock()
				say("451 4.4.2 Timeout waiting for data from client.")
			}
			return
		}
		line = strings.TrimRight(line, "\r\n")
		verb, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO":
			if !say("250-sesim\r\n250-8BITMIME\r\n250 OK") {
				return
			}
		case "HELO", "NOOP":
			say("250 OK")
		case "RSET":
			rcpts = nil
			say("250 OK")
		case "QUIT":
			say("221 Bye")
			return
		case "MAIL":
			mu.Lock()
			mails++
			mu.Unlock()
			if firstMail && n <= first451N {
				firstMail = false
				mu.Lock()
				first451++
				mu.Unlock()
				say("451 4.4.2 Timeout waiting for data from client.")
				return
			}
			firstMail = false
			rcpts = nil
			say("250 OK")
		case "RCPT":
			a := strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(arg), "to:<"), ">")
			rcpts = append(rcpts, a)
			say("250 OK")
		case "DATA":
			say("354 End data with <CR><LF>.<CR><LF>")
			for {
				if idleDur > 0 {
					c.SetReadDeadline(time.Now().Add(idleDur))
				}
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" || l == ".\n" {
					break
				}
			}
			mu.Lock()
			accepted++
			for _, a := range rcpts {
				perRecip[a]++
			}
			mu.Unlock()
			say("250 OK queued")
		default:
			say("500 unrecognized command")
		}
	}
}
