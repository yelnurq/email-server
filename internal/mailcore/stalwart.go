package mailcore

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

// Stalwart manages a Stalwart Mail Server through its management REST API.
// All calls authenticate with the fallback administrator account; those
// credentials never leave the backend process.
type Stalwart struct {
	// BaseURL is the management endpoint, e.g. http://localhost:8180.
	BaseURL string
	// AdminUser/AdminPass authenticate management calls (HTTP Basic).
	AdminUser string
	AdminPass string

	// HTTP is the client used for API calls; a 10s-timeout client is used
	// when nil.
	HTTP *http.Client
}

func (s *Stalwart) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return stalwartDefaultClient
}

var stalwartDefaultClient = &http.Client{Timeout: 10 * time.Second}

func (s *Stalwart) Name() string  { return "stalwart" }
func (s *Stalwart) Enabled() bool { return true }

// apiError is a semantic (non-transport) failure reported by Stalwart.
// Stalwart v0.13 reports application errors as HTTP 200 with an "error"
// field in the body (verified live: GET of a missing principal returns
// `{"error":"notFound","item":"..."}`), so Code carries that field and
// Status the HTTP code.
type apiError struct {
	Status int
	Code   string
	Body   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("stalwart api: HTTP %d %s: %s", e.Status, e.Code, e.Body)
}

// do executes one management call. Transport failures wrap ErrUnavailable;
// HTTP-level and body-level errors come back as *apiError with the response
// body (Stalwart error bodies are short JSON descriptions, never secrets).
func (s *Stalwart) do(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.BaseURL, "/")+path, rd)
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.AdminUser, s.AdminPass)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &apiError{Status: resp.StatusCode, Body: strings.TrimSpace(string(raw))}
	}
	var envelope struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Error != "" {
		return &apiError{Status: resp.StatusCode, Code: envelope.Error, Body: strings.TrimSpace(string(raw))}
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// notFound reports whether err says the principal does not exist.
func notFound(err error) bool {
	var ae *apiError
	if ok := asAPIError(err, &ae); ok {
		return ae.Status == http.StatusNotFound || ae.Code == "notFound"
	}
	return false
}

func asAPIError(err error, target **apiError) bool {
	for err != nil {
		if ae, ok := err.(*apiError); ok {
			*target = ae
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// Health probes the liveness endpoint and best-effort fetches the version.
func (s *Stalwart) Health(ctx context.Context) Status {
	st := Status{Provider: "stalwart", CheckedAt: time.Now().UTC()}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(s.BaseURL, "/")+"/healthz/live", nil)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	resp, err := s.client().Do(req)
	st.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		st.Error = "unreachable: " + err.Error()
		return st
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode != http.StatusOK {
		st.Error = fmt.Sprintf("health returned HTTP %d", resp.StatusCode)
		return st
	}
	st.Available = true

	// Version is informative only; failures are not a health problem.
	var ver struct {
		Data json.RawMessage `json:"data"`
	}
	if err := s.do(ctx, http.MethodGet, "/api/version", nil, &ver); err == nil {
		var plain string
		if json.Unmarshal(ver.Data, &plain) == nil {
			st.Version = plain
		} else {
			var obj struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(ver.Data, &obj) == nil {
				st.Version = obj.Version
			}
		}
	}
	return st
}

// principal is the subset of Stalwart's principal object the control plane
// manages.
type principal struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Secrets     []string `json:"secrets,omitempty"`
	Emails      []string `json:"emails,omitempty"`
	Quota       int64    `json:"quota,omitempty"`
	Roles       []string `json:"roles,omitempty"`
}

type patchOp struct {
	Action string `json:"action"` // set | addItem | removeItem
	Field  string `json:"field"`
	Value  any    `json:"value"`
}

func (s *Stalwart) principalExists(ctx context.Context, name string) (bool, error) {
	err := s.do(ctx, http.MethodGet, "/api/principal/"+name, nil, nil)
	if err == nil {
		return true, nil
	}
	if notFound(err) {
		return false, nil
	}
	return false, err
}

// Note: JMAP account ids are deliberately NOT derived from the management
// API's numeric principal id. Stalwart's id codec is internal (its alphabet
// is not plain base32), so mailservice reads the id from the JMAP session
// instead.

// EnsureDomain creates the domain principal when absent.
func (s *Stalwart) EnsureDomain(ctx context.Context, name string) error {
	exists, err := s.principalExists(ctx, name)
	if err != nil || exists {
		return err
	}
	return s.do(ctx, http.MethodPost, "/api/principal", principal{
		Type: "domain", Name: name, Description: "managed by mail platform",
	}, nil)
}

// DeleteDomain removes the domain principal (missing is fine).
func (s *Stalwart) DeleteDomain(ctx context.Context, name string) error {
	err := s.do(ctx, http.MethodDelete, "/api/principal/"+name, nil, nil)
	if notFound(err) {
		return nil
	}
	return err
}

// EnsureAccount creates the account when absent or updates display name and
// quota when present. Authentication secrets are managed separately as app
// passwords.
func (s *Stalwart) EnsureAccount(ctx context.Context, a Account) error {
	exists, err := s.principalExists(ctx, a.Email)
	if err != nil {
		return err
	}
	if !exists {
		return s.do(ctx, http.MethodPost, "/api/principal", principal{
			Type: "individual", Name: a.Email, Description: a.DisplayName,
			Emails: []string{a.Email}, Quota: a.QuotaBytes, Roles: []string{"user"},
		}, nil)
	}
	return s.do(ctx, http.MethodPatch, "/api/principal/"+a.Email, []patchOp{
		{Action: "set", Field: "description", Value: a.DisplayName},
		{Action: "set", Field: "quota", Value: a.QuotaBytes},
	}, nil)
}

// SetAccountQuota updates the storage quota.
func (s *Stalwart) SetAccountQuota(ctx context.Context, email string, quotaBytes int64) error {
	return s.do(ctx, http.MethodPatch, "/api/principal/"+email, []patchOp{
		{Action: "set", Field: "quota", Value: quotaBytes},
	}, nil)
}

// DisableAccount clears every authentication secret so protocol logins stop.
// The account and its stored mail remain; new app passwords must be issued
// after re-enabling.
func (s *Stalwart) DisableAccount(ctx context.Context, email string) error {
	err := s.do(ctx, http.MethodPatch, "/api/principal/"+email, []patchOp{
		{Action: "set", Field: "secrets", Value: []string{}},
	}, nil)
	if notFound(err) {
		return nil
	}
	return err
}

// appSecret encodes a labelled app password in Stalwart's secret format; the
// label prefix is what allows revocation without knowing the password.
func appSecret(label, password string) string {
	return "$app$" + label + "$" + password
}

// AddAppPassword registers a labelled application password.
func (s *Stalwart) AddAppPassword(ctx context.Context, email, label, password string) error {
	return s.do(ctx, http.MethodPatch, "/api/principal/"+email, []patchOp{
		{Action: "addItem", Field: "secrets", Value: appSecret(label, password)},
	}, nil)
}

// RemoveAppPassword revokes the labelled application password. Stalwart
// matches app-password secrets by their "$app$<label>$" prefix on removal.
func (s *Stalwart) RemoveAppPassword(ctx context.Context, email, label string) error {
	err := s.do(ctx, http.MethodPatch, "/api/principal/"+email, []patchOp{
		{Action: "removeItem", Field: "secrets", Value: "$app$" + label + "$"},
	}, nil)
	if notFound(err) {
		return nil
	}
	return err
}
