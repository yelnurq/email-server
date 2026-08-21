package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/mailcore"
)

// Component is one infrastructure component's observed state.
type Component struct {
	Name      string `json:"name"`
	Status    string `json:"status"` // ok | unavailable | disabled
	LatencyMS int64  `json:"latency_ms"`
	Version   string `json:"version,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// InfraReport aggregates all component checks at one point in time.
type InfraReport struct {
	Components []Component `json:"components"`
	CheckedAt  string      `json:"checked_at"`
}

// InfraMonitor serves the admin infrastructure view. Checks are cached for
// TTL so frontend polling never hammers the dependencies.
type InfraMonitor struct {
	Health   *HealthChecker
	Provider mailcore.Provider
	TTL      time.Duration

	// Optional security-scanner probes (V4 §114). nil reports the
	// component as disabled.
	RspamdPing func(ctx context.Context) error
	ClamAVPing func(ctx context.Context) error

	mu      sync.Mutex
	cached  InfraReport
	expires time.Time
}

// Report returns the cached report, refreshing it when stale.
func (m *InfraMonitor) Report(ctx context.Context) InfraReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Now().Before(m.expires) {
		return m.cached
	}
	m.cached = m.collect(ctx)
	ttl := m.TTL
	if ttl == 0 {
		ttl = 10 * time.Second
	}
	m.expires = time.Now().Add(ttl)
	return m.cached
}

func (m *InfraMonitor) collect(ctx context.Context) InfraReport {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	type result struct {
		idx int
		c   Component
	}
	// Fixed display order.
	checks := []struct {
		name string
		fn   func(context.Context) Component
	}{
		{"postgres", func(ctx context.Context) Component {
			return timed("postgres", func() error { return m.Health.Pool.Ping(ctx) })
		}},
		{"redis", func(ctx context.Context) Component {
			return timed("redis", func() error { return m.Health.Redis.Ping(ctx).Err() })
		}},
		{"nats", func(ctx context.Context) Component {
			return timed("nats", func() error {
				if !m.Health.NATS.IsConnected() {
					return errNATSDisconnected
				}
				return nil
			})
		}},
		{"minio", func(ctx context.Context) Component {
			return timed("minio", func() error {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.Health.S3HealthURL, nil)
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
		}},
		{"stalwart", func(ctx context.Context) Component {
			if !m.Provider.Enabled() {
				return Component{Name: "stalwart", Status: "disabled", Detail: "mail core not configured"}
			}
			st := m.Provider.Health(ctx)
			c := Component{Name: "stalwart", LatencyMS: st.LatencyMS, Version: st.Version}
			if st.Available {
				c.Status = "ok"
			} else {
				c.Status = "unavailable"
				c.Detail = st.Error
			}
			return c
		}},
		{"rspamd", func(ctx context.Context) Component {
			if m.RspamdPing == nil {
				return Component{Name: "rspamd", Status: "disabled", Detail: "spam scanner not configured"}
			}
			return timed("rspamd", func() error { return m.RspamdPing(ctx) })
		}},
		{"clamav", func(ctx context.Context) Component {
			if m.ClamAVPing == nil {
				return Component{Name: "clamav", Status: "disabled", Detail: "malware scanner not configured"}
			}
			// clamd answers only once its signature database is loaded, so
			// "ok" here means ready, not merely alive (§115).
			return timed("clamav", func() error { return m.ClamAVPing(ctx) })
		}},
		{"worker", func(ctx context.Context) Component {
			var beatAt time.Time
			err := m.Health.Pool.QueryRow(ctx,
				`SELECT beat_at FROM worker_heartbeats WHERE name = 'worker'`).Scan(&beatAt)
			switch {
			case err != nil:
				return Component{Name: "worker", Status: "unavailable", Detail: "no heartbeat recorded"}
			case time.Since(beatAt) > 30*time.Second:
				return Component{Name: "worker", Status: "unavailable",
					Detail: "last heartbeat " + beatAt.UTC().Format(time.RFC3339)}
			default:
				return Component{Name: "worker", Status: "ok",
					Detail: "last heartbeat " + beatAt.UTC().Format(time.RFC3339)}
			}
		}},
	}

	results := make(chan result, len(checks))
	for i, ch := range checks {
		go func(i int, fn func(context.Context) Component) {
			results <- result{i, fn(ctx)}
		}(i, ch.fn)
	}
	components := make([]Component, len(checks))
	for range checks {
		r := <-results
		components[r.idx] = r.c
	}
	return InfraReport{Components: components, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
}

func timed(name string, fn func() error) Component {
	start := time.Now()
	err := fn()
	c := Component{Name: name, LatencyMS: time.Since(start).Milliseconds()}
	if err != nil {
		c.Status = "unavailable"
		c.Detail = err.Error()
	} else {
		c.Status = "ok"
	}
	return c
}

// Infrastructure is the admin endpoint handler.
func (m *InfraMonitor) Infrastructure(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, m.Report(r.Context()))
}
