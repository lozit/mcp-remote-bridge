package launcher

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// The secret value used throughout. Distinctive so a leak is unambiguous.
const secretValue = "SECRET-VALUE-THAT-MUST-NEVER-APPEAR-IN-ARGV"

type fakeSource map[string]string

func (f fakeSource) Get(key string) (string, error) {
	v, ok := f[key]
	if !ok {
		return "", errors.New("no such secret")
	}
	return v, nil
}

func fakeLookPath(string) (string, error) { return "/usr/local/bin/mcp-proxy", nil }

func testEntry() bridge.Entry {
	return bridge.Entry{
		Name:      "standardnotes",
		Command:   "mcp-standardnotes",
		Args:      []string{"--verbose"},
		Env:       map[string]string{"SN_SERVER": "https://sync.example.com"},
		Secrets:   map[string]string{"SN_TOKEN": "keychain:sn-token"},
		Subdomain: "sn-mcp",
		Domain:    "example.com",
	}
}

func build(t *testing.T, e bridge.Entry, src bridge.SecretSource) Plan {
	t.Helper()
	p, err := Build(e, src, 8080, fakeLookPath)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return p
}

// Rule 3, as a test: no secret value in argv, where ps exposes it to every
// local account.
func TestBuildNeverPutsASecretInArgv(t *testing.T) {
	p := build(t, testEntry(), fakeSource{"keychain:sn-token": secretValue})

	for _, arg := range p.Args {
		if strings.Contains(arg, secretValue) {
			t.Fatalf("secret value found in argv: %q\nfull argv: %v", arg, p.Args)
		}
	}
	// The specific mechanism that would do it.
	if slices.Contains(p.Args, "-e") || slices.Contains(p.Args, "--env") {
		t.Errorf("argv uses mcp-proxy's -e/--env, which places values in argv: %v", p.Args)
	}
	// And the mechanism that must be there instead.
	if !slices.Contains(p.Args, "--pass-environment") {
		t.Errorf("argv lacks --pass-environment, so the MCP will not receive the secrets: %v", p.Args)
	}
}

func TestBuildPutsTheSecretInTheEnvironment(t *testing.T) {
	p := build(t, testEntry(), fakeSource{"keychain:sn-token": secretValue})

	if !slices.Contains(p.Env, "SN_TOKEN="+secretValue) {
		t.Error("the resolved secret is not in the environment, so the MCP would start without it")
	}
	if !slices.Contains(p.Env, "SN_SERVER=https://sync.example.com") {
		t.Errorf("the entry's declared env is missing: %v", p.Env)
	}
}

// The environment is constructed, not inherited (ADR 0002): mcp-proxy is run
// with --pass-environment, so anything inherited here reaches the MCP too.
func TestBuildDoesNotInheritTheAmbientEnvironment(t *testing.T) {
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-credential-that-must-not-leak")
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", "/Users/test")

	p := build(t, testEntry(), fakeSource{"keychain:sn-token": secretValue})

	for _, kv := range p.Env {
		if strings.HasPrefix(kv, "AWS_SECRET_ACCESS_KEY=") {
			t.Fatal("an ambient variable was inherited; --pass-environment would forward it to the MCP")
		}
	}
	if !slices.Contains(p.Env, "PATH=/usr/bin") || !slices.Contains(p.Env, "HOME=/Users/test") {
		t.Errorf("PATH and HOME should be carried over: %v", p.Env)
	}
	// Exactly: PATH, HOME, one declared env var, one secret.
	if len(p.Env) != 4 {
		t.Errorf("environment has %d entries, want exactly 4 (PATH, HOME, SN_SERVER, SN_TOKEN): %v", len(p.Env), p.Env)
	}
}

// An absent secret must stop everything, not launch a proxy that 401s silently.
func TestBuildFailsWhenASecretIsMissing(t *testing.T) {
	_, err := Build(testEntry(), fakeSource{}, 8080, fakeLookPath)
	if err == nil {
		t.Fatal("Build succeeded with an unresolvable secret reference")
	}
	if !strings.Contains(err.Error(), "SN_TOKEN") {
		t.Errorf("the error should name the variable so it is actionable: %v", err)
	}
}

// The error path is where leaks hide: it is the branch nobody re-reads.
func TestBuildErrorsNeverContainASecretValue(t *testing.T) {
	failing := fakeSource{}
	_, err := Build(testEntry(), failing, 8080, fakeLookPath)
	if err != nil && strings.Contains(err.Error(), secretValue) {
		t.Errorf("error leaked the secret value: %v", err)
	}

	// Also when the source returns a value alongside an error.
	weird := brokenSource{value: secretValue}
	_, err = Build(testEntry(), weird, 8080, fakeLookPath)
	if err == nil {
		t.Fatal("Build ignored an error from the secret source")
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Errorf("error leaked the secret value: %v", err)
	}
}

type brokenSource struct{ value string }

func (b brokenSource) Get(string) (string, error) {
	return b.value, errors.New("keychain is locked")
}

func TestBuildBindsLoopbackExplicitly(t *testing.T) {
	p := build(t, testEntry(), fakeSource{"keychain:sn-token": secretValue})

	i := slices.Index(p.Args, "--host")
	if i < 0 || i+1 >= len(p.Args) {
		t.Fatalf("argv does not pass --host explicitly: %v", p.Args)
	}
	if p.Args[i+1] != "127.0.0.1" {
		t.Errorf("--host is %q, want 127.0.0.1", p.Args[i+1])
	}
	for _, arg := range p.Args {
		if arg == "0.0.0.0" {
			t.Error("argv mentions 0.0.0.0; the proxy must bind loopback only")
		}
	}
}

// -- separates proxy flags from the MCP command.
func TestBuildSeparatesTheMCPCommand(t *testing.T) {
	p := build(t, testEntry(), fakeSource{"keychain:sn-token": secretValue})

	i := slices.Index(p.Args, "--")
	if i < 0 {
		t.Fatalf("argv has no -- separator: %v", p.Args)
	}
	got := p.Args[i+1:]
	want := []string{"mcp-standardnotes", "--verbose"}
	if !slices.Equal(got, want) {
		t.Errorf("after --, got %v, want %v", got, want)
	}
}

func TestBuildRejectsAnUnusablePort(t *testing.T) {
	for _, port := range []int{0, -1, 70000} {
		if _, err := Build(testEntry(), fakeSource{"keychain:sn-token": secretValue}, port, fakeLookPath); err == nil {
			t.Errorf("Build accepted port %d", port)
		}
	}
}

func TestBuildFailsWhenTheProxyIsAbsent(t *testing.T) {
	absent := func(string) (string, error) { return "", errors.New("not found") }
	_, err := Build(testEntry(), fakeSource{"keychain:sn-token": secretValue}, 8080, absent)
	if err == nil {
		t.Fatal("Build succeeded without mcp-proxy, which is a precondition")
	}
	if !strings.Contains(err.Error(), "precondition") {
		t.Errorf("the error should say this is a precondition: %v", err)
	}
}

// An entry with no secrets must not require a source at all.
func TestBuildWorksWithoutSecrets(t *testing.T) {
	e := testEntry()
	e.Secrets = nil
	p, err := Build(e, nil, 8080, fakeLookPath)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(p.Env) == 0 {
		t.Error("environment should still carry PATH/HOME and the declared env")
	}
	_ = os.Environ()
}
