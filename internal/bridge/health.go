package bridge

// CheckName identifies one probe in a HealthReport.
type CheckName string

const (
	// CheckServiceLoaded reports whether the ServiceManager has the service loaded.
	CheckServiceLoaded CheckName = "service_loaded"
	// CheckProxyListening reports whether something answers on 127.0.0.1:PORT.
	CheckProxyListening CheckName = "proxy_listening"
	// CheckHostnameResolves reports whether the public hostname resolves.
	CheckHostnameResolves CheckName = "hostname_resolves"
	// CheckHostnameResponds reports whether the public hostname answers a request.
	CheckHostnameResponds CheckName = "hostname_responds"
	// CheckMCPInitialize is the deep probe: a real MCP initialize handshake
	// driven all the way through the exposed hostname.
	//
	// It exists because of the subtle trap: the proxy can be listening while the
	// MCP inside it is dead. Every other check passes in that state.
	CheckMCPInitialize CheckName = "mcp_initialize"
)

// Check is one probe actually run, with the evidence behind its verdict.
//
// A Check is never inferred from "we wrote the files". If a probe did not run,
// it does not appear in a HealthReport.
type Check struct {
	Name CheckName
	OK   bool
	// Detail names where the check looked (an address, a hostname, a label), so
	// a red result is actionable rather than merely negative.
	Detail string
	// Err is the underlying failure, if any.
	Err error
}

// HealthReport is the verdict for one entry, carrying the evidence behind it.
//
// Load-bearing rule 2: this is a record of probes run, never of files written.
// A health check that cannot fail is worse than none — it manufactures
// confidence.
type HealthReport struct {
	// Entry is the name of the entry this report describes.
	Entry string
	// Checks are the probes that actually ran, in the order they ran.
	Checks []Check
}

// Healthy reports whether every check passed.
//
// It is derived rather than stored, so a report cannot claim health that its
// checks do not support. An empty report is not healthy: nothing was proven.
func (r HealthReport) Healthy() bool {
	if len(r.Checks) == 0 {
		return false
	}
	for _, c := range r.Checks {
		if !c.OK {
			return false
		}
	}
	return true
}

// Failed returns the checks that did not pass.
func (r HealthReport) Failed() []Check {
	var out []Check
	for _, c := range r.Checks {
		if !c.OK {
			out = append(out, c)
		}
	}
	return out
}
