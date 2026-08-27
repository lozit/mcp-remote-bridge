// Package doctor checks the preconditions the primitive assumes but never
// creates.
//
// It answers a different question from `status`: not "is this entry healthy"
// but "could it be". A precondition failure is why an entry is red, so doctor
// exists to keep people from debugging an MCP when the real problem is a
// missing binary or an unresolvable token.
//
// It changes nothing, and it never tests a credential with a write.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/lozit/mcp-remote-bridge/internal/config"
)

// Check is one precondition, with the evidence behind it.
//
// Hint is what makes doctor worth running: a red check that does not say what
// to do next is a status line, not a diagnosis.
type Check struct {
	Name   string
	OK     bool
	Detail string
	Err    error
	Hint   string
}

// Deps are the environment lookups, injected so the checks are testable
// without the tools being installed.
type Deps struct {
	// LookPath resolves a binary on PATH. Nil means exec.LookPath.
	LookPath func(string) (string, error)
	// ProcessMatches reports whether a process matching substr is running.
	// Nil means a real `pgrep`.
	ProcessMatches func(substr string) bool
	// ResolveSecret resolves a SecretSource reference. Nil means no source, and
	// every secret check reports that it could not be verified.
	ResolveSecret func(ref string) (string, error)
	// Executable is this binary's own path. Nil means os.Executable.
	Executable func() (string, error)
}

func (d Deps) lookPath(name string) (string, error) {
	if d.LookPath != nil {
		return d.LookPath(name)
	}
	return exec.LookPath(name)
}

func (d Deps) processMatches(substr string) bool {
	if d.ProcessMatches != nil {
		return d.ProcessMatches(substr)
	}
	return exec.Command("pgrep", "-f", substr).Run() == nil
}

func (d Deps) executable() (string, error) {
	if d.Executable != nil {
		return d.Executable()
	}
	return os.Executable()
}

