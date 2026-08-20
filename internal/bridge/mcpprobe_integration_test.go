package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This file exercises ProbeMCPResponds against a REAL mcp-proxy wrapping a real
// stdio MCP process, because a hand-written fake of mcp-proxy would only prove
// the probe works against what we believe mcp-proxy does — and that belief was
// measured wrong once already (ADR 0003). The load-bearing test here is
// TestProbeMCPRespondsGoesRedWhenTheMCPDies: a probe never observed failing has
// not been verified.

const fakeMCPEnv = "MCP_REMOTE_BRIDGE_FAKE_MCP"

// TestMain lets this test binary re-exec itself as a minimal stdio MCP server,
// so the fixture needs no separate build step and no Python.
func TestMain(m *testing.M) {
	if os.Getenv(fakeMCPEnv) == "1" {
		runFakeMCP()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runFakeMCP speaks just enough MCP over stdin/stdout: initialize (declaring
// tools), notifications/initialized, and tools/list.
func runFakeMCP() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 1<<20)
	out := json.NewEncoder(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if len(msg.ID) == 0 { // a notification is never answered
			continue
		}
		var result any
		switch msg.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": mcpProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "fake-mcp", "version": "0.0.1"},
			}
		case "tools/list":
			result = map[string]any{"tools": []any{}}
		case "ping":
			result = map[string]any{}
		default:
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
				"error": map[string]any{"code": -32601, "message": "method not found"}})
			continue
		}
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
	}
}

// proxyFixture is a running mcp-proxy wrapping the fake MCP.
type proxyFixture struct {
	endpoint string
	cmd      *exec.Cmd
}

// startProxy boots mcp-proxy on a free loopback port, or skips the test when
// mcp-proxy is not installed (it is a precondition of the tool, not a Go dep).
func startProxy(t *testing.T) *proxyFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: needs a real mcp-proxy process")
	}
	if _, err := exec.LookPath("mcp-proxy"); err != nil {
		t.Skip("integration test: mcp-proxy not on PATH")
	}

	port := freePort(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("locating the test binary: %v", err)
	}

	cmd := exec.Command("mcp-proxy",
		"--host", "127.0.0.1", "--port", strconv.Itoa(port),
		"--pass-environment", "--", self)
	// --pass-environment forwards this to the spawned child, which is this same
	// binary: that is what turns it into the fake MCP.
	cmd.Env = append(os.Environ(), fakeMCPEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting mcp-proxy: %v", err)
	}

	f := &proxyFixture{endpoint: fmt.Sprintf("http://127.0.0.1:%d/mcp", port), cmd: cmd}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	waitListening(t, port)
	return f
}

// childPID returns the PID of the MCP process mcp-proxy spawned.
//
// Deliberately by parent PID: `pkill -f <script>` would also match mcp-proxy's
// own argv, which contains the child's path, killing both and proving nothing.
func (f *proxyFixture) childPID(t *testing.T) int {
	t.Helper()
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(f.cmd.Process.Pid)).Output()
	if err != nil {
		t.Fatalf("no child process under mcp-proxy (pid %d): %v", f.cmd.Process.Pid, err)
	}
	first := strings.Fields(strings.TrimSpace(string(out)))
	if len(first) == 0 {
		t.Fatal("mcp-proxy has no child process")
	}
	pid, err := strconv.Atoi(first[0])
	if err != nil {
		t.Fatalf("unreadable pid %q: %v", first[0], err)
	}
	return pid
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitListening(t *testing.T, port int) {
	t.Helper()
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			c.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("mcp-proxy never listened on %s", addr)
}

func probe(t *testing.T, endpoint string) Check {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), MCPProbeTimeout)
	defer cancel()
	return ProbeMCPResponds(ctx, &http.Client{Timeout: MCPProbeTimeout}, endpoint, nil)
}

// The positive control. Without it, "it went red when I broke it" means nothing.
func TestProbeMCPRespondsIsGreenAgainstALiveMCP(t *testing.T) {
	f := startProxy(t)

	got := probe(t, f.endpoint)

	if !got.OK {
		t.Fatalf("probe red against a live MCP: %v (detail: %s)", got.Err, got.Detail)
	}
	if got.Name != CheckMCPResponds {
		t.Errorf("Name = %q, want %q", got.Name, CheckMCPResponds)
	}
	if !strings.Contains(got.Detail, "fake-mcp") {
		t.Errorf("Detail = %q, want it to name the server it reached", got.Detail)
	}
}

// The load-bearing test: the whole point of this probe is that it CAN fail.
//
// Every cheaper check stays green in this exact state — the port is open, and
// mcp-proxy answers both `initialize` and `ping` from its startup state. If
// this test ever goes green, the probe has become a health check that cannot
// fail, which is worse than no probe at all.
func TestProbeMCPRespondsGoesRedWhenTheMCPDies(t *testing.T) {
	f := startProxy(t)

	if got := probe(t, f.endpoint); !got.OK {
		t.Fatalf("precondition failed: probe was already red before killing the MCP: %v", got.Err)
	}

	pid := f.childPID(t)
	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("finding the MCP process %d: %v", pid, err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatalf("killing the MCP process %d: %v", pid, err)
	}
	_, _ = proc.Wait()
	time.Sleep(500 * time.Millisecond)

	// Guard the guard: if the proxy died with its child, this test would be
	// measuring a closed port instead of the trap it exists to cover.
	if err := f.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("mcp-proxy died with its child, so this test proves nothing: %v", err)
	}

	got := probe(t, f.endpoint)

	if got.OK {
		t.Fatal("probe GREEN with the MCP process killed — this is the health check that cannot fail")
	}
	if got.Err == nil {
		t.Error("a red probe carried no error; the failure must state its cause")
	}
	if got.Detail == "" {
		t.Error("a red probe carried no Detail; a red result must say where it looked")
	}
}
