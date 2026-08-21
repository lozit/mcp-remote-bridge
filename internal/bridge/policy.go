package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// PolicyVerdict says what an unauthenticated request to a public hostname
// revealed about whether anything guards it.
//
// The three values are deliberately asymmetric: openness can be PROVEN, and
// protection can only be INFERRED. See ADR 0001.
type PolicyVerdict int

const (
	// PolicyUnknown: the request failed without a recognisable authentication
	// signature. A dead tunnel, an unpropagated DNS record and a crashed proxy
	// all land here, and so does an unfamiliar identity provider. Warn; never
	// block on this.
	PolicyUnknown PolicyVerdict = iota

	// PolicyGuarded: something answered with a positive authentication
	// signature. Inferred, best-effort, never a guarantee.
	PolicyGuarded

	// PolicyOpen: an unauthenticated MCP initialize SUCCEEDED. This is proof,
	// not a heuristic — the endpoint demonstrably serves anyone.
	PolicyOpen
)

func (v PolicyVerdict) String() string {
	switch v {
	case PolicyGuarded:
		return "guarded"
	case PolicyOpen:
		return "open"
	default:
		return "unknown"
	}
}

// accessHeaders are the response headers Cloudflare Access sets when it refuses
// an unauthenticated request. Measured on a live tunnel, 2026-08-21:
//
//	HTTP/2 403
//	cf-access-aud: 52050dc2…
//	cf-access-domain: hermes-mcp.paranoid.foo
var accessHeaders = []string{"cf-access-aud", "cf-access-domain"}

// CheckAccessPolicy asks whether hostname answers an MCP handshake with no
// credentials at all.
//
// It is the same `initialize` request the deep probe sends, stripped of its
// authentication — the check reuses the probe's question rather than adding a
// new one. What differs is how the answer is read: here the HTTP response
// itself is the evidence, not the JSON-RPC result.
//
// Never conclude "guarded" from a generic failure. A dead tunnel fails exactly
// like a policy does, and reading that as security would turn an outage into a
// false sense of safety.
func CheckAccessPolicy(ctx context.Context, client *http.Client, hostname string) (PolicyVerdict, string) {
	endpoint := PublicEndpoint(hostname)

	body := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "mcp-remote-bridge-policy-check", "version": "0"},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return PolicyUnknown, fmt.Sprintf("could not build the request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return PolicyUnknown, fmt.Sprintf("could not build the request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	// Deliberately no credentials: that is the whole question.

	// A redirect to the identity provider IS the signature, so it must be
	// observed rather than followed. Go's default client follows redirects, and
	// following one would land on a login page and return "unknown" for a
	// hostname that is in fact guarded — the exact wrong direction for this
	// check. The caller's client is copied rather than mutated.
	noFollow := *client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := noFollow.Do(req)
	if err != nil {
		return PolicyUnknown, fmt.Sprintf("%s did not answer (%v); this says nothing about whether it is guarded", hostname, err)
	}
	defer resp.Body.Close()

	if v, why := verdictFromHeaders(resp, hostname); v != PolicyUnknown {
		return v, why
	}

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return PolicyUnknown, fmt.Sprintf("%s answered HTTP %d but the body could not be read", hostname, resp.StatusCode)
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		payload = firstSSEData(payload)
	}

	var rpc jsonRPCResponse
	if err := json.Unmarshal(payload, &rpc); err == nil && rpc.Error == nil && len(rpc.Result) > 0 {
		// An unauthenticated handshake completed. Nothing is guarding this.
		return PolicyOpen, fmt.Sprintf("an unauthenticated MCP initialize on %s succeeded (HTTP %d)", hostname, resp.StatusCode)
	}

	return PolicyUnknown, fmt.Sprintf(
		"%s answered HTTP %d without an authentication signature and without completing a handshake; "+
			"it may be guarded by something unrecognised, or simply broken", hostname, resp.StatusCode)
}

// verdictFromHeaders looks for a POSITIVE authentication signature.
//
// Only two shapes count, and both are evidence that something deliberately
// refused the request rather than that something is merely absent:
//   - Cloudflare Access headers on the response
//   - a redirect to an identity provider
func verdictFromHeaders(resp *http.Response, hostname string) (PolicyVerdict, string) {
	for _, h := range accessHeaders {
		if resp.Header.Get(h) != "" {
			return PolicyGuarded, fmt.Sprintf("%s is behind Cloudflare Access (HTTP %d, %s present)", hostname, resp.StatusCode, h)
		}
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "cloudflareaccess.com") || strings.Contains(loc, "/cdn-cgi/access/") {
			return PolicyGuarded, fmt.Sprintf("%s redirects to an identity provider (HTTP %d)", hostname, resp.StatusCode)
		}
	}
	return PolicyUnknown, ""
}
