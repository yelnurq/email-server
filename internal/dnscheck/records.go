package dnscheck

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

// Check is one record's verification result — the §14 snapshot shape.
type Check struct {
	Type     string    `json:"type"`
	Host     string    `json:"host"`     // where the record lives
	Expected string    `json:"expected"` // what the platform wants there
	Detected []string  `json:"detected"` // what DNS actually returned
	Status   string    `json:"status"`
	Detail   string    `json:"detail,omitempty"` // human explanation
	CheckedAt time.Time `json:"checked_at"`
}

// Expectations parameterizes the checks for one domain. Hostnames come from
// platform configuration (§17: never hardcoded).
type Expectations struct {
	Domain string
	// MailHostname is the MX target and PTR expectation (e.g. mail.example.kz).
	MailHostname string
	// OutboundIP is the sending IP for SPF ip4 and the PTR check; empty in
	// local development ("not applicable").
	OutboundIP string
	// OwnershipToken is the per-domain proof value for the _mailplatform TXT.
	OwnershipToken string
	// DKIMSelector/DKIMPublicKey describe the active DKIM key; empty until a
	// key is generated (then the DKIM check reports pending).
	DKIMSelector  string
	DKIMPublicKey string
}

// Checker runs record verifications through a Resolver.
type Checker struct {
	Resolver Resolver
}

func now() time.Time { return time.Now().UTC() }

func trimDot(h string) string { return strings.TrimSuffix(strings.ToLower(h), ".") }

// CheckAll runs every record check for the domain and returns them in a
// stable order. Individual failures never abort the set.
func (c *Checker) CheckAll(ctx context.Context, e Expectations) []Check {
	checks := []Check{
		c.CheckOwnership(ctx, e),
		c.CheckMX(ctx, e),
		c.CheckSPF(ctx, e),
		c.CheckDKIM(ctx, e),
		c.CheckDMARC(ctx, e),
		c.CheckMTASTS(ctx, e),
		c.CheckTLSRPT(ctx, e),
		c.CheckPTR(ctx, e),
	}
	return checks
}

// CheckOwnership verifies the domain-control TXT record
// ("mailplatform-verify=<token>" at _mailplatform.<domain>).
func (c *Checker) CheckOwnership(ctx context.Context, e Expectations) Check {
	host := "_mailplatform." + e.Domain
	want := "mailplatform-verify=" + e.OwnershipToken
	ck := Check{Type: TypeOwnership, Host: host, Expected: want, CheckedAt: now()}
	if e.OwnershipToken == "" {
		ck.Status = StatusPending
		ck.Detail = "no verification token issued for this domain"
		return ck
	}
	txts, err := c.Resolver.LookupTXT(ctx, host)
	if err != nil {
		ck.Status = missingOrError(err)
		ck.Detail = lookupDetail(ck.Status, err)
		return ck
	}
	ck.Detected = txts
	for _, t := range txts {
		if strings.TrimSpace(t) == want {
			ck.Status = StatusVerified
			return ck
		}
	}
	if len(txts) == 0 {
		ck.Status = StatusMissing
		return ck
	}
	ck.Status = StatusInvalid
	ck.Detail = "a TXT record exists but does not carry the expected verification value"
	return ck
}

// CheckMX compares the domain's MX set against the platform mail host.
func (c *Checker) CheckMX(ctx context.Context, e Expectations) Check {
	ck := Check{Type: TypeMX, Host: e.Domain, Expected: e.MailHostname, CheckedAt: now()}
	mxs, err := c.Resolver.LookupMX(ctx, e.Domain)
	if err != nil {
		ck.Status = missingOrError(err)
		ck.Detail = lookupDetail(ck.Status, err)
		return ck
	}
	sort.Slice(mxs, func(i, j int) bool { return mxs[i].Pref < mxs[j].Pref })
	found := false
	for _, mx := range mxs {
		ck.Detected = append(ck.Detected, fmt.Sprintf("%d %s", mx.Pref, trimDot(mx.Host)))
		if trimDot(mx.Host) == trimDot(e.MailHostname) {
			found = true
		}
	}
	switch {
	case len(mxs) == 0:
		ck.Status = StatusMissing
	case found && len(mxs) == 1:
		ck.Status = StatusVerified
	case found:
		ck.Status = StatusWarning
		ck.Detail = "the platform MX is present, but other MX records also accept mail for this domain"
	default:
		ck.Status = StatusInvalid
		ck.Detail = "MX records exist but none point at the platform mail host"
	}
	return ck
}

