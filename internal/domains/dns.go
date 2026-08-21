package domains

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/dnscheck"
	"github.com/yelnurq/email-server/internal/httpx"
)

// recheckTimeout bounds one full DNS recheck (8 lookups); the endpoint is
// synchronous by design — the UI shows "Checking…" for its duration (§15).
const recheckTimeout = 9 * time.Second

// dnsRecordTypes is the presentation order of the record table.
var dnsRecordTypes = []string{
	dnscheck.TypeOwnership, dnscheck.TypeMX, dnscheck.TypeSPF, dnscheck.TypeDKIM,
	dnscheck.TypeDMARC, dnscheck.TypePTR, dnscheck.TypeMTASTS, dnscheck.TypeTLSRPT,
}

// newVerificationToken issues the random proof value for the ownership TXT.
func newVerificationToken() string {
	var raw [20]byte
	_, _ = rand.Read(raw[:])
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:]))
}

// domainForDNS loads one domain with tenant/org scoping and guarantees it
// carries a verification token.
func (h *Handlers) domainForDNS(ctx context.Context, id *auth.Identity, domainID string) (name, mode, status, token string, ok bool) {
	query := `
		SELECT name::text, verification_mode, status, verification_token
		FROM domains WHERE id = $1 AND tenant_id = $2`
	args := []any{domainID, id.TenantID}
	if !id.TenantWide() {
		query += ` AND organization_id = $3`
		args = append(args, id.OrganizationID)
	}
	if err := h.Pool.QueryRow(ctx, query, args...).Scan(&name, &mode, &status, &token); err != nil {
		return "", "", "", "", false
	}
	if token == "" {
		token = newVerificationToken()
		if _, err := h.Pool.Exec(ctx,
			`UPDATE domains SET verification_token = $1 WHERE id = $2 AND verification_token = ''`,
			token, domainID); err != nil {
			return "", "", "", "", false
		}
	}
	return name, mode, status, token, true
}

// expectations assembles what DNS should contain for the domain, including
// the active DKIM key registered for it (public material only).
func (h *Handlers) expectations(ctx context.Context, domainID, name, token string) dnscheck.Expectations {
	e := dnscheck.Expectations{
		Domain:         name,
		MailHostname:   h.MailHostname,
		OutboundIP:     h.OutboundIP,
		OwnershipToken: token,
	}
	// No row is fine (key not generated yet) — the DKIM check reports
	// pending in that case.
	_ = h.Pool.QueryRow(ctx, `
		SELECT selector, public_key FROM dkim_keys
		WHERE domain_id = $1 AND status = 'active'
		ORDER BY activated_at DESC NULLS LAST LIMIT 1`,
		domainID).Scan(&e.DKIMSelector, &e.DKIMPublicKey)
	return e
}

type dnsResponse struct {
	DomainID  string           `json:"domain_id"`
	Domain    string           `json:"domain"`
	Mode      string           `json:"verification_mode"`
	Status    string           `json:"status"`
	Token     string           `json:"verification_token"`
	CheckedAt *string          `json:"checked_at,omitempty"`
	Records   []dnscheck.Check `json:"records"`
}

