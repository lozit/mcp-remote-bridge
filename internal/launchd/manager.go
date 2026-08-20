// Package launchd implements bridge.ServiceManager on macOS, through
// ~/Library/LaunchAgents plists and launchctl bootstrap/bootout.
//
// It is the only place in the codebase allowed to know about launchctl.
package launchd

import (
	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// Manager keeps processes alive using launchd.
type Manager struct {
	// AgentsDir is where plists are written. Empty means ~/Library/LaunchAgents.
	AgentsDir string
}

// New returns a Manager writing to the default LaunchAgents directory.
func New() *Manager { return &Manager{} }

// Ensure writes the plist and bootstraps it.
//
// The plist must never contain a secret value: it is world-readable. Only
// spec.SecretRefs travel into the generated launcher.
func (m *Manager) Ensure(label string, spec bridge.ServiceSpec) error {
	return bridge.ErrNotImplemented
}

// Remove boots the service out and deletes its plist.
func (m *Manager) Remove(label string) error {
	return bridge.ErrNotImplemented
}

// Status reports what launchd currently knows about the label.
func (m *Manager) Status(label string) (bridge.ServiceState, error) {
	return bridge.ServiceState{}, bridge.ErrNotImplemented
}