// CheckSPF finds and evaluates the domain's SPF policy (§18-19).
func (c *Checker) CheckSPF(ctx context.Context, e Expectations) Check {
	ck := Check{Type: TypeSPF, Host: e.Domain, Expected: ExpectedSPF(e), CheckedAt: now()}
	txts, err := c.Resolver.LookupTXT(ctx, e.Domain)
	if err != nil {
		ck.Status = missingOrError(err)
		ck.Detail = lookupDetail(ck.Status, err)
		return ck
	}
	var spfs []string
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=spf1") {
			spfs = append(spfs, strings.TrimSpace(t))
		}
	}
	ck.Detected = spfs
	switch {
	case len(spfs) == 0:
		ck.Status = StatusMissing
		return ck
	case len(spfs) > 1:
		// RFC 7208 §3.2: multiple SPF records are a permanent error.
		ck.Status = StatusInvalid
		ck.Detail = "multiple SPF records found; receivers treat this as a permanent error — merge them into one"
		return ck
	}
	rec, perr := ParseSPF(spfs[0])
	if perr != nil {
		ck.Status = StatusInvalid
		ck.Detail = "SPF record cannot be parsed: " + perr.Error()
		return ck
	}
	if satisfiesSPF(rec, e) {
		ck.Status = StatusVerified
		if rec.AllQualifier == "+" {
			ck.Status = StatusWarning
			ck.Detail = `"+all" authorizes the whole internet to send as this domain; use "~all" or "-all"`
		}
		return ck
	}
	// An SPF record exists but does not authorize the platform: guidance to
	// modify the existing record, never to add a second one (§18).
	ck.Status = StatusWarning
	ck.Detail = "an SPF record exists but does not authorize the platform; add the missing mechanism to the existing record instead of creating a second one"
	return ck
}

// ExpectedSPF renders the record the platform recommends for the domain.
func ExpectedSPF(e Expectations) string {
	parts := []string{"v=spf1"}
	if e.OutboundIP != "" {
		parts = append(parts, "ip4:"+e.OutboundIP)
	}
	parts = append(parts, "mx", "~all")
	return strings.Join(parts, " ")
}

// satisfiesSPF reports whether the parsed record authorizes the platform's
// sending architecture: the outbound IP directly, or the mx mechanism when
// the platform host is the MX, or an a mechanism naming the mail host.
func satisfiesSPF(rec *SPFRecord, e Expectations) bool {
	for _, m := range rec.Mechanisms {
		switch m.Kind {
		case "ip4", "ip6":
			if e.OutboundIP != "" && m.Value == e.OutboundIP {
				return true
			}
			if e.OutboundIP != "" {
				if _, cidr, err := net.ParseCIDR(m.Value); err == nil {
					if ip := net.ParseIP(e.OutboundIP); ip != nil && cidr.Contains(ip) {
						return true
					}
				}
			}
		case "mx":
			// mx with no argument covers the domain's MX host — the platform
			// itself once the MX check passes.
			if m.Value == "" || trimDot(m.Value) == trimDot(e.Domain) {
				return true
			}
		case "a", "include":
			if trimDot(m.Value) == trimDot(e.MailHostname) {
				return true
			}
		}
	}
	return false
}

