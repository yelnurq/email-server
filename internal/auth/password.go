package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. Kept as constants so the encoded PHC string records
// the exact parameters used, allowing future upgrades with rehash-on-login.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB → 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword derives an Argon2id hash and encodes it as a PHC string:
// $argon2id$v=19$m=65536,t=3,p=4$<salt-b64>$<hash-b64>
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// ErrPasswordMismatch is returned when a password does not match its hash.
var ErrPasswordMismatch = errors.New("password does not match")

// VerifyPassword checks password against an encoded PHC Argon2id string using
// constant-time comparison. The parameters are read from the stored string so
// old hashes keep verifying after parameter upgrades.
func VerifyPassword(password, encoded string) error {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return errors.New("unsupported password hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return errors.New("malformed password hash version")
	}
	var mem, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &time, &threads); err != nil {
		return errors.New("malformed password hash parameters")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return errors.New("malformed password hash salt")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return errors.New("malformed password hash digest")
	}
	got := argon2.IDKey([]byte(password), salt, time, mem, threads, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrPasswordMismatch
	}
	return nil
}
