package bridge

import (
	"fmt"
	"net"
	"strconv"
)

// proxyHost is the only address the proxy probe is allowed to dial.
//
// Loopback is a security control, not a default: the proxy binds 127.0.0.1
// only, so a probe that succeeded against 0.0.0.0 or the public hostname would
// be proving the wrong thing.
const proxyHost = "127.0.0.1"

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
// proxy is alive — that is CheckMCPResponds's job.
func ProbeProxyListening(port int) Check {
	addr := net.JoinHostPort(proxyHost, strconv.Itoa(port))
	check := Check{Name: CheckProxyListening, Detail: addr}

	conn, err := net.DialTimeout("tcp", addr, ProxyDialTimeout)
	if err != nil {
		check.Err = fmt.Errorf("dialling %s: %w", addr, err)
		return check
	}
	// The probe only needs the handshake; holding the connection open would leak
	// a descriptor and, against a real proxy, occupy a slot for nothing.
	conn.Close()

	check.OK = true
	return check
}
