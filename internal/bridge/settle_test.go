package bridge

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// settleServer answers "not there yet" for the first n calls, then the given
// handler — the shape of a hostname the edge has not started serving.
func settleServer(t *testing.T, n int32, then http.HandlerFunc) (*Bridge, *int32, *time.Duration) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= n {
			// A connection that is accepted but answers nothing conclusive is the
			// closest a test server gets to "the edge is not serving this yet".
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		then(w, r)
	}))
	t.Cleanup(srv.Close)

	var slept time.Duration
	b := New(nil, nil, nil)
	b.Sleep = func(d time.Duration) { slept += d }
	b.HTTPClientForTest = &http.Client{Transport: rewriteTo(srv.URL), Timeout: 5 * time.Second}
	return b, &calls, &slept
}

func TestSettleAccessPolicyWaitsForTheEdge(t *testing.T) {
	guarded := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("cf-access-aud", "aud")
		w.WriteHeader(http.StatusForbidden)
	}
	b, calls, slept := settleServer(t, 3, guarded)

	verdict, why := b.settleAccessPolicy("probe.example.com")

	if verdict != PolicyGuarded {
		t.Fatalf("verdict = %v, want guarded after the edge came up: %s", verdict, why)
	}
	if *calls != 4 {
		t.Errorf("probed %d times, want 4 (three inconclusive then a verdict)", *calls)
	}
	if *slept == 0 {
		t.Error("did not wait at all for a hostname that was not being served yet")
	}
}

// An open hostname is a conclusion too: the wait must stop, not keep hoping it
// turns guarded.
func TestSettleAccessPolicyStopsOnAnOpenVerdict(t *testing.T) {
	open := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"x"}}}`))
	}
	b, calls, slept := settleServer(t, 0, open)

	verdict, _ := b.settleAccessPolicy("probe.example.com")

	if verdict != PolicyOpen {
		t.Fatalf("verdict = %v, want open", verdict)
	}
	if *calls != 1 || *slept != 0 {
		t.Errorf("probed %d times and slept %v for an immediate verdict", *calls, *slept)
	}
}

// Giving up keeps the last real explanation rather than inventing one.
func TestSettleAccessPolicyKeepsTheLastReasonWhenItGivesUp(t *testing.T) {
	never := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }
	b, calls, _ := settleServer(t, 1<<30, never)

	verdict, why := b.settleAccessPolicy("probe.example.com")

	if verdict != PolicyUnknown {
		t.Fatalf("verdict = %v, want unknown", verdict)
	}
	if why == "" {
		t.Error("gave up without an explanation")
	}
	if *calls < 2 {
		t.Errorf("probed %d times before giving up on a 5-minute budget", *calls)
	}
}
