// Package bridge holds the expose primitive: the atomic operation of making one
// local stdio MCP server reachable from outside.
//
// The primitive talks only to the three interfaces in seams.go. It never shells
// out to launchctl, cloudflared or security itself — that lives inside an
// implementation. See docs/SPEC-primitive.md.
package bridge

// Entry is the declarative unit: one stdio MCP to expose.
//
// It is the in-memory form of a [mcp.<name>] table in the config file, and it
// carries secret *references* only — see the Secrets field.
type Entry struct {
	// Name is the unique id. It becomes a service label, a hostname component
	// and a log path, so it must be validated against a strict charset before
	// use: a Name containing "/" or ".." is rejected, never sanitized.
	Name string

	// Command and Args say how to launch the stdio MCP.
	Command string
	Args    []string

	// Env holds plain, non-secret environment variables the MCP needs.
	//
	// It exists because the launcher builds a minimal, explicit environment
	// rather than inheriting the ambient one (ADR 0002): a variable the MCP
	// depends on must be written here. A missing one fails loudly and is fixed
	// by one config line, which is the intended trade against an MCP silently
	// receiving whatever launchd happened to hold.
	//
	// Never put a secret here — use Secrets.
	Env map[string]string

	// Secrets maps an environment variable name to a SecretSource key
	// (e.g. "SN_EMAIL" -> "keychain:mcp-sn-email").
	//
	// These are NAMES, never values. Load-bearing rule 3: a secret value must
	// never reach this struct, the config file, the service file, or a command
	// line. It is fetched at launch time by the generated launcher.
	Secrets map[string]string

	// Port is the local port the proxy binds on 127.0.0.1. Zero means
	// auto-assign; a non-zero value is honoured as-is.
	//
	// The proxy binds 127.0.0.1 only, never 0.0.0.0. That is a security
	// control, not a default.
	Port int

	// Subdomain, with Domain, forms the public hostname.
	Subdomain string

	// Domain and the tunnel identifiers reference shared infrastructure,
	// supplied by the config's [infra] table.
	//
	// The tunnel is a precondition: it is assumed to exist as a REMOTELY-MANAGED
	// tunnel (installed from a token), whose ingress configuration lives in
	// Cloudflare rather than on disk. See ADR 0006.
	Domain string

	// AccountID, ZoneID and TunnelID address the tunnel through the Cloudflare
	// API. TunnelID is a UUID, not a name: the API addresses tunnels by id, and
	// the DNS target is {TunnelID}.cfargotunnel.com.
	AccountID string
	ZoneID    string
	TunnelID  string

	// APIToken is a SecretSource REFERENCE to a Cloudflare API token — never a
	// value. It is the most powerful credential in the system: it can modify the
	// zone's DNS. See docs/SECURITY.md.
	APIToken string
}

// Hostname is the public name this entry is reachable at.
func (e Entry) Hostname() string {
	return e.Subdomain + "." + e.Domain
}
