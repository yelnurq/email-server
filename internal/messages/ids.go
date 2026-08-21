package messages

import (
	"crypto/rand"
	"encoding/base32"
	"strings"

	"github.com/google/uuid"
)

// NewPublicID returns a stable public message identifier ("msg_..."), safe to
// expose in APIs instead of database ids.
func NewPublicID() string {
	var raw [20]byte
	_, _ = rand.Read(raw[:])
	return "msg_" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:]))
}

// NewRFCMessageID returns a new RFC 5322 Message-ID scoped to the sender
// domain, in the platform's canonical bare form (no angle brackets — those
// are wire format, added only by the MIME renderer; see
// mailaddr.NormalizeMessageID).
func NewRFCMessageID(domain string) string {
	return uuid.NewString() + "@" + strings.ToLower(domain)
}
