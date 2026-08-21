package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"strings"
)

// imapSession is a tiny IMAP client: enough to prove that the mailbox the
// webmail and JMAP interfaces serve is the same one mail clients see, and
// that flags set through either side are visible to the other.
type imapSession struct {
	conn *tls.Conn
	r    *bufio.Reader
	n    int
}

func imapDial(addr string, insecure bool) (*imapSession, error) {
	host := addr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		host = addr[:i]
	}
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, InsecureSkipVerify: insecure})
	if err != nil {
		return nil, err
	}
	s := &imapSession{conn: conn, r: bufio.NewReader(conn)}
	if _, err := s.r.ReadString('\n'); err != nil { // greeting
		return nil, err
	}
	return s, nil
}

// cmd issues one tagged command and collects the response up to its tag.
func (s *imapSession) cmd(format string, args ...any) (string, error) {
	s.n++
	tag := fmt.Sprintf("a%03d", s.n)
	line := tag + " " + fmt.Sprintf(format, args...) + "\r\n"
	if _, err := s.conn.Write([]byte(line)); err != nil {
		return "", err
	}
	var sb strings.Builder
	for {
		l, err := s.r.ReadString('\n')
		if err != nil {
			return sb.String(), err
		}
		sb.WriteString(l)
		if strings.HasPrefix(l, tag+" ") {
			if strings.Contains(l, tag+" OK") {
				return sb.String(), nil
			}
			return sb.String(), fmt.Errorf("imap: %s", strings.TrimSpace(l))
		}
	}
}

func (s *imapSession) Close() { _ = s.conn.Close() }

// imapCheck logs into INBOX, locates a message by subject and reports its
// flags — optionally adding one first. Used to prove that webmail, JMAP and
// IMAP operate on the same mailbox and the same flag state.
func imapCheck(addr, user, pass, subject, addFlag string, insecure bool) error {
	s, err := imapDial(addr, insecure)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer s.Close()
	if _, err := s.cmd(`LOGIN "%s" "%s"`, user, pass); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	sel, err := s.cmd("SELECT INBOX")
	if err != nil {
		return fmt.Errorf("select: %w", err)
	}
	fmt.Print(summarizeSelect(sel))

	if subject == "" {
		return nil
	}
	res, err := s.cmd(`SEARCH HEADER SUBJECT "%s"`, subject)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	seq := firstSearchHit(res)
	if seq == "" {
		return fmt.Errorf("message with subject %q not found over IMAP", subject)
	}
	fmt.Printf("IMAP FOUND: seq=%s subject=%q\n", seq, subject)

	if addFlag != "" {
		if _, err := s.cmd(`STORE %s +FLAGS (%s)`, seq, addFlag); err != nil {
			return fmt.Errorf("store: %w", err)
		}
		fmt.Printf("IMAP SET: %s on seq %s\n", addFlag, seq)
	}
	fetched, err := s.cmd(`FETCH %s (FLAGS BODY.PEEK[HEADER.FIELDS (MESSAGE-ID SUBJECT)])`, seq)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	for _, line := range strings.Split(fetched, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "FLAGS") || strings.HasPrefix(strings.ToLower(line), "message-id:") {
			fmt.Println("IMAP:", line)
		}
	}
	return nil
}

func summarizeSelect(resp string) string {
	var sb strings.Builder
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, "EXISTS") || strings.HasSuffix(line, "RECENT") {
			sb.WriteString("IMAP INBOX: " + line + "\n")
		}
	}
	return sb.String()
}

func firstSearchHit(resp string) string {
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "* SEARCH") {
			fields := strings.Fields(strings.TrimPrefix(line, "* SEARCH"))
			if len(fields) > 0 {
				return fields[len(fields)-1] // most recent match
			}
		}
	}
	return ""
}
