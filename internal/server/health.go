package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

// HealthChecker verifies connectivity to the platform's core dependencies.
// Any dependency may be nil (not wired yet); nil dependencies are skipped.
type HealthChecker struct {
	Pool        *pgxpool.Pool
	Redis       *redis.Client
	NATS        *nats.Conn
	S3HealthURL string // e.g. http://localhost:9000/minio/health/live
}

type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// Live reports that the process is running. It performs no dependency checks.
func (h *HealthChecker) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Checks: map[string]string{}})
}

// Ready reports whether required dependencies can serve traffic.
// Returns 503 with per-dependency detail when any check fails.
func (h *HealthChecker) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		checks = map[string]string{}
		failed bool
	)
	record := func(name string, err error) {
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			checks[name] = "unavailable: " + err.Error()
			failed = true
		} else {
			checks[name] = "ok"
		}
	}
	run := func(name string, fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record(name, fn(ctx))
		}()
	}

	if h.Pool != nil {
		run("postgres", h.Pool.Ping)
	}
	if h.Redis != nil {
		run("redis", func(ctx context.Context) error {
			return h.Redis.Ping(ctx).Err()
		})
	}
	if h.NATS != nil {
		run("nats", func(context.Context) error {
			if !h.NATS.IsConnected() {
				return errNATSDisconnected
			}
			return nil
		})
	}
	if h.S3HealthURL != "" {
		run("minio", func(ctx context.Context) error {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.S3HealthURL, nil)
			if err != nil {
				return err
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return &unexpectedStatusError{code: resp.StatusCode}
			}
			return nil
		})
	}
	wg.Wait()

	status, code := "ok", http.StatusOK
	if failed {
		status, code = "degraded", http.StatusServiceUnavailable
	}
	writeJSON(w, code, healthResponse{Status: status, Checks: checks})
}

var errNATSDisconnected = &simpleError{"nats connection is not established"}

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }

type unexpectedStatusError struct{ code int }

func (e *unexpectedStatusError) Error() string {
	return "unexpected HTTP status " + http.StatusText(e.code)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
