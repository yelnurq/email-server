// Package scanner holds the thin clients for the external security engines
// (V4 §38-47): Rspamd's HTTP scan API and ClamAV's clamd protocol. The
// milter path (inbound SMTP) is configured inside the mail core; these
// clients serve the control-plane acceptance path and health checks.
package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Rspamd talks to the rspamd controller.
type Rspamd struct {
	BaseURL  string
	Password string
	HTTP     *http.Client
}

func (r *Rspamd) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return defaultClient
}

var defaultClient = &http.Client{Timeout: 15 * time.Second}

// Enabled reports whether an rspamd endpoint is configured.
func (r *Rspamd) Enabled() bool { return r != nil && r.BaseURL != "" }

// Ping checks controller liveness.
func (r *Rspamd) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(r.BaseURL, "/")+"/ping", nil)
	if err != nil {
		return err
	}
	resp, err := r.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(string(body)), "pong") {
		return fmt.Errorf("rspamd ping returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Symbol is one matched rule in a scan result.
type Symbol struct {
	Name        string  `json:"name"`
	Score       float64 `json:"score"`
	Description string  `json:"description,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// CheckResult is the subset of /checkv2 output the platform records (§41).
type CheckResult struct {
	Score         float64  `json:"score"`
	RequiredScore float64  `json:"required_score"`
	Action        string   `json:"action"` // no action | add header | soft reject | reject | ...
	Symbols       []Symbol `json:"symbols"`
	URLs          []string `json:"urls,omitempty"`
}

// CheckMeta carries envelope context that improves scan accuracy.
type CheckMeta struct {
	From string
	Rcpt []string
	IP   string
	User string // authenticated sender (suppresses AUTH-related penalties)
}

// Check scans one raw RFC822 message through POST /checkv2.
func (r *Rspamd) Check(ctx context.Context, raw []byte, meta CheckMeta) (*CheckResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(r.BaseURL, "/")+"/checkv2", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if r.Password != "" {
		req.Header.Set("Password", r.Password)
	}
	if meta.From != "" {
		req.Header.Set("From", meta.From)
	}
	for _, rc := range meta.Rcpt {
		req.Header.Add("Rcpt", rc)
	}
	if meta.IP != "" {
		req.Header.Set("IP", meta.IP)
	}
	if meta.User != "" {
		req.Header.Set("User", meta.User)
	}
	resp, err := r.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rspamd /checkv2 returned HTTP %d", resp.StatusCode)
	}
	var wire struct {
		Score         float64 `json:"score"`
		RequiredScore float64 `json:"required_score"`
		Action        string  `json:"action"`
		Symbols       map[string]struct {
			Score       float64  `json:"score"`
			Description string   `json:"description"`
			Options     []string `json:"options"`
		} `json:"symbols"`
		URLs []string `json:"urls"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("rspamd response unparsable: %w", err)
	}
	out := &CheckResult{
		Score: wire.Score, RequiredScore: wire.RequiredScore,
		Action: wire.Action, URLs: wire.URLs,
	}
	for name, s := range wire.Symbols {
		out.Symbols = append(out.Symbols, Symbol{
			Name: name, Score: s.Score, Description: s.Description, Options: s.Options,
		})
	}
	return out, nil
}
