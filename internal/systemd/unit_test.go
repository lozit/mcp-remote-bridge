package systemd

import (
	"strings"
	"testing"
	"time"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

func spec() bridge.ServiceSpec {
	return bridge.ServiceSpec{
		Label:            "com.mcp-remote-bridge.sn",
		Program:          "/usr/local/bin/mcp-remote-bridge",
		Args:             []string{"__launch", "sn", "--port", "8080"},
		StdoutPath:       "/home/g/.local/state/mcp-remote-bridge/sn.log",
		StderrPath:       "/home/g/.local/state/mcp-remote-bridge/sn.log",
		KeepAlive:        bridge.KeepAlivePolicy{OnFailure: true, OnCrash: true},
		ThrottleInterval: 60 * time.Second,
	}
}

func build(t *testing.T, s bridge.ServiceSpec) string {
	t.Helper()
	raw, err := BuildUnit(s)
	if err != nil {
		t.Fatalf("BuildUnit: %v", err)
	}
	return string(raw)
}

// The unit file is world-readable, exactly as a plist is. This is the same
// invariant `TestBuildPlistCarriesExactlyTheExpectedKeys` guards on macOS, and
// it is the one a new backend is most likely to break by reaching for the
// convenient thing systemd offers.
func TestTheUnitCarriesNoEnvironment(t *testing.T) {
	out := build(t, spec())

	for _, forbidden := range []string{"Environment=", "EnvironmentFile=", "SetCredential="} {
		if strings.Contains(out, forbidden) {
			t.Errorf("the unit contains %q — secrets are resolved at launch, not written here:\n%s",
				forbidden, out)
		}
	}
}

func TestExecStartCarriesTheProgramAndItsArguments(t *testing.T) {
	out := build(t, spec())

	want := "ExecStart=/usr/local/bin/mcp-remote-bridge __launch sn --port 8080"
	if !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
}

// systemd splits ExecStart itself. An unquoted path containing a space becomes
// two arguments and the service starts with the WRONG command rather than
// failing — a silent substitution, which is worse than a crash.
func TestAnArgumentWithASpaceIsQuoted(t *testing.T) {
	s := spec()
	s.Program = "/opt/my tools/mcp-remote-bridge"
	s.Args = []string{"__launch", "an entry"}

	out := build(t, s)

	if !strings.Contains(out, `"/opt/my tools/mcp-remote-bridge"`) {
		t.Errorf("the program path was not quoted:\n%s", out)
	}
	if !strings.Contains(out, `"an entry"`) {
		t.Errorf("the argument was not quoted:\n%s", out)
	}
}

// A literal % is a specifier to systemd. Left alone, %h in a log path expands
// to the home directory of whoever the unit runs as — writing somewhere the
// caller never named.
func TestAPercentIsNotReadAsASpecifier(t *testing.T) {
	s := spec()
	s.StdoutPath = "/var/log/100%-full.log"

	out := build(t, s)

	if !strings.Contains(out, "100%%-full.log") {
		t.Errorf("a literal %% was left as a specifier:\n%s", out)
	}
}

// The label comes from user config. A newline in a directive value ends that
// directive, and everything after it is read as a new one.
func TestANewlineCannotInjectADirective(t *testing.T) {
	s := spec()
	s.Label = "evil\nExecStartPre=/bin/rm -rf /"

	out := build(t, s)

	// The property is that the text stays on ONE line, not that the string
	// never appears: as the value of Description= it is inert text. Asserting
	// its absence would have failed on safe output — which it did, first time
	// round.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "ExecStartPre") {
			t.Errorf("a newline in the label started a new directive:\n%s", out)
		}
	}
	if !strings.Contains(out, "Description=evil ExecStartPre=") {
		t.Errorf("the newline was not neutralised into the description:\n%s", out)
	}
}

// The policy is what asks for supervision. A service must never be restarted
// forever because the renderer defaulted to it.
func TestRestartFollowsThePolicyAndNothingElse(t *testing.T) {
	cases := []struct {
		name   string
		policy bridge.KeepAlivePolicy
		want   string
	}{
		{"both", bridge.KeepAlivePolicy{OnFailure: true, OnCrash: true}, "Restart=always"},
		{"on failure only", bridge.KeepAlivePolicy{OnFailure: true}, "Restart=on-failure"},
		{"on crash only", bridge.KeepAlivePolicy{OnCrash: true}, "Restart=on-abnormal"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := spec()
			s.KeepAlive = tt.policy
			if out := build(t, s); !strings.Contains(out, tt.want) {
				t.Errorf("want %q in:\n%s", tt.want, out)
			}
		})
	}

	t.Run("a zero policy asks for no supervision at all", func(t *testing.T) {
		s := spec()
		s.KeepAlive = bridge.KeepAlivePolicy{}
		if out := build(t, s); strings.Contains(out, "Restart=") {
			t.Errorf("a service was supervised without being asked:\n%s", out)
		}
	})
}

// Measured on launchd 2026-08-21: a sub-second throttle rendered as 0 and
// disabled throttling entirely. systemd's RestartSec has the same integer
// truncation, so it gets the same refusal rather than a silent round-down.
func TestASubSecondThrottleIsRefusedRatherThanRounded(t *testing.T) {
	s := spec()
	s.ThrottleInterval = 500 * time.Millisecond

	if _, err := BuildUnit(s); err == nil {
		t.Fatal("a sub-second throttle was accepted; it would render as 0 and disable throttling")
	}
}

func TestASpecThatCannotBeRenderedHonestlyIsRefused(t *testing.T) {
	cases := map[string]func(*bridge.ServiceSpec){
		"no label":         func(s *bridge.ServiceSpec) { s.Label = "" },
		"no program":       func(s *bridge.ServiceSpec) { s.Program = "" },
		"relative program": func(s *bridge.ServiceSpec) { s.Program = "mcp-remote-bridge" },
	}
	for name, break_ := range cases {
		t.Run(name, func(t *testing.T) {
			s := spec()
			break_(&s)
			if _, err := BuildUnit(s); err == nil {
				t.Errorf("a spec with %s was rendered instead of refused", name)
			}
		})
	}
}

func TestTheUnitIsEnabledForADefaultTarget(t *testing.T) {
	out := build(t, spec())

	for _, want := range []string{"[Install]", "WantedBy=default.target", "After=network-online.target"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}
