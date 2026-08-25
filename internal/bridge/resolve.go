package bridge

// ProbeHostnameResolves checks whether hostname resolves in DNS.
//
// It is the shallowest of the hostname checks and exists to separate two
// failures that otherwise look alike: a name that does not resolve at all, and
// one that resolves but does not answer. Told apart, the first points at DNS or
// a missing record and the second at the tunnel or the origin.
//
// The returned Check always names the hostname in Detail, whether it passed or
// failed: a red result that does not say what it looked up is not actionable.
func ProbeHostnameResolves(hostname string) Check {
	// STUB — the loop's maker replaces this body. Returning a zero-value Check
	// is what resolve_test.go proves wrong.
	return Check{Name: CheckHostnameResolves}
}
