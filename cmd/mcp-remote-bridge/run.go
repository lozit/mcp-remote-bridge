package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
	"github.com/lozit/mcp-remote-bridge/internal/cloudflared"
	"github.com/lozit/mcp-remote-bridge/internal/config"
	"github.com/lozit/mcp-remote-bridge/internal/doctor"

	"github.com/lozit/mcp-remote-bridge/internal/keychain"
	"github.com/lozit/mcp-remote-bridge/internal/launchd"
)

// assemble wires the primitive to its three seams for a loaded config.
//
// The CLI owns no logic beyond this: load, loop, report. Everything it resolves
// here — the binary's own path, mcp-proxy's path — is resolved ONCE, at apply
// time, and written into the service definition. Under launchd the PATH is
// minimal, so a lookup deferred to launch time would not find a proxy installed
// in ~/.local/bin.
func assemble(cfg *config.File, configPath string) (*bridge.Bridge, error) {
	binary, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locating this binary: %w", err)
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return nil, fmt.Errorf("resolving this binary's path: %w", err)
	}

	proxy, err := exec.LookPath("mcp-proxy")
	if err != nil {
		return nil, fmt.Errorf("mcp-proxy is a precondition and was not found on PATH: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locating the home directory: %w", err)
	}
	logDir := filepath.Join(home, "Library", "Logs", "mcp-remote-bridge")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating the log directory: %w", err)
	}

	secrets := keychain.New(cfg.Infra.Keychain)

	// The Exposer needs the API token as a VALUE, so it is resolved here and
	// handed straight to the seam — never stored on the Bridge, never logged.
	apiToken, err := secrets.Get(cfg.Infra.APIToken)
	if err != nil {
		return nil, fmt.Errorf("resolving the Cloudflare API token from %q: %w", cfg.Infra.APIToken, err)
	}
	exposer := cloudflared.New(cfg.Infra.AccountID, cfg.Infra.ZoneID, cfg.Infra.TunnelID, apiToken)
	exposer.AccessPolicyID = cfg.Infra.AccessPolicyID

	b := bridge.New(launchd.New(), exposer, secrets)
	b.BinaryPath = binary
	b.ConfigPath = configPath
	b.LogDir = logDir
	b.ProxyPath = proxy
	b.Warn = func(msg string) { fmt.Fprintf(os.Stderr, "warning: %s\n", msg) }
	return b, nil
}

// runApply reconciles the machine to the config.
//
// Usage: apply [name] [--config <path>]
//
// Reconciles every entry by default. An entry removed from the config is NOT
// torn down here: an edit must never be silently destructive, so `remove` stays
// explicit.
func runApply(args []string) (int, error) {
	name, configPath, err := parseEntryArgs("apply", args)
	if err != nil {
		return exitPrecondition, err
	}
	cfg, path, err := loadConfig(configPath)
	if err != nil {
		return exitPrecondition, err
	}
	b, err := assemble(cfg, path)
	if err != nil {
		return exitPrecondition, err
	}
	entries, err := selectEntries(cfg, name)
	if err != nil {
		return exitPrecondition, err
	}

	var reports []bridge.HealthReport
	for _, e := range entries {
		report, err := b.EnsureExposed(e)
		if err != nil {
			// Print what was established before the failure, then stop: the
			// remaining entries would run against a machine in an unknown state.
			printReport(os.Stdout, report)
			return exitPrecondition, fmt.Errorf("%s: %w", e.Name, err)
		}
		printReport(os.Stdout, report)
		reports = append(reports, report)
	}
	return exitCodeFor(reports), nil
}

// runStatus probes every entry and changes nothing.
func runStatus(args []string) (int, error) {
	name, configPath, err := parseEntryArgs("status", args)
	if err != nil {
		return exitPrecondition, err
	}
	cfg, path, err := loadConfig(configPath)
	if err != nil {
		return exitPrecondition, err
	}
	b, err := assemble(cfg, path)
	if err != nil {
		return exitPrecondition, err
	}
	entries, err := selectEntries(cfg, name)
	if err != nil {
		return exitPrecondition, err
	}

	reports := make([]bridge.HealthReport, 0, len(entries))
	for _, e := range entries {
		report := b.Probe(e)
		printReport(os.Stdout, report)
		reports = append(reports, report)
	}
	return exitCodeFor(reports), nil
}

