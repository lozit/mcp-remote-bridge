package bridge

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

// LabelPrefix namespaces the service labels this tool owns, so a label it did
// not create is recognisably not ours.
const LabelPrefix = "com.mcp-remote-bridge."

// Label is the service label for an entry.
func Label(name string) string { return LabelPrefix + name }

// LogPath is where an entry's proxy output goes — one file per entry, so two
// MCPs' output never interleaves.
func (b *Bridge) LogPath(name string) string {
	return filepath.Join(b.LogDir, name+".log")
}

// Endpoint is the local URL the proxy serves an entry on.
func Endpoint(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
}

// PublicEndpoint is the URL the entry is reachable at from outside.
func PublicEndpoint(hostname string) string {
	return fmt.Sprintf("https://%s/mcp", hostname)
}

// accessCredentials resolves the entry's Access service token, if it declares
// one.
//
// The secret is fetched at the moment of use and returned to the caller's stack
// only — like every other secret in this codebase, it is never stored on the
// Bridge and never logged.
func (b *Bridge) accessCredentials(entry Entry) (AccessCredentials, error) {
	if entry.AccessClientID == "" || entry.AccessClientSecret == "" {
		return AccessCredentials{}, nil
	}
	if b.Secrets == nil {
		return AccessCredentials{}, fmt.Errorf("entry %q declares an Access service token but no secret source is configured", entry.Name)
	}
	secret, err := b.Secrets.Get(entry.AccessClientSecret)
	if err != nil {
		return AccessCredentials{}, fmt.Errorf("entry %q: resolving the Access client secret from %q: %w",
			entry.Name, entry.AccessClientSecret, err)
	}
	return AccessCredentials{ClientID: entry.AccessClientID, ClientSecret: secret}, nil
}

// EnsureExposed guarantees that entry is reachable, and returns a probed
// HealthReport.
//
// Order matters: the service is ensured before the hostname is exposed, so the
// tunnel never points at something that is not there. Probing comes last and is
// what the verdict is built from — never the fact that the writes succeeded.
//
// It reconciles (rule 1): each seam's Ensure is itself idempotent, so a healthy
// entry passes through untouched and only what drifted is repaired.
func (b *Bridge) EnsureExposed(entry Entry) (HealthReport, error) {
	report := HealthReport{Entry: entry.Name}

	if err := ValidateName(entry.Name); err != nil {
		return report, err
	}
	if err := ValidateSubdomain(entry.Subdomain); err != nil {
		return report, err
	}
	port := ResolvePort(entry)
	if b.ProxyPath == "" {
		return report, fmt.Errorf("no mcp-proxy path resolved: it is a precondition and must be located before applying")
	}

	// Rule 3: resolve every referenced secret NOW, before anything is written or
	// launched. An absent secret must fail here, loudly, rather than produce a
	// proxy that 401s silently. The values are discarded — only the launcher
	// resolves them for real, immediately before exec.
	if err := b.checkSecrets(entry); err != nil {
		return report, err
	}

	spec := ServiceSpec{
		Label:            Label(entry.Name),
		Program:          b.BinaryPath,
		Args:             []string{"__launch", entry.Name, "--config", b.ConfigPath, "--port", fmt.Sprint(port), "--proxy", b.ProxyPath},
		StdoutPath:       b.LogPath(entry.Name),
		StderrPath:       b.LogPath(entry.Name),
		KeepAlive:        KeepAlivePolicy{OnFailure: true, OnCrash: true},
		ThrottleInterval: b.throttleInterval(),
	}
	if err := b.Services.Ensure(spec.Label, spec); err != nil {
		return report, fmt.Errorf("ensuring the service for %q: %w", entry.Name, err)
	}

	if b.Exposer != nil {
		if err := b.Exposer.Ensure(entry.Subdomain, entry.Domain, port); err != nil {
			return report, fmt.Errorf("exposing %s: %w", entry.Hostname(), err)
		}
	}

	if err := b.enforceAccessPolicy(entry); err != nil {
		return report, err
	}

	return b.Probe(entry), nil
}

