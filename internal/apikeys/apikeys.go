// Package apikeys implements API key issuance, verification and management.
// Raw secrets are shown exactly once at creation; only SHA-256 hashes are
// stored.
package apikeys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Prefix marks platform API keys in Authorization headers.
const Prefix = "mpk_"

// Key is a verified API key identity.
type Key struct {
	ID             string
	TenantID       string
	OrganizationID string
	Name           string
	Scopes         map[string]bool
}

// HasScope reports whether the key holds a scope.
func (k *Key) HasScope(scope string) bool { return k.Scopes[scope] }

// Generate returns a new raw key and its storage hash.
func Generate() (raw string, hash []byte) {
	var buf [30]byte
	_, _ = rand.Read(buf[:])
	raw = Prefix + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf[:]))
	sum := sha256.Sum256([]byte(raw))
	return raw, sum[:]
}

// ErrInvalidKey covers unknown, revoked and expired keys.
var ErrInvalidKey = errors.New("invalid api key")

// Service verifies keys against PostgreSQL.
type Service struct {
	Pool *pgxpool.Pool
}

// Verify resolves a raw key to its identity and stamps last_used_at.
func (s *Service) Verify(ctx context.Context, raw string) (*Key, error) {
	if !strings.HasPrefix(raw, Prefix) {
		return nil, ErrInvalidKey
	}
	sum := sha256.Sum256([]byte(raw))
	k := &Key{Scopes: map[string]bool{}}
	var scopes []string
	err := s.Pool.QueryRow(ctx, `
		SELECT id, tenant_id, organization_id, name, scopes
		FROM api_keys
		WHERE secret_hash = $1 AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > now())`,
		sum[:]).Scan(&k.ID, &k.TenantID, &k.OrganizationID, &k.Name, &scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidKey
	}
	if err != nil {
		return nil, err
	}
	for _, sc := range scopes {
		k.Scopes[sc] = true
	}
	// Best-effort usage stamp; not worth failing the request over.
	_, _ = s.Pool.Exec(ctx,
		`UPDATE api_keys SET last_used_at = $1 WHERE id = $2`, time.Now().UTC(), k.ID)
	return k, nil
}
