package security

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yelnurq/email-server/internal/scanner"
)

const rawWithAttachment = "Subject: t\r\nContent-Type: multipart/mixed; boundary=B\r\n\r\n" +
	"--B\r\nContent-Disposition: attachment; filename=a.bin\r\n\r\ndata\r\n--B--\r\n"

func rspamdStub(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/checkv2" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
}

func TestRspamdSpamVerdict(t *testing.T) {
	srv := rspamdStub(t, `{"score":8.1,"required_score":15,"action":"add header",
		"symbols":{"PLATFORM_JUNK_TEST":{"score":8.0,"description":"test"}}}`)
	defer srv.Close()
	e := &Engine{Rspamd: &scanner.Rspamd{BaseURL: srv.URL}}
	v := Verdict{Action: ActionAllow}
	if err := e.scanExternal(context.Background(), &v, "u@x.test", []byte("Subject: hi\r\n\r\nbody")); err != nil {
		t.Fatal(err)
	}
	if v.Score < 41 || v.RspamdAction != "add header" || v.RspamdScore != 8.1 {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestRspamdMalwareVerdict(t *testing.T) {
	srv := rspamdStub(t, `{"score":16,"required_score":15,"action":"reject",
		"symbols":{"CLAM_VIRUS":{"score":0,"options":["Eicar-Signature"]}}}`)
	defer srv.Close()
	e := &Engine{Rspamd: &scanner.Rspamd{BaseURL: srv.URL}}
	v := Verdict{Action: ActionAllow}
	if err := e.scanExternal(context.Background(), &v, "u@x.test", []byte(rawWithAttachment)); err != nil {
		t.Fatal(err)
	}
	if v.Virus != "Eicar-Signature" || v.Reason != ReasonMalware || v.Score < 100 {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestClamdDownWithAttachmentDefers(t *testing.T) {
	// rspamd answered but reported its clamav connection failed.
	srv := rspamdStub(t, `{"score":0,"required_score":15,"action":"no action",
		"symbols":{"CLAM_VIRUS_FAIL":{"score":0}}}`)
	defer srv.Close()
	e := &Engine{Rspamd: &scanner.Rspamd{BaseURL: srv.URL}}
	v := Verdict{Action: ActionAllow}
	err := e.scanExternal(context.Background(), &v, "u@x.test", []byte(rawWithAttachment))
	if !errors.Is(err, ErrScanUnavailable) {
		t.Fatalf("err = %v, want ErrScanUnavailable", err)
	}
}

func TestBothScannersDownWithAttachmentDefers(t *testing.T) {
	e := &Engine{
		Rspamd: &scanner.Rspamd{BaseURL: "http://127.0.0.1:1"}, // refused
		Clamd:  &scanner.Clamd{Addr: "127.0.0.1:1"},
	}
	v := Verdict{Action: ActionAllow}
	err := e.scanExternal(context.Background(), &v, "u@x.test", []byte(rawWithAttachment))
	if !errors.Is(err, ErrScanUnavailable) {
		t.Fatalf("err = %v, want ErrScanUnavailable", err)
	}
}

func TestScannersDownWithoutAttachmentFailsOpen(t *testing.T) {
	e := &Engine{Rspamd: &scanner.Rspamd{BaseURL: "http://127.0.0.1:1"}}
	v := Verdict{Action: ActionAllow, Signals: []string{}}
	if err := e.scanExternal(context.Background(), &v, "u@x.test", []byte("Subject: hi\r\n\r\ntext only")); err != nil {
		t.Fatalf("plain text must fail open, got %v", err)
	}
	found := false
	for _, s := range v.Signals {
		if s == "rspamd-unavailable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing rspamd-unavailable signal: %+v", v.Signals)
	}
}

// fakeClamd speaks just enough of the clamd protocol for one INSTREAM scan.
func fakeClamd(t *testing.T, verdict string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				cmd, _ := r.ReadString('\x00')
				if strings.Contains(cmd, "PING") {
					c.Write([]byte("PONG\x00"))
					return
				}
				// Drain the INSTREAM chunks until the zero-length terminator.
				buf := make([]byte, 4)
				for {
					if _, err := readFull(r, buf); err != nil {
						return
					}
					n := int(buf[0])<<24 | int(buf[1])<<16 | int(buf[2])<<8 | int(buf[3])
					if n == 0 {
						break
					}
					if _, err := readFull(r, make([]byte, n)); err != nil {
						return
					}
				}
				c.Write([]byte("stream: " + verdict + "\x00"))
			}(conn)
		}
	}()
	return ln
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func TestClamdFallbackDetectsMalware(t *testing.T) {
	ln := fakeClamd(t, "Eicar-Test-Signature FOUND")
	defer ln.Close()
	e := &Engine{
		Rspamd: &scanner.Rspamd{BaseURL: "http://127.0.0.1:1"}, // down → fallback
		Clamd:  &scanner.Clamd{Addr: ln.Addr().String()},
	}
	v := Verdict{Action: ActionAllow, Signals: []string{}}
	if err := e.scanExternal(context.Background(), &v, "u@x.test", []byte(rawWithAttachment)); err != nil {
		t.Fatal(err)
	}
	if v.Virus != "Eicar-Test-Signature" || v.Reason != ReasonMalware {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestClamdFallbackCleanPasses(t *testing.T) {
	ln := fakeClamd(t, "OK")
	defer ln.Close()
	e := &Engine{
		Rspamd: &scanner.Rspamd{BaseURL: "http://127.0.0.1:1"},
		Clamd:  &scanner.Clamd{Addr: ln.Addr().String()},
	}
	v := Verdict{Action: ActionAllow, Signals: []string{}}
	if err := e.scanExternal(context.Background(), &v, "u@x.test", []byte(rawWithAttachment)); err != nil {
		t.Fatal(err)
	}
	if v.Virus != "" || v.Score != 0 {
		t.Fatalf("clean attachment scored: %+v", v)
	}
}

func TestClamdPing(t *testing.T) {
	ln := fakeClamd(t, "OK")
	defer ln.Close()
	c := &scanner.Clamd{Addr: ln.Addr().String()}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
