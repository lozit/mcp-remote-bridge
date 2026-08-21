// Package cloudflared implements bridge.Exposer against a REMOTELY-MANAGED
// Cloudflare tunnel, through the Cloudflare API.
//
// The tunnel itself is a precondition: this package adds hostnames to a tunnel,
// it never creates one.
//
// It does NOT shell out to `cloudflared tunnel route dns`. That command needs
// ~/.cloudflared/cert.pem, which a token-installed connector does not have; and
// a remotely-managed tunnel keeps its ingress configuration in Cloudflare
// rather than on disk, so there is no local rule to write. See ADR 0006.
package cloudflared

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the Cloudflare API v4 root.
const DefaultBaseURL = "https://api.cloudflare.com/client/v4"

// RequestTimeout bounds a single API call.
const RequestTimeout = 30 * time.Second

// Exposer routes a public hostname to a local port through a Cloudflare tunnel.
type Exposer struct {
	AccountID string
	ZoneID    string
	TunnelID  string

	// APIToken is the resolved token value. It can modify the zone's DNS, so it
	// is never logged and never placed in a URL or a command line — only in an
	// Authorization header.
	APIToken string

	// BaseURL defaults to DefaultBaseURL. Tests point it at a local server.
	BaseURL string

	// HTTPClient defaults to one bounded by RequestTimeout.
	HTTPClient *http.Client
}

// New returns an Exposer bound to a remotely-managed tunnel.
func New(accountID, zoneID, tunnelID, apiToken string) *Exposer {
	return &Exposer{AccountID: accountID, ZoneID: zoneID, TunnelID: tunnelID, APIToken: apiToken}
}

// Ensure adds the ingress entry and the DNS record for subdomain.domain.
//
// Idempotent: an entry that is already correct is left alone, and one that
// points elsewhere is corrected.
func (e *Exposer) Ensure(subdomain, domain string, localPort int) error {
	hostname := subdomain + "." + domain
	service := fmt.Sprintf("http://localhost:%d", localPort)

	if err := e.updateIngress(func(ingress []map[string]any) ([]map[string]any, bool) {
		return upsertIngress(ingress, hostname, service)
	}); err != nil {
		return err
	}
	return e.ensureDNS(hostname)
}

// Remove drops the ingress entry and the DNS record.
//
// Idempotent: an entry that is already gone is the desired state.
func (e *Exposer) Remove(subdomain, domain string) error {
	hostname := subdomain + "." + domain

	if err := e.updateIngress(func(ingress []map[string]any) ([]map[string]any, bool) {
		return deleteIngress(ingress, hostname)
	}); err != nil {
		return err
	}
	return e.removeDNS(hostname)
}

// upsertIngress places hostname -> service before the catch-all, preserving
// every other entry byte for byte.
//
// It reports whether anything changed, so an unchanged configuration is not
// written back at all — rule 1, and one less chance to lose a concurrent edit.
func upsertIngress(ingress []map[string]any, hostname, service string) ([]map[string]any, bool) {
	for i, rule := range ingress {
		if rule["hostname"] == hostname {
			if rule["service"] == service {
				return ingress, false
			}
			// Only the service is corrected: originRequest and any field this
			// build does not know about stay exactly as they were.
			out := cloneRules(ingress)
			out[i]["service"] = service
			return out, true
		}
	}

	out := cloneRules(ingress)
	entry := map[string]any{"hostname": hostname, "service": service}

	// The catch-all is the entry with no hostname, and Cloudflare requires it to
	// be last. Insert before it rather than appending, or the new rule would sit
	// after a rule that matches everything and never be reached.
	if i := catchAllIndex(out); i >= 0 {
		out = append(out[:i], append([]map[string]any{entry}, out[i:]...)...)
		return out, true
	}
	return append(out, entry), true
}

func deleteIngress(ingress []map[string]any, hostname string) ([]map[string]any, bool) {
	out := make([]map[string]any, 0, len(ingress))
	changed := false
	for _, rule := range ingress {
		if rule["hostname"] == hostname {
			changed = true
			continue
		}
		out = append(out, cloneRule(rule))
	}
	return out, changed
}

// catchAllIndex finds the first rule with no hostname.
func catchAllIndex(ingress []map[string]any) int {
	for i, rule := range ingress {
		if _, has := rule["hostname"]; !has {
			return i
		}
	}
	return -1
}

func cloneRules(in []map[string]any) []map[string]any {
	out := make([]map[string]any, len(in))
	for i, r := range in {
		out[i] = cloneRule(r)
	}
	return out
}

