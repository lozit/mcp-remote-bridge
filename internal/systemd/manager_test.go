package systemd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// These tests exercise the LOGIC around systemctl — what is written, in what
// order, and how output is read — with a fake runner. They cannot prove the
// service actually starts: that needs a Linux host, and ADR 0012 says so
// plainly rather than letting a green suite imply otherwise.
//
// What they do prove is the part that is genuinely easy to get wrong and
// invisible until production: that an absent unit is read from LoadState and
// not from an exit code, that a changed definition triggers a daemon-reload,
// and that "active" alone is not taken for "running".

type fake struct {
	calls  []string
	states map[string]string // unit name -> `systemctl show` output
	fail   map[string]error  // verb -> error to return
}

func newFake() *fake {
	return &fake{states: map[string]string{}, fail: map[string]error{}}
}

func (f *fake) run(args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	verb := args[0]
	if err, ok := f.fail[verb]; ok {
		return "", err
	}
	if verb == "show" {
		if out, ok := f.states[args[1]]; ok {
			return out, nil
		}
		return "LoadState=not-found\nActiveState=inactive\nSubState=dead\nMainPID=0\nExecMainStatus=0", nil
	}
	return "", nil
}

func (f *fake) becomes(unit, loadState, activeState string, pid int) {
	f.states[unit] = "LoadState=" + loadState +
		"\nActiveState=" + activeState +
		"\nSubState=running\nMainPID=" + itoa(pid) + "\nExecMainStatus=0"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func manager(t *testing.T, f *fake) *Manager {
	t.Helper()
	return &Manager{UnitsDir: t.TempDir(), runner: f.run}
}

func spec2() bridge.ServiceSpec {
	s := spec()
	return s
}

func called(f *fake, substr string) bool {
	for _, c := range f.calls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// An unknown unit answers with LoadState=not-found and exit 0. Reading the exit
// code instead would report every absent unit as a failure — the same trap as
// launchd's exit 5 meaning "already there".
func TestAnAbsentUnitIsNotAnError(t *testing.T) {
	f := newFake()
	m := manager(t, f)

	state, err := m.Status("com.mcp-remote-bridge.sn")
	if err != nil {
		t.Fatalf("an absent unit reported an error: %v", err)
	}
	if state.Loaded {
		t.Error("an absent unit reported itself loaded")
	}
}

// A real failure of systemctl must NOT be read as "absent", or a broken user
// manager would look like a clean machine.
func TestAFailingSystemctlIsAnError(t *testing.T) {
	f := newFake()
	f.fail["show"] = errors.New("Failed to connect to bus")
	m := manager(t, f)

	if _, err := m.Status("com.mcp-remote-bridge.sn"); err == nil {
		t.Fatal("a failing systemctl was reported as an absent unit")
	}
}

// ActiveState alone is not evidence. A unit configured to restart passes
// through activating/deactivating repeatedly, so a crash-looping service can be
// caught mid-cycle and reported alive.
func TestRunningIsDerivedFromAPidNotAWord(t *testing.T) {
	cases := []struct {
		name        string
		activeState string
		pid         int
		wantRunning bool
	}{
		{"active with a pid", "active", 4242, true},
		{"active with no pid", "active", 0, false},
		{"activating with a pid", "activating", 4242, false},
		{"deactivating with a pid", "deactivating", 4242, false},
		{"failed", "failed", 0, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.becomes("com.mcp-remote-bridge.sn.service", "loaded", tt.activeState, tt.pid)
			m := manager(t, f)

			state, err := m.Status("com.mcp-remote-bridge.sn")
			if err != nil {
				t.Fatal(err)
			}
			if state.Running != tt.wantRunning {
				t.Errorf("Running = %v, want %v (ActiveState=%s pid=%d)",
					state.Running, tt.wantRunning, tt.activeState, tt.pid)
			}
		})
	}
}

// systemd caches unit files. Writing a new definition without reloading makes
// it start the PREVIOUS one and report success — a change that appears applied
// and was not.
func TestAChangedDefinitionReloadsTheManager(t *testing.T) {
	f := newFake()
	f.becomes("com.mcp-remote-bridge.sn.service", "loaded", "active", 4242)
	m := manager(t, f)

	if err := m.Ensure("com.mcp-remote-bridge.sn", spec2()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !called(f, "daemon-reload") {
		t.Errorf("a new unit was written without a daemon-reload: %v", f.calls)
	}
	if !called(f, "restart") {
		t.Errorf("a changed definition was not restarted, so the old process keeps serving: %v", f.calls)
	}
}

// Reconcile, not create: re-running must not interrupt a healthy service.
func TestAnUnchangedActiveUnitIsLeftAlone(t *testing.T) {
	f := newFake()
	f.becomes("com.mcp-remote-bridge.sn.service", "loaded", "active", 4242)
	m := manager(t, f)

	if err := m.Ensure("com.mcp-remote-bridge.sn", spec2()); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	f.calls = nil

	if err := m.Ensure("com.mcp-remote-bridge.sn", spec2()); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	for _, forbidden := range []string{"restart", "daemon-reload"} {
		if called(f, forbidden) {
			t.Errorf("a healthy, unchanged service was disturbed by %q: %v", forbidden, f.calls)
		}
	}
}

func TestTheUnitFileIsWrittenWorldReadableAndCarriesNoSecret(t *testing.T) {
	f := newFake()
	f.becomes("com.mcp-remote-bridge.sn.service", "loaded", "active", 4242)
	dir := t.TempDir()
	m := &Manager{UnitsDir: dir, runner: f.run}

	if err := m.Ensure("com.mcp-remote-bridge.sn", spec2()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	path := filepath.Join(dir, "com.mcp-remote-bridge.sn.service")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the unit was not written: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("unit mode = %v, want 0644", info.Mode().Perm())
	}
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), "Environment=") {
		t.Errorf("the unit carries an environment section:\n%s", body)
	}
}

// Remove is the exact inverse: nothing loaded, and no file left behind.
func TestRemoveDisablesAndDeletesTheUnit(t *testing.T) {
	f := newFake()
	f.becomes("com.mcp-remote-bridge.sn.service", "loaded", "active", 4242)
	dir := t.TempDir()
	m := &Manager{UnitsDir: dir, runner: f.run}

	if err := m.Ensure("com.mcp-remote-bridge.sn", spec2()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	delete(f.states, "com.mcp-remote-bridge.sn.service") // systemd forgets it once disabled

	if err := m.Remove("com.mcp-remote-bridge.sn"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !called(f, "disable --now") {
		t.Errorf("the unit was deleted without being stopped first: %v", f.calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "com.mcp-remote-bridge.sn.service")); !os.IsNotExist(err) {
		t.Error("the unit file was left behind")
	}
}

// Removing something that is already gone is the desired end state, not a
// failure — otherwise `remove` cannot be re-run after a partial one.
func TestRemovingAnAbsentUnitSucceeds(t *testing.T) {
	f := newFake()
	f.fail["disable"] = errors.New("Failed to disable unit: Unit file does not exist")
	m := manager(t, f)

	if err := m.Remove("com.mcp-remote-bridge.sn"); err != nil {
		t.Fatalf("removing an absent unit failed: %v", err)
	}
}
