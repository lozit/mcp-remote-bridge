package launcher_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// End-to-end proof of rule 3: build the real binary, run its __launch
// subcommand against a real keychain entry and a real mcp-proxy, then inspect
// the RUNNING processes' argv with ps.
//
// The unit tests assert that Build does not put a secret in argv. This asserts
// that nothing downstream — mcp-proxy's own handling, the shell, the exec —
// puts it back.

const e2eSecret = "E2E-SECRET-VALUE-MUST-NOT-APPEAR-IN-PS"

func TestLaunchKeepsTheSecretOutOfProcessArgv(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: builds the binary and starts real processes")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only: uses the keychain")
	}
	for _, bin := range []string{"mcp-proxy", "security", "ps"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("integration test: %s not on PATH", bin)
		}
	}

	dir := t.TempDir()

	// A throwaway keychain holding the secret. Never the user's own.
	kc := filepath.Join(dir, "launch-test.keychain")
	run(t, "security", "create-keychain", "-p", "testpass", kc)
	t.Cleanup(func() { _ = exec.Command("security", "delete-keychain", kc).Run() })
	run(t, "security", "unlock-keychain", "-p", "testpass", kc)
	run(t, "security", "add-generic-password", "-s", "launch-test-secret", "-a", "t", "-w", e2eSecret, kc)

	// A stdio MCP that completes mcp-proxy's startup handshake and echoes the
	// environment it received back through tools/list.
	mcpPath := filepath.Join(dir, "fakemcp")
	run(t, "go", "build", "-o", mcpPath, "../../internal/testutil/fakemcp")

	port := freePort(t)
	cfgPath := filepath.Join(dir, "config.toml")
	writeFile(t, cfgPath, fmt.Sprintf(`
[infra]
domain     = "example.com"
account_id = "a"
zone_id    = "z"
tunnel_id  = "u"
api_token  = "keychain:launch-test-secret"
keychain   = %q

[mcp.sleeper]
command   = %q
subdomain = "sleeper"
port      = %d
secrets   = { E2E_TOKEN = "keychain:launch-test-secret" }
`, kc, mcpPath, port))

	binary := filepath.Join(dir, "mcp-remote-bridge")
	run(t, "go", "build", "-o", binary, "../../cmd/mcp-remote-bridge")

	cmd := exec.Command(binary, "__launch", "sleeper", "--config", cfgPath, "--port", strconv.Itoa(port))
	// The keychain must be searchable by the child.
	cmd.Env = append(os.Environ(), "HOME="+os.Getenv("HOME"))
	stderr := &strings.Builder{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting __launch: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	waitListening(t, port, stderr)

	// The whole point: what does ps show for every process in this tree?
	psOut := run(t, "ps", "-eo", "pid=,ppid=,command=")
	var inspected int
	for _, line := range strings.Split(psOut, "\n") {
		if !strings.Contains(line, "launch-test") && !strings.Contains(line, "sleeper") && !strings.Contains(line, "mcp-proxy") {
			continue
		}
		inspected++
		if strings.Contains(line, e2eSecret) {
			t.Fatalf("the secret is visible in ps output:\n%s", line)
		}
	}
	if inspected == 0 {
		t.Fatal("ps showed no process from the launched tree; the check proved nothing")
	}
	t.Logf("inspected %d process lines, no secret in any argv", inspected)

	// POSITIVE CONTROL, and the other half of rule 3: the secret must actually
	// have REACHED the MCP, through the environment. Without this, a launcher
	// that passed no secret at all would sail through the check above.
	//
	// fakemcp echoes the environment variables it received as tool descriptions,
	// so asking the MCP for its tool list asks the MCP what it was given.
	tools := listTools(t, port)
	if !strings.Contains(tools, e2eSecret) {
		t.Fatalf("the MCP did not receive E2E_TOKEN through its environment; "+
			"the argv check above therefore proves nothing.\ntools/list returned: %s", tools)
	}
	t.Log("the secret reached the MCP's environment and appears in no argv")
	// And the launcher must not have leaked it to stderr either.
	if strings.Contains(stderr.String(), e2eSecret) {
		t.Errorf("the secret appeared on stderr:\n%s", stderr.String())
	}
}

// listTools drives an MCP session far enough to read the tool list, which is
// where fakemcp reports the environment it was handed.
func listTools(t *testing.T, port int) string {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
	post := func(session, body string) (string, string) {
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if session != "" {
			req.Header.Set("mcp-session-id", session)
		}
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", url, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b), resp.Header.Get("mcp-session-id")
	}

	_, session := post("", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)
	post(session, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	body, _ := post(session, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	return body
}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func readSecret(t *testing.T, keychain string) string {
	t.Helper()
	out, err := exec.Command("security", "find-generic-password", "-s", "launch-test-secret", "-w", keychain).Output()
	if err != nil {
		t.Fatalf("reading back the fixture: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitListening(t *testing.T, port int, stderr fmt.Stringer) {
	t.Helper()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			c.Close()
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("nothing listening on %s after __launch; stderr:\n%s", addr, stderr.String())
}
