package dnscheck

import (
	"context"
	"net"
	"strings"
	"testing"
)

// fakeResolver is the controlled DNS fixture (§123): tests never touch the
// public internet.
type fakeResolver struct {
	txt  map[string][]string
	mx   map[string][]*net.MX
	ip   map[string][]net.IP
	ptr  map[string][]string
	errs map[string]error // per-name forced error
}

func nxdomain(name string) *net.DNSError {
	return &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func servfail(name string) *net.DNSError {
	return &net.DNSError{Err: "server misbehaving", Name: name, IsTemporary: true}
}

func (f *fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if err := f.errs[name]; err != nil {
		return nil, err
	}
	if v, ok := f.txt[name]; ok {
		return v, nil
	}
	return nil, nxdomain(name)
}

func (f *fakeResolver) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	if err := f.errs[name]; err != nil {
		return nil, err
	}
	if v, ok := f.mx[name]; ok {
		return v, nil
	}
	return nil, nxdomain(name)
}

func (f *fakeResolver) LookupIP(_ context.Context, host string) ([]net.IP, error) {
	if v, ok := f.ip[host]; ok {
		return v, nil
	}
	return nil, nxdomain(host)
}

func (f *fakeResolver) LookupAddr(_ context.Context, addr string) ([]string, error) {
	if err := f.errs[addr]; err != nil {
		return nil, err
	}
	if v, ok := f.ptr[addr]; ok {
		return v, nil
	}
	return nil, nxdomain(addr)
}

func exp() Expectations {
	return Expectations{
		Domain:         "example.kz",
		MailHostname:   "mail.example.kz",
		OutboundIP:     "203.0.113.25",
		OwnershipToken: "tok123",
		DKIMSelector:   "s1",
		DKIMPublicKey:  "PUBKEYBASE64",
	}
}

func TestMX(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		mx     []*net.MX
		err    error
		status string
	}{
		{"verified", []*net.MX{{Host: "mail.example.kz.", Pref: 10}}, nil, StatusVerified},
		{"missing", nil, nxdomain("example.kz"), StatusMissing},
		{"foreign", []*net.MX{{Host: "mx.google.com.", Pref: 10}}, nil, StatusInvalid},
		{"mixed", []*net.MX{{Host: "mail.example.kz.", Pref: 10}, {Host: "mx.other.kz.", Pref: 20}}, nil, StatusWarning},
		{"timeout", nil, servfail("example.kz"), StatusDNSError},
	}
	for _, c := range cases {
		f := &fakeResolver{mx: map[string][]*net.MX{}, errs: map[string]error{}}
		if c.err != nil {
			f.errs["example.kz"] = c.err
		} else if c.mx != nil {
			f.mx["example.kz"] = c.mx
		}
		ck := (&Checker{Resolver: f}).CheckMX(ctx, exp())
		if ck.Status != c.status {
			t.Errorf("%s: status = %s, want %s (%s)", c.name, ck.Status, c.status, ck.Detail)
		}
	}
}

// TestSPF is the §124 matrix: correct / missing / malformed / multiple.
func TestSPF(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		txts   []string
		status string
	}{
		{"correct-ip4", []string{"v=spf1 ip4:203.0.113.25 mx ~all"}, StatusVerified},
		{"correct-cidr", []string{"v=spf1 ip4:203.0.113.0/24 ~all"}, StatusVerified},
		{"correct-mx", []string{"v=spf1 mx ~all"}, StatusVerified},
		{"missing", nil, StatusMissing},
		{"unrelated-txt-only", []string{"google-site-verification=abc"}, StatusMissing},
		{"malformed", []string{"v=spf1 ip44:banana zz"}, StatusInvalid},
		{"multiple", []string{"v=spf1 mx ~all", "v=spf1 include:other.kz ~all"}, StatusInvalid},
		{"foreign-only", []string{"v=spf1 include:_spf.google.com ~all"}, StatusWarning},
		{"plus-all", []string{"v=spf1 mx +all"}, StatusWarning},
	}
	for _, c := range cases {
		f := &fakeResolver{txt: map[string][]string{}}
		if c.txts != nil {
			f.txt["example.kz"] = c.txts
		}
		ck := (&Checker{Resolver: f}).CheckSPF(ctx, exp())
		if ck.Status != c.status {
			t.Errorf("%s: status = %s, want %s (%s)", c.name, ck.Status, c.status, ck.Detail)
		}
	}
}