// enforceAccessPolicy refuses an entry proven to serve anyone.
//
// The asymmetry is the decision (ADR 0001): refuse only on PROOF that the
// endpoint is open, warn on anything ambiguous. Refusing on ambiguity would
// block on a broken tunnel or an unpropagated DNS record, turning a security
// control into an availability problem — which is how security controls end up
// switched off.
//
// It runs after the hostname exists, because there is nothing to probe before
// that. An entry that turns out to be open is refused here, having been
// created: the caller is told to guard it or to say allow_public, and `remove`
// is the way back.
func (b *Bridge) enforceAccessPolicy(entry Entry) error {
	if b.Exposer == nil || entry.Subdomain == "" || entry.Domain == "" {
		return nil
	}
	if entry.AllowPublic {
		return nil
	}

	verdict, why := b.settleAccessPolicy(entry.Hostname())
	switch verdict {
	case PolicyOpen:
		return fmt.Errorf(
			"%s is reachable without authentication: %s. Put an Access policy in front of the "+
				"HOSTNAME (an MCP Portal application does not guard the tunnel hostname), or set "+
				"allow_public = true on this entry if that is intended", entry.Hostname(), why)
	case PolicyUnknown:
		// A warning, not a failure: this is exactly where a broken tunnel and a
		// guarded one look alike.
		b.warnf("could not confirm that %s is guarded: %s", entry.Hostname(), why)
	}
	return nil
}

// settleAccessPolicy waits for a freshly published hostname before judging it.
//
// Measured 2026-08-21: a new hostname is not served by the edge for about two
// minutes after it is published — the TCP connect fails outright, before TLS.
// Judged immediately, every fresh entry reports "could not confirm that it is
// guarded", which is true and useless.
//
// PolicyUnknown is exactly the retryable verdict: it means "no answer, or an
// answer that proves nothing". Guarded and Open are both conclusions, so the
// wait ends as soon as either appears.
//
// The wait lives HERE and not in Probe on purpose: `status` must stay a fast,
// side-effect-free read, and it is `apply` that just changed the world and
// therefore owes it time to settle.
func (b *Bridge) settleAccessPolicy(hostname string) (PolicyVerdict, string) {
	var verdict PolicyVerdict
	var why string

	probe := func() Check {
		ctx, cancel := context.WithTimeout(context.Background(), MCPProbeTimeout)
		defer cancel()
		verdict, why = CheckAccessPolicy(ctx, b.httpClient(), hostname)
		return Check{Name: CheckHostnameResponds, Detail: hostname, OK: verdict != PolicyUnknown}
	}

	RetryCheck(probe, HostnameSettleTimeout, HostnameSettleInterval, b.sleep)
	return verdict, why
}

// httpClient is the client used for hostname probes.
func (b *Bridge) httpClient() *http.Client {
	if b.HTTPClientForTest != nil {
		return b.HTTPClientForTest
	}
	return &http.Client{Timeout: MCPProbeTimeout}
}

// throttleInterval is the restart throttle written into service definitions.
func (b *Bridge) throttleInterval() time.Duration {
	if b.ThrottleInterval > 0 {
		return b.ThrottleInterval
	}
	return DefaultThrottleInterval
}

// sleep is the Bridge's wait, defaulting to the real clock.
func (b *Bridge) sleep(d time.Duration) {
	if b.Sleep != nil {
		b.Sleep(d)
		return
	}
	time.Sleep(d)
}

// warnf reports something the caller should see but that does not stop the run.
func (b *Bridge) warnf(format string, args ...any) {
	if b.Warn != nil {
		b.Warn(fmt.Sprintf(format, args...))
	}
}

// RemoveExposed tears the entry down, and returns a probed HealthReport
// confirming it.
//
// It takes the whole entry rather than a name, because removing the hostname
// needs the subdomain and domain, and the primitive must not read the config
// itself — that is the CLI's job.
//
// Reverse order: the hostname stops answering before the service goes away, so
// there is no window where the tunnel points at a dead port.
func (b *Bridge) RemoveExposed(entry Entry) (HealthReport, error) {
	if b.Exposer != nil {
		if err := b.Exposer.Remove(entry.Subdomain, entry.Domain); err != nil {
			return HealthReport{Entry: entry.Name}, fmt.Errorf("removing %s: %w", entry.Hostname(), err)
		}
	}
	if err := b.Services.Remove(Label(entry.Name)); err != nil {
		return HealthReport{Entry: entry.Name}, fmt.Errorf("removing the service for %q: %w", entry.Name, err)
	}
	return b.Probe(entry), nil
}

