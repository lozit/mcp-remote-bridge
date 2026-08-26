package bridge

import "time"

// CheckName identifies one probe in a HealthReport.
type CheckName string

const (
	// CheckServiceLoaded reports whether the ServiceManager has the service loaded.
	CheckServiceLoaded CheckName = "service_loaded"
	// CheckProxyListening reports whether something answers on 127.0.0.1:PORT.
	CheckProxyListening CheckName = "proxy_listening"
	// CheckHostnameResponds reports whether the public hostname answers a request.
	CheckHostnameResponds CheckName = "hostname_responds"
	// CheckMCPResponds is the deep probe: a JSON-RPC call that must carry data
	// back from the MCP process itself, with the verdict read from the response
	// body rather than the HTTP status.
	//
	// It exists because of the subtle trap: the proxy can be listening while the
	// MCP inside it is dead. Every other check passes in that state.
	//
	// It is deliberately NOT named mcp_initialize. Measured against mcp-proxy
	// 0.12.0, both `initialize` AND `ping` are answered by the proxy from the
	// state it negotiated at startup, so neither reaches the MCP and neither can
	// fail when the MCP is dead. `ping` is the worse trap of the two: the
	// protocol advertises it for liveness, so its name disarms the reader.
	// A dead MCP also still returns HTTP 200 — the failure is in the body.
	// See docs/decisions/0003-liveness-probe-must-carry-data.md.
	CheckMCPResponds CheckName = "mcp_responds"
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

// ProxyDialTimeout bounds the TCP dial in ProbeProxyListening. A probe that can
// hang is a probe that never reports.
const ProxyDialTimeout = 2 * time.Second