// runRemove tears one entry down. Always explicit, never triggered by apply.
func runRemove(args []string) (int, error) {
	name, configPath, err := parseEntryArgs("remove", args)
	if err != nil {
		return exitPrecondition, err
	}
	if name == "" {
		return exitPrecondition, fmt.Errorf("remove needs an entry name: removing everything is never implicit")
	}
	cfg, path, err := loadConfig(configPath)
	if err != nil {
		return exitPrecondition, err
	}
	b, err := assemble(cfg, path)
	if err != nil {
		return exitPrecondition, err
	}
	entry, err := cfg.Entry(name)
	if err != nil {
		return exitPrecondition, err
	}

	report, err := b.RemoveExposed(entry)
	if err != nil {
		return exitPrecondition, err
	}
	// After a teardown the checks SHOULD be red; printing them raw would read as
	// a failure, so say what was verified instead.
	if report.Healthy() {
		return exitUnhealthy, fmt.Errorf("%s still answers after being removed", entry.Hostname())
	}
	fmt.Printf("ok   %s removed; it no longer answers\n", entry.Name)
	return exitOK, nil
}

func selectEntries(cfg *config.File, name string) ([]bridge.Entry, error) {
	if name == "" {
		return cfg.Entries(), nil
	}
	e, err := cfg.Entry(name)
	if err != nil {
		return nil, err
	}
	return []bridge.Entry{e}, nil
}

func loadConfig(path string) (*config.File, string, error) {
	if path == "" {
		var err error
		if path, err = config.DefaultPath(); err != nil {
			return nil, "", err
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(abs)
	if err != nil {
		return nil, "", err
	}
	return cfg, abs, nil
}

// parseEntryArgs reads "[name] [--config path]".
func parseEntryArgs(command string, args []string) (name, configPath string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--config needs a path")
			}
			configPath = args[i+1]
			i++
		default:
			if name != "" {
				return "", "", fmt.Errorf("usage: %s [name] [--config <path>]", command)
			}
			name = args[i]
		}
	}
	return name, configPath, nil
}

// runDoctor checks the preconditions and changes nothing.
//
// It loads the config best-effort: a config that will not load is itself the
// first thing to report, and the environment checks still run without it —
// exactly the case where someone most needs to be told what is missing.
func runDoctor(args []string) (int, error) {
	_, configPath, err := parseEntryArgs("doctor", args)
	if err != nil {
		return exitPrecondition, err
	}

	cfg, path, loadErr := loadConfig(configPath)
	if loadErr != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n\n", loadErr)
		cfg = nil
	}

	deps := doctor.Deps{}
	if cfg != nil {
		secrets := keychain.New(cfg.Infra.Keychain)
		deps.ResolveSecret = secrets.Get
	}

	checks := doctor.Run(cfg, deps)
	fmt.Print(doctor.Render(checks))

	if !doctor.Healthy(checks) {
		return exitPrecondition, nil
	}
	if cfg != nil {
		fmt.Printf("\n  preconditions met; config at %s\n", path)
	}
	return exitOK, nil
}

// AccessSecretRef is where setup stores the service token's secret.
const AccessSecretRef = "keychain:cf-access-secret"

// serviceTokenName is the name setup gives the token it creates.
const serviceTokenName = "mcp-remote-bridge"

