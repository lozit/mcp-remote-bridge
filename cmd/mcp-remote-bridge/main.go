// Command mcp-remote-bridge makes local stdio MCP servers reachable from a
// remote agent.
//
// The CLI owns no logic beyond load -> loop -> report; every real action is a
// primitive call. See docs/SPEC-config-cli.md.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

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

	if len(os.Args) < 2 {
		usage()
		os.Exit(exitPrecondition)
	}

	command, args := os.Args[1], os.Args[2:]

	if command == "set-secret" {
		if err := runSetSecret(args); err != nil {
			fmt.Fprintf(os.Stderr, "mcp-remote-bridge set-secret: %v\n", err)
			os.Exit(exitPrecondition)
		}
		return
	}

	var (
		code int
		err  error
	)
	switch command {
	case "apply":
		code, err = runApply(args)
	case "status":
		code, err = runStatus(args)
	case "remove":
		code, err = runRemove(args)
	case "doctor":
		code, err = runDoctor(args)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", command)
		usage()
		os.Exit(exitPrecondition)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "mcp-remote-bridge %s: %v\n", command, err)
	}
	os.Exit(code)
}

// runLaunch resolves one entry's secrets and execs the proxy. On success it
// does not return: this process becomes mcp-proxy.
//
// Usage: __launch <name> --config <path> --port <n> [--proxy <path>]
func runLaunch(args []string) error {
	var name, configPath, proxyPath string
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
		case "--proxy":
			if len(args) < 2 {
				return fmt.Errorf("--proxy needs a path")
			}
			proxyPath, args = args[1], args[2:]
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
	plan, err := launcher.Build(entry, keychain.New(cfg.Infra.Keychain), port, proxyPath, nil)
	if err != nil {
		return err
	}
	return plan.Exec()
}

// runSetSecret stores a secret under a reference, reading it from a masked
// prompt.
//
// Usage: set-secret keychain:<service>
//
// The value is never an argument, never in the environment, and never echoed:
// those are the three places a shell would keep a copy of it. It reaches the
// keychain through the child process's stdin — see keychain.Source.Set.
func runSetSecret(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: set-secret keychain:<service>")
	}
	key := args[0]

	// Check the reference first: prompting for a secret and only then rejecting
	// the key wastes the one input that is annoying to produce twice.
	if err := keychain.ValidateKey(key); err != nil {
		return err
	}

	value, err := readMasked(fmt.Sprintf("Value for %s: ", key))
	if err != nil {
		return err
	}
	if value == "" {
		// An empty secret is almost always a mis-paste, and it would fail much
		// later as an authentication error with no obvious cause.
		return fmt.Errorf("refusing to store an empty value for %s", key)
	}

	if err := keychain.New("").Set(key, value); err != nil {
		return err
	}
	fmt.Printf("Stored %s\n", key)
	return nil
}

// readMasked reads one line without echoing it.
//
// It shells out to stty rather than taking a terminal dependency: the project
// has one dependency, argued for in ADR 0005, and disabling echo does not
// warrant a second.
//
// If stdin is not a terminal the value is read plainly from it, which supports
// piping in scripts — but it says so, because the caller then owns the risk of
// the value sitting in a shell history or a file.
func readMasked(prompt string) (string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("inspecting stdin: %w", err)
	}
	isTTY := stat.Mode()&os.ModeCharDevice != 0

	if !isTTY {
		fmt.Fprintln(os.Stderr, "warning: stdin is not a terminal, reading the value unmasked")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("reading the value: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	fmt.Fprint(os.Stderr, prompt)
	if err := stty("-echo"); err != nil {
		// Refuse rather than fall back to an unmasked read: someone typing a
		// token expects it not to appear, and surprising them with an echoed
		// secret is worse than failing.
		fmt.Fprintln(os.Stderr)
		return "", fmt.Errorf("cannot disable terminal echo, so the value would be shown as you type it: %w", err)
	}
	// Restore the terminal whatever happens next, including a read error: a
	// shell left with echo disabled is a broken shell.
	defer func() {
		_ = stty("echo")
		fmt.Fprintln(os.Stderr)
	}()

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("reading the value: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func stty(mode string) error {
	cmd := exec.Command("stty", mode)
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setting terminal echo %s: %w", mode, err)
	}
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, `mcp-remote-bridge — make local stdio MCP servers reachable from a remote agent

  apply [name]        reconcile the machine to the config (idempotent)
  status [name]       probe every entry and change nothing
  remove <name>       tear one entry down; never implicit
  doctor              check the preconditions; changes nothing
  set-secret <ref>    store a secret from a masked prompt

Options:
  --config <path>     defaults to $XDG_CONFIG_HOME/mcp-remote-bridge/config.toml

Exit codes: 0 all healthy · 1 a precondition failed · 2 at least one entry unhealthy.
A green exit means the probes passed, not that a file was written.
`)
}
