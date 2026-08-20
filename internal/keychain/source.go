// Package keychain implements bridge.SecretSource against the macOS keychain.
//
// It is the only place in the codebase allowed to know about the security
// binary. Values it returns are never logged and never written to disk.
package keychain

import (
	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// Source resolves secrets from the macOS keychain.
type Source struct{}

// New returns a keychain-backed SecretSource.
func New() *Source { return &Source{} }

// Get resolves key to a secret value.
//
// An absent key must return an error rather than an empty string: silently
// launching an MCP with a blank credential is the failure mode this whole path
// exists to prevent.
func (s *Source) Get(key string) (string, error) {
	return "", bridge.ErrNotImplemented
}
