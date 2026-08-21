package launchd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// These tests drive the real launchctl, because the behaviour that matters is
// undocumented and its error messages mislead: bootstrap on an already-loaded
// label fails with "Input/output error", which means "already there". A fake
// would only encode what we believe launchctl does.
//
// Every test uses a unique throwaway label under its own temp LaunchAgents
// directory, and boots it out on cleanup — never the user's real agents.

func newManager(t *testing.T) (*Manager, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: loads and unloads real launchd services")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if _, err := exec.LookPath("launchctl"); err != nil {
		t.Skip("launchctl not on PATH")
	}

	label := fmt.Sprintf("com.mcp-remote-bridge.test-%d-%s", os.Getpid(), strings.ReplaceAll(t.Name(), "/", "-"))
	m := &Manager{AgentsDir: t.TempDir()}

	t.Cleanup(func() {
		// Belt and braces: Remove is under test, so tear down directly.
		_ = exec.Command("launchctl", "bootout", m.domain()+"/"+label).Run()
	})
	return m, label
}

func specFor(t *testing.T, label, script string) bridge.ServiceSpec {
	t.Helper()
	log := filepath.Join(t.TempDir(), "out.log")
	return bridge.ServiceSpec{
		Label:            label,
		Program:          "/bin/sh",
		Args:             []string{"-c", script},
		StdoutPath:       log,
		StderrPath:       log,
		ThrottleInterval: 3600 * time.Second, // never retry during a test
	}
}

func TestEnsureLoadsTheService(t *testing.T) {
	m, label := newManager(t)

	if err := m.Ensure(label, specFor(t, label, "sleep 300")); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	st, err := m.Status(label)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Loaded {
		t.Error("service is not loaded after Ensure")
	}
	if !st.Running || st.PID == 0 {
		t.Errorf("service is loaded but not running: %+v", st)
	}
}

// Rule 1, as a test. It is also the case a naive implementation gets wrong:
// `launchctl bootstrap` on an already-loaded label fails outright.
func TestEnsureIsIdempotent(t *testing.T) {
	m, label := newManager(t)
	spec := specFor(t, label, "sleep 300")

	if err := m.Ensure(label, spec); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	first, _ := m.Status(label)

	if err := m.Ensure(label, spec); err != nil {
		t.Fatalf("second Ensure on an unchanged service: %v — bootstrap refuses an already-loaded label, so this must be a no-op", err)
	}

	second, _ := m.Status(label)
	if second.PID != first.PID {
		t.Errorf("the service was restarted by a no-op Ensure: pid %d -> %d", first.PID, second.PID)
	}
}

// A changed definition must be applied, not silently ignored.
func TestEnsureRepairsAChangedDefinition(t *testing.T) {
	m, label := newManager(t)

	if err := m.Ensure(label, specFor(t, label, "sleep 300")); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	before, _ := m.Status(label)

	changed := specFor(t, label, "sleep 301")
	if err := m.Ensure(label, changed); err != nil {
		t.Fatalf("Ensure with a changed spec: %v", err)
	}

	after, _ := m.Status(label)
	if !after.Loaded || !after.Running {
		t.Fatalf("service is not running after a definition change: %+v", after)
	}
	if after.PID == before.PID {
		t.Error("the service kept its old process after its definition changed")
	}

	path := filepath.Join(m.AgentsDir, label+".plist")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the plist: %v", err)
	}
	if !strings.Contains(string(raw), "sleep 301") {
		t.Error("the plist on disk was not updated")
	}
}

// Drift: the service was unloaded behind our back. Ensure must reload it.
func TestEnsureReloadsAfterExternalUnload(t *testing.T) {
	m, label := newManager(t)
	spec := specFor(t, label, "sleep 300")

	if err := m.Ensure(label, spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if out, err := exec.Command("launchctl", "bootout", m.domain()+"/"+label).CombinedOutput(); err != nil {
		t.Fatalf("external bootout: %v: %s", err, out)
	}
	if st, _ := m.Status(label); st.Loaded {
		t.Fatal("precondition failed: the service is still loaded after an external bootout")
	}

	if err := m.Ensure(label, spec); err != nil {
		t.Fatalf("Ensure after drift: %v", err)
	}
	if st, _ := m.Status(label); !st.Loaded || !st.Running {
		t.Errorf("Ensure did not repair the drift: %+v", st)
	}
}

func TestRemoveIsTheInverseOfEnsure(t *testing.T) {
	m, label := newManager(t)

	if err := m.Ensure(label, specFor(t, label, "sleep 300")); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	path := filepath.Join(m.AgentsDir, label+".plist")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the plist was not written: %v", err)
	}

	if err := m.Remove(label); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if st, _ := m.Status(label); st.Loaded {
		t.Error("the service is still loaded after Remove")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the plist is still on disk after Remove")
	}
}

// Remove must reach the desired state, not complain that it is already there.
func TestRemoveIsIdempotent(t *testing.T) {
	m, label := newManager(t)

	if err := m.Remove(label); err != nil {
		t.Errorf("Remove on a service that was never created: %v — the desired state already holds", err)
	}
	if err := m.Ensure(label, specFor(t, label, "sleep 300")); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if err := m.Remove(label); err != nil {
		t.Fatalf("first Remove: %v", err)
	}
	if err := m.Remove(label); err != nil {
		t.Errorf("second Remove: %v — must be a no-op", err)
	}
}

// An unknown label is a fact to report, not a failure.
func TestStatusReportsAnUnknownLabelAsNotLoaded(t *testing.T) {
	m, label := newManager(t)

	st, err := m.Status(label + "-never-created")
	if err != nil {
		t.Fatalf("Status on an unknown label returned an error: %v", err)
	}
	if st.Loaded {
		t.Error("an unknown label reported as loaded")
	}
}

// A service that exited is still loaded. Conflating the two would make a dead
// MCP look absent instead of broken.
func TestStatusDistinguishesLoadedFromRunning(t *testing.T) {
	m, label := newManager(t)

	if err := m.Ensure(label, specFor(t, label, "exit 42")); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	var st bridge.ServiceState
	for time.Now().Before(deadline) {
		st, _ = m.Status(label)
		if st.Loaded && !st.Running {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if !st.Loaded {
		t.Fatalf("a service that exited should still be loaded: %+v", st)
	}
	if st.Running {
		t.Errorf("a service that exited is reported as running: %+v", st)
	}
	if st.LastExitCode != 42 {
		t.Errorf("LastExitCode = %d, want 42 — the exit status is how a caller learns why it died", st.LastExitCode)
	}
}