func cloneRule(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// updateIngress performs the read-modify-write on the tunnel configuration.
//
// The PUT replaces the WHOLE configuration, so the read is deserialised as
// loosely as possible: only `ingress` is understood, and the surrounding object
// — `warp-routing` and anything Cloudflare adds later — is carried across
// untouched. Parsing it into a typed struct would silently drop what that
// struct does not know about.
//
// The read happens immediately before the write to keep the lost-update window
// as small as this API allows; see ADR 0006.
func (e *Exposer) updateIngress(mutate func([]map[string]any) ([]map[string]any, bool)) error {
	var got struct {
		Result struct {
			Version int            `json:"version"`
			Config  map[string]any `json:"config"`
		} `json:"result"`
	}
	if err := e.call(http.MethodGet, e.tunnelConfigPath(), nil, &got); err != nil {
		return fmt.Errorf("reading the tunnel configuration: %w", err)
	}

	config := got.Result.Config
	if config == nil {
		return fmt.Errorf("the tunnel returned no configuration; it may not be remotely-managed")
	}

	rawIngress, _ := config["ingress"].([]any)
	ingress := make([]map[string]any, 0, len(rawIngress))
	for _, r := range rawIngress {
		rule, ok := r.(map[string]any)
		if !ok {
			return fmt.Errorf("the tunnel configuration has an ingress entry of an unexpected shape")
		}
		ingress = append(ingress, rule)
	}

	updated, changed := mutate(ingress)
	if !changed {
		return nil
	}
	config["ingress"] = updated

	body := map[string]any{"config": config}
	if err := e.call(http.MethodPut, e.tunnelConfigPath(), body, nil); err != nil {
		return fmt.Errorf("writing the tunnel configuration: %w", err)
	}
	return nil
}

func (e *Exposer) tunnelConfigPath() string {
	return fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/configurations", e.AccountID, e.TunnelID)
}

// dnsTarget is the CNAME every hostname on this tunnel points at.
func (e *Exposer) dnsTarget() string { return e.TunnelID + ".cfargotunnel.com" }

type dnsRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

// ensureDNS creates or corrects the proxied CNAME for hostname.
func (e *Exposer) ensureDNS(hostname string) error {
	existing, err := e.findDNS(hostname)
	if err != nil {
		return err
	}
	want := dnsRecord{Type: "CNAME", Name: hostname, Content: e.dnsTarget(), Proxied: true}

	switch {
	case existing == nil:
		if err := e.call(http.MethodPost, "/zones/"+e.ZoneID+"/dns_records", want, nil); err != nil {
			return fmt.Errorf("creating the DNS record for %s: %w", hostname, err)
		}
	case existing.Content != want.Content || existing.Type != want.Type || !existing.Proxied:
		// Proxied matters: an unproxied CNAME to cfargotunnel.com does not
		// resolve for clients, so a record that drifted must be corrected.
		if err := e.call(http.MethodPut, "/zones/"+e.ZoneID+"/dns_records/"+existing.ID, want, nil); err != nil {
			return fmt.Errorf("correcting the DNS record for %s: %w", hostname, err)
		}
	}
	return nil
}

func (e *Exposer) removeDNS(hostname string) error {
	existing, err := e.findDNS(hostname)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil // already the desired state
	}
	// Refuse to delete a record that is not ours: a hostname pointing somewhere
	// else was not created by this tool, and removing it would take down
	// something we never put up.
	if existing.Content != e.dnsTarget() {
		return fmt.Errorf("refusing to delete the DNS record for %s: it points at %q, not at this tunnel",
			hostname, existing.Content)
	}
	if err := e.call(http.MethodDelete, "/zones/"+e.ZoneID+"/dns_records/"+existing.ID, nil, nil); err != nil {
		return fmt.Errorf("deleting the DNS record for %s: %w", hostname, err)
	}
	return nil
}

func (e *Exposer) findDNS(hostname string) (*dnsRecord, error) {
	var got struct {
		Result []dnsRecord `json:"result"`
	}
	path := fmt.Sprintf("/zones/%s/dns_records?name=%s", e.ZoneID, hostname)
	if err := e.call(http.MethodGet, path, nil, &got); err != nil {
		return nil, fmt.Errorf("looking up the DNS record for %s: %w", hostname, err)
	}
	for i := range got.Result {
		if got.Result[i].Name == hostname {
			return &got.Result[i], nil
		}
	}
	return nil, nil
}

// apiError is Cloudflare's error shape. A 200 with success:false is a failure.
type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// call performs one API request.
//
// The token travels in the Authorization header only — never a query parameter,
// which would land in server logs and in any proxy along the way.
func (e *Exposer) call(method, path string, body, out any) error {
	base := e.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	client := e.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: RequestTimeout}
	}

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}

	// Cloudflare answers 200 with success:false for application-level failures,
	// so the status code alone is not the verdict — the same trap as the MCP
	// probe (ADR 0003).
	var envelope struct {
		Success bool       `json:"success"`
		Errors  []apiError `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("HTTP %d, unreadable response", resp.StatusCode)
	}
	if !envelope.Success {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, describeErrors(envelope.Errors))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("unreadable response body: %w", err)
		}
	}
	return nil
}

func describeErrors(errs []apiError) string {
	if len(errs) == 0 {
		return "the API reported a failure with no error detail"
	}
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, fmt.Sprintf("%s (code %d)", e.Message, e.Code))
	}
	return strings.Join(parts, "; ")
}