// CheckDKIM verifies the published DKIM public key for the active selector.
func (c *Checker) CheckDKIM(ctx context.Context, e Expectations) Check {
	ck := Check{Type: TypeDKIM, CheckedAt: now()}
	if e.DKIMSelector == "" || e.DKIMPublicKey == "" {
		ck.Host = "<selector>._domainkey." + e.Domain
		ck.Status = StatusPending
		ck.Detail = "no DKIM key generated for this domain yet"
		return ck
	}
	ck.Host = e.DKIMSelector + "._domainkey." + e.Domain
	ck.Expected = "v=DKIM1; k=" + dkimKeyAlgo(e.DKIMPublicKey) + "; p=" + e.DKIMPublicKey
	txts, err := c.Resolver.LookupTXT(ctx, ck.Host)
	if err != nil {
		ck.Status = missingOrError(err)
		ck.Detail = lookupDetail(ck.Status, err)
		return ck
	}
	// DNS splits long TXT records into 255-byte strings; join before parsing.
	joined := make([]string, 0, len(txts))
	for _, t := range txts {
		joined = append(joined, strings.ReplaceAll(t, "\" \"", ""))
	}
	ck.Detected = joined
	for _, t := range joined {
		tags := parseTagList(t)
		if v, ok := tags["v"]; ok && !strings.EqualFold(v, "DKIM1") {
			continue
		}
		p := strings.Map(dropSpace, tags["p"])
		if p == "" {
			ck.Status = StatusInvalid
			ck.Detail = "the DKIM record has an empty public key (revoked key)"
			return ck
		}
		if p == strings.Map(dropSpace, e.DKIMPublicKey) {
			ck.Status = StatusVerified
			return ck
		}
	}
	if len(joined) == 0 {
		ck.Status = StatusMissing
		return ck
	}
	ck.Status = StatusInvalid
	ck.Detail = "a DKIM record exists at this selector but its public key does not match the platform's key"
	return ck
}

func dkimKeyAlgo(pubKey string) string {
	// Ed25519 public keys are 44 base64 chars; RSA keys are far longer.
	if len(pubKey) <= 64 {
		return "ed25519"
	}
	return "rsa"
}

func dropSpace(r rune) rune {
	if r == ' ' || r == '\t' {
		return -1
	}
	return r
}

// CheckDMARC evaluates _dmarc.<domain> (§29): presence, parsability, policy
// and reporting address. A monitoring-only policy is a warning, not a
// failure — and the platform never auto-generates p=reject.
func (c *Checker) CheckDMARC(ctx context.Context, e Expectations) Check {
	host := "_dmarc." + e.Domain
	ck := Check{
		Type: TypeDMARC, Host: host,
		Expected:  "v=DMARC1; p=<policy>; rua=mailto:<reports>",
		CheckedAt: now(),
	}
	txts, err := c.Resolver.LookupTXT(ctx, host)
	if err != nil {
		ck.Status = missingOrError(err)
		ck.Detail = lookupDetail(ck.Status, err)
		return ck
	}
	var dmarcs []string
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=dmarc1") {
			dmarcs = append(dmarcs, strings.TrimSpace(t))
		}
	}
	ck.Detected = dmarcs
	switch {
	case len(dmarcs) == 0:
		ck.Status = StatusMissing
		return ck
	case len(dmarcs) > 1:
		ck.Status = StatusInvalid
		ck.Detail = "multiple DMARC records found; receivers ignore all of them — keep exactly one"
		return ck
	}
	tags := parseTagList(dmarcs[0])
	policy := strings.ToLower(tags["p"])
	rua := tags["rua"]
	switch policy {
	case "none":
		ck.Status = StatusWarning
		ck.Detail = "policy is monitoring-only (p=none); spoofed mail is still delivered. Move to quarantine/reject once reports look clean"
	case "quarantine", "reject":
		ck.Status = StatusVerified
		ck.Detail = "policy: p=" + policy
	default:
		ck.Status = StatusInvalid
		ck.Detail = "the DMARC record has no valid p= policy tag"
		return ck
	}
	if rua == "" {
		ck.Detail += "; no rua= reporting address — you will not receive aggregate reports"
	}
	return ck
}

