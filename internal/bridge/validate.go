package bridge

import "fmt"

// maxLabelLen is the DNS label maximum, and the upper bound shared by a name
// and a subdomain.
const maxLabelLen = 63

// ValidateName reports whether s is usable as an Entry name.
//
// A name becomes three things at once: a service label, a hostname component
// and a log path. It is therefore validated against a strict charset and
// REJECTED when it does not match — never sanitized, because a sanitized name
// silently addresses something other than what the user wrote.
//
// Rules (all must hold):
//   - length 1..63 (a DNS label maximum)
//   - characters a-z, 0-9 and '-' only (lowercase; no uppercase, no unicode)
//   - does not start or end with '-'
//
// Returns nil when s is valid, a non-nil error describing the violation otherwise.
func ValidateName(s string) error {
	return validateLabel("name", s)
}

// ValidateSubdomain reports whether s is usable as an Entry subdomain.
//
// Same rules as [ValidateName]: the subdomain becomes a DNS label.
func ValidateSubdomain(s string) error {
	return validateLabel("subdomain", s)
}

// validateLabel holds the rules both exported validators enforce. kind names
// the field in the error so the caller can act on it without wrapping.
//
// The scan is byte-wise on purpose: the charset is ASCII-only, so a multi-byte
// rune is rejected by the very first of its bytes rather than silently counted
// as one character.
func validateLabel(kind, s string) error {
	if s == "" {
		return fmt.Errorf("%s is empty: expected 1 to %d characters", kind, maxLabelLen)
	}
	if len(s) > maxLabelLen {
		return fmt.Errorf("%s %q is %d characters long: the maximum is %d", kind, s, len(s), maxLabelLen)
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			continue
		default:
			return fmt.Errorf("%s %q contains %q at position %d: only a-z, 0-9 and '-' are allowed", kind, s, string(rune(c)), i)
		}
	}
	if s[0] == '-' {
		return fmt.Errorf("%s %q starts with '-'", kind, s)
	}
	if s[len(s)-1] == '-' {
		return fmt.Errorf("%s %q ends with '-'", kind, s)
	}
	return nil
}