// TestDKIM is the §125 matrix: correct key / missing / wrong selector /
// wrong key, plus the not-yet-generated state.
func TestDKIM(t *testing.T) {
	ctx := context.Background()
	e := exp()
	sel := "s1._domainkey.example.kz"

	f := &fakeResolver{txt: map[string][]string{sel: {"v=DKIM1; k=rsa; p=PUBKEYBASE64"}}}
	if ck := (&Checker{Resolver: f}).CheckDKIM(ctx, e); ck.Status != StatusVerified {
		t.Errorf("correct key: %s (%s)", ck.Status, ck.Detail)
	}

	f = &fakeResolver{txt: map[string][]string{}}
	if ck := (&Checker{Resolver: f}).CheckDKIM(ctx, e); ck.Status != StatusMissing {
		t.Errorf("missing: %s", ck.Status)
	}

	// Wrong selector: the record exists at s2, we query s1 → missing.
	f = &fakeResolver{txt: map[string][]string{"s2._domainkey.example.kz": {"v=DKIM1; p=PUBKEYBASE64"}}}
	if ck := (&Checker{Resolver: f}).CheckDKIM(ctx, e); ck.Status != StatusMissing {
		t.Errorf("wrong selector: %s", ck.Status)
	}

	f = &fakeResolver{txt: map[string][]string{sel: {"v=DKIM1; k=rsa; p=OTHERKEY"}}}
	if ck := (&Checker{Resolver: f}).CheckDKIM(ctx, e); ck.Status != StatusInvalid {
		t.Errorf("wrong key: %s", ck.Status)
	}

	// Revoked (empty p=).
	f = &fakeResolver{txt: map[string][]string{sel: {"v=DKIM1; p="}}}
	if ck := (&Checker{Resolver: f}).CheckDKIM(ctx, e); ck.Status != StatusInvalid {
		t.Errorf("revoked: %s", ck.Status)
	}

	// No key generated yet.
	e2 := exp()
	e2.DKIMSelector, e2.DKIMPublicKey = "", ""
	if ck := (&Checker{Resolver: f}).CheckDKIM(ctx, e2); ck.Status != StatusPending {
		t.Errorf("ungenerated: %s", ck.Status)
	}
}

// TestDMARC is the §126 matrix: none / p=none / quarantine / reject /
// malformed.
func TestDMARC(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		txts   []string
		status string
	}{
		{"absent", nil, StatusMissing},
		{"p-none", []string{"v=DMARC1; p=none; rua=mailto:d@example.kz"}, StatusWarning},
		{"quarantine", []string{"v=DMARC1; p=quarantine; rua=mailto:d@example.kz"}, StatusVerified},
		{"reject", []string{"v=DMARC1; p=reject"}, StatusVerified},
		{"malformed", []string{"v=DMARC1; policy=broken"}, StatusInvalid},
		{"multiple", []string{"v=DMARC1; p=none", "v=DMARC1; p=reject"}, StatusInvalid},
	}
	for _, c := range cases {
		f := &fakeResolver{txt: map[string][]string{}}
		if c.txts != nil {
			f.txt["_dmarc.example.kz"] = c.txts
		}
		ck := (&Checker{Resolver: f}).CheckDMARC(ctx, exp())
		if ck.Status != c.status {
			t.Errorf("%s: status = %s, want %s (%s)", c.name, ck.Status, c.status, ck.Detail)
		}
	}
}

func TestOwnership(t *testing.T) {
	ctx := context.Background()
	host := "_mailplatform.example.kz"

	f := &fakeResolver{txt: map[string][]string{host: {"mailplatform-verify=tok123"}}}
	if ck := (&Checker{Resolver: f}).CheckOwnership(ctx, exp()); ck.Status != StatusVerified {
		t.Errorf("verified: %s", ck.Status)
	}
	f = &fakeResolver{txt: map[string][]string{host: {"mailplatform-verify=WRONG"}}}
	if ck := (&Checker{Resolver: f}).CheckOwnership(ctx, exp()); ck.Status != StatusInvalid {
		t.Errorf("wrong token: %s", ck.Status)
	}
	f = &fakeResolver{txt: map[string][]string{}}
	if ck := (&Checker{Resolver: f}).CheckOwnership(ctx, exp()); ck.Status != StatusMissing {
		t.Errorf("missing: %s", ck.Status)
	}
}

