package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

func render(r bridge.HealthReport) string {
	var b strings.Builder
	printReport(&b, r)
	return b.String()
}

// An empty report proved nothing, so it must not read as a pass.
func TestPrintReportMarksAnEmptyReportAsFailing(t *testing.T) {
	out := render(bridge.HealthReport{Entry: "sn"})
	if !strings.HasPrefix(out, "FAIL") {
		t.Errorf("an empty report rendered as:\n%s", out)
	}
}

// A red check is only useful with its reason beside it.
func TestPrintReportShowsTheReasonForAFailure(t *testing.T) {
	out := render(bridge.HealthReport{Entry: "sn", Checks: []bridge.Check{
		{Name: bridge.CheckProxyListening, OK: true, Detail: "127.0.0.1:8080"},
		{Name: bridge.CheckMCPResponds, Detail: "http://127.0.0.1:8080/mcp",
			Err: errors.New("the MCP did not answer tools/list")},
	}})

	if !strings.Contains(out, "the MCP did not answer tools/list") {
		t.Errorf("the failure reason is missing:\n%s", out)
	}
	if !strings.Contains(out, "127.0.0.1:8080") {
		t.Errorf("the detail is missing, so the reader cannot tell where it looked:\n%s", out)
	}
	if !strings.HasPrefix(out, "FAIL") {
		t.Errorf("a report with a failing check rendered as a pass:\n%s", out)
	}
}

// Only the checks that ran are printed: a blank line would read as a pass.
func TestPrintReportPrintsOnlyChecksThatRan(t *testing.T) {
	out := render(bridge.HealthReport{Entry: "sn", Checks: []bridge.Check{
		{Name: bridge.CheckProxyListening, OK: true, Detail: "127.0.0.1:8080"},
	}})

	if strings.Contains(out, string(bridge.CheckHostnameResponds)) {
		t.Errorf("a check that did not run was printed:\n%s", out)
	}
	if !strings.HasPrefix(out, "ok") {
		t.Errorf("a report whose only check passed rendered as a failure:\n%s", out)
	}
}

// The exit code composes in scripts, so it must follow the checks.
func TestExitCodeFollowsTheReports(t *testing.T) {
	green := bridge.HealthReport{Entry: "a", Checks: []bridge.Check{{Name: bridge.CheckProxyListening, OK: true}}}
	red := bridge.HealthReport{Entry: "b", Checks: []bridge.Check{{Name: bridge.CheckProxyListening}}}

	if got := exitCodeFor([]bridge.HealthReport{green}); got != exitOK {
		t.Errorf("all healthy gave %d, want %d", got, exitOK)
	}
	if got := exitCodeFor([]bridge.HealthReport{green, red}); got != exitUnhealthy {
		t.Errorf("one unhealthy gave %d, want %d", got, exitUnhealthy)
	}
	// No entries means nothing was proved, but nothing failed either: apply on an
	// empty selection is not an error, and the config parser already refuses a
	// config with no entries at all.
	if got := exitCodeFor(nil); got != exitOK {
		t.Errorf("no reports gave %d, want %d", got, exitOK)
	}
}

func TestParseEntryArgs(t *testing.T) {
	tests := []struct {
		args       []string
		name, path string
		wantErr    bool
	}{
		{args: nil},
		{args: []string{"sn"}, name: "sn"},
		{args: []string{"--config", "/p/c.toml"}, path: "/p/c.toml"},
		{args: []string{"sn", "--config", "/p/c.toml"}, name: "sn", path: "/p/c.toml"},
		{args: []string{"--config"}, wantErr: true},
		{args: []string{"a", "b"}, wantErr: true},
	}
	for _, tt := range tests {
		name, path, err := parseEntryArgs("apply", tt.args)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseEntryArgs(%v) accepted it", tt.args)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseEntryArgs(%v): %v", tt.args, err)
			continue
		}
		if name != tt.name || path != tt.path {
			t.Errorf("parseEntryArgs(%v) = %q, %q, want %q, %q", tt.args, name, path, tt.name, tt.path)
		}
	}
}
