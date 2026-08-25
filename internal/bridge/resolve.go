package bridge

import (
	"errors"
	"fmt"
	"net"
)

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
	check := Check{Name: CheckHostnameResolves, Detail: hostname}

	// An empty hostname is a caller bug, not a DNS verdict. Reporting it as "does
	// not resolve" would send the reader hunting for a record that was never
	// asked for.
	if hostname == "" {
		check.Detail = "(empty hostname)"
		check.Err = errors.New("no hostname to look up")
		return check
	}

	addrs, err := net.LookupHost(hostname)
	if err != nil {
		check.Err = fmt.Errorf("resolving %s: %w", hostname, err)
		return check
	}
	// LookupHost reports an empty answer as an error today, but the check is on
	// the evidence, not on the contract: no address is no proof of resolution.
	if len(addrs) == 0 {
		check.Err = fmt.Errorf("resolving %s: no addresses returned", hostname)
		return check
	}

	check.OK = true
	return check
}
