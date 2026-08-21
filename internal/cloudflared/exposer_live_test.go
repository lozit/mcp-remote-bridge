package cloudflared_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/lozit/mcp-remote-bridge/internal/cloudflared"
	"github.com/lozit/mcp-remote-bridge/internal/keychain"
)

// The only test that touches the real Cloudflare API. Everything else in this
// package runs against a local stand-in, which proves the Exposer behaves as we
// believe the API behaves — the failure mode this project has hit four times.
//
// Opt-in: it needs MRB_LIVE=1 plus the account/zone/tunnel ids in the
// environment (they are identifiers, not secrets) and the API token in the
// keychain (it is).
//
//	MRB_LIVE=1 MRB_ACCOUNT_ID=… MRB_ZONE_ID=… MRB_TUNNEL_ID=… MRB_DOMAIN=… \
//	  go test ./internal/cloudflared/ -run TestLive -v
//
// It uses a throwaway hostname and asserts that the tunnel configuration is
// byte-for-byte what it was before, afterwards.
func TestLiveExposerRoundTrip(t *testing.T) {
	if os.Getenv("MRB_LIVE") != "1" {
		t.Skip("live test: set MRB_LIVE=1 to run it against the real Cloudflare API")
	}
	accountID := mustEnv(t, "MRB_ACCOUNT_ID")
	zoneID := mustEnv(t, "MRB_ZONE_ID")
	tunnelID := mustEnv(t, "MRB_TUNNEL_ID")
	domain := mustEnv(t, "MRB_DOMAIN")

	token, err := keychain.New("").Get("keychain:cf-api-token")
	if err != nil {
		t.Fatalf("reading the API token from the keychain: %v", err)
	}

	e := cloudflared.New(accountID, zoneID, tunnelID, token)

	// A name that cannot be mistaken for something in service.
	subdomain := fmt.Sprintf("mrb-throwaway-%d", time.Now().UnixNano()%1e6)
	hostname := subdomain + "." + domain
	const port = 29999

	before := snapshot(t, e, accountID, tunnelID, token)
	t.Logf("ingress before: %d rules", len(before))
	if hostnameIn(before, hostname) {
		t.Fatalf("refusing to run: %s already exists in the tunnel configuration", hostname)
	}

	// Tear down even if an assertion fails, so a failed run does not leave the
	// user's tunnel carrying a stray hostname.
	t.Cleanup(func() {
		if err := e.Remove(subdomain, domain); err != nil {
			t.Errorf("CLEANUP FAILED — %s may still be in your tunnel: %v", hostname, err)
		}
	})

	if err := e.Ensure(subdomain, domain, port); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	during := snapshot(t, e, accountID, tunnelID, token)
	if !hostnameIn(during, hostname) {
		t.Fatalf("Ensure reported success but %s is not in the ingress list", hostname)
	}
	if len(during) != len(before)+1 {
		t.Errorf("ingress went from %d to %d rules, want exactly one more", len(before), len(during))
	}
	// The production entries must be untouched, field for field.
	for _, rule := range before {
		if h, ok := rule["hostname"].(string); ok && !ruleIn(during, rule) {
			t.Errorf("the existing entry for %s was altered:\n before %v", h, rule)
		}
	}
	if last := during[len(during)-1]; hasHostname(last) {
		t.Errorf("the catch-all is no longer last: %v", last)
	}

	// Rule 1 against the real API: a second Ensure must change nothing.
	if err := e.Ensure(subdomain, domain, port); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	again := snapshot(t, e, accountID, tunnelID, token)
	if !reflect.DeepEqual(during, again) {
		t.Errorf("a repeated Ensure changed the configuration:\n before %v\n after  %v", during, again)
	}

	if err := e.Remove(subdomain, domain); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	after := snapshot(t, e, accountID, tunnelID, token)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("the tunnel configuration did not return to its original state.\n before %v\n after  %v", before, after)
	} else {
		t.Log("configuration restored exactly; production entries untouched throughout")
	}
}

func mustEnv(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Skipf("live test: %s is not set", name)
	}
	return v
}

// snapshot reads the ingress list straight from the API, independently of the
// Exposer's own code paths — a check that reused them could not catch a bug in
// them.
func snapshot(t *testing.T, e *cloudflared.Exposer, accountID, tunnelID, token string) []map[string]any {
	t.Helper()
	url := fmt.Sprintf("%s/accounts/%s/cfd_tunnel/%s/configurations",
		cloudflared.DefaultBaseURL, accountID, tunnelID)
	req, err := newAuthedRequest(url, token)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("reading the tunnel configuration: %v", err)
	}
	defer resp.Body.Close()

	var got struct {
		Success bool `json:"success"`
		Result  struct {
			Config struct {
				Ingress []map[string]any `json:"ingress"`
			} `json:"config"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding the tunnel configuration: %v", err)
	}
	if !got.Success {
		t.Fatalf("the API refused the configuration read (HTTP %d)", resp.StatusCode)
	}
	return got.Result.Config.Ingress
}

func hostnameIn(rules []map[string]any, hostname string) bool {
	for _, r := range rules {
		if r["hostname"] == hostname {
			return true
		}
	}
	return false
}

func ruleIn(rules []map[string]any, want map[string]any) bool {
	for _, r := range rules {
		if reflect.DeepEqual(r, want) {
			return true
		}
	}
	return false
}

func hasHostname(rule map[string]any) bool {
	_, ok := rule["hostname"]
	return ok
}
