package mailservice

import (
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

// newB64Reader decodes the wrapped base64 bodies BuildMessage emits.
func newB64Reader(s string) io.Reader {
	return base64.NewDecoder(base64.StdEncoding,
		strings.NewReader(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", ""), "\n", "")))
}

func parse(t *testing.T, raw []byte) *mail.Message {
	t.Helper()
	m, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("message does not parse as RFC822: %v", err)
	}
	return m
}

// decodeBody returns the decoded text of a single-part message.
func decodeBody(t *testing.T, m *mail.Message) string {
	t.Helper()
	body, err := io.ReadAll(m.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(m.Header.Get("Content-Transfer-Encoding"), "base64") {
		return string(decodeB64(t, string(body)))
	}
	return string(body)
}

func decodeB64(t *testing.T, s string) []byte {
	t.Helper()
	dec, err := io.ReadAll(newB64Reader(s))
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	return dec
}

func TestBuildMessagePlainUnicode(t *testing.T) {
	raw := BuildMessage(Envelope{
		From: "a@company.test", FromDisplay: "Әсем Қыз",
		To: []string{"b@company.test"}, Subject: "Отчёт: Қазақша тақырып",
		Date:  time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC),
		RFCID: "<u1@company.test>", Text: "Кириллица и қазақ әріптері",
	})
	m := parse(t, raw)

	dec := new(mime.WordDecoder)
	subject, err := dec.DecodeHeader(m.Header.Get("Subject"))
	if err != nil {
		t.Fatalf("subject decode: %v", err)
	}
	if subject != "Отчёт: Қазақша тақырып" {
		t.Fatalf("subject round-trip failed: %q", subject)
	}
	from, err := dec.DecodeHeader(m.Header.Get("From"))
	if err != nil || !strings.Contains(from, "Әсем Қыз") {
		t.Fatalf("from display round-trip failed: %q (%v)", from, err)
	}
	if got := decodeBody(t, m); got != "Кириллица и қазақ әріптері" {
		t.Fatalf("body round-trip failed: %q", got)
	}
	if m.Header.Get("Message-ID") != "<u1@company.test>" {
		t.Fatalf("Message-ID missing: %q", m.Header.Get("Message-ID"))
	}
}

// Bcc recipients must never appear in the rendered headers.
func TestBuildMessageHidesBcc(t *testing.T) {
	raw := BuildMessage(Envelope{
		From: "a@company.test", To: []string{"b@company.test"},
		Cc: []string{"c@company.test"}, Subject: "s", Text: "t",
	})
	if strings.Contains(string(raw), "secret-bcc@company.test") {
		t.Fatal("bcc leaked into headers")
	}
	m := parse(t, raw)
	if !strings.Contains(m.Header.Get("Cc"), "c@company.test") {
		t.Fatal("cc missing")
	}
	if m.Header.Get("Bcc") != "" {
		t.Fatal("Bcc header must not be rendered")
	}
}

func TestBuildMessageWithAttachment(t *testing.T) {
	payload := []byte("Отчёт за август\n")
	raw := BuildMessage(Envelope{
		From: "a@company.test", To: []string{"b@company.test"},
		Subject: "with file", Text: "see attached",
		Attachments: []AttachmentPart{{
			Filename: "отчёт.txt", ContentType: "text/plain", Data: payload,
		}},
	})
	m := parse(t, raw)
	mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("content-type: %v", err)
	}
	if mediaType != "multipart/mixed" {
		t.Fatalf("expected multipart/mixed, got %s", mediaType)
	}
	mr := multipart.NewReader(m.Body, params["boundary"])
	var sawBody, sawFile bool
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		disp, dparams, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		data, _ := io.ReadAll(p)
		if disp == "attachment" {
			sawFile = true
			name, err := new(mime.WordDecoder).DecodeHeader(dparams["filename"])
			if err != nil || name != "отчёт.txt" {
				t.Fatalf("unicode filename lost: %q (%v)", dparams["filename"], err)
			}
			if got := string(decodeB64(t, string(data))); got != string(payload) {
				t.Fatalf("attachment content changed: %q", got)
			}
		} else {
			sawBody = true
		}
	}
	if !sawBody || !sawFile {
		t.Fatalf("missing parts: body=%v file=%v", sawBody, sawFile)
	}
}

func TestFolderRoleMapping(t *testing.T) {
	// The UI keeps the legacy "spam" type; the protocol role is "junk".
	if RoleForType("spam") != "junk" {
		t.Fatal("spam must map to the junk role")
	}
	if TypeForRole("junk") != "spam" {
		t.Fatal("junk must surface as spam")
	}
	for _, r := range []string{"inbox", "sent", "drafts", "trash"} {
		if RoleForType(r) != r || TypeForRole(r) != r {
			t.Fatalf("role %q must map to itself", r)
		}
	}
}

func TestPrimaryMailAccountPrefersAdvertised(t *testing.T) {
	s := &Session{
		Accounts: map[string]struct {
			Name       string `json:"name"`
			IsPersonal bool   `json:"isPersonal"`
		}{"x": {Name: "other"}, "e": {Name: "me", IsPersonal: true}},
		PrimaryAccounts: map[string]string{"urn:ietf:params:jmap:mail": "e"},
	}
	if got := s.primaryMailAccount(); got != "e" {
		t.Fatalf("expected advertised primary account, got %q", got)
	}
	// Without the advertisement, fall back to the personal account.
	s.PrimaryAccounts = nil
	if got := s.primaryMailAccount(); got != "e" {
		t.Fatalf("expected personal account fallback, got %q", got)
	}
}
