// Package launcher builds and performs the final step before an MCP runs:
// resolve its secrets, construct its environment, and exec mcp-proxy.
//
// This is where rule 3 stops being a principle and becomes code. Everything
// here is arranged so a secret value exists only in this process's memory, for
// as long as it takes to hand it to exec:
//
//   - it is fetched here, not at config-parse time and not at service-write time
//   - it goes into the environment, never into argv
//   - it is never logged, including on the error path
//
// See docs/SPEC-launcher.md and ADR 0002.
package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"syscall"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// ProxyBinary is the program that wraps a stdio MCP into HTTP.
const ProxyBinary = "mcp-proxy"

// LoopbackHost is the address the proxy binds.
//
// Passed explicitly even though it is mcp-proxy's default: loopback binding is
// a security control here, and a control that relies on someone else's default
// is one upstream release away from being gone.
const LoopbackHost = "127.0.0.1"

// inheritedVars are the only variables carried over from the ambient
// environment. Everything else the MCP receives is declared in the config.
//
// The launcher does not pass os.Environ() through: mcp-proxy is invoked with
// --pass-environment, which forwards its whole environment to the MCP, so an
// inherited environment would be forwarded too. See ADR 0002.
var inheritedVars = []string{"PATH", "HOME"}

// Plan is everything needed to exec the proxy: what to run, with which
// arguments, in which environment.
//
// Args is safe to log. Env is NOT — it holds the resolved secrets.
type Plan struct {
	Program string
	Args    []string
	Env     []string
}

// Build resolves entry's secrets and assembles the exec plan.
//
// port is the resolved local port (Build does not auto-assign; that is the
// primitive's job, decided before the service is written).
//
// A referenced secret that cannot be resolved fails here, before anything is
// launched: a proxy that starts without its credential would 401 silently,
// which is the failure this whole path exists to prevent.
// proxyPath, when non-empty, is the absolute path of mcp-proxy resolved at
// apply time. It is preferred over a PATH lookup because under launchd the PATH
// is minimal and would not find a proxy installed in ~/.local/bin.
func Build(entry bridge.Entry, src bridge.SecretSource, port int, proxyPath string, lookPath func(string) (string, error)) (Plan, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if port <= 0 || port > 65535 {
		return Plan{}, fmt.Errorf("entry %q has no usable port (%d)", entry.Name, port)
	}
	proxy := proxyPath
	if proxy == "" {
		var err error
		if proxy, err = lookPath(ProxyBinary); err != nil {
			return Plan{}, fmt.Errorf("%s is a precondition and was not found: %w", ProxyBinary, err)
		}
	}
	if entry.Command == "" {
		return Plan{}, fmt.Errorf("entry %q has no command", entry.Name)
	}

	env, err := buildEnv(entry, src)
	if err != nil {
		return Plan{}, err
	}

	// -- separates the proxy's flags from the MCP's command, so an MCP argument
	// can never be eaten as a proxy flag.
	args := []string{
		ProxyBinary,
		"--host", LoopbackHost,
		"--port", strconv.Itoa(port),
		"--pass-environment",
		"--", entry.Command,
	}
	args = append(args, entry.Args...)

	return Plan{Program: proxy, Args: args, Env: env}, nil
}

// buildEnv constructs the environment from scratch.
//
// The returned slice holds secret values. It must not be logged or written.
func buildEnv(entry bridge.Entry, src bridge.SecretSource) ([]string, error) {
	env := make([]string, 0, len(inheritedVars)+len(entry.Env)+len(entry.Secrets))

	for _, name := range inheritedVars {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	for _, name := range sortedKeys(entry.Env) {
		env = append(env, name+"="+entry.Env[name])
	}

	if len(entry.Secrets) > 0 && src == nil {
		return nil, fmt.Errorf("entry %q references secrets but no secret source is configured", entry.Name)
	}
	for _, name := range sortedKeys(entry.Secrets) {
		ref := entry.Secrets[name]
		value, err := src.Get(ref)
		if err != nil {
			// Names the variable and the reference, never the value: the value is
			// what we are protecting, and an error message ends up in a log.
			return nil, fmt.Errorf("resolving %s from %q for entry %q: %w", name, ref, entry.Name, err)
		}
		env = append(env, name+"="+value)
	}
	return env, nil
}

// Exec replaces this process with the proxy.
//
// It does not fork: launchd then supervises mcp-proxy directly, so the reported
// PID is the proxy's and there are no signals to forward. It only returns on
// failure.
func (p Plan) Exec() error {
	if err := syscall.Exec(p.Program, p.Args, p.Env); err != nil {
		return fmt.Errorf("exec %s: %w", p.Program, err)
	}
	return nil // unreachable on success
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
