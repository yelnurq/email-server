package mailaddr

import "testing"

func TestNormalizeAddress(t *testing.T) {
	local, domain, err := NormalizeAddress(" User.One+tag@Company.TEST ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if local != "user.one+tag" || domain != "company.test" {
		t.Fatalf("got %q@%q", local, domain)
	}
}

func TestNormalizeAddressRejects(t *testing.T) {
	for _, s := range []string{
		"", "@", "a@", "@b.test", "no-at-sign", "a b@c.test",
		"a@company", "a@-bad.test", "..dots@c.test", "x@y..test",
	} {
		if _, _, err := NormalizeAddress(s); err == nil {
			t.Fatalf("address %q accepted", s)
		}
	}
}
