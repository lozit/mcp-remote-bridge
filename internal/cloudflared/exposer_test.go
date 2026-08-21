package cloudflared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The fixture is a real tunnel configuration, captured 2026-08-21. It is not a
// tidy invention: the entries are deliberately NOT uniform (one carries
// originRequest, one does not), warp-routing sits alongside ingress, and the
// catch-all has no hostname. Those three details are what a read-modify-write
// gets wrong, so the tests are written against the real shape.
const realConfig = `{
  "success": true, "errors": [],
  "result": {
    "tunnel_id": "96c48771-9744-4d14-b0fa-16361efcdcf4",
    "version": 2,
    "config": {
      "ingress": [
        {"service": "http://localhost:8080", "hostname": "hermes-mcp.paranoid.foo"},
        {"service": "http://localhost:8081", "hostname": "freestyle-mcp.paranoid.foo", "originRequest": {}},
        {"service": "http_status:404"}
      ],
      "warp-routing": {"enabled": false}
    },
    "source": "cloudflare"
  }
}`

type capture struct {
	puts     []map[string]any
	posts    []map[string]any
	deletes  []string
	dnsEmpty bool
}

// newAPI stands in for the Cloudflare API. It answers the two GETs the Exposer
// makes and records what it is asked to write.
func newAPI(t *testing.T, c *capture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("missing or wrong Authorization header: %q", got)
		}
		if strings.Contains(r.URL.RawQuery, "test-token") || strings.Contains(r.URL.Path, "test-token") {
			t.Errorf("the token leaked into the URL: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/configurations") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(realConfig))

		case strings.Contains(r.URL.Path, "/configurations") && r.Method == http.MethodPut:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			c.puts = append(c.puts, body)
			_, _ = w.Write([]byte(`{"success":true,"errors":[]}`))

		case strings.Contains(r.URL.Path, "/dns_records") && r.Method == http.MethodGet:
			if c.dnsEmpty {
				_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[
				{"id":"rec1","type":"CNAME","name":"skeleton.paranoid.foo",
				 "content":"96c48771-9744-4d14-b0fa-16361efcdcf4.cfargotunnel.com","proxied":true}]}`))

		case strings.Contains(r.URL.Path, "/dns_records") && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			c.posts = append(c.posts, body)
			_, _ = w.Write([]byte(`{"success":true,"errors":[]}`))

		case strings.Contains(r.URL.Path, "/dns_records") && r.Method == http.MethodDelete:
			c.deletes = append(c.deletes, r.URL.Path)
			_, _ = w.Write([]byte(`{"success":true,"errors":[]}`))

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newExposer(t *testing.T, c *capture) *Exposer {
	t.Helper()
	srv := newAPI(t, c)
	t.Cleanup(srv.Close)
	e := New("acc", "zone", "96c48771-9744-4d14-b0fa-16361efcdcf4", "test-token")
	e.BaseURL = srv.URL
	return e
}

func ingressOf(t *testing.T, put map[string]any) []map[string]any {
	t.Helper()
	cfg, ok := put["config"].(map[string]any)
	if !ok {
		t.Fatalf("the PUT body has no config object: %v", put)
	}
	raw, ok := cfg["ingress"].([]any)
	if !ok {
		t.Fatalf("the PUT body has no ingress array: %v", cfg)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(map[string]any))
	}
	return out
}

// The load-bearing test: a PUT replaces the whole configuration, so everything
// that was there must still be there.
func TestEnsurePreservesTheExistingConfiguration(t *testing.T) {
	c := &capture{dnsEmpty: true}
	e := newExposer(t, c)

	if err := e.Ensure("skeleton", "paranoid.foo", 29777); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if len(c.puts) != 1 {
		t.Fatalf("expected exactly one configuration PUT, got %d", len(c.puts))
	}

	cfg := c.puts[0]["config"].(map[string]any)

	// warp-routing lives beside ingress; a PUT that sent only ingress would drop it.
	wr, ok := cfg["warp-routing"].(map[string]any)
	if !ok || wr["enabled"] != false {
		t.Errorf("warp-routing was lost or altered: %v", cfg["warp-routing"])
	}

	ingress := ingressOf(t, c.puts[0])
	if len(ingress) != 4 {
		t.Fatalf("expected 4 ingress rules (2 existing + 1 new + catch-all), got %d: %v", len(ingress), ingress)
	}

	// Both production hostnames survive, with their service AND their extra fields.
	byHost := map[string]map[string]any{}
	for _, r := range ingress {
		if h, ok := r["hostname"].(string); ok {
			byHost[h] = r
		}
	}
	if got := byHost["hermes-mcp.paranoid.foo"]; got == nil || got["service"] != "http://localhost:8080" {
		t.Errorf("the existing hermes entry was lost or rewritten: %v", got)
	}
	fs := byHost["freestyle-mcp.paranoid.foo"]
	if fs == nil || fs["service"] != "http://localhost:8081" {
		t.Errorf("the existing freestyle entry was lost or rewritten: %v", fs)
	}
	if _, kept := fs["originRequest"]; !kept {
		t.Error("originRequest was dropped from the freestyle entry; unknown fields must survive a round trip")
	}
}

// Cloudflare requires the catch-all last. A rule appended after it would never
// be reached, because the catch-all matches everything.
func TestEnsureKeepsTheCatchAllLast(t *testing.T) {
	c := &capture{dnsEmpty: true}
	e := newExposer(t, c)

	if err := e.Ensure("skeleton", "paranoid.foo", 29777); err != nil {
		t.Fatal(err)
	}
	ingress := ingressOf(t, c.puts[0])

	last := ingress[len(ingress)-1]
	if _, hasHost := last["hostname"]; hasHost {
		t.Errorf("the last rule has a hostname, so the catch-all is no longer last: %v", ingress)
	}
	if last["service"] != "http_status:404" {
		t.Errorf("the catch-all was altered: %v", last)
	}
	// And the new rule must be before it.
	newIdx := -1
	for i, r := range ingress {
		if r["hostname"] == "skeleton.paranoid.foo" {
			newIdx = i
		}
	}
	if newIdx < 0 {
		t.Fatal("the new hostname is not in the ingress list")
	}
	if newIdx > len(ingress)-2 {
		t.Errorf("the new rule is at %d, after the catch-all", newIdx)
	}
}

// Rule 1: an entry that is already correct must not be written back at all.
func TestEnsureIsANoOpWhenAlreadyCorrect(t *testing.T) {
	c := &capture{}
	e := newExposer(t, c)

	// hermes is already mapped to 8080 in the fixture.
	if err := e.Ensure("hermes-mcp", "paranoid.foo", 8080); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if len(c.puts) != 0 {
		t.Errorf("an unchanged configuration was written back: %v", c.puts)
	}
}

// A drifted port must be corrected without touching anything else.
func TestEnsureCorrectsADriftedPort(t *testing.T) {
	c := &capture{}
	e := newExposer(t, c)

	if err := e.Ensure("hermes-mcp", "paranoid.foo", 9999); err != nil {
		t.Fatal(err)
	}
	if len(c.puts) != 1 {
		t.Fatalf("expected one PUT, got %d", len(c.puts))
	}
	ingress := ingressOf(t, c.puts[0])
	if len(ingress) != 3 {
		t.Errorf("correcting a port changed the number of rules: %v", ingress)
	}
	for _, r := range ingress {
		if r["hostname"] == "hermes-mcp.paranoid.foo" && r["service"] != "http://localhost:9999" {
			t.Errorf("the port was not corrected: %v", r)
		}
	}
}

func TestEnsureCreatesAProxiedCNAME(t *testing.T) {
	c := &capture{dnsEmpty: true}
	e := newExposer(t, c)

	if err := e.Ensure("skeleton", "paranoid.foo", 29777); err != nil {
		t.Fatal(err)
	}
	if len(c.posts) != 1 {
		t.Fatalf("expected one DNS record creation, got %d", len(c.posts))
	}
	rec := c.posts[0]
	if rec["type"] != "CNAME" {
		t.Errorf("type = %v, want CNAME", rec["type"])
	}
	if rec["content"] != "96c48771-9744-4d14-b0fa-16361efcdcf4.cfargotunnel.com" {
		t.Errorf("content = %v", rec["content"])
	}
	// An unproxied CNAME to cfargotunnel.com does not resolve for clients.
	if rec["proxied"] != true {
		t.Errorf("proxied = %v, want true", rec["proxied"])
	}
}

func TestRemoveDropsOnlyItsOwnRule(t *testing.T) {
	c := &capture{}
	e := newExposer(t, c)

	if err := e.Remove("hermes-mcp", "paranoid.foo"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	ingress := ingressOf(t, c.puts[0])
	if len(ingress) != 2 {
		t.Fatalf("expected 2 rules left, got %d: %v", len(ingress), ingress)
	}
	for _, r := range ingress {
		if r["hostname"] == "hermes-mcp.paranoid.foo" {
			t.Error("the removed hostname is still present")
		}
	}
	if ingress[len(ingress)-1]["service"] != "http_status:404" {
		t.Error("the catch-all is no longer last after a removal")
	}
}

// Removing a hostname that is not ours would take down something we never
// created.
func TestRemoveRefusesAForeignDNSRecord(t *testing.T) {
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/configurations") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(realConfig))
		case strings.Contains(r.URL.Path, "/configurations"):
			_, _ = w.Write([]byte(`{"success":true,"errors":[]}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":[
				{"id":"rec9","type":"CNAME","name":"hermes-mcp.paranoid.foo",
				 "content":"somewhere-else.example.com","proxied":true}]}`))
		default:
			c.deletes = append(c.deletes, r.URL.Path)
			_, _ = w.Write([]byte(`{"success":true,"errors":[]}`))
		}
	}))
	defer srv.Close()

	e := New("acc", "zone", "96c48771-9744-4d14-b0fa-16361efcdcf4", "test-token")
	e.BaseURL = srv.URL

	err := e.Remove("hermes-mcp", "paranoid.foo")

	if err == nil {
		t.Fatal("Remove deleted a DNS record pointing somewhere else")
	}
	if len(c.deletes) != 0 {
		t.Errorf("a foreign DNS record was deleted: %v", c.deletes)
	}
}

// Cloudflare answers 200 with success:false for application-level failures.
func TestCallTreatsSuccessFalseAsAFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"Authentication error"}]}`))
	}))
	defer srv.Close()

	e := New("acc", "zone", "tun", "test-token")
	e.BaseURL = srv.URL

	err := e.Ensure("x", "example.com", 1234)

	if err == nil {
		t.Fatal("an HTTP 200 carrying success:false was treated as a success")
	}
	if !strings.Contains(err.Error(), "Authentication error") {
		t.Errorf("the error should carry the API's message: %v", err)
	}
}

// The token must never appear in a URL: query strings reach server logs and
// every proxy in between.
func TestTokenTravelsOnlyInTheHeader(t *testing.T) {
	c := &capture{dnsEmpty: true}
	e := newExposer(t, c) // the handler asserts this on every request
	if err := e.Ensure("skeleton", "paranoid.foo", 29777); err != nil {
		t.Fatal(err)
	}
}
