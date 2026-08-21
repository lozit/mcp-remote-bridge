package bridge_test

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
	"github.com/lozit/mcp-remote-bridge/internal/config"
	"github.com/lozit/mcp-remote-bridge/internal/keychain"
	"github.com/lozit/mcp-remote-bridge/internal/launchd"
)

// The walking skeleton: EnsureExposed driving the REAL launchd and the REAL
// keychain, end to end, with the Exposer left out — everything except the
// public hostname, which needs a tunnel.
//
// This is the first test of the actual assembly. Everything before it verified
// one seam at a time; this one asks whether they compose.

const skeletonSecret = "SKELETON-SECRET-VALUE"

func TestEnsureExposedBringsUpAnEntryEndToEnd(t *testing.T) {
	env := setupSkeleton(t)

	report, err := env.bridge.EnsureExposed(env.entry)
	if err != nil {
		t.Fatalf("EnsureExposed: %v", err)
	}

	// Wait for the proxy to finish starting: EnsureExposed returns as soon as
	// launchd accepts the job, and the MCP handshake takes a moment.
	report = waitHealthy(t, env)

	if !report.Healthy() {
		var sb strings.Builder
		for _, c := range report.Checks {
			fmt.Fprintf(&sb, "\n  %-16s ok=%-5v %s %v", c.Name, c.OK, c.Detail, c.Err)
		}
		t.Fatalf("entry is not healthy:%s\n--- log ---\n%s", sb.String(), readLog(t, env))
	}

	// Every check must have actually run — a report that proved nothing is not
	// healthy, and a partial one must not read as a full pass.
	names := map[bridge.CheckName]bool{}
	for _, c := range report.Checks {
		names[c.Name] = true
	}
	for _, want := range []bridge.CheckName{bridge.CheckServiceLoaded, bridge.CheckProxyListening, bridge.CheckMCPResponds} {
		if !names[want] {
			t.Errorf("the report is missing the %s check", want)
		}
	}
}

// Rule 1: running twice on a healthy entry changes nothing.
func TestEnsureExposedIsANoOpOnAHealthyEntry(t *testing.T) {
	env := setupSkeleton(t)

	if _, err := env.bridge.EnsureExposed(env.entry); err != nil {
		t.Fatalf("first EnsureExposed: %v", err)
	}
	waitHealthy(t, env)
	before, _ := env.manager.Status(bridge.Label(env.entry.Name))

	if _, err := env.bridge.EnsureExposed(env.entry); err != nil {
		t.Fatalf("second EnsureExposed: %v", err)
	}

	after, _ := env.manager.Status(bridge.Label(env.entry.Name))
	if after.PID != before.PID {
		t.Errorf("a no-op EnsureExposed restarted the MCP: pid %d -> %d", before.PID, after.PID)
	}
}

// Rule 1 again, the other half: repair what drifted.
func TestEnsureExposedRepairsDrift(t *testing.T) {
	env := setupSkeleton(t)

	if _, err := env.bridge.EnsureExposed(env.entry); err != nil {
		t.Fatalf("EnsureExposed: %v", err)
	}
	waitHealthy(t, env)

	// Drift: something unloaded the service behind our back.
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), bridge.Label(env.entry.Name))).Run()
	if report := waitUnhealthy(t, env); report.Healthy() {
		t.Fatal("precondition failed: the entry still reports healthy 30s after being unloaded")
	}

	if _, err := env.bridge.EnsureExposed(env.entry); err != nil {
		t.Fatalf("EnsureExposed after drift: %v", err)
	}
	if report := waitHealthy(t, env); !report.Healthy() {
		t.Error("EnsureExposed did not repair the drift")
	}
}

// Remove is the exact inverse.
func TestRemoveExposedIsTheInverse(t *testing.T) {
	env := setupSkeleton(t)

	if _, err := env.bridge.EnsureExposed(env.entry); err != nil {
		t.Fatalf("EnsureExposed: %v", err)
	}
	waitHealthy(t, env)

	if _, err := env.bridge.RemoveExposed(env.entry); err != nil {
		t.Fatalf("RemoveExposed: %v", err)
	}

	report := env.bridge.Probe(env.entry)
	if report.Healthy() {
		t.Error("the entry is still healthy after RemoveExposed")
	}
	st, _ := env.manager.Status(bridge.Label(env.entry.Name))
	if st.Loaded {
		t.Error("the service is still loaded after RemoveExposed")
	}
}

// Rule 3: an absent secret stops everything before anything is launched.
func TestEnsureExposedRefusesAnAbsentSecret(t *testing.T) {
	env := setupSkeleton(t)
	entry := env.entry
	entry.Secrets = map[string]string{"MISSING": "keychain:definitely-not-there"}

	_, err := env.bridge.EnsureExposed(entry)

	if err == nil {
		t.Fatal("EnsureExposed launched an entry whose secret cannot be resolved")
	}
	if !strings.Contains(err.Error(), "MISSING") {
		t.Errorf("the error should name the variable: %v", err)
	}
	// Nothing must have been created.
	if st, _ := env.manager.Status(bridge.Label(entry.Name)); st.Loaded {
		t.Error("a service was created despite the unresolvable secret")
	}
}