// CheckMTASTS detects an MTA-STS policy record (§33 — diagnosis only; the
// platform does not host the policy file yet).
func (c *Checker) CheckMTASTS(ctx context.Context, e Expectations) Check {
	host := "_mta-sts." + e.Domain
	ck := Check{Type: TypeMTASTS, Host: host, Expected: "v=STSv1; id=<policy-id>", CheckedAt: now()}
	txts, err := c.Resolver.LookupTXT(ctx, host)
	if err != nil {
		ck.Status = missingOrError(err)
		ck.Detail = lookupDetail(ck.Status, err)
		return ck
	}
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=stsv1") {
			ck.Detected = append(ck.Detected, strings.TrimSpace(t))
		}
	}
	if len(ck.Detected) > 0 {
		ck.Status = StatusVerified
		ck.Detail = "MTA-STS record present; the policy file at https://mta-sts." + e.Domain + "/.well-known/mta-sts.txt must be served separately"
		return ck
	}
	ck.Status = StatusMissing
	ck.Detail = "optional: MTA-STS tells senders to require TLS toward this domain"
	return ck
}

// CheckTLSRPT detects a TLS reporting record (§34).
func (c *Checker) CheckTLSRPT(ctx context.Context, e Expectations) Check {
	host := "_smtp._tls." + e.Domain
	ck := Check{Type: TypeTLSRPT, Host: host, Expected: "v=TLSRPTv1; rua=mailto:<reports>", CheckedAt: now()}
	txts, err := c.Resolver.LookupTXT(ctx, host)
	if err != nil {
		ck.Status = missingOrError(err)
		ck.Detail = lookupDetail(ck.Status, err)
		return ck
	}
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=tlsrptv1") {
			ck.Detected = append(ck.Detected, strings.TrimSpace(t))
		}
	}
	if len(ck.Detected) > 0 {
		ck.Status = StatusVerified
		return ck
	}
	ck.Status = StatusMissing
	ck.Detail = "optional: TLS-RPT delivers reports about TLS failures toward this domain"
	return ck
}

// CheckPTR verifies reverse DNS for the outbound IP, forward-confirmed
// (§31-32): IP → PTR hostname → A/AAAA → the same IP.
func (c *Checker) CheckPTR(ctx context.Context, e Expectations) Check {
	ck := Check{Type: TypePTR, Host: e.OutboundIP, Expected: e.MailHostname, CheckedAt: now()}
	if e.OutboundIP == "" {
		ck.Status = StatusPending
		ck.Detail = "not applicable: no public outbound IP is configured (local development)"
		return ck
	}
	names, err := c.Resolver.LookupAddr(ctx, e.OutboundIP)
	if err != nil {
		ck.Status = missingOrError(err)
		ck.Detail = lookupDetail(ck.Status, err)
		return ck
	}
	for _, n := range names {
		ck.Detected = append(ck.Detected, trimDot(n))
	}
	if len(names) == 0 {
		ck.Status = StatusMissing
		return ck
	}
	matched := ""
	for _, n := range names {
		if trimDot(n) == trimDot(e.MailHostname) {
			matched = trimDot(n)
			break
		}
	}
	if matched == "" {
		ck.Status = StatusInvalid
		ck.Detail = "the PTR record does not name the platform mail host; many receivers reject mail from IPs whose reverse DNS mismatches HELO"
		return ck
	}
	// Forward confirmation.
	ips, err := c.Resolver.LookupIP(ctx, matched)
	if err != nil {
		ck.Status = StatusWarning
		ck.Detail = "PTR matches, but the forward lookup of that hostname failed — forward-confirmed rDNS could not be proven"
		return ck
	}
	for _, ip := range ips {
		if ip.String() == e.OutboundIP {
			ck.Status = StatusVerified
			return ck
		}
	}
	ck.Status = StatusWarning
	ck.Detail = "PTR matches, but the hostname does not resolve back to the outbound IP (forward-confirmation mismatch)"
	return ck
}

func lookupDetail(status string, err error) string {
	if status == StatusDNSError {
		return "temporary DNS failure: " + err.Error()
	}
	return ""
}
