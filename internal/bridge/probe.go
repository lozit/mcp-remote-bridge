package bridge

// ProbeProxyListening checks whether something is accepting TCP connections on
// 127.0.0.1:port, and returns the Check recording what it found.
//
// It dials loopback only — never 0.0.0.0, never the public hostname. The dial
// times out after ProxyDialTimeout.
//
// The returned Check always has Name == CheckProxyListening and a Detail naming
// the address that was dialled, whether it passed or failed: a red result that
// does not say where it looked is not actionable.
//
// This is the shallow check. It says nothing about whether the MCP behind the
// proxy is alive — that is CheckMCPInitialize's job.
func ProbeProxyListening(port int) Check {
	// STUB — the loop's maker replaces this body. Returning a zero-value Check
	// is exactly what probe_test.go proves wrong.
	return Check{Name: CheckProxyListening}
}
