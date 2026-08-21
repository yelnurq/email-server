package mailservice

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"
)

// AttachmentPart is one file to embed into an outgoing message.
type AttachmentPart struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Envelope carries everything needed to render one RFC822 message. The
// platform composes text mail; HTML is accepted for the Email API path.
type Envelope struct {
	From        string
	FromDisplay string
	To          []string
	Cc          []string
	Subject     string
	Date        time.Time
	RFCID       string // Message-ID, e.g. <abc@domain>
	InReplyTo   string
	References  string
	Text        string
	HTML        string
	Attachments []AttachmentPart
}

// encodeWord RFC2047-encodes a header word when it contains non-ASCII.
func encodeWord(s string) string {
	return mime.QEncoding.Encode("utf-8", s)
}

func formatAddress(display, email string) string {
	if display == "" {
		return "<" + email + ">"
	}
	return encodeWord(display) + " <" + email + ">"
}

func writeB64Body(buf *bytes.Buffer, data []byte) {
	enc := base64.StdEncoding.EncodeToString(data)
	for len(enc) > 76 {
		buf.WriteString(enc[:76])
		buf.WriteString("\r\n")
		enc = enc[76:]
	}
	buf.WriteString(enc)
	buf.WriteString("\r\n")
}

// BuildMessage renders the canonical RFC822 bytes for an envelope: base64
// text (and optional HTML alternative) plus base64 attachments in
// multipart/mixed. Base64 transfer encoding sidesteps line-length and
// charset pitfalls for Cyrillic/Kazakh content.
func BuildMessage(env Envelope) []byte {
	var buf bytes.Buffer
	h := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&buf, "%s: %s\r\n", k, v)
		}
	}
	h("From", formatAddress(env.FromDisplay, env.From))
	if len(env.To) > 0 {
		h("To", "<"+strings.Join(env.To, ">, <")+">")
	}
	if len(env.Cc) > 0 {
		h("Cc", "<"+strings.Join(env.Cc, ">, <")+">")
	}
	h("Subject", encodeWord(env.Subject))
	date := env.Date
	if date.IsZero() {
		date = time.Now().UTC()
	}
	h("Date", date.Format(time.RFC1123Z))
	h("Message-ID", env.RFCID)
	h("In-Reply-To", env.InReplyTo)
	h("References", env.References)
	h("MIME-Version", "1.0")

	// Inner body: plain text, or alternative when HTML is present.
	var body bytes.Buffer
	bodyType := "text/plain; charset=utf-8"
	bodyHeaderExtra := "Content-Transfer-Encoding: base64\r\n"
	switch {
	case env.HTML != "" && env.Text != "":
		altw := multipart.NewWriter(&body)
		bodyType = `multipart/alternative; boundary="` + altw.Boundary() + `"`
		bodyHeaderExtra = ""
		pw, _ := altw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {"text/plain; charset=utf-8"},
			"Content-Transfer-Encoding": {"base64"},
		})
		var tmp bytes.Buffer
		writeB64Body(&tmp, []byte(env.Text))
		pw.Write(tmp.Bytes())
		hw, _ := altw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {"text/html; charset=utf-8"},
			"Content-Transfer-Encoding": {"base64"},
		})
		tmp.Reset()
		writeB64Body(&tmp, []byte(env.HTML))
		hw.Write(tmp.Bytes())
		altw.Close()
	case env.HTML != "":
		bodyType = "text/html; charset=utf-8"
		writeB64Body(&body, []byte(env.HTML))
	default:
		writeB64Body(&body, []byte(env.Text))
	}

	if len(env.Attachments) == 0 {
		h("Content-Type", bodyType)
		if bodyHeaderExtra != "" {
			buf.WriteString(bodyHeaderExtra)
		}
		buf.WriteString("\r\n")
		buf.Write(body.Bytes())
		return buf.Bytes()
	}

	mw := multipart.NewWriter(&buf)
	h("Content-Type", `multipart/mixed; boundary="`+mw.Boundary()+`"`)
	buf.WriteString("\r\n")
	bh := textproto.MIMEHeader{"Content-Type": {bodyType}}
	if bodyHeaderExtra != "" {
		bh.Set("Content-Transfer-Encoding", "base64")
	}
	bw, _ := mw.CreatePart(bh)
	bw.Write(body.Bytes())
	for _, a := range env.Attachments {
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		name := encodeWord(a.Filename)
		aw, _ := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {ct + `; name="` + name + `"`},
			"Content-Disposition":       {`attachment; filename="` + name + `"`},
			"Content-Transfer-Encoding": {"base64"},
		})
		var tmp bytes.Buffer
		writeB64Body(&tmp, a.Data)
		aw.Write(tmp.Bytes())
	}
	mw.Close()
	return buf.Bytes()
}
