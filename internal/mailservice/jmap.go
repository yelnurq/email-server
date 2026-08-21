// Package mailservice is the application mail layer over the authoritative
// Stalwart mail store (ADR-003). It speaks JMAP with master-user
// authentication ("<account>%<master>"), so the backend can serve any
// mailbox without holding per-user protocol secrets. Master credentials
// never leave this process.
package mailservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func urlPathEscape(s string) string  { return url.PathEscape(s) }
func urlQueryEscape(s string) string { return url.QueryEscape(s) }

// ErrUnavailable marks transport-level mail-store failures; handlers map it
// to MAIL_SERVICE_UNAVAILABLE instead of leaking internals.
var ErrUnavailable = errors.New("mail store unavailable")

// Session is the JMAP session object, the authoritative source of an
// account's id. Deriving ids from the management API's numeric principal id
// was tried and rejected: Stalwart's id codec is internal and undocumented
// (its alphabet is not plain base32), so the server is asked instead.
type Session struct {
	Accounts map[string]struct {
		Name       string `json:"name"`
		IsPersonal bool   `json:"isPersonal"`
	} `json:"accounts"`
	PrimaryAccounts map[string]string `json:"primaryAccounts"`
	Username        string            `json:"username"`
}

// primaryMailAccount picks the account id to operate on: the advertised
// primary mail account, else the single personal account.
func (s *Session) primaryMailAccount() string {
	if id := s.PrimaryAccounts["urn:ietf:params:jmap:mail"]; id != "" {
		return id
	}
	for id, acct := range s.Accounts {
		if acct.IsPersonal {
			return id
		}
	}
	for id := range s.Accounts {
		return id
	}
	return ""
}

// Session fetches the JMAP session for one account. Note the path: the
// v0.13 server only populates accounts on /jmap/session; the
// /.well-known/jmap alias returns an empty (anonymous) session for Basic
// auth, which cost a debugging cycle — do not switch back to it.
func (c *JMAP) Session(ctx context.Context, account string) (*Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(c.BaseURL, "/")+"/jmap/session", nil)
	if err != nil {
		return nil, err
	}
	c.auth(req, account)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jmap session: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("jmap session: %w", err)
	}
	return &s, nil
}

// JMAP is a minimal JMAP client bound to one Stalwart server.
type JMAP struct {
	// BaseURL is the Stalwart HTTP endpoint, e.g. http://localhost:8180.
	BaseURL string
	// MasterUser/MasterSecret authenticate as any account via
	// "<account>%<MasterUser>".
	MasterUser   string
	MasterSecret string

	HTTP *http.Client
}

func (c *JMAP) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return jmapDefaultClient
}

var jmapDefaultClient = &http.Client{Timeout: 20 * time.Second}

func (c *JMAP) auth(req *http.Request, account string) {
	req.SetBasicAuth(account+"%"+c.MasterUser, c.MasterSecret)
}

// Invocation is one JMAP method call/response triplet.
type Invocation struct {
	Name string
	Args json.RawMessage
	ID   string
}

func (inv *Invocation) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{inv.Name, inv.Args, inv.ID})
}

func (inv *Invocation) UnmarshalJSON(raw []byte) error {
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return err
	}
	if len(parts) != 3 {
		return fmt.Errorf("jmap: invocation with %d elements", len(parts))
	}
	if err := json.Unmarshal(parts[0], &inv.Name); err != nil {
		return err
	}
	inv.Args = parts[1]
	return json.Unmarshal(parts[2], &inv.ID)
}

// Call builds an invocation from a method name and args value.
func Call(id, name string, args any) Invocation {
	raw, _ := json.Marshal(args)
	return Invocation{Name: name, Args: raw, ID: id}
}

var jmapUsing = []string{
	"urn:ietf:params:jmap:core",
	"urn:ietf:params:jmap:mail",
}

// Request executes method calls as the given account and returns the
// method responses in order.
func (c *JMAP) Request(ctx context.Context, account string, calls ...Invocation) ([]Invocation, error) {
	callPtrs := make([]*Invocation, len(calls))
	for i := range calls {
		callPtrs[i] = &calls[i]
	}
	body, err := json.Marshal(map[string]any{
		"using":       jmapUsing,
		"methodCalls": callPtrs,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.BaseURL, "/")+"/jmap/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.auth(req, account)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jmap: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var envelope struct {
		MethodResponses []Invocation `json:"methodResponses"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("jmap: bad response: %w", err)
	}
	return envelope.MethodResponses, nil
}

// methodError is a JMAP method-level error response.
type methodError struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// Decode unmarshals one method response, converting "error" methods into Go
// errors.
func Decode(resp Invocation, out any) error {
	if resp.Name == "error" {
		var me methodError
		_ = json.Unmarshal(resp.Args, &me)
		return fmt.Errorf("jmap method error: %s %s", me.Type, me.Description)
	}
	return json.Unmarshal(resp.Args, out)
}

// UploadBlob stores raw bytes in the account's blob store.
func (c *JMAP) UploadBlob(ctx context.Context, account, accountID, contentType string, data []byte) (blobID string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.BaseURL, "/")+"/jmap/upload/"+accountID+"/", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	c.auth(req, account)
	req.Header.Set("Content-Type", contentType)
	resp, err := c.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("jmap upload: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var out struct {
		BlobID string `json:"blobId"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out.BlobID, nil
}

// DownloadBlob streams a blob from the account's store. The caller must
// close the returned reader.
func (c *JMAP) DownloadBlob(ctx context.Context, account, accountID, blobID, name, accept string) (io.ReadCloser, int64, error) {
	if name == "" {
		name = "attachment"
	}
	if accept == "" {
		accept = "application/octet-stream"
	}
	u := fmt.Sprintf("%s/jmap/download/%s/%s/%s?accept=%s",
		strings.TrimRight(c.BaseURL, "/"), accountID, blobID, urlPathEscape(name), urlQueryEscape(accept))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	c.auth(req, account)
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("jmap download: HTTP %d", resp.StatusCode)
	}
	return resp.Body, resp.ContentLength, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
