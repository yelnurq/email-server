package dnscheck

import (
	"fmt"
	"strings"
)

// SPFMechanism is one term of an SPF record ("ip4:1.2.3.4", "include:x.kz",
// "mx", "a:mail.x.kz", "all").
type SPFMechanism struct {
	Qualifier string // "+", "-", "~", "?" ("" means "+")
	Kind      string // ip4 | ip6 | a | mx | include | all | exists | ptr
	Value     string // argument after ":" or "/", "" when absent
}

// SPFRecord is a minimally parsed SPF policy — enough to evaluate whether
// the platform's sending architecture is authorized (§19). This is not a
// full RFC 7208 engine and never claims to be: macro expansion, recursive
// include evaluation and exp= are out of scope.
type SPFRecord struct {
	Raw          string
	Mechanisms   []SPFMechanism
	AllQualifier string // qualifier of the final "all" term, "" if none
	Redirect     string // redirect= modifier target, "" if none
}

// ParseSPF parses "v=spf1 ..." into terms.
func ParseSPF(raw string) (*SPFRecord, error) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 || !strings.EqualFold(fields[0], "v=spf1") {
		return nil, fmt.Errorf("record does not start with v=spf1")
	}
	rec := &SPFRecord{Raw: raw}
	for _, f := range fields[1:] {
		lf := strings.ToLower(f)
		if v, ok := strings.CutPrefix(lf, "redirect="); ok {
			rec.Redirect = v
			continue
		}
		if strings.Contains(lf, "=") {
			continue // other modifiers (exp=) are ignored
		}
		m := SPFMechanism{Qualifier: "+"}
		switch {
		case strings.HasPrefix(lf, "+"), strings.HasPrefix(lf, "-"),
			strings.HasPrefix(lf, "~"), strings.HasPrefix(lf, "?"):
			m.Qualifier = lf[:1]
			lf = lf[1:]
		}
		kind, value, _ := strings.Cut(lf, ":")
		// "a/24" and "mx/24" carry the CIDR after a slash instead.
		if value == "" {
			kind, value, _ = strings.Cut(kind, "/")
			if value != "" {
				value = "/" + value
			}
		}
		switch kind {
		case "ip4", "ip6", "a", "mx", "include", "all", "exists", "ptr":
			m.Kind = kind
			m.Value = value
		default:
			return nil, fmt.Errorf("unknown SPF mechanism %q", f)
		}
		if m.Kind == "all" {
			rec.AllQualifier = m.Qualifier
		}
		rec.Mechanisms = append(rec.Mechanisms, m)
	}
	return rec, nil
}

// parseTagList parses the "tag=value; tag=value" syntax shared by DKIM
// (RFC 6376 §3.2) and DMARC (RFC 7489 §6.4) records.
func parseTagList(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return out
}
