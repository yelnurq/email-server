// Package webhooks fans platform events out to subscriber endpoints with
// HMAC-SHA256 signed deliveries and exponential-backoff retries.
package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Sign computes the v1 signature for a delivery: HMAC-SHA256 over
// "<unix-timestamp>.<body>" keyed with the webhook secret.
func Sign(secret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a v1 signature (constant-time). Receivers can copy this.
func Verify(secret string, timestamp int64, body []byte, signature string) bool {
	return hmac.Equal([]byte(Sign(secret, timestamp, body)), []byte(signature))
}
