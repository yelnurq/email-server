package mailaddr

import "strings"

// Canonical Message-ID form (V4 data-integrity decision): the platform
// stores and compares RFC 5322 Message-IDs as the bare "id-left@id-right"
// with the domain half lowercased. Angle brackets are wire format only —
// they are stripped on every ingest boundary and re-added exclusively when
// rendering headers. Rationale: PostgreSQL used to store "<id@domain>" while
// JMAP reports "id@domain"; comparing the two raw forms silently never
// matched (the V3 duplicate-import incident and two dead webmail queries).
//
// Case: RFC 5322 defines id-left (the local part) as case-SENSITIVE, so only
// the domain half is lowercased. Lowercasing the whole id would merge two
// distinct foreign Message-IDs and could destroy real mail during dedupe.

// NormalizeMessageID returns the canonical form of one RFC 5322 Message-ID:
// surrounding whitespace and angle brackets removed, domain half lowercased.
// Malformed input degrades gracefully — the result is still deterministic,
// so equal inputs always compare equal. Empty input returns "".
func NormalizeMessageID(id string) string {
	id = strings.TrimSpace(id)
	// Strip balanced bracket pairs (tolerates "<<a@b>>" and "< a@b >").
	for len(id) >= 2 && id[0] == '<' && id[len(id)-1] == '>' {
		id = strings.TrimSpace(id[1 : len(id)-1])
	}
	// Tolerate a single unbalanced bracket from truncated input.
	id = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(id, "<"), ">"))
	if id == "" {
		return ""
	}
	if at := strings.LastIndexByte(id, '@'); at >= 0 {
		id = id[:at+1] + strings.ToLower(id[at+1:])
	}
	return id
}

// BracketMessageID renders a Message-ID in RFC 5322 wire form ("<id>").
// The input is normalized first, so already-bracketed input never becomes
// double-bracketed. Empty input renders as "".
func BracketMessageID(id string) string {
	id = NormalizeMessageID(id)
	if id == "" {
		return ""
	}
	return "<" + id + ">"
}

// ParseMessageIDList splits an In-Reply-To/References header value (or the
// platform's space-joined storage form) into canonical Message-IDs, dropping
// empties and duplicates while preserving order.
func ParseMessageIDList(s string) []string {
	fields := strings.Fields(strings.ReplaceAll(s, ",", " "))
	var out []string
	seen := map[string]bool{}
	for _, f := range fields {
		id := NormalizeMessageID(f)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// FormatMessageIDList renders canonical ids as an RFC 5322 header value:
// each id bracketed, space-separated.
func FormatMessageIDList(ids []string) string {
	var b strings.Builder
	for i, id := range ids {
		w := BracketMessageID(id)
		if w == "" {
			continue
		}
		if i > 0 && b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(w)
	}
	return b.String()
}

// maxReferences caps the stored References chain: the thread root is kept
// and the tail keeps the most recent entries, per RFC 5322 App. B guidance
// that trimming happens in the middle, never at the ends.
const maxReferences = 20

// AppendReference extends a canonical References list with the parent's id,
// deduplicating and capping the chain length.
func AppendReference(refs []string, parent string) []string {
	parent = NormalizeMessageID(parent)
	if parent != "" {
		found := false
		for _, r := range refs {
			if r == parent {
				found = true
				break
			}
		}
		if !found {
			refs = append(refs, parent)
		}
	}
	if len(refs) > maxReferences {
		head := refs[0]
		tail := refs[len(refs)-(maxReferences-1):]
		refs = append([]string{head}, tail...)
	}
	return refs
}
