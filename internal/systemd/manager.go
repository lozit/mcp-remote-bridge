package systemd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// settleTimeout bounds the wait for a unit to reach the state just asked for.
//
// `systemctl --user enable --now` returns before the unit has finished
// starting, so reading the state immediately after would report the state
// before the change. Same reason launchd's manager waits.
const (
	settleTimeout  = 5 * time.Second
	settleInterval = 100 * time.Millisecond
)

// Manager keeps processes alive using per-user systemd units.
//
// User units, not system ones: it keeps the tool out of root. The cost is
// lingering — see doctor — since a user manager is torn down with the last
// session unless the user has it enabled.
type Manager struct {
	// UnitsDir is where units are written. Empty means
	// ~/.config/systemd/user, or $XDG_CONFIG_HOME/systemd/user when set.
	UnitsDir string

	// runner executes systemctl. Nil means the real one; tests replace it.
	runner func(args ...string) (string, error)
}

// New returns a Manager writing to the default user unit directory.
func New() *Manager { return &Manager{} }

func (m *Manager) unitsDir() (string, error) {
	if m.UnitsDir != "" {
		return m.UnitsDir, nil
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "systemd", "user"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating the systemd user unit directory: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

func (m *Manager) unitPath(label string) (string, error) {
	dir, err := m.unitsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, UnitName(label)), nil
}

func (m *Manager) run(args ...string) (string, error) {
	if m.runner != nil {
		return m.runner(args...)
	}
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// Ensure reconciles the unit and the running service to spec.
//
// Reconcile, not create: an unchanged unit that is already active is left
// alone, so re-running costs nothing and cannot interrupt a healthy service.
func (m *Manager) Ensure(label string, spec bridge.ServiceSpec) error {
	want, err := BuildUnit(spec)
	if err != nil {
		return err
	}
	path, err := m.unitPath(label)
	if err != nil {
		return err
	}

	state, err := m.Status(label)
	if err != nil {
		return err
	}
	onDisk, _ := os.ReadFile(path) // absent reads as empty, which differs from want
	unchanged := bytes.Equal(onDisk, want)

	if state.Loaded && state.Running && unchanged {
		return nil
	}

	if !unchanged {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating the systemd user unit directory: %w", err)
		}
		// 0644: the unit is world-readable by design, and carries no secret.
		if err := os.WriteFile(path, want, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		// systemd caches unit files. Without this it starts the previous
		// definition and reports success, which is a change that appears to
		// have been applied and was not.
		if out, err := m.run("daemon-reload"); err != nil {
			return fmt.Errorf("reloading the systemd user manager: %w (%s)", err, out)
		}
	}

	// restart rather than start: `start` on an already-active unit is a no-op,
	// so a changed definition would keep serving from the old process.
	if out, err := m.run("enable", "--now", UnitName(label)); err != nil {
		return fmt.Errorf("enabling %s: %w (%s)", label, err, out)
	}
	if !unchanged {
		if out, err := m.run("restart", UnitName(label)); err != nil {
			return fmt.Errorf("restarting %s: %w (%s)", label, err, out)
		}
	}

	// Verify the effect, not the exit code. systemctl exits 0 for a unit that
	// went active and immediately failed, so the exit code alone cannot tell
	// "running" from "tried".
	return m.waitRunning(label)
}

// Remove is the exact inverse of Ensure: no unit file, nothing loaded.
func (m *Manager) Remove(label string) error {
	path, err := m.unitPath(label)
	if err != nil {
		return err
	}

	// disable --now on an absent unit is not an error worth propagating: the
	// desired end state is "gone", and it is already partly true.
	if out, err := m.run("disable", "--now", UnitName(label)); err != nil {
		if state, sErr := m.Status(label); sErr == nil && !state.Loaded {
			// already gone
		} else {
			return fmt.Errorf("disabling %s: %w (%s)", label, err, out)
		}
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", path, err)
	}
	if out, err := m.run("daemon-reload"); err != nil {
		return fmt.Errorf("reloading the systemd user manager: %w (%s)", err, out)
	}

	return m.waitGone(label)
}

// Status reports what systemd currently holds for label.
func (m *Manager) Status(label string) (bridge.ServiceState, error) {
	out, err := m.run("show", UnitName(label),
		"--property=LoadState,ActiveState,SubState,MainPID,ExecMainStatus")
	if err != nil {
		// `show` answers for an unknown unit with LoadState=not-found and exit
		// 0, so a non-zero exit here is a real failure — not the "absent" case.
		return bridge.ServiceState{}, fmt.Errorf("reading the state of %s: %w (%s)", label, err, out)
	}
	return parseShow(out), nil
}

// parseShow reads `systemctl show` key=value output.
//
// The absent case is LoadState=not-found, NOT a non-zero exit — reading the
// exit code here would report every absent unit as an error. That is the same
// shape as launchd's exit 5 meaning "already there": the code answers a
// different question than the one being asked.
func parseShow(out string) bridge.ServiceState {
	fields := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			fields[key] = value
		}
	}

	state := bridge.ServiceState{
		Loaded: fields["LoadState"] == "loaded",
	}
	state.PID, _ = strconv.Atoi(fields["MainPID"])
	state.LastExitCode, _ = strconv.Atoi(fields["ExecMainStatus"])

	// Running is derived from a pid, not from a word. ActiveState is
	// "activating" while the process is being set up and "deactivating" while
	// it is going away, and a unit configured to restart passes through both
	// repeatedly — so reading the word alone reports a crash-looping service as
	// alive. A pid is the evidence that something is actually there.
	state.Running = state.PID > 0 && fields["ActiveState"] == "active"

	return state
}

func (m *Manager) waitRunning(label string) error {
	deadline := time.Now().Add(settleTimeout)
	var last bridge.ServiceState
	for {
		state, err := m.Status(label)
		if err != nil {
			return err
		}
		if state.Loaded && state.Running {
			return nil
		}
		last = state
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(settleInterval)
	}
	return fmt.Errorf("%s did not come up within %s (loaded=%v running=%v last exit=%d)",
		label, settleTimeout, last.Loaded, last.Running, last.LastExitCode)
}

func (m *Manager) waitGone(label string) error {
	deadline := time.Now().Add(settleTimeout)
	for {
		state, err := m.Status(label)
		if err != nil {
			return err
		}
		if !state.Loaded {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s was still loaded %s after being removed", label, settleTimeout)
		}
		time.Sleep(settleInterval)
	}
}
