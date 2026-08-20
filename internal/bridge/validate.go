package bridge

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
	// STUB — the loop's maker replaces this body. Returning nil means "everything
	// is valid", which is exactly what validate_test.go proves wrong.
	return nil
}

// ValidateSubdomain reports whether s is usable as an Entry subdomain.
//
// Same rules as [ValidateName]: the subdomain becomes a DNS label.
func ValidateSubdomain(s string) error {
	// STUB — see ValidateName.
	return nil
}
