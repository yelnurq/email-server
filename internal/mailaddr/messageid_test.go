package mailaddr

import (
	"fmt"
	"reflect"
	"testing"
)

// TestNormalizeMessageID is the V4 §6 matrix: every wire form of the same
// logical id must normalize to one canonical value, and RFC 5322 semantics
// (case-sensitive local part) must survive.
func TestNormalizeMessageID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// The four §6 forms.
		{"<abc@example.kz>", "abc@example.kz"},
		{"abc@example.kz", "abc@example.kz"},
		{" <abc@example.kz>", "abc@example.kz"},
		{"<ABC@example.kz>", "ABC@example.kz"}, // local part case is preserved
		// Domain case folds; local part does not.
		{"<Abc@EXAMPLE.KZ>", "Abc@example.kz"},
		{"ABC@Example.Kz", "ABC@example.kz"},
		// Whitespace and bracket tolerance.
		{"  abc@example.kz  ", "abc@example.kz"},
		{"< abc@example.kz >", "abc@example.kz"},
		{"<<abc@example.kz>>", "abc@example.kz"},
		{"<abc@example.kz", "abc@example.kz"}, // unbalanced (truncated input)
		{"abc@example.kz>", "abc@example.kz"},
		// Missing / malformed ids degrade deterministically, never panic.
		{"", ""},
		{"   ", ""},
		{"<>", ""},
		{"< >", ""},
		{"no-at-sign", "no-at-sign"}, // opaque token: no domain to fold
		{"<UPPER-NO-AT>", "UPPER-NO-AT"},
		{"@example.kz", "@example.kz"},
		{"abc@", "abc@"},
	}
	for _, c := range cases {
		if got := NormalizeMessageID(c.in); got != c.want {
			t.Errorf("NormalizeMessageID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeIsIdempotent: normalizing a canonical value is a no-op —
// required because every boundary normalizes defensively.
func TestNormalizeIsIdempotent(t *testing.T) {
	for _, in := range []string{"<abc@example.kz>", "ABC@Example.KZ", "no-at", "<<x@y>>"} {
		once := NormalizeMessageID(in)
		if twice := NormalizeMessageID(once); twice != once {
			t.Errorf("not idempotent: %q -> %q -> %q", in, once, twice)
		}
	}
}

func TestBracketMessageID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc@example.kz", "<abc@example.kz>"},
		{"<abc@example.kz>", "<abc@example.kz>"}, // never double-brackets
		{"ABC@Example.KZ", "<ABC@example.kz>"},
		{"", ""},
		{"<>", ""},
	}
	for _, c := range cases {
		if got := BracketMessageID(c.in); got != c.want {
			t.Errorf("BracketMessageID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseMessageIDList(t *testing.T) {
	got := ParseMessageIDList(" <a@x.kz> <B@Y.KZ>, a@x.kz  <c@z.kz> ")
	want := []string{"a@x.kz", "B@y.kz", "c@z.kz"} // deduped, order kept
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseMessageIDList = %v, want %v", got, want)
	}
	if out := ParseMessageIDList(""); len(out) != 0 {
		t.Errorf("empty input parsed to %v", out)
	}
}

func TestFormatMessageIDList(t *testing.T) {
	got := FormatMessageIDList([]string{"a@x.kz", "b@y.kz"})
	if got != "<a@x.kz> <b@y.kz>" {
		t.Errorf("FormatMessageIDList = %q", got)
	}
	if got := FormatMessageIDList(nil); got != "" {
		t.Errorf("nil list rendered %q", got)
	}
}

func TestAppendReference(t *testing.T) {
	refs := AppendReference(nil, "<root@x.kz>")
	refs = AppendReference(refs, "reply1@x.kz")
	refs = AppendReference(refs, "reply1@x.kz") // duplicate ignored
	if want := []string{"root@x.kz", "reply1@x.kz"}; !reflect.DeepEqual(refs, want) {
		t.Fatalf("AppendReference chain = %v, want %v", refs, want)
	}

	// Cap: root survives, middle is trimmed, newest entries kept.
	long := []string{"root@x.kz"}
	for i := 0; i < 30; i++ {
		long = AppendReference(long, fmt.Sprintf("reply%d@x.kz", i))
	}
	if len(long) > maxReferences {
		t.Fatalf("chain length %d exceeds cap %d", len(long), maxReferences)
	}
	if long[0] != "root@x.kz" {
		t.Fatalf("thread root was trimmed: %v", long[0])
	}
	if last := long[len(long)-1]; last != "reply29@x.kz" {
		t.Fatalf("newest reference lost, tail is %v", last)
	}
}
