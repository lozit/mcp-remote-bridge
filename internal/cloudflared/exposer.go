// Package cloudflared implements bridge.Exposer against a REMOTELY-MANAGED
// Cloudflare tunnel, through the Cloudflare API.
//
// The tunnel itself is a precondition: this package adds hostnames to a tunnel,
// it never creates one.
//
// It does NOT shell out to `cloudflared tunnel route dns`. That command needs
// ~/.cloudflared/cert.pem, which a token-installed connector does not have; and
// more fundamentally a remotely-managed tunnel keeps its ingress configuration
// in Cloudflare rather than on disk, so there is no local rule to write. See
// ADR 0006.
//
// Ensure is therefore two API calls: PUT the tunnel's ingress configuration,
// and create a proxied CNAME to {tunnel_id}.cfargotunnel.com.
//
// The ingress PUT replaces the WHOLE list, so it is a read-modify-write: re-read
// immediately before writing and preserve entries this tool did not create, or a
// concurrent run — or a dashboard edit — silently drops them.
package cloudflared

import (
	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// Exposer routes a public hostname to a local port through a Cloudflare tunnel.
type Exposer struct {
	// AccountID, ZoneID and TunnelID address the tunnel through the API.
	AccountID string
	ZoneID    string
	TunnelID  string

	// APIToken is the resolved token value. It can modify the zone's DNS, so it
	// is never logged and never placed in a URL or a command line.
	APIToken string
}

// New returns an Exposer bound to a remotely-managed tunnel.
func New(accountID, zoneID, tunnelID, apiToken string) *Exposer {
	return &Exposer{AccountID: accountID, ZoneID: zoneID, TunnelID: tunnelID, APIToken: apiToken}
}

// Ensure adds the ingress rule subdomain.domain -> localhost:localPort and its
// DNS route.
func (e *Exposer) Ensure(subdomain, domain string, localPort int) error {
	return bridge.ErrNotImplemented
}

// Remove drops the ingress rule and its DNS route.
func (e *Exposer) Remove(subdomain, domain string) error {
	return bridge.ErrNotImplemented
}
