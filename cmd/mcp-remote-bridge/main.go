// Command mcp-remote-bridge makes local stdio MCP servers reachable from a
// remote agent.
//
// The CLI owns no logic beyond load -> loop -> report; every real action is a
// primitive call. See docs/SPEC-config-cli.md.
package main

import (
	"fmt"
	"os"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
	"github.com/lozit/mcp-remote-bridge/internal/cloudflared"
	"github.com/lozit/mcp-remote-bridge/internal/keychain"
	"github.com/lozit/mcp-remote-bridge/internal/launchd"
)

// Exit codes, so the tool composes in scripts and CI. A green exit means the
// probes passed, not that a file was written.
const (
	exitOK           = 0 // all healthy
	exitPrecondition = 1 // a precondition failed (doctor territory)
	exitUnhealthy    = 2 // at least one entry unhealthy after the command
)

func main() {
	// Cobra command structure arrives with Milestone 2, together with the TOML
	// config. For now this only proves the seams wire together and the module
	// builds.
	_ = bridge.New(launchd.New(), cloudflared.New(""), keychain.New())

	fmt.Fprintln(os.Stderr, "mcp-remote-bridge: not implemented yet — see PLAN.md")
	os.Exit(exitPrecondition)
}
