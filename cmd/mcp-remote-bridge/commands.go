package main

import "github.com/spf13/cobra"

// newRootCommand builds the command tree.
//
// Every command delegates to the run* functions, which own the actual work.
// Cobra is the outer layer only: it parses and dispatches, and the exit codes
// stay the ones SPEC-config-cli.md defines.
func newRootCommand(exitCode *int) *cobra.Command {
	var configPath string

	root := &cobra.Command{
		Use:   "mcp-remote-bridge",
		Short: "Make local stdio MCP servers reachable from a remote agent",
		Long: "mcp-remote-bridge wraps a local stdio MCP server into a supervised HTTP service\n" +
			"and publishes it through a tunnel — the setup people otherwise do by hand.\n\n" +
			"Exit codes: 0 all healthy · 1 a precondition failed · 2 at least one entry unhealthy.\n" +
			"A green exit means the probes passed, not that a file was written.",
		SilenceUsage:  true, // a runtime failure is not a usage error
		SilenceErrors: true, // main prints them, so the format stays ours
	}
	root.PersistentFlags().StringVar(&configPath, "config", "",
		"config file (default $XDG_CONFIG_HOME/mcp-remote-bridge/config.toml)")

	// entryArgs rebuilds the argument list the run* functions already parse, so
	// adopting Cobra changed no behaviour it could get wrong.
	entryArgs := func(args []string) []string {
		out := append([]string{}, args...)
		if configPath != "" {
			out = append(out, "--config", configPath)
		}
		return out
	}

	run := func(fn func([]string) (int, error)) func(*cobra.Command, []string) error {
		return func(_ *cobra.Command, args []string) error {
			code, err := fn(entryArgs(args))
			*exitCode = code
			return err
		}
	}

	root.AddCommand(
		&cobra.Command{
			Use:   "apply [name]",
			Short: "Reconcile the machine to the config",
			Long: "Reconciles every entry, or one when named. Idempotent: a healthy entry is left\n" +
				"untouched, and only what drifted is repaired.\n\n" +
				"An entry deleted from the config is NOT torn down — an edit must never be\n" +
				"silently destructive. Use `remove` for that.",
			Args: cobra.MaximumNArgs(1),
			RunE: run(runApply),
		},
		&cobra.Command{
			Use:   "status [name]",
			Short: "Probe every entry and change nothing",
			Long: "Probes each entry and prints what it found. Changes nothing, and never waits:\n" +
				"it is a fast read, unlike `apply`, which has just changed the world and owes it\n" +
				"time to settle.",
			Args: cobra.MaximumNArgs(1),
			RunE: run(runStatus),
		},
		&cobra.Command{
			Use:   "remove <name>",
			Short: "Tear one entry down",
			Long: "Removes the service, the ingress rule, the DNS record and the Access\n" +
				"application for one entry, then verifies the hostname stops answering.\n\n" +
				"Always explicit: reconciling a config never triggers it.",
			Args: cobra.ExactArgs(1),
			RunE: run(runRemove),
		},
		&cobra.Command{
			Use:   "doctor",
			Short: "Check the preconditions; changes nothing",
			Long: "Checks what the tool assumes but never creates: mcp-proxy, cloudflared, a\n" +
				"running connector, this binary's recorded path, and the credentials.\n\n" +
				"It reports a credential's PRESENCE, never its validity — running doctor cannot\n" +
				"itself change anything or trip a rate limit.",
			Args: cobra.NoArgs,
			RunE: run(runDoctor),
		},
		&cobra.Command{
			Use:   "setup",
			Short: "Create the Access service token, once",
			Long: "Creates the Cloudflare Access service token and stores its secret in the\n" +
				"keychain. Cloudflare returns a secret only at creation, so this goes straight\n" +
				"from the API response to the keychain — never through a terminal or clipboard.\n\n" +
				"Shared across entries, which is why it is not part of apply.",
			Args: cobra.NoArgs,
			RunE: run(runSetup),
		},
		&cobra.Command{
			Use:   "logs <name>",
			Short: "Show the tail of an entry's proxy log",
			Args:  cobra.ExactArgs(1),
			RunE:  run(runLogs),
		},
		&cobra.Command{
			Use:   "restart <name>",
			Short: "Bounce an entry's service",
			Long: "Restarts the service and re-probes it. Touches the service only: the hostname,\n" +
				"ingress rule and DNS record are left alone, so a restart never risks the\n" +
				"published name.",
			Args: cobra.ExactArgs(1),
			RunE: run(runRestart),
		},
		&cobra.Command{
			Use:    launchCommand + " <name>",
			Short:  "Internal: resolve secrets and exec the proxy",
			Hidden: true, // launchd's entry point, not a human's
			RunE: func(_ *cobra.Command, args []string) error {
				return runLaunch(args)
			},
			DisableFlagParsing: true, // its flags are its own, not Cobra's
		},
		&cobra.Command{
			Use:   "set-secret <reference>",
			Short: "Store a secret from a masked prompt",
			Long: "Reads a value without echoing it and stores it in the keychain. The value\n" +
				"never reaches an argv, an environment variable, or the terminal.",
			Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return runSetSecret(args)
			},
		},
	)
	return root
}
