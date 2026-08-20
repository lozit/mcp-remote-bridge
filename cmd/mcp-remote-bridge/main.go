// Command mcp-remote-bridge makes local stdio MCP servers reachable from a
// remote agent.
//
// The CLI owns no logic beyond load -> loop -> report; every real action is a
// primitive call. See docs/SPEC-config-cli.md.
package main

import (
	"fmt"
	"os"

	"github.com/lozit/mcp-remote-bridge/internal/config"
	"github.com/lozit/mcp-remote-bridge/internal/keychain"
	"github.com/lozit/mcp-remote-bridge/internal/launcher"
)

// Exit codes, so the tool composes in scripts and CI. A green exit means the
// probes passed, not that a file was written.
const (
	exitOK           = 0 // all healthy
	exitPrecondition = 1 // a precondition failed (doctor territory)
	exitUnhealthy    = 2 // at least one entry unhealthy after the command
)

// launchCommand is the hidden subcommand launchd execs. It is not for humans,
// but it is still a public entry point of a public binary, so it must be safe
// to run by hand and must never print a secret on any path.
const launchCommand = "__launch"

func main() {
	if len(os.Args) > 1 && os.Args[1] == launchCommand {
		if err := runLaunch(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "mcp-remote-bridge %s: %v\n", launchCommand, err)
			os.Exit(exitPrecondition)
		}
		return
	}

	// Cobra command structure arrives with the rest of Milestone 2.
	fmt.Fprintln(os.Stderr, "mcp-remote-bridge: not implemented yet — see PLAN.md")
	os.Exit(exitPrecondition)
}

// runLaunch resolves one entry's secrets and execs the proxy. On success it
// does not return: this process becomes mcp-proxy.
//
// Usage: __launch <name> --config <path> --port <n>
func runLaunch(args []string) error {
	var name, configPath string
	port := 0

	if len(args) == 0 {
		return fmt.Errorf("usage: %s <name> --config <path> --port <n>", launchCommand)
	}
	name, args = args[0], args[1:]

	for len(args) > 0 {
		switch args[0] {
		case "--config":
			if len(args) < 2 {
				return fmt.Errorf("--config needs a path")
			}
			configPath, args = args[1], args[2:]
		case "--port":
			if len(args) < 2 {
				return fmt.Errorf("--port needs a number")
			}
			if _, err := fmt.Sscanf(args[1], "%d", &port); err != nil {
				return fmt.Errorf("--port %q is not a number", args[1])
			}
			args = args[2:]
		default:
			return fmt.Errorf("unexpected argument %q", args[0])
		}
	}

	if configPath == "" {
		var err error
		if configPath, err = config.DefaultPath(); err != nil {
			return err
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	entry, err := cfg.Entry(name)
	if err != nil {
		return err
	}
	if port == 0 {
		port = entry.Port
	}

	// Build resolves the secrets. It fails here, before anything is launched,
	// if one is missing — rather than starting a proxy that 401s silently.
	plan, err := launcher.Build(entry, keychain.New(cfg.Infra.Keychain), port, nil)
	if err != nil {
		return err
	}
	return plan.Exec()
}
