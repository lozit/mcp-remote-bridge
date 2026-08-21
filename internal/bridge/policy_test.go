package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The response shapes are copied from live measurements on 2026-08-21, not
// invented: a guarded hostname answered 403 with cf-access-aud, and an open one
// answered 200 with a valid initialize result.

func checkAgainst(t *testing.T, h http.HandlerFunc) (PolicyVerdict, string) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// CheckAccessPolicy builds an https:// URL from the hostname, so the test
	// server is reached through a client whose transport rewrites the target.
	client := &http.Client{Transport: rewriteTo(srv.URL)}
	return CheckAccessPolicy(context.Background(), client, "probe.example.com")
}

// rewriteTo sends every request to the test server, preserving the path.
type rewriter struct {
	base string
	next http.RoundTripper
}

func (r rewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	u := *req.URL
	target, err := http.NewRequest(req.Method, r.base+u.Path, req.Body)
	if err != nil {
		return nil, err
	}
	target.Header = req.Header
	return r.next.RoundTrip(target)
}

func rewriteTo(base string) http.RoundTripper {
	return rewriter{base: base, next: http.DefaultTransport}
}

// PROOF of openness: an unauthenticated handshake completed.
func TestCheckAccessPolicyDetectsAnOpenHostname(t *testing.T) {
	verdict, why := checkAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(AccessClientIDHeader) != "" || r.Header.Get(AccessClientSecretHeader) != "" {
			t.Error("the policy check sent credentials; the whole question is whether it works WITHOUT them")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"mcp-freestyle","version":"1.29.0"}}}`))
	})

	if verdict != PolicyOpen {
		t.Errorf("verdict = %v, want open — an unauthenticated initialize succeeded", verdict)
	}
	if !strings.Contains(why, "probe.example.com") {
		t.Errorf("the explanation should name the hostname: %q", why)
	}
}

// Cloudflare Access, exactly as measured.
func TestCheckAccessPolicyRecognisesCloudflareAccess(t *testing.T) {
	verdict, why := checkAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("cf-access-aud", "52050dc2d198baff7dbe17da872154cb8acfa8ee351099aba81d4b1ce6b16431")
		w.Header().Set("cf-access-domain", "probe.example.com")
		w.WriteHeader(http.StatusForbidden)
	})

	if verdict != PolicyGuarded {
		t.Errorf("verdict = %v, want guarded: %s", verdict, why)
	}
}

func TestCheckAccessPolicyRecognisesAnIdPRedirect(t *testing.T) {
	verdict, _ := checkAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://team.cloudflareaccess.com/cdn-cgi/access/login/probe.example.com")
		w.WriteHeader(http.StatusFound)
	})

	if verdict != PolicyGuarded {
		t.Errorf("verdict = %v, want guarded", verdict)
	}
}

// The dangerous direction: a generic failure must NEVER read as protection. A
// dead tunnel fails exactly like a policy does.
func TestCheckAccessPolicyNeverReadsAFailureAsProtection(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"bare 403, no Access headers": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		},
		"502 from a dead origin": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		},
		"406 from the proxy itself": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotAcceptable)
		},
		"redirect somewhere unrelated": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", "https://example.com/elsewhere")
			w.WriteHeader(http.StatusFound)
		},
		"a JSON-RPC error rather than a result": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":0,"message":""}}`))
		},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			verdict, why := checkAgainst(t, h)
			if verdict == PolicyGuarded {
				t.Errorf("a generic failure was read as protection: %s", why)
			}
			if verdict != PolicyUnknown {
				t.Errorf("verdict = %v, want unknown: %s", verdict, why)
			}
		})
	}
}

// A hostname that does not answer at all says nothing about protection.
func TestCheckAccessPolicyOnAnUnreachableHostIsUnknown(t *testing.T) {
	client := &http.Client{Transport: rewriteTo("http://127.0.0.1:1")}
	verdict, why := CheckAccessPolicy(context.Background(), client, "probe.example.com")

	if verdict != PolicyUnknown {
		t.Errorf("verdict = %v, want unknown", verdict)
	}
	if !strings.Contains(why, "says nothing") {
		t.Errorf("the explanation should be explicit that this proves nothing: %q", why)
	}
}