func TestPTRForwardConfirmed(t *testing.T) {
	ctx := context.Background()
	ip := "203.0.113.25"

	// Full forward-confirmed loop.
	f := &fakeResolver{
		ptr: map[string][]string{ip: {"mail.example.kz."}},
		ip:  map[string][]net.IP{"mail.example.kz": {net.ParseIP(ip)}},
	}
	if ck := (&Checker{Resolver: f}).CheckPTR(ctx, exp()); ck.Status != StatusVerified {
		t.Errorf("fcrdns: %s (%s)", ck.Status, ck.Detail)
	}

	// PTR names another host.
	f = &fakeResolver{ptr: map[string][]string{ip: {"vps-1234.hosting.example."}}}
	if ck := (&Checker{Resolver: f}).CheckPTR(ctx, exp()); ck.Status != StatusInvalid {
		t.Errorf("mismatch: %s", ck.Status)
	}

	// Forward lookup disagrees → warning (§32).
	f = &fakeResolver{
		ptr: map[string][]string{ip: {"mail.example.kz."}},
		ip:  map[string][]net.IP{"mail.example.kz": {net.ParseIP("198.51.100.9")}},
	}
	if ck := (&Checker{Resolver: f}).CheckPTR(ctx, exp()); ck.Status != StatusWarning {
		t.Errorf("forward mismatch: %s", ck.Status)
	}

	// Local development: no outbound IP.
	e := exp()
	e.OutboundIP = ""
	if ck := (&Checker{Resolver: f}).CheckPTR(ctx, e); ck.Status != StatusPending {
		t.Errorf("dev: %s", ck.Status)
	}
	if !strings.Contains(ck2Detail(t, f, e), "not applicable") {
		t.Error("dev detail should say not applicable")
	}
}

func ck2Detail(t *testing.T, f Resolver, e Expectations) string {
	t.Helper()
	return (&Checker{Resolver: f}).CheckPTR(context.Background(), e).Detail
}

func TestTransientNeverMissing(t *testing.T) {
	ctx := context.Background()
	f := &fakeResolver{errs: map[string]error{
		"example.kz":               servfail("example.kz"),
		"_dmarc.example.kz":        servfail("_dmarc.example.kz"),
		"_mailplatform.example.kz": servfail("_mailplatform.example.kz"),
	}}
	c := &Checker{Resolver: f}
	for _, ck := range []Check{
		c.CheckSPF(ctx, exp()), c.CheckDMARC(ctx, exp()), c.CheckOwnership(ctx, exp()), c.CheckMX(ctx, exp()),
	} {
		if ck.Status == StatusMissing {
			t.Errorf("%s: transient failure classified as missing", ck.Type)
		}
		if ck.Status != StatusDNSError {
			t.Errorf("%s: status = %s, want dns_error", ck.Type, ck.Status)
		}
	}
}

func TestParseSPF(t *testing.T) {
	rec, err := ParseSPF("v=spf1 ip4:1.2.3.0/24 a mx include:spf.other.kz ~all")
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Mechanisms) != 5 {
		t.Fatalf("mechanisms = %d, want 5", len(rec.Mechanisms))
	}
	if rec.AllQualifier != "~" {
		t.Errorf("all qualifier = %q", rec.AllQualifier)
	}
	if rec.Mechanisms[0].Kind != "ip4" || rec.Mechanisms[0].Value != "1.2.3.0/24" {
		t.Errorf("first mechanism = %+v", rec.Mechanisms[0])
	}
	if _, err := ParseSPF("v=spf2 x"); err == nil {
		t.Error("v=spf2 must not parse")
	}
	if _, err := ParseSPF("v=spf1 bogus:thing"); err == nil {
		t.Error("unknown mechanism must not parse")
	}
	if rec, err := ParseSPF("v=spf1 redirect=_spf.example.com"); err != nil || rec.Redirect != "_spf.example.com" {
		t.Errorf("redirect parse: %v %+v", err, rec)
	}
}
