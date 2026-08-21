// Command smtpsink is a controlled SMTP target for delivery tests. It lets
// the outbound path be exercised without depending on a public mail
// provider: the response for each recipient is chosen by its local part, so
// one target can produce acceptance, temporary failure and permanent
// rejection deterministically.
//
// Recipient conventions (local part prefix):
//
//	ok…      → 250 accepted
//	defer…   → 451 4.3.2 temporary failure (sender must retry)
//	bounce…  → 550 5.1.1 permanent rejection (sender must bounce)
//	slow…    → accepted after a delay (timeout behaviour)
//
// Accepted messages are appended to -log as one JSON object per message so
// tests can assert on what actually arrived.
//
//	go run ./cmd/smtpsink -addr :2526 -log bin/sink.jsonl
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type received struct {
	At         string   `json:"at"`
	From       string   `json:"from"`
	To         []string `json:"to"`
	SizeBytes  int      `json:"size_bytes"`
	Subject    string   `json:"subject"`
	MessageID  string   `json:"message_id"`
	RemoteAddr string   `json:"remote_addr"`
}

var (
	logMu   sync.Mutex
	logPath string
	slowFor time.Duration
)

func main() {
	addr := flag.String("addr", ":2526", "listen address")
	flag.StringVar(&logPath, "log", "", "append accepted messages as JSON lines to this file")
	flag.DurationVar(&slowFor, "slow-delay", 5*time.Second, "delay applied to slow… recipients")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	log.Printf("smtpsink listening on %s (log=%q)", *addr, logPath)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go handle(conn)
	}
}

// verdictFor picks the SMTP response for one recipient from its local part.
func verdictFor(rcpt string) (code string, delay time.Duration) {
	local := strings.ToLower(rcpt)
	if i := strings.IndexByte(local, '@'); i >= 0 {
		local = local[:i]
	}
	local = strings.TrimPrefix(local, "<")
	switch {
	case strings.HasPrefix(local, "defer"):
		return "451 4.3.2 Service temporarily unavailable, try again later", 0
	case strings.HasPrefix(local, "bounce"):
		return "550 5.1.1 <" + rcpt + ">: Recipient address rejected: User unknown", 0
	case strings.HasPrefix(local, "slow"):
		return "250 2.1.5 Ok", slowFor
	default:
		return "250 2.1.5 Ok", 0
	}
}

func handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	say := func(s string) {
		fmt.Fprintf(w, "%s\r\n", s)
		_ = w.Flush()
	}

	say("220 smtpsink.test ESMTP controlled test target")
	var from string
	var rcpts []string

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			say("250-smtpsink.test greets you")
			say("250-SIZE 52428800")
			say("250 8BITMIME")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			// Strip ESMTP parameters (SIZE=, BODY=…) that follow the path.
			from = strings.Trim(splitPath(line[len("MAIL FROM:"):]), "<>")
			rcpts = nil
			say("250 2.1.0 Ok")
		case strings.HasPrefix(upper, "RCPT TO:"):
			rcpt := strings.Trim(splitPath(line[len("RCPT TO:"):]), "<>")
			code, delay := verdictFor(rcpt)
			if delay > 0 {
				time.Sleep(delay)
			}
			if strings.HasPrefix(code, "250") {
				rcpts = append(rcpts, rcpt)
			}
			say(code)
		case strings.HasPrefix(upper, "DATA"):
			if len(rcpts) == 0 {
				say("554 5.5.1 No valid recipients")
				continue
			}
			say("354 End data with <CR><LF>.<CR><LF>")
			body, err := readDot(r)
			if err != nil {
				return
			}
			record(conn, from, rcpts, body)
			say("250 2.0.0 Ok: queued as sink-" + fmt.Sprint(time.Now().UnixNano()))
		case strings.HasPrefix(upper, "RSET"):
			from, rcpts = "", nil
			say("250 2.0.0 Ok")
		case strings.HasPrefix(upper, "NOOP"):
			say("250 2.0.0 Ok")
		case strings.HasPrefix(upper, "QUIT"):
			say("221 2.0.0 Bye")
			return
		case strings.HasPrefix(upper, "STARTTLS"):
			// Plain-text test target: decline so the sender continues
			// unencrypted instead of hanging on a TLS handshake.
			say("454 4.7.0 TLS not available")
		default:
			say("502 5.5.2 Command not implemented")
		}
	}
}

// splitPath returns the <path> part of a MAIL/RCPT argument, dropping any
// ESMTP parameters that follow it.
func splitPath(arg string) string {
	arg = strings.TrimSpace(arg)
	if i := strings.IndexByte(arg, '>'); i >= 0 {
		return arg[:i+1]
	}
	if i := strings.IndexByte(arg, ' '); i > 0 {
		return arg[:i]
	}
	return arg
}

// readDot reads a DATA payload terminated by a lone dot.
func readDot(r *bufio.Reader) (string, error) {
	var sb strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		if line == ".\r\n" || line == ".\n" {
			return sb.String(), nil
		}
		if strings.HasPrefix(line, "..") {
			line = line[1:] // undo dot-stuffing
		}
		sb.WriteString(line)
		if sb.Len() > 64<<20 {
			return "", io.ErrUnexpectedEOF
		}
	}
}

func record(conn net.Conn, from string, rcpts []string, body string) {
	rec := received{
		At: time.Now().UTC().Format(time.RFC3339), From: from, To: rcpts,
		SizeBytes: len(body), RemoteAddr: conn.RemoteAddr().String(),
	}
	for _, line := range strings.Split(body, "\n") {
		l := strings.TrimRight(line, "\r")
		if l == "" {
			break // end of headers
		}
		lower := strings.ToLower(l)
		if strings.HasPrefix(lower, "subject:") {
			rec.Subject = strings.TrimSpace(l[len("subject:"):])
		}
		if strings.HasPrefix(lower, "message-id:") {
			rec.MessageID = strings.TrimSpace(l[len("message-id:"):])
		}
	}
	log.Printf("accepted from=%s to=%v subject=%q size=%d", from, rcpts, rec.Subject, rec.SizeBytes)
	if logPath == "" {
		return
	}
	raw, _ := json.Marshal(rec)
	logMu.Lock()
	defer logMu.Unlock()
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("log open: %v", err)
		return
	}
	defer f.Close()
	_, _ = f.Write(append(raw, '\n'))
}