// Run performs every precondition check and returns them in a stable order.
//
// cfg may be nil: a config that would not load is itself the first thing to
// report, and the remaining checks that depend on it are then skipped rather
// than reported as failing — a check that could not run must not read as a
// check that failed.
func Run(cfg *config.File, deps Deps) []Check {
	checks := []Check{
		binaryPresent(deps, "mcp-proxy",
			"mcp-proxy wraps a stdio MCP into HTTP; the tool assumes it, and does not install it"),
		binaryPresent(deps, "cloudflared",
			"cloudflared is the tunnel connector; install it and connect the tunnel from the dashboard"),
		connectorRunning(deps),
		selfPath(deps),
	}
	if cfg == nil {
		return append(checks, Check{
			Name: "config", Err: fmt.Errorf("the config could not be loaded"),
			Hint: "fix the config first: every check below it depends on what it declares",
		})
	}
	return append(checks,
		Check{Name: "config", OK: true, Detail: fmt.Sprintf("%d entr%s declared", len(cfg.MCP), plural(len(cfg.MCP)))},
		secretResolves(deps, "cloudflare_api_token", cfg.Infra.APIToken,
			"store it with `mcp-remote-bridge set-secret`, scoped to Zone:DNS:Edit, Account:Cloudflare Tunnel:Edit and Access edit rights"),
		accessCredentials(deps, cfg.Infra),
		portalReminder(),
	)
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func binaryPresent(deps Deps, name, hint string) Check {
	c := Check{Name: name, Hint: hint}
	path, err := deps.lookPath(name)
	if err != nil {
		c.Err = fmt.Errorf("%s is not on PATH", name)
		return c
	}
	c.OK, c.Detail = true, path
	return c
}

// connectorRunning looks for a running cloudflared, which is what actually
// carries traffic — an installed binary that is not running carries nothing.
func connectorRunning(deps Deps) Check {
	c := Check{
		Name: "tunnel_connector",
		// The hint prints the command rather than describing it. This is the one
		// precondition the tool deliberately does not fix — installing the
		// connector means a root LaunchDaemon, and this tool writes only
		// per-user LaunchAgents — so the least it can do is not make the reader
		// go and look it up.
		//
		// A tunnel accepts SEVERAL connectors at once; that is how Cloudflare
		// does high availability. So moving a tunnel between machines needs no
		// downtime window: install on the new host, confirm the dashboard lists
		// both, then remove the old one.
		Hint: "install the connector as a service from the tunnel's token:\n" +
			"  cloudflared service install <TUNNEL_TOKEN>\n" +
			"the token comes from the tunnel's page in the dashboard, not from api_token.\n" +
			"Then check the dashboard shows the connector Running. A tunnel accepts several\n" +
			"connectors at once, so you can install on a new machine before removing the old.",
	}
	if !deps.processMatches("cloudflared tunnel run") {
		c.Err = fmt.Errorf("no running cloudflared connector found")
		return c
	}
	c.OK, c.Detail = true, "cloudflared tunnel run"
	return c
}

// selfPath checks this binary's own path, because it is written into every
// service definition and the services outlive the shell that created them.
func selfPath(deps Deps) Check {
	c := Check{
		Name: "binary_path",
		Hint: "services record this path; moving or reinstalling the binary elsewhere breaks them until the next apply",
	}
	path, err := deps.executable()
	if err != nil {
		c.Err = fmt.Errorf("could not locate this binary: %w", err)
		return c
	}
	if _, err := os.Stat(path); err != nil {
		c.Err = fmt.Errorf("this binary's recorded path does not exist: %s", path)
		return c
	}
	c.OK, c.Detail = true, path
	return c
}

// secretResolves checks a reference resolves, and never reports the value.
//
// It also never USES the credential: doctor reports presence, not validity, so
// that running it cannot itself change anything or trip a rate limit.
func secretResolves(deps Deps, name, ref, hint string) Check {
	c := Check{Name: name, Detail: ref, Hint: hint}
	if ref == "" {
		c.Err = fmt.Errorf("no reference configured")
		return c
	}
	if deps.ResolveSecret == nil {
		c.Err = fmt.Errorf("no secret source available to check %q", ref)
		return c
	}
	value, err := deps.ResolveSecret(ref)
	if err != nil {
		c.Err = fmt.Errorf("cannot resolve %q: %w", ref, err)
		return c
	}
	if value == "" {
		// The measured failure mode of `security`: it can report success while
		// having stored nothing.
		c.Err = fmt.Errorf("%q resolves to an empty value", ref)
		return c
	}
	c.OK = true
	return c
}

// accessCredentials reports the service token used to probe guarded hostnames.
//
// Absent is not a failure: an unguarded setup needs none. It is reported as a
// note so that a permanently red hostname check has an obvious first suspect.
func accessCredentials(deps Deps, infra config.Infra) Check {
	if infra.AccessClientID == "" && infra.AccessClientSecret == "" {
		return Check{
			Name: "access_service_token", OK: true, Detail: "not configured",
			Hint: "without one, a hostname behind an Access policy reports red forever — set access_client_id and access_client_secret if any hostname is guarded",
		}
	}
	return secretResolves(deps, "access_service_token", infra.AccessClientSecret,
		"store it with `mcp-remote-bridge set-secret`; the value is only shown once, at the token's creation")
}

// portalReminder reports the one step the tool cannot take.
//
// It is always OK because it is not a failure — it is an outstanding manual
// step, and saying so beats letting someone believe the tool did it.
func portalReminder() Check {
	return Check{
		Name: "portal_server", OK: true, Detail: "manual step",
		Hint: "if an MCP Portal fronts these hostnames, declare each server's header authentication AT CREATION — the field cannot be added later, and /access/mcp_servers is closed to API tokens while Portals are Beta",
	}
}

// Healthy reports whether every check passed.
func Healthy(checks []Check) bool {
	for _, c := range checks {
		if !c.OK {
			return false
		}
	}
	return len(checks) > 0
}

// Render writes the checks, with a hint under each failure.
func Render(checks []Check) string {
	var b strings.Builder
	for _, c := range checks {
		symbol := "x"
		if c.OK {
			symbol = "v"
		}
		fmt.Fprintf(&b, "  %s %-22s %s\n", symbol, c.Name, c.Detail)
		if c.Err != nil {
			fmt.Fprintf(&b, "      %s\n", c.Err)
		}
		if !c.OK && c.Hint != "" {
			// A hint may span several lines when the next step is a command
			// worth printing verbatim. Indentation belongs here rather than
			// inside the hint text: a hint that carries its own leading spaces
			// renders ragged the moment this prefix changes.
			for i, line := range strings.Split(c.Hint, "\n") {
				marker := "→"
				if i > 0 {
					marker = " "
				}
				fmt.Fprintf(&b, "      %s %s\n", marker, line)
			}
		}
	}
	return b.String()
}
