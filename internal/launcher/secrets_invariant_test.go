package launcher_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
	"github.com/lozit/mcp-remote-bridge/internal/launchd"
	"github.com/lozit/mcp-remote-bridge/internal/launcher"
)

// The cross-cutting check for the secret path: build EVERY artefact the tool
// produces for an entry, and assert the secret value appears in exactly one of
// them — the process environment — and nowhere else.
//
// The individual invariants each have their own test in their own package. This
// one exists because those tests can all pass while a NEW artefact leaks: the
// failure mode is not a broken check, it is an unchecked surface. Adding
// something the tool writes without adding it here is the mistake this is
// aimed at.
//
// It is deliberately written outside the loop's reach: the task IS the oracle,
// and a maker authoring its own grading criteria has no back pressure.

const leak = "SECRET-VALUE-THAT-MUST-LEAK-NOWHERE"

type oneSecret struct{}

func (oneSecret) Get(key string) (string, error) {
	if key == "keychain:the-ref" {
		return leak, nil
	}
	return "", fmt.Errorf("no such secret %q", key)
}

func entryWithSecret() bridge.Entry {
	return bridge.Entry{
		Name:      "invariants",
		Command:   "/usr/local/bin/some-mcp",
		Args:      []string{"--verbose"},
		Env:       map[string]string{"PLAIN": "not-a-secret"},
		Secrets:   map[string]string{"THE_TOKEN": "keychain:the-ref"},
		Subdomain: "inv",
		Domain:    "example.com",
	}
}

func TestTheSecretReachesTheEnvironmentAndNothingElse(t *testing.T) {
	entry := entryWithSecret()

	plan, err := launcher.Build(entry, oneSecret{}, 8080, "/usr/local/bin/mcp-proxy", nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// The one place it belongs.
	if !containsValue(plan.Env, leak) {
		t.Fatal("the secret did not reach the process environment; the MCP would start without it")
	}

	// Every other artefact, checked by rendering it to text — the form in which
	// it would actually be readable by someone.
	artefacts := map[string]string{
		"argv":             strings.Join(plan.Args, " "),
		"program path":     plan.Program,
		"entry as printed": fmt.Sprintf("%+v", entry),
		"service spec":     fmt.Sprintf("%+v", specFor(t, entry)),
		"plist":            plistFor(t, entry),
	}
	for name, rendered := range artefacts {
		if strings.Contains(rendered, leak) {
			t.Errorf("the secret value appears in the %s:\n%s", name, rendered)
		}
	}
}

// The reference may travel anywhere; only the value may not. Asserting this
// keeps the test honest: it proves the artefacts really do describe this entry,
// rather than passing because they are empty.
func TestTheReferenceTravelsWhereTheValueMayNot(t *testing.T) {
	entry := entryWithSecret()
	printed := fmt.Sprintf("%+v", entry)

	if !strings.Contains(printed, "keychain:the-ref") {
		t.Error("the entry does not carry the reference; this test would pass vacuously")
	}
	if strings.Contains(printed, leak) {
		t.Error("the entry carries the value, not just the reference")
	}
}

// The plist is world-readable. It must carry no environment section at all —
// the natural place someone would later put credentials.
func TestThePlistHasNoEnvironmentSection(t *testing.T) {
	rendered := plistFor(t, entryWithSecret())

	for _, forbidden := range []string{"EnvironmentVariables", "THE_TOKEN", leak} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("the plist contains %q:\n%s", forbidden, rendered)
		}
	}
}

// An unresolvable secret must stop everything before anything is launched.
func TestNothingIsBuiltWhenTheSecretCannotBeResolved(t *testing.T) {
	entry := entryWithSecret()
	entry.Secrets = map[string]string{"THE_TOKEN": "keychain:absent"}

	plan, err := launcher.Build(entry, oneSecret{}, 8080, "/usr/local/bin/mcp-proxy", nil)

	if err == nil {
		t.Fatal("a plan was built for an entry whose secret cannot be resolved")
	}
	if len(plan.Args) != 0 || len(plan.Env) != 0 {
		t.Errorf("a partial plan was returned alongside the error: %+v", plan)
	}
	if !strings.Contains(err.Error(), "THE_TOKEN") {
		t.Errorf("the error should name the variable: %v", err)
	}
}

func specFor(t *testing.T, entry bridge.Entry) bridge.ServiceSpec {
	t.Helper()
	return bridge.ServiceSpec{
		Label:            bridge.Label(entry.Name),
		Program:          "/usr/local/bin/mcp-remote-bridge",
		Args:             []string{"__launch", entry.Name, "--config", "/tmp/config.toml", "--port", "8080"},
		StdoutPath:       "/tmp/log",
		StderrPath:       "/tmp/log",
		KeepAlive:        bridge.KeepAlivePolicy{OnFailure: true, OnCrash: true},
		ThrottleInterval: 60 * time.Second,
	}
}

func plistFor(t *testing.T, entry bridge.Entry) string {
	t.Helper()
	raw, err := launchd.BuildPlist(specFor(t, entry))
	if err != nil {
		t.Fatalf("BuildPlist: %v", err)
	}
	return string(raw)
}

func containsValue(env []string, value string) bool {
	for _, kv := range env {
		if strings.Contains(kv, value) {
			return true
		}
	}
	return false
}
