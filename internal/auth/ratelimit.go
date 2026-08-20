package auth

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// LoginLimiter throttles login attempts per key (client IP and email) using
// Redis fixed windows. Fails open when Redis is unavailable so an infra
// outage cannot lock every user out; the event is surfaced by readiness.
type LoginLimiter struct {
	Redis  *redis.Client
	Limit  int
	Window time.Duration
}

// NewLoginLimiter returns the default limiter: 10 attempts per 5 minutes.
func NewLoginLimiter(rdb *redis.Client) *LoginLimiter {
	return &LoginLimiter{Redis: rdb, Limit: 10, Window: 5 * time.Minute}
}

// Allow reports whether another attempt is permitted for the key.
func (l *LoginLimiter) Allow(ctx context.Context, key string) bool {
	full := "rl:login:" + key
	n, err := l.Redis.Incr(ctx, full).Result()
	if err != nil {
		return true // fail open on Redis outage
	}
	if n == 1 {
		l.Redis.Expire(ctx, full, l.Window)
	}
	return n <= int64(l.Limit)
}
