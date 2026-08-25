package bridge

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// DNSLookupTimeout bounds the lookup in ProbeHostnameResolves.
//
// Every other probe is bounded; this one was not, and it is the one most
// exposed to a resolver that accepts the query and never answers. Unbounded, a
// single unreachable nameserver turns `apply` into a command that hangs with no
// output — the failure mode the health report exists to replace. The value is
// generous next to a working resolver (milliseconds) and short next to a human
// waiting.
const DNSLookupTimeout = 5 * time.Second

// resolver is the resolver the probe uses. It is a variable so a test can point
// it at a nameserver that never answers, which is the only way to observe that
// the bound is enforced rather than merely written.
var resolver = &net.Resolver{}

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
	return probeHostnameResolvesWithin(hostname, DNSLookupTimeout)
}

func probeHostnameResolvesWithin(hostname string, timeout time.Duration) Check {
	check := Check{Name: CheckHostnameResolves, Detail: hostname}

	// An empty hostname is a caller bug, not a DNS verdict. Reporting it as "does
	// not resolve" would send the reader hunting for a record that was never
	// asked for.
	if hostname == "" {
		check.Detail = "(empty hostname)"
		check.Err = errors.New("no hostname to look up")
		return check
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	addrs, err := resolver.LookupHost(ctx, hostname)
	if err != nil {
		// A timeout and a genuine NXDOMAIN send the reader to different places,
		// so the deadline says so rather than hiding inside a generic i/o error.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			check.Err = fmt.Errorf("resolving %s: no answer within %s", hostname, timeout)
			return check
		}
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
