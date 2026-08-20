// Command mailcheck is an operational smoke test for the mail core: it
// submits a message over authenticated SMTP (587 + STARTTLS) and verifies by
// JMAP that the recipient's mail-core mailbox received it. Used by the SMTP
// E2E check and for troubleshooting a deployment.
//
//	go run ./cmd/mailcheck \
//	  -smtp localhost:1587 -http http://localhost:8180 \
//	  -from admin@company.test -from-pass <smtp credential password> \
//	  -to user1@company.test -to-pass <recipient credential password>
//
// Development note: the local Stalwart uses a self-signed TLS certificate,
// so certificate verification is skipped (-insecure defaults to true). Never
// disable verification against production hosts.
package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"
)

func main() {
	smtpAddr := flag.String("smtp", "localhost:1587", "SMTP submission host:port (STARTTLS)")
	httpBase := flag.String("http", "http://localhost:8180", "mail core HTTP base URL (JMAP)")
	from := flag.String("from", "", "sender address (SMTP AUTH login)")
	fromPass := flag.String("from-pass", "", "sender password (SMTP credential)")
	to := flag.String("to", "", "recipient address")
	toPass := flag.String("to-pass", "", "recipient password for JMAP verification (optional)")
	insecure := flag.Bool("insecure", true, "skip TLS verification (self-signed dev certificate)")
	flag.Parse()

	if *from == "" || *fromPass == "" || *to == "" {
		fmt.Fprintln(os.Stderr, "usage: mailcheck -from <addr> -from-pass <pw> -to <addr> [-to-pass <pw>]")
		os.Exit(2)
	}

	subject := fmt.Sprintf("mailcheck %d", time.Now().UTC().UnixNano())
	if err := submit(*smtpAddr, *from, *fromPass, *to, subject, *insecure); err != nil {
		fmt.Fprintf(os.Stderr, "SMTP FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("SMTP OK: message accepted for %s (subject %q)\n", *to, subject)

	if *toPass == "" {
		fmt.Println("JMAP SKIPPED: no -to-pass provided")
		return
	}
	if err := verifyJMAP(*httpBase, *to, *toPass, subject); err != nil {
		fmt.Fprintf(os.Stderr, "JMAP FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("JMAP OK: message found in recipient mailbox")
}

// submit sends one message over SMTP submission with STARTTLS + AUTH PLAIN.
func submit(addr, from, pass, to, subject string, insecure bool) error {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer c.Close()
	if err := c.StartTLS(&tls.Config{ServerName: host, InsecureSkipVerify: insecure}); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}
	if err := c.Auth(smtp.PlainAuth("", from, pass, host)); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	msg := fmt.Sprintf("From: <%s>\r\nTo: <%s>\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%d@mailcheck>\r\n\r\nmail core smoke test\r\n",
		from, to, subject, time.Now().UTC().Format(time.RFC1123Z), time.Now().UnixNano())
	if _, err := wc.Write([]byte(msg)); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}
	return c.Quit()
}

// verifyJMAP polls the recipient's JMAP mailbox for the submitted subject.
func verifyJMAP(base, user, pass, subject string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	get := func(url string, out any) error {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.SetBasicAuth(user, pass)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		}
		return json.Unmarshal(raw, out)
	}

	var session struct {
		APIURL          string            `json:"apiUrl"`
		PrimaryAccounts map[string]string `json:"primaryAccounts"`
	}
	if err := get(strings.TrimRight(base, "/")+"/.well-known/jmap", &session); err != nil {
		return fmt.Errorf("session: %w", err)
	}
	accountID := session.PrimaryAccounts["urn:ietf:params:jmap:mail"]
	if accountID == "" {
		return fmt.Errorf("no primary mail account in JMAP session")
	}
	// The session's apiUrl carries the server's configured public hostname,
	// which may not resolve from the operator's machine (development). Reach
	// the API through the same base we used for the session instead.
	if i := strings.Index(session.APIURL, "/jmap"); i >= 0 {
		session.APIURL = strings.TrimRight(base, "/") + session.APIURL[i:]
	}

	query := map[string]any{
		"using": []string{"urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"},
		"methodCalls": []any{
			[]any{"Email/query", map[string]any{
				"accountId": accountID,
				"filter":    map[string]string{"subject": subject},
			}, "0"},
		},
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		raw, _ := json.Marshal(query)
		req, _ := http.NewRequest(http.MethodPost, session.APIURL, bytes.NewReader(raw))
		req.SetBasicAuth(user, pass)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && strings.Contains(string(body), `"ids":[`) &&
			!strings.Contains(string(body), `"ids":[]`) {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("message %q not found in recipient JMAP mailbox within 30s", subject)
}