// --- fixture ---

// entryName derives a unique, valid entry name from the test's name.
//
// It must satisfy ValidateName: lowercase a-z, 0-9 and '-', 1..63 characters.
func entryName(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("skeleton-")
	for _, r := range strings.ToLower(t.Name()) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if len(name) > 63 {
		name = strings.Trim(name[:63], "-")
	}
	if err := bridge.ValidateName(name); err != nil {
		t.Fatalf("the fixture built an invalid entry name %q: %v", name, err)
	}
	return name
}

type skeleton struct {
	bridge  *bridge.Bridge
	manager *launchd.Manager
	entry   bridge.Entry
	logPath string
}

func setupSkeleton(t *testing.T) *skeleton {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: drives real launchd and the real keychain")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	for _, bin := range []string{"launchctl", "security", "mcp-proxy"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("integration test: %s not on PATH", bin)
		}
	}

	dir := t.TempDir()
	// One identity per test, not per process. The entry name determines both the
	// launchd label AND the auto-assigned port, so a shared name makes these
	// tests fight over the same two global resources: one test's asynchronous
	// cleanup bootout overlaps the next test's bootstrap, and since bootstrap on
	// an already-loaded label is treated as "already there", the second test
	// silently inherits the first one's service.
	name := entryName(t)

	// A throwaway keychain, never the user's.
	kc := filepath.Join(dir, "skeleton.keychain")
	mustRun(t, "security", "create-keychain", "-p", "testpass", kc)
	t.Cleanup(func() { _ = exec.Command("security", "delete-keychain", kc).Run() })
	mustRun(t, "security", "unlock-keychain", "-p", "testpass", kc)
	mustRun(t, "security", "add-generic-password", "-s", "skeleton-secret", "-a", "t", "-w", skeletonSecret, kc)

	mcpPath := filepath.Join(dir, "fakemcp")
	mustRun(t, "go", "build", "-o", mcpPath, "../../internal/testutil/fakemcp")

	binary := filepath.Join(dir, "mcp-remote-bridge")
	mustRun(t, "go", "build", "-o", binary, "../../cmd/mcp-remote-bridge")

	cfgPath := filepath.Join(dir, "config.toml")
	write(t, cfgPath, fmt.Sprintf(`
[infra]
domain     = "example.com"
account_id = "a"
zone_id    = "z"
tunnel_id  = "u"
api_token  = "keychain:skeleton-secret"
keychain   = %q

[mcp.%s]
command   = %q
subdomain = "skeleton"
secrets   = { E2E_TOKEN = "keychain:skeleton-secret" }
`, kc, name, mcpPath))

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading the fixture config: %v", err)
	}
	entry, err := cfg.Entry(name)
	if err != nil {
		t.Fatal(err)
	}

	mgr := &launchd.Manager{AgentsDir: dir}
	proxy, err := exec.LookPath("mcp-proxy")
	if err != nil {
		t.Skipf("mcp-proxy not on PATH: %v", err)
	}

	b := bridge.New(mgr, nil, keychain.New(kc)) // no Exposer: no tunnel in this test
	b.BinaryPath = binary
	b.ConfigPath = cfgPath
	b.LogDir = dir
	b.ProxyPath = proxy

	t.Cleanup(func() {
		_ = exec.Command("launchctl", "bootout",
			fmt.Sprintf("gui/%d/%s", os.Getuid(), bridge.Label(name))).Run()
	})

	return &skeleton{bridge: b, manager: mgr, entry: entry, logPath: b.LogPath(name)}
}

// waitUnhealthy is waitHealthy's mirror, and it exists for a measured reason:
// tearing a service down is not instantaneous. `launchctl bootout` returns in a
// few milliseconds while the job is still loaded and its port still listening —
// with mcp-proxy behind it, everything was gone only ~230ms later.
//
// So a test that boots a service out and probes immediately is asserting that
// the system is synchronous, which it is not. Waiting here is not slackening
// the assertion: the report must still go red, within a bound.
func waitUnhealthy(t *testing.T, env *skeleton) bridge.HealthReport {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var report bridge.HealthReport
	for time.Now().Before(deadline) {
		report = env.bridge.Probe(env.entry)
		if !report.Healthy() {
			return report
		}
		time.Sleep(200 * time.Millisecond)
	}
	return report
}

func waitHealthy(t *testing.T, env *skeleton) bridge.HealthReport {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var report bridge.HealthReport
	for time.Now().Before(deadline) {
		report = env.bridge.Probe(env.entry)
		if report.Healthy() {
			return report
		}
		time.Sleep(300 * time.Millisecond)
	}
	return report
}

func readLog(t *testing.T, env *skeleton) string {
	t.Helper()
	b, err := os.ReadFile(env.logPath)
	if err != nil {
		return "(no log: " + err.Error() + ")"
	}
	return string(b)
}

func mustRun(t *testing.T, name string, args ...string) {
	t.Helper()
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
