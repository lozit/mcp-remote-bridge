package bridge

import "time"

// HostnameSettleTimeout bounds how long EnsureExposed waits for a freshly
// published hostname to become reachable.
//
// Measured 2026-08-21: a new hostname took about two minutes before the edge
// served it — the TCP connect failed outright until then, before TLS, so it is
// not a certificate matter. The spec names this as a handled failure mode
// ("hostname added but DNS not yet propagated → wait and verify"), not a real
// failure, so the tool waits rather than reporting red.
const HostnameSettleTimeout = 5 * time.Minute

// HostnameSettleInterval is the gap between attempts while waiting.
const HostnameSettleInterval = 10 * time.Second

// RetryCheck re-runs probe until it passes, or until timeout elapses.
//
// It returns the LAST result, so a caller that runs out of time reports the
// real reason rather than a synthetic "timed out": a red check must still say
// where it looked and what failed.
//
// probe is called at least once, even with a timeout of zero — a wait that
// never probes would report on nothing.
func RetryCheck(probe func() Check, timeout, interval time.Duration, sleep func(time.Duration)) Check {
	// STUB — the loop's maker replaces this body. Probing once and returning is
	// what retry_test.go proves insufficient.
	return probe()
}
