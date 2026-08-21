// Package launchd implements bridge.ServiceManager on macOS, through
// ~/Library/LaunchAgents plists and launchctl bootstrap/bootout.
//
// It is the only place in the codebase allowed to know about launchctl.
package launchd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// launchctl exit codes, as measured on macOS 2026-08-21. They are not
// documented, and their messages mislead, so they are named here with what they
// actually mean:
//
//	bootstrap on an already-loaded label -> 5, "Input/output error"
//	bootout   on an absent label         -> 3, "No such process"
//	print     on an absent label         -> 113, "Bad request"
//
// Code 5 in particular does not mean an I/O problem; it means "already there".
// Re-measure these when the macOS major version changes.
const (
	exitAlreadyBootstrapped = 5
	exitNoSuchProcess       = 3
	exitNoSuchService       = 113
)

// Manager keeps processes alive using launchd.
type Manager struct {
	// AgentsDir is where plists are written. Empty means ~/Library/LaunchAgents.
	AgentsDir string

	// Domain is the launchd domain target. Empty means the calling user's GUI
	// domain (gui/<uid>), which is where a per-user agent belongs.
	Domain string
}

// New returns a Manager writing to the default LaunchAgents directory.
func New() *Manager { return &Manager{} }

func (m *Manager) agentsDir() (string, error) {
	if m.AgentsDir != "" {
		return m.AgentsDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating the LaunchAgents directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents"), nil
}

func (m *Manager) domain() string {
	if m.Domain != "" {
		return m.Domain
	}
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func (m *Manager) plistPath(label string) (string, error) {
	dir, err := m.agentsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, label+".plist"), nil
}

// Ensure writes the plist and loads it, reconciling rather than creating.
//
// Load-bearing rule 1 in practice: if the service is already loaded with the
// same definition, this is a no-op. If the definition changed, or the service
// drifted out of the loaded state, only that is repaired.
//
// The reconcile is not optional politeness — `launchctl bootstrap` on an
// already-loaded label FAILS (exit 5), so "just bootstrap it" is not an
// implementation of this method.
func (m *Manager) Ensure(label string, spec bridge.ServiceSpec) error {
	want, err := BuildPlist(spec)
	if err != nil {
		return err
	}
	path, err := m.plistPath(label)
	if err != nil {
		return err
	}

	state, err := m.Status(label)
	if err != nil {
		return err
	}
	onDisk, _ := os.ReadFile(path) // absent reads as empty, which differs from want
	unchanged := bytes.Equal(onDisk, want)

	if state.Loaded && unchanged {
		return nil // already in the desired state
	}

	if state.Loaded {
		// The definition changed, so the running service is stale. bootout first,
		// since bootstrap refuses a label that is already there.
		if err := m.bootout(label); err != nil {
			return err
		}
	}

	if !unchanged {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating the LaunchAgents directory: %w", err)
		}
		// 0644: the plist is world-readable by design, and carries no secret.
		if err := os.WriteFile(path, want, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}

	out, err := run("launchctl", "bootstrap", m.domain(), path)
	if err != nil {
		if code(err) == exitAlreadyBootstrapped {
			// Raced with something else loading it. The desired state holds.
			return nil
		}
		return fmt.Errorf("bootstrapping %s: %w (%s)", label, err, out)
	}
	return nil
}

// UnloadTimeout bounds how long Remove waits for a booted-out service to
// actually disappear.
//
// Measured 2026-08-21: `launchctl bootout` returns in ~5ms while the job is
// still loaded and its port still listening; it was gone ~230ms later. The
// command reports that it was accepted, not that it took effect — so Remove
// waits for the effect rather than trusting the write. The bound is generous
// because launchd gives a job an exit timeout of its own before killing it.
const UnloadTimeout = 15 * time.Second

// Remove unloads the service and deletes its definition.
//
// It is the exact inverse of Ensure, and idempotent: a label that is already
// gone is the desired state, not an error.
//
// It returns only once the service has actually gone, not once launchctl has
// accepted the request — see UnloadTimeout.
func (m *Manager) Remove(label string) error {
	if err := m.bootout(label); err != nil {
		return err
	}
	if err := m.waitUnloaded(label); err != nil {
		return err
	}
	path, err := m.plistPath(label)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing %s: %w", path, err)
	}
	return nil
}

// waitUnloaded blocks until the label is gone from the domain.
func (m *Manager) waitUnloaded(label string) error {
	deadline := time.Now().Add(UnloadTimeout)
	for {
		state, err := m.Status(label)
		if err != nil {
			return err
		}
		if !state.Loaded {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s is still loaded %v after being booted out", label, UnloadTimeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// bootout unloads a label, treating "not loaded" as success.
func (m *Manager) bootout(label string) error {
	out, err := run("launchctl", "bootout", m.domain()+"/"+label)
	if err != nil && code(err) != exitNoSuchProcess {
		return fmt.Errorf("booting out %s: %w (%s)", label, err, out)
	}
	return nil
}

var (
	rePID      = regexp.MustCompile(`(?m)^\s*pid\s*=\s*(\d+)`)
	reExitCode = regexp.MustCompile(`(?m)^\s*last exit code\s*=\s*(-?\d+)`)
)

// Status reports what launchd currently knows about the label.
//
// An unknown label is not an error: it is the honest answer "not loaded", which
// is what a reconciler needs in order to decide what to do.
func (m *Manager) Status(label string) (bridge.ServiceState, error) {
	out, err := run("launchctl", "print", m.domain()+"/"+label)
	if err != nil {
		if code(err) == exitNoSuchService {
			return bridge.ServiceState{Loaded: false}, nil
		}
		return bridge.ServiceState{}, fmt.Errorf("reading the state of %s: %w (%s)", label, err, out)
	}
	return parsePrint(out), nil
}

// parsePrint reads the fields we need out of `launchctl print`.
//
// Running is derived from the presence of a pid, NOT from the `state` field.
// Measured 2026-08-21: immediately after bootstrap the state reads
// `state = xpcproxy` — a transient value while launchd is spawning the job —
// and only later becomes `running`. Comparing against "running" is therefore a
// race that reports a healthy service as stopped. A job with a pid is a job
// with a process; a job that has exited has no pid line at all.
func parsePrint(out string) bridge.ServiceState {
	st := bridge.ServiceState{Loaded: true}

	if m := rePID.FindStringSubmatch(out); m != nil {
		if pid, err := strconv.Atoi(m[1]); err == nil {
			st.PID = pid
			st.Running = pid > 0
		}
	}
	// "last exit code = (never exited)" for a job that has not run yet, which is
	// not a number and correctly leaves LastExitCode at zero.
	if m := reExitCode.FindStringSubmatch(out); m != nil {
		if c, err := strconv.Atoi(m[1]); err == nil {
			st.LastExitCode = c
		}
	}
	return st
}

// run executes a command and returns its combined output.
func run(name string, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

// code returns the process exit code, or -1 when the error is not an exit.
func code(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}
