package doctor

import (
	"errors"
	"strings"
	"testing"

	"github.com/lozit/mcp-remote-bridge/internal/config"
)

func healthyDeps() Deps {
	return Deps{
		LookPath:       func(name string) (string, error) { return "/usr/local/bin/" + name, nil },
		ProcessMatches: func(string) bool { return true },
		ResolveSecret:  func(string) (string, error) { return "a-value", nil },
		Executable:     func() (string, error) { return "/bin/sh", nil }, // exists
	}
}

func fullConfig() *config.File {
	return &config.File{
		Infra: config.Infra{
			Domain: "example.com", AccountID: "a", ZoneID: "z", TunnelID: "t",
			APIToken:           "keychain:cf-api-token",
			AccessClientID:     "abc.access",
			AccessClientSecret: "keychain:cf-access-secret",
		},
		MCP: map[string]config.MCP{"sn": {Command: "x", Subdomain: "sn"}},
	}
}

func find(t *testing.T, checks []Check, name string) Check {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check named %q in %v", name, names(checks))
	return Check{}
}

func names(checks []Check) []string {
	out := make([]string, 0, len(checks))
	for _, c := range checks {
		out = append(out, c.Name)
	}
	return out
}

func TestRunPassesWithEverythingInPlace(t *testing.T) {
	checks := Run(fullConfig(), healthyDeps())

	if !Healthy(checks) {
		t.Errorf("healthy environment reported unhealthy:\n%s", Render(checks))
	}
}

// A missing precondition must be named, and say what to do about it.
func TestRunReportsAMissingBinaryWithAHint(t *testing.T) {
	deps := healthyDeps()
	deps.LookPath = func(name string) (string, error) {
		if name == "mcp-proxy" {
			return "", errors.New("not found")
		}
		return "/usr/local/bin/" + name, nil
	}

	checks := Run(fullConfig(), deps)
	got := find(t, checks, "mcp-proxy")

	if got.OK {
		t.Error("a missing mcp-proxy reported as present")
	}
	if got.Hint == "" {
		t.Error("a red check with no hint is a status line, not a diagnosis")
	}
	if Healthy(checks) {
		t.Error("Healthy() true with a failing check")
	}
}

// An installed connector that is not running carries no traffic.
func TestRunDistinguishesInstalledFromRunning(t *testing.T) {
	deps := healthyDeps()
	deps.ProcessMatches = func(string) bool { return false }

	checks := Run(fullConfig(), deps)

	if !find(t, checks, "cloudflared").OK {
		t.Error("cloudflared is on PATH but reported missing")
	}
	if find(t, checks, "tunnel_connector").OK {
		t.Error("a connector that is not running reported as fine")
	}
}

// The measured failure mode of security(1): success with an empty value.
func TestRunCatchesASecretThatResolvesToNothing(t *testing.T) {
	deps := healthyDeps()
	deps.ResolveSecret = func(string) (string, error) { return "", nil }

	got := find(t, Run(fullConfig(), deps), "cloudflare_api_token")

	if got.OK {
		t.Error("a secret resolving to an empty value reported as present")
	}
	if !strings.Contains(got.Err.Error(), "empty") {
		t.Errorf("the error should say the value is empty: %v", got.Err)
	}
}

// The secret's value must never reach the output, on any path.
func TestRunNeverPrintsASecretValue(t *testing.T) {
	const secret = "SECRET-VALUE-MUST-NOT-BE-RENDERED"
	deps := healthyDeps()
	deps.ResolveSecret = func(string) (string, error) { return secret, nil }

	out := Render(Run(fullConfig(), deps))

	if strings.Contains(out, secret) {
		t.Errorf("doctor printed a secret value:\n%s", out)
	}
}

// No Access token is a legitimate setup, not a failure — but it is the first
// suspect when a guarded hostname is permanently red, so it must be visible.
func TestRunTreatsAnAbsentAccessTokenAsANote(t *testing.T) {
	cfg := fullConfig()
	cfg.Infra.AccessClientID = ""
	cfg.Infra.AccessClientSecret = ""

	got := find(t, Run(cfg, healthyDeps()), "access_service_token")

	if !got.OK {
		t.Error("an unguarded setup reported as broken")
	}
	if got.Hint == "" {
		t.Error("the note carries no hint, so a red hostname check has no first suspect")
	}
}

// A config that would not load: report that, and do not run the checks that
// depend on it — a check that could not run must not read as one that failed.
func TestRunWithNoConfigSkipsDependentChecks(t *testing.T) {
	checks := Run(nil, healthyDeps())

	if find(t, checks, "config").OK {
		t.Error("a missing config reported as fine")
	}
	for _, name := range []string{"cloudflare_api_token", "access_service_token"} {
		for _, c := range checks {
			if c.Name == name {
				t.Errorf("%s was evaluated without a config", name)
			}
		}
	}
	// The environment checks do not depend on the config and must still run.
	if !find(t, checks, "mcp-proxy").OK {
		t.Error("environment checks were skipped along with the config-dependent ones")
	}
}

// The Portal step is not a failure; it is a step the tool cannot take.
func TestRunAlwaysReportsThePortalStep(t *testing.T) {
	got := find(t, Run(fullConfig(), healthyDeps()), "portal_server")

	if !got.OK {
		t.Error("the Portal reminder reported as a failure; it is an outstanding manual step")
	}
	if !strings.Contains(got.Hint, "AT CREATION") {
		t.Errorf("the hint should carry the ordering that cannot be recovered from: %q", got.Hint)
	}
}

func TestRenderShowsHintsOnlyForFailures(t *testing.T) {
	out := Render([]Check{
		{Name: "fine", OK: true, Detail: "d", Hint: "should not appear"},
		{Name: "broken", Err: errors.New("nope"), Hint: "do this"},
	})

	if strings.Contains(out, "should not appear") {
		t.Errorf("a hint was printed for a passing check:\n%s", out)
	}
	if !strings.Contains(out, "do this") {
		t.Errorf("the failing check's hint is missing:\n%s", out)
	}
}
