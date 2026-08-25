package bridge

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// A resolver that accepts the query and never answers is the failure this bound
// exists for: unbounded, the probe waits forever and `apply` hangs with no
// output. Asserting the bound without one would test nothing — the local
// resolver answers in milliseconds either way.
func TestProbeHostnameResolvesGivesUpOnASilentResolver(t *testing.T) {
	restore := resolver
	resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			<-ctx.Done() // accept, then never answer
			return nil, ctx.Err()
		},
	}
	defer func() { resolver = restore }()

	start := time.Now()
	check := probeHostnameResolvesWithin("silent.example.com", 100*time.Millisecond)
	elapsed := time.Since(start)

	if check.OK {
		t.Fatal("a resolver that never answered was reported as resolving")
	}
	if elapsed > time.Second {
		t.Errorf("the probe waited %s on a 100ms bound; it is not bounded", elapsed)
	}
	if check.Detail != "silent.example.com" {
		t.Errorf("Detail = %q, want the hostname looked up", check.Detail)
	}
	if !strings.Contains(check.Err.Error(), "no answer within") {
		t.Errorf("a timeout should say so rather than read as NXDOMAIN: %v", check.Err)
	}
}
