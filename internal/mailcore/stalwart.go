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

// redactBody reduces an upstream error body to its secret-free parts.
// Stalwart error envelopes can echo submitted values verbatim — observed
// live on v0.13.4: fieldAlreadyExists returns the full private-key PEM in
// its "value" field, and principal errors could echo "$app$…$password"
// secrets the same way. Only error code, field and item names survive into
// error strings, logs, provisioning_error columns and audit details;
// values never do (V4 §120).
func redactBody(raw []byte) string {
	var env struct {
		Error   string `json:"error"`
		Field   string `json:"field"`
		Item    string `json:"item"`
		Reason  string `json:"reason"`
		Details string `json:"details"`
	}
	if json.Unmarshal(raw, &env) == nil && env.Error != "" {
		out := "error=" + env.Error
		if env.Field != "" {
			out += " field=" + env.Field
		}
		if env.Item != "" {
			out += " item=" + env.Item
		}
		for _, extra := range []string{env.Reason, env.Details} {
			if extra != "" && !strings.Contains(extra, "-----BEGIN") && len(extra) <= 200 {
				out += " " + extra
			}
		}
		return out
	}
	s := strings.TrimSpace(string(raw))
	if strings.Contains(s, "-----BEGIN") || strings.Contains(s, "$app$") {
		return "[redacted: upstream error body carried credential material]"
	}
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// do executes one management call. Transport failures wrap ErrUnavailable;
// HTTP-level and body-level errors come back as *apiError whose Body has
// been passed through redactBody — the raw upstream body never reaches
// error strings or logs.
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
		return &apiError{Status: resp.StatusCode, Body: redactBody(raw)}
	}
	var envelope struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Error != "" {
		return &apiError{Status: resp.StatusCode, Code: envelope.Error, Body: redactBody(raw)}
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

// dkimSignatureID is the settings id Stalwart's default signing rule looks
// up for outbound mail: 'rsa-' + sender_domain / 'ed25519-' + sender_domain
// (verified live on v0.13.4 — the "DKIM signer not found" warning names
// exactly these ids). Using them means signing needs no auth.dkim.sign
// override in the config.
func dkimSignatureID(domain, algorithm string) string {
	return algorithm + "-" + domain
}

// EnsureDKIMKey has the mail core generate a signing key for the domain and
// returns its base64 public key. Posting the same id again (rotation)
// replaces the key material and selector in place.
//
// SECURITY (V4 §22): the private key is created inside the mail core and
// never read back here. GET /api/dkim/{id} returns only the public key.
// Product code must never call /api/settings/list with a "signature."
// prefix — that response would include private key PEMs.
func (s *Stalwart) EnsureDKIMKey(ctx context.Context, domain, selector, algorithm string) (string, error) {
	algo := strings.ToLower(algorithm)
	var apiAlgo string
	switch algo {
	case "rsa":
		apiAlgo = "Rsa"
	case "ed25519":
		apiAlgo = "Ed25519"
	default:
		return "", fmt.Errorf("unsupported DKIM algorithm %q", algorithm)
	}
	id := dkimSignatureID(domain, algo)
	// Rotation: Stalwart refuses to overwrite an existing signature id
	// (fieldAlreadyExists, verified live), so clear its settings first.
	// Clearing a non-existent prefix is a no-op.
	if err := s.do(ctx, http.MethodPost, "/api/settings", []map[string]string{
		{"type": "clear", "prefix": "signature." + id + "."},
	}, nil); err != nil {
		return "", err
	}
	if err := s.do(ctx, http.MethodPost, "/api/dkim", map[string]any{
		"id": id, "algorithm": apiAlgo, "domain": domain, "selector": selector,
	}, nil); err != nil {
		return "", err
	}
	var pub struct {
		Data string `json:"data"`
	}
	if err := s.do(ctx, http.MethodGet, "/api/dkim/"+id, nil, &pub); err != nil {
		return "", err
	}
	if pub.Data == "" {
		return "", fmt.Errorf("mail core returned an empty DKIM public key for %s", id)
	}
	// Apply live: settings changes take effect after a config reload
	// (GET /api/reload, verified live on v0.13.4).
	if err := s.do(ctx, http.MethodGet, "/api/reload", nil, nil); err != nil {
		return "", err
	}
	return pub.Data, nil
}
