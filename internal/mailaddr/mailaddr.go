// Package mailaddr validates and normalizes email addresses, local parts and
// domain names for the platform. Validation is deliberately stricter than RFC
// 5321 (no quoted local parts, no address literals) — this is a provisioning
// policy, not a parser for arbitrary internet mail.
package mailaddr

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	localPartRe = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9._+-]{0,62}[a-zA-Z0-9])?$`)
	domainRe    = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$`)
)

// NormalizeLocalPart lowercases and validates a mailbox local part.
func NormalizeLocalPart(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !localPartRe.MatchString(s) {
		return "", fmt.Errorf("invalid local part %q", s)
	}
	if strings.Contains(s, "..") {
		return "", fmt.Errorf("invalid local part %q", s)
	}
	return s, nil
}

// NormalizeDomain lowercases and validates a domain name.
func NormalizeDomain(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(s, ".")))
	if len(s) > 253 || !domainRe.MatchString(s) {
		return "", fmt.Errorf("invalid domain %q", s)
	}
	return s, nil
}

// NormalizeAddress splits and validates local@domain.
func NormalizeAddress(s string) (local, domain string, err error) {
	s = strings.TrimSpace(s)
	at := strings.LastIndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return "", "", fmt.Errorf("invalid email address %q", s)
	}
	local, err = NormalizeLocalPart(s[:at])
	if err != nil {
		return "", "", fmt.Errorf("invalid email address %q", s)
	}
	domain, err = NormalizeDomain(s[at+1:])
	if err != nil {
		return "", "", fmt.Errorf("invalid email address %q", s)
	}
	return local, domain, nil
}

// Join renders local@domain.
func Join(local, domain string) string { return local + "@" + domain }
