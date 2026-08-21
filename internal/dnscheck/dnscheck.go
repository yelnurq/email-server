// Package dnscheck is the DNS verification subsystem (V4 §11): it resolves
// and classifies the records a mail domain needs — MX, SPF, DKIM, DMARC,
// ownership TXT, MTA-STS, TLS-RPT and PTR — against what the platform
// expects, with an explicit status model instead of booleans.
//
// Resolution is behind the Resolver interface so integration tests run
// against a fixture, never the public internet (§123). Transient DNS
// failures (timeout, SERVFAIL) are classified as StatusDNSError, never as
// StatusMissing (§12).
package dnscheck

import (
	"context"
	"errors"
	"net"
	"time"
)

// Record statuses (§13). Stored in domain_dns_records.status and returned
// to the UI verbatim.
const (
	StatusVerified = "verified"
	StatusMissing  = "missing"
	StatusInvalid  = "invalid"
	StatusWarning  = "warning"
	StatusPending  = "pending"
	StatusDNSError = "dns_error"
)

// Record types checked for a domain.
const (
	TypeOwnership = "ownership" // _mailplatform TXT proving domain control
	TypeMX        = "mx"
	TypeSPF       = "spf"
	TypeDKIM      = "dkim"
	TypeDMARC     = "dmarc"
	TypeMTASTS    = "mta_sts"
	TypeTLSRPT    = "tls_rpt"
	TypePTR       = "ptr"
)

// Resolver is the DNS access seam. The production implementation wraps
// *net.Resolver; tests provide fixtures.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupIP(ctx context.Context, host string) ([]net.IP, error)
	LookupAddr(ctx context.Context, addr string) ([]string, error)
}

// NetResolver resolves through the OS or a specific DNS server.
type NetResolver struct {
	r *net.Resolver
	// Timeout bounds every individual lookup.
	Timeout time.Duration
}

// NewNetResolver returns a resolver. When serverAddr is non-empty
// ("host:port"), all queries go to that server — this is how a controlled
// DNS fixture is wired in tests and local development; empty uses the
// system resolver.
func NewNetResolver(serverAddr string, timeout time.Duration) *NetResolver {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	nr := &NetResolver{Timeout: timeout}
	if serverAddr == "" {
		nr.r = net.DefaultResolver
		return nr
	}
	nr.r = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: timeout}
			return d.DialContext(ctx, network, serverAddr)
		},
	}
	return nr
}

func (n *NetResolver) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, n.Timeout)
}

func (n *NetResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	ctx, cancel := n.withTimeout(ctx)
	defer cancel()
	return n.r.LookupTXT(ctx, name)
}

func (n *NetResolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	ctx, cancel := n.withTimeout(ctx)
	defer cancel()
	return n.r.LookupMX(ctx, name)
}

func (n *NetResolver) LookupIP(ctx context.Context, host string) ([]net.IP, error) {
	ctx, cancel := n.withTimeout(ctx)
	defer cancel()
	addrs, err := n.r.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}

func (n *NetResolver) LookupAddr(ctx context.Context, addr string) ([]string, error) {
	ctx, cancel := n.withTimeout(ctx)
	defer cancel()
	return n.r.LookupAddr(ctx, addr)
}

// missingOrError classifies a lookup failure: NXDOMAIN/no-records is a
// definitive "missing"; anything else (timeout, SERVFAIL, network) is a
// transient DNS error that must not be reported as a missing record (§12).
func missingOrError(err error) string {
	var de *net.DNSError
	if errors.As(err, &de) && de.IsNotFound {
		return StatusMissing
	}
	return StatusDNSError
}