// checkSecrets verifies every referenced secret resolves, discarding the values.
//
// The error names the variable and the reference, never the value.
func (b *Bridge) checkSecrets(entry Entry) error {
	if len(entry.Secrets) == 0 {
		return nil
	}
	if b.Secrets == nil {
		return fmt.Errorf("entry %q references secrets but no secret source is configured", entry.Name)
	}
	for name, ref := range entry.Secrets {
		if _, err := b.Secrets.Get(ref); err != nil {
			return fmt.Errorf("entry %q: resolving %s from %q: %w", entry.Name, name, ref, err)
		}
	}
	return nil
}

// Probe runs the health checks for entry without changing anything.
//
// The report carries only the checks that actually ran. A probe that is not
// implemented yet is absent from the report rather than reported as passing:
// an empty report is not healthy, and a partial one claims only what it proved.
func (b *Bridge) Probe(entry Entry) HealthReport {
	report := HealthReport{Entry: entry.Name}
	port := ResolvePort(entry)

	if b.Services != nil {
		check := Check{Name: CheckServiceLoaded, Detail: Label(entry.Name)}
		state, err := b.Services.Status(Label(entry.Name))
		switch {
		case err != nil:
			check.Err = err
		case !state.Loaded:
			check.Err = fmt.Errorf("the service is not loaded")
		case !state.Running:
			check.Err = fmt.Errorf("the service is loaded but not running (last exit code %d)", state.LastExitCode)
		default:
			check.OK = true
			check.Detail = fmt.Sprintf("%s (pid %d)", check.Detail, state.PID)
		}
		report.Checks = append(report.Checks, check)
	}

	report.Checks = append(report.Checks, ProbeProxyListening(port))

	ctx, cancel := context.WithTimeout(context.Background(), MCPProbeTimeout)
	defer cancel()
	client := &http.Client{Timeout: MCPProbeTimeout}

	// Loopback needs no credentials: 127.0.0.1 does not pass through Access.
	report.Checks = append(report.Checks, ProbeMCPResponds(ctx, client, Endpoint(port), nil))

	// The public hostname is probed only when this Bridge actually exposes one.
	// Without an Exposer nothing was published, so the hostname is not expected
	// to answer, and a red check would report a failure that is really an
	// absence — the report must carry facts it established, not facts it
	// assumed.
	if b.Exposer != nil && entry.Subdomain != "" && entry.Domain != "" {
		report.Checks = append(report.Checks, b.probeHostname(ctx, client, entry))
	}

	return report
}

// probeHostname runs the deep probe through the public hostname, authenticating
// to Cloudflare Access when the entry declares a service token.
//
// This is the check that answers the question the local probe cannot: is the
// MCP reachable from where the remote agent actually is?
func (b *Bridge) probeHostname(ctx context.Context, client *http.Client, entry Entry) Check {
	hostname := entry.Hostname()
	check := Check{Name: CheckHostnameResponds, Detail: hostname}

	creds, err := b.accessCredentials(entry)
	if err != nil {
		check.Err = err
		return check
	}

	got := ProbeMCPResponds(ctx, client, PublicEndpoint(hostname), creds.Decorate())
	check.OK = got.OK
	check.Err = got.Err
	check.Detail = got.Detail
	if !got.OK && !creds.Configured() {
		// The likeliest cause of a red here, and the one whose error message
		// would otherwise send someone debugging a healthy MCP.
		check.Err = fmt.Errorf("%w — if this hostname is behind a Cloudflare Access policy, "+
			"set access_client_id and access_client_secret so the probe can authenticate", got.Err)
	}
	return check
}

// DefaultThrottleInterval bounds how fast a repeatedly-failing service retries.
//
// Generous on purpose: the failure it guards is unrecoverable (a secret deleted
// after apply makes the launcher exit before starting the proxy), and a slow,
// visible loop is diagnosable where a spin is noise.
const DefaultThrottleInterval = 60 * time.Second
