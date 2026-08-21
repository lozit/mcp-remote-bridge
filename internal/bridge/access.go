package bridge

import "net/http"

// Cloudflare Access service-token headers.
//
// NOT YET MEASURED against a real service token — unlike everything else in
// this package, these come from documentation. The header names are stable and
// widely used, but this project has been wrong four times about what an
// external system does, so treat the first live run as the verification and
// correct this file rather than the test if they disagree.
const (
	AccessClientIDHeader     = "CF-Access-Client-Id"
	AccessClientSecretHeader = "CF-Access-Client-Secret"
)

// AccessCredentials authenticate a probe to Cloudflare Access.
//
// ClientID is an identifier (it looks like "<hex>.access") and is not secret;
// it sits in the config in clear, like the account and zone ids. ClientSecret
// is a real credential and travels as a SecretSource reference until the moment
// it is used.
type AccessCredentials struct {
	ClientID     string
	ClientSecret string
}

// Configured reports whether both halves are present.
//
// Both or neither: a half-configured token would be sent, rejected, and read as
// "the MCP is down" — a misleading red rather than an obvious misconfiguration.
func (c AccessCredentials) Configured() bool {
	return c.ClientID != "" && c.ClientSecret != ""
}

// Decorate returns the request decorator ProbeMCPResponds takes, or nil when no
// credentials are configured.
//
// nil is meaningful: it is what the access-policy check of ADR 0001 uses to ask
// "does this endpoint answer WITHOUT credentials?".
func (c AccessCredentials) Decorate() func(*http.Request) {
	if !c.Configured() {
		return nil
	}
	return func(req *http.Request) {
		req.Header.Set(AccessClientIDHeader, c.ClientID)
		req.Header.Set(AccessClientSecretHeader, c.ClientSecret)
	}
}
