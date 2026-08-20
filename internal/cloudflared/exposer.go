// Package cloudflared implements bridge.Exposer against a named, already
// authenticated Cloudflare tunnel.
//
// The tunnel itself is a precondition: this package adds hostnames to a tunnel,
// it never creates or authenticates one.
package cloudflared

import (
	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// Exposer routes a public hostname to a local port through a Cloudflare tunnel.
type Exposer struct {
	// Tunnel is the name of an existing, authenticated tunnel.
	Tunnel string
}

// New returns an Exposer bound to the named tunnel.
func New(tunnel string) *Exposer { return &Exposer{Tunnel: tunnel} }

// Ensure adds the ingress rule subdomain.domain -> localhost:localPort and its
// DNS route.
func (e *Exposer) Ensure(subdomain, domain string, localPort int) error {
	return bridge.ErrNotImplemented
}

// Remove drops the ingress rule and its DNS route.
func (e *Exposer) Remove(subdomain, domain string) error {
	return bridge.ErrNotImplemented
}
