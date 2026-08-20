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

// NewRFCMessageID returns an RFC 5322 Message-ID scoped to the sender domain.
func NewRFCMessageID(domain string) string {
	return "<" + uuid.NewString() + "@" + domain + ">"
}
