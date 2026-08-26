package bridge

import "testing"

// An empty report must not be healthy. This is the whole point of rule 2: a
// report that proves nothing claims nothing. If this ever flips, a command that
// ran no probes at all would exit green.
func TestEmptyReportIsNotHealthy(t *testing.T) {
	var r HealthReport
	if r.Healthy() {
		t.Fatal("an empty HealthReport reported healthy; it proved nothing")
	}
}

func TestHealthyRequiresEveryCheck(t *testing.T) {
	tests := []struct {
		name   string
		checks []Check
		want   bool
	}{
		{
			name:   "all pass",
			checks: []Check{{Name: CheckProxyListening, OK: true}, {Name: CheckMCPResponds, OK: true}},
			want:   true,
		},
		{
			name: "the deep probe fails while everything else passes",
			checks: []Check{
				{Name: CheckServiceLoaded, OK: true},
				{Name: CheckProxyListening, OK: true},
				{Name: CheckHostnameResponds, OK: true},
				{Name: CheckMCPResponds, OK: false},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := HealthReport{Entry: "test", Checks: tt.checks}
			if got := r.Healthy(); got != tt.want {
				t.Errorf("Healthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFailedNamesTheFailingChecks(t *testing.T) {
	r := HealthReport{Entry: "test", Checks: []Check{
		{Name: CheckProxyListening, OK: true},
		{Name: CheckMCPResponds, OK: false, Detail: "sn-mcp.example.com"},
	}}
	failed := r.Failed()
	if len(failed) != 1 {
		t.Fatalf("Failed() returned %d checks, want 1", len(failed))
	}
	if failed[0].Name != CheckMCPResponds {
		t.Errorf("Failed() named %q, want %q", failed[0].Name, CheckMCPResponds)
	}
	// A red result has to say where it looked, or it is not actionable.
	if failed[0].Detail == "" {
		t.Error("a failed check carried no Detail; a red result must be actionable")
	}
}

func TestHostname(t *testing.T) {
	e := Entry{Subdomain: "sn-mcp", Domain: "example.com"}
	if got := e.Hostname(); got != "sn-mcp.example.com" {
		t.Errorf("Hostname() = %q, want %q", got, "sn-mcp.example.com")
	}
}
