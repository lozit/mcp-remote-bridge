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
		ThrottleInterval: DefaultThrottleInterval,
	}
	if err := b.Services.Ensure(spec.Label, spec); err != nil {
		return report, fmt.Errorf("ensuring the service for %q: %w", entry.Name, err)
	}

	if b.Exposer != nil {
		if err := b.Exposer.Ensure(entry.Subdomain, entry.Domain, port); err != nil {
			return report, fmt.Errorf("exposing %s: %w", entry.Hostname(), err)
		}
	}

	return b.Probe(entry), nil
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
	report.Checks = append(report.Checks,
		ProbeMCPResponds(ctx, &http.Client{Timeout: MCPProbeTimeout}, Endpoint(port), nil))

	return report
}

// DefaultThrottleInterval bounds how fast a repeatedly-failing service retries.
//
// Generous on purpose: the failure it guards is unrecoverable (a secret deleted
// after apply makes the launcher exit before starting the proxy), and a slow,
// visible loop is diagnosable where a spin is noise.
const DefaultThrottleInterval = 60 * time.Second