// DNS returns the stored record snapshot plus expected values for records
// never checked yet — the wizard's "configure these records" table.
func (h *Handlers) DNS(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	domainID := chi.URLParam(r, "id")
	name, mode, status, token, ok := h.domainForDNS(r.Context(), id, domainID)
	if !ok {
		httpx.Error(w, r, http.StatusNotFound, "DOMAIN_NOT_FOUND", "Domain not found")
		return
	}

	stored := map[string]dnscheck.Check{}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT record_type, host, expected, detected, status, detail, checked_at
		FROM domain_dns_records WHERE domain_id = $1`, domainID)
	if err != nil {
		h.Log.Error("dns snapshot read failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	var lastChecked *time.Time
	for rows.Next() {
		var c dnscheck.Check
		var detected []byte
		if err := rows.Scan(&c.Type, &c.Host, &c.Expected, &detected, &c.Status, &c.Detail, &c.CheckedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		_ = json.Unmarshal(detected, &c.Detected)
		stored[c.Type] = c
		if lastChecked == nil || c.CheckedAt.After(*lastChecked) {
			t := c.CheckedAt
			lastChecked = &t
		}
	}

	e := h.expectations(r.Context(), domainID, name, token)
	resp := dnsResponse{DomainID: domainID, Domain: name, Mode: mode, Status: status, Token: token}
	if lastChecked != nil {
		s := lastChecked.UTC().Format(time.RFC3339)
		resp.CheckedAt = &s
	}
	for _, t := range dnsRecordTypes {
		if c, ok := stored[t]; ok {
			resp.Records = append(resp.Records, c)
			continue
		}
		resp.Records = append(resp.Records, pendingCheck(t, e))
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// pendingCheck renders the expected record before any check ran.
func pendingCheck(recordType string, e dnscheck.Expectations) dnscheck.Check {
	c := dnscheck.Check{Type: recordType, Status: dnscheck.StatusPending}
	switch recordType {
	case dnscheck.TypeOwnership:
		c.Host = "_mailplatform." + e.Domain
		c.Expected = "mailplatform-verify=" + e.OwnershipToken
	case dnscheck.TypeMX:
		c.Host = e.Domain
		c.Expected = e.MailHostname
	case dnscheck.TypeSPF:
		c.Host = e.Domain
		c.Expected = dnscheck.ExpectedSPF(e)
	case dnscheck.TypeDKIM:
		if e.DKIMSelector != "" {
			c.Host = e.DKIMSelector + "._domainkey." + e.Domain
			c.Expected = "v=DKIM1; p=" + e.DKIMPublicKey
		} else {
			c.Host = "<selector>._domainkey." + e.Domain
			c.Detail = "generated together with the domain's DKIM key"
		}
	case dnscheck.TypeDMARC:
		c.Host = "_dmarc." + e.Domain
		c.Expected = "v=DMARC1; p=none; rua=mailto:dmarc@" + e.Domain
		c.Detail = "recommended starting policy: monitor first, tighten after reports look clean"
	case dnscheck.TypePTR:
		c.Host = e.OutboundIP
		c.Expected = e.MailHostname
		if e.OutboundIP == "" {
			c.Detail = "not applicable: no public outbound IP configured"
		}
	case dnscheck.TypeMTASTS:
		c.Host = "_mta-sts." + e.Domain
		c.Expected = "v=STSv1; id=<policy-id>"
		c.Detail = "optional"
	case dnscheck.TypeTLSRPT:
		c.Host = "_smtp._tls." + e.Domain
		c.Expected = "v=TLSRPTv1; rua=mailto:<reports>"
		c.Detail = "optional"
	}
	return c
}

// RecheckDNS performs a live verification of every record, stores the
// snapshot, and — for dns-mode domains — promotes the domain to verified
// once ownership and MX both check out (which also unlocks mail-core
// provisioning; §16 step 4).
func (h *Handlers) RecheckDNS(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	domainID := chi.URLParam(r, "id")
	name, mode, status, token, ok := h.domainForDNS(r.Context(), id, domainID)
	if !ok {
		httpx.Error(w, r, http.StatusNotFound, "DOMAIN_NOT_FOUND", "Domain not found")
		return
	}
	if h.DNSChecker == nil {
		httpx.Error(w, r, http.StatusServiceUnavailable, "DNS_UNAVAILABLE", "DNS verification is not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), recheckTimeout)
	defer cancel()
	e := h.expectations(r.Context(), domainID, name, token)
	checks := h.DNSChecker.CheckAll(ctx, e)

	for _, c := range checks {
		detected, _ := json.Marshal(c.Detected)
		if detected == nil {
			detected = []byte("[]")
		}
		if _, err := h.Pool.Exec(r.Context(), `
			INSERT INTO domain_dns_records (domain_id, record_type, host, expected, detected, status, detail, checked_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (domain_id, record_type) DO UPDATE SET
				host = EXCLUDED.host, expected = EXCLUDED.expected,
				detected = EXCLUDED.detected, status = EXCLUDED.status,
				detail = EXCLUDED.detail, checked_at = EXCLUDED.checked_at`,
			domainID, c.Type, c.Host, c.Expected, detected, c.Status, c.Detail, c.CheckedAt); err != nil {
			h.Log.Error("dns snapshot write failed", slog.String("error", err.Error()))
			httpx.Internal(w, r)
			return
		}
	}
	if _, err := h.Pool.Exec(r.Context(),
		`UPDATE domains SET dns_checked_at = now() WHERE id = $1`, domainID); err != nil {
		httpx.Internal(w, r)
		return
	}

	byType := map[string]dnscheck.Check{}
	for _, c := range checks {
		byType[c.Type] = c
	}
	// Ownership + MX proven ⇒ the domain is verified and may be provisioned
	// into the mail core. Verification is a one-way transition here; going
	// back requires an explicit admin action, not a flaky DNS answer.
	newStatus := status
	if mode == "dns" && status != "verified" &&
		byType[dnscheck.TypeOwnership].Status == dnscheck.StatusVerified &&
		byType[dnscheck.TypeMX].Status == dnscheck.StatusVerified {
		if _, err := h.Pool.Exec(r.Context(), `
			UPDATE domains SET status = 'verified', verified_at = now()
			WHERE id = $1 AND status <> 'verified'`, domainID); err == nil {
			newStatus = "verified"
			h.Audit.Record(r.Context(), audit.Entry{
				TenantID: id.TenantID, ActorUserID: id.UserID, Action: "domain.verified",
				ResourceType: "domain", ResourceID: domainID,
				Detail: map[string]any{"name": name, "via": "dns"},
			})
			// Verification unlocks mail-core provisioning (deferred at create
			// for dns-mode domains).
			h.Provisioner.Enqueue(r.Context(), "domain", id.TenantID, domainID, id.UserID)
		}
	}

	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "domain.dns_recheck",
		ResourceType: "domain", ResourceID: domainID,
		Detail: map[string]any{"name": name, "statuses": statusSummary(checks)},
	})

	resp := dnsResponse{DomainID: domainID, Domain: name, Mode: mode, Status: newStatus, Token: token, Records: checks}
	nowStr := time.Now().UTC().Format(time.RFC3339)
	resp.CheckedAt = &nowStr
	httpx.JSON(w, http.StatusOK, resp)
}

func statusSummary(checks []dnscheck.Check) map[string]string {
	out := map[string]string{}
	for _, c := range checks {
		out[c.Type] = c.Status
	}
	return out
}
