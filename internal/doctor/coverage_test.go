package doctor

import (
	"errors"
	"testing"

	"github.com/lozit/mcp-remote-bridge/internal/config"
)

// Every check doctor reports must be able to fail, and say what to do when it
// does. Each one already has its own test; this is the guard against the NEXT
// one — a check added later with no failing path is a line that always prints
// green, and a report of those is a decoration, not a diagnosis.
//
// The table is keyed by check name and the set is compared against what Run
// actually produces, so adding a check without adding a way to break it fails
// here rather than passing silently.

// breakers maps each check to a world in which it must go red.
var breakers = map[string]func(Deps, *config.File) (Deps, *config.File){
	"mcp-proxy": func(d Deps, c *config.File) (Deps, *config.File) {
		d.LookPath = func(n string) (string, error) {
			if n == "mcp-proxy" {
				return "", errors.New("not found")
			}
			return "/usr/local/bin/" + n, nil
		}
		return d, c
	},
	"cloudflared": func(d Deps, c *config.File) (Deps, *config.File) {
		d.LookPath = func(n string) (string, error) {
			if n == "cloudflared" {
				return "", errors.New("not found")
			}
			return "/usr/local/bin/" + n, nil
		}
		return d, c
	},
	"tunnel_connector": func(d Deps, c *config.File) (Deps, *config.File) {
		d.ProcessMatches = func(string) bool { return false }
		return d, c
	},
	"binary_path": func(d Deps, c *config.File) (Deps, *config.File) {
		d.Executable = func() (string, error) { return "", errors.New("cannot determine") }
		return d, c
	},
	"config": func(d Deps, _ *config.File) (Deps, *config.File) {
		return d, nil // an unloadable config
	},
	"cloudflare_api_token": func(d Deps, c *config.File) (Deps, *config.File) {
		d.ResolveSecret = func(string) (string, error) { return "", errors.New("no such secret") }
		return d, c
	},
	"access_service_token": func(d Deps, c *config.File) (Deps, *config.File) {
		d.ResolveSecret = func(string) (string, error) { return "", errors.New("no such secret") }
		return d, c
	},
}

// portal_server is deliberately exempt: it is a reminder of a manual step, not
// a probe of anything, because /access/mcp_servers is closed to API tokens
// while Portals are Beta. It is listed here rather than skipped silently, so
// the exemption is a decision on the record and not an oversight.
var alwaysGreen = map[string]string{
	"portal_server": "a reminder of a manual step the API cannot verify",
}

func TestEveryCheckCanFail(t *testing.T) {
	for _, c := range Run(fullConfig(), healthyDeps()) {
		if reason, exempt := alwaysGreen[c.Name]; exempt {
			t.Logf("%s: exempt — %s", c.Name, reason)
			continue
		}
		breaker, ok := breakers[c.Name]
		if !ok {
			t.Errorf("check %q has no entry in `breakers`: either show how it fails, "+
				"or record it in `alwaysGreen` with the reason", c.Name)
			continue
		}
		deps, cfg := breaker(healthyDeps(), fullConfig())
		if got := find(t, Run(cfg, deps), c.Name); got.OK {
			t.Errorf("check %q stayed green in a world built to break it", c.Name)
		}
	}
}

// A red line with no next step is a status, not a diagnosis.
func TestEveryFailureCarriesAHint(t *testing.T) {
	for name, breaker := range breakers {
		deps, cfg := breaker(healthyDeps(), fullConfig())
		got := find(t, Run(cfg, deps), name)
		if got.OK {
			continue // TestEveryCheckCanFail owns that failure
		}
		if got.Hint == "" {
			t.Errorf("check %q fails without a hint: %v", name, got.Err)
		}
		if got.Err == nil {
			t.Errorf("check %q is red but carries no error to explain it", name)
		}
	}
}

// The table must not outlive the checks: an entry naming a check Run no longer
// produces is a test asserting nothing, and it would keep passing forever.
func TestNoStaleEntriesInTheTable(t *testing.T) {
	live := map[string]bool{}
	for _, c := range Run(fullConfig(), healthyDeps()) {
		live[c.Name] = true
	}
	for name := range breakers {
		if !live[name] {
			t.Errorf("`breakers` names %q, which doctor no longer reports", name)
		}
	}
	for name := range alwaysGreen {
		if !live[name] {
			t.Errorf("`alwaysGreen` names %q, which doctor no longer reports", name)
		}
	}
}