// runSetup creates the Access service token, once.
//
// It is a separate command rather than part of apply because the token is
// SHARED across entries: creating it per-entry would produce one token per MCP,
// each one a credential to track and revoke.
//
// It is also not part of doctor, which must stay a read-only diagnostic.
func runSetup(args []string) (int, error) {
	_, configPath, err := parseEntryArgs("setup", args)
	if err != nil {
		return exitPrecondition, err
	}
	cfg, _, err := loadConfig(configPath)
	if err != nil {
		return exitPrecondition, err
	}

	secrets := keychain.New(cfg.Infra.Keychain)
	apiToken, err := secrets.Get(cfg.Infra.APIToken)
	if err != nil {
		return exitPrecondition, fmt.Errorf("resolving the Cloudflare API token: %w", err)
	}
	exposer := cloudflared.New(cfg.Infra.AccountID, cfg.Infra.ZoneID, cfg.Infra.TunnelID, apiToken)

	// Already set up? Say so and change nothing. Re-running must be safe: this
	// command creates a credential, and the failure mode of a careless retry is
	// a second indistinguishable token.
	existing, err := exposer.FindServiceToken(serviceTokenName)
	if err != nil {
		return exitPrecondition, err
	}
	if existing != nil {
		if stored, err := secrets.Get(AccessSecretRef); err == nil && stored != "" {
			fmt.Printf("ok   service token %q already exists and its secret is stored\n", serviceTokenName)
			fmt.Printf("     access_client_id     = %q\n", existing.ClientID)
			fmt.Printf("     access_client_secret = %q\n", AccessSecretRef)
			return exitOK, nil
		}
		// The token exists but we cannot read its secret — Cloudflare shows it
		// once. Guessing would strand the user; say exactly what to do.
		return exitPrecondition, fmt.Errorf(
			"a service token named %q exists (client id %s) but its secret is not in the keychain.\n"+
				"Cloudflare shows a secret only at creation, so it cannot be recovered.\n"+
				"Either store it with `set-secret %s` if you kept it, or delete the token in the dashboard and re-run setup",
			serviceTokenName, existing.ClientID, AccessSecretRef)
	}

	token, err := exposer.CreateServiceToken(serviceTokenName)
	if err != nil {
		return exitPrecondition, err
	}
	// Straight from the response into the keychain: the value never reaches the
	// terminal, an argv, or a clipboard.
	if err := secrets.Set(AccessSecretRef, token.Secret); err != nil {
		return exitPrecondition, fmt.Errorf(
			"the service token was created (client id %s) but its secret could not be stored: %w.\n"+
				"The secret is now unrecoverable — delete the token in the dashboard and re-run setup",
			token.ClientID, err)
	}

	fmt.Printf("ok   created service token %q; its secret is in the keychain\n\n", serviceTokenName)
	fmt.Printf("     Add to [infra] in your config:\n")
	fmt.Printf("       access_client_id     = %q\n", token.ClientID)
	fmt.Printf("       access_client_secret = %q\n", AccessSecretRef)
	return exitOK, nil
}

// defaultLogLines is how much of a log `logs` shows without being asked.
const defaultLogLines = 50

// runLogs prints the tail of one entry's proxy log.
//
// It always prints the path, even when the file is missing or empty: "there is
// nothing to show" and "you are looking in the wrong place" are different
// problems, and the path is what tells them apart.
func runLogs(args []string) (int, error) {
	name, configPath, err := parseEntryArgs("logs", args)
	if err != nil {
		return exitPrecondition, err
	}
	if name == "" {
		return exitPrecondition, fmt.Errorf("logs needs an entry name")
	}
	cfg, path, err := loadConfig(configPath)
	if err != nil {
		return exitPrecondition, err
	}
	b, err := assemble(cfg, path)
	if err != nil {
		return exitPrecondition, err
	}
	if _, err := cfg.Entry(name); err != nil {
		return exitPrecondition, err
	}

	logPath := b.LogPath(name)
	fmt.Fprintf(os.Stderr, "%s\n\n", logPath)

	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Not an error: a service that never started writes nothing, and
			// saying so beats a bare "no such file".
			fmt.Fprintln(os.Stderr, "no log yet — the service may never have started; try `status`")
			return exitOK, nil
		}
		return exitPrecondition, fmt.Errorf("reading %s: %w", logPath, err)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		fmt.Fprintln(os.Stderr, "the log is empty")
		return exitOK, nil
	}
	if len(lines) > defaultLogLines {
		lines = lines[len(lines)-defaultLogLines:]
	}
	fmt.Println(strings.Join(lines, "\n"))
	return exitOK, nil
}

// runRestart bounces one entry's service and re-probes it.
//
// It touches the service only: the hostname, ingress rule and DNS record are
// left alone, so restarting never risks the published name.
func runRestart(args []string) (int, error) {
	name, configPath, err := parseEntryArgs("restart", args)
	if err != nil {
		return exitPrecondition, err
	}
	if name == "" {
		return exitPrecondition, fmt.Errorf("restart needs an entry name: restarting everything is never implicit")
	}
	cfg, path, err := loadConfig(configPath)
	if err != nil {
		return exitPrecondition, err
	}
	b, err := assemble(cfg, path)
	if err != nil {
		return exitPrecondition, err
	}
	entry, err := cfg.Entry(name)
	if err != nil {
		return exitPrecondition, err
	}

	report, err := b.RestartService(entry)
	if err != nil {
		return exitPrecondition, err
	}
	printReport(os.Stdout, report)
	return exitCodeFor([]bridge.HealthReport{report}), nil
}
