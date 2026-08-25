<!-- generated-by: groundrules v1.10.0 -->
# Changelog

All notable changes to this project are documented in this file.

Format inspired by [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- `--version`, stamped from `git describe` at build time. A plain `go build` still reports
  `dev`, since an unstamped binary is not a release and should not claim a number.
- `make release` — build both darwin architectures, sign, notarise, checksum. Hand-rolled
  rather than GoReleaser ([ADR 0009](docs/decisions/0009-release-is-hand-rolled-and-darwin-only.md)).
  It refuses a dirty tree and refuses to build without a Developer ID.
- A compile-time guard against non-darwin builds. The tool compiled cleanly for linux and
  could never run there — it drives `launchctl` and `/usr/bin/security` — so the failure used
  to surface as `exec: not found` at the user's first `apply`.
- `ProbeHostnameResolves` is now bounded by `DNSLookupTimeout` (5s). It was the only probe
  inheriting whatever the system resolver decided, against the house rule that a probe which can
  hang is a probe that never reports. A deadline is reported as such rather than as a generic
  lookup failure, since a timeout and an NXDOMAIN point at different things.
- A cross-cutting test for the secret path: every artefact the tool produces is built and the
  secret value must appear in exactly one of them, the process environment. The per-invariant
  tests can all pass while a *new* artefact leaks — that is the surface this covers.
- Cobra for the CLI (ADR 0008) — per-command `--help` and shell completion, neither worth
  hand-writing. Behaviour and exit codes are unchanged, and `__launch` is still dispatched before
  Cobra so a usage change cannot break an installed service.
- `logs <name>` and `restart <name>`. `restart` bounces the service only — the hostname, ingress
  rule and DNS record are untouched, so it never risks the published name. `logs` always prints
  the path, since "nothing to show" and "wrong place" are different problems.
- `doctor` — checks the preconditions and changes nothing. It reports a credential's *presence*,
  never its validity, so running it cannot itself alter state or trip a rate limit; and every
  failing check carries a hint, since a red line with no next step is a status, not a diagnosis.
- `RetryCheck` and `ProbeHostnameResolves`, and `apply` now waits for a freshly published
  hostname before judging whether it is guarded — measured, the edge takes about two minutes,
  and judging immediately reported "could not confirm" on every new entry. `status` does not
  wait: it stays a fast read.
- `apply`, `status` and `remove` — the CLI. Load, loop, report, with the documented exit codes
  (`0` all healthy, `1` a precondition failed, `2` at least one entry unhealthy). The report
  prints only the checks that actually ran.
- `[infra] access_policy_id` — the Exposer now puts a Cloudflare Access application in front of
  each hostname it publishes, attaching an **existing** policy rather than authoring one
  (ADR 0007). It guards before publishing and unguards last, so a hostname is never reachable
  while unguarded.
- ADR 0007 — the tool takes ownership of the Access configuration it publishes: service token and
  Access application, so that ADR 0001's refusal becomes coherent (it refuses an open hostname
  because it can close it, not instead of closing it). The Portal's own MCP server configuration
  stays manual while that API route is closed to tokens.
- The access-policy check of ADR 0001 — `apply` refuses an entry whose hostname answers an
  unauthenticated MCP handshake, unless `allow_public = true`. Anything ambiguous warns rather
  than blocking, so a broken tunnel never masquerades as a security failure.
- Cloudflare Access service-token support — `[infra] access_client_id` / `access_client_secret`,
  used by a new `hostname_responds` probe that drives a full MCP session through the **public**
  hostname. Without it, a hostname behind an Access policy would report red forever, punishing
  the setup that did the right thing.
- `CloudflaredExposer` — adds and removes an ingress entry plus a proxied `CNAME` through the
  Cloudflare API, verified against the real API on a throwaway hostname. The read-modify-write preserves everything it does not own, keeps the
  catch-all last, and refuses to delete a DNS record pointing anywhere other than this tunnel.
- `set-secret keychain:<service>` — stores a secret from a masked prompt. The value never
  touches an `argv`, an environment variable, or the terminal: it reaches the keychain through
  `security`'s stdin.
- `EnsureExposed` / `RemoveExposed` / `Probe` — the primitive, assembling the seams. Verified by
  a walking-skeleton test running against real launchd, real mcp-proxy and a real keychain.
- `AutoPort` — a stable port derived from the entry name, so a re-apply does not rewrite the
  service definition and restart the MCP.
- `LaunchdManager` — `Ensure` / `Remove` / `Status` over `launchctl`, reconciling rather than
  creating: a no-op when the definition is unchanged, a repair when it changed or the service
  drifted out of the loaded state, and an idempotent `Remove`.
- `BuildPlist` — renders a `bridge.ServiceSpec` as a launchd plist carrying exactly seven
  keys and no environment section, verified by having launchd itself load the generated
  document. Refuses a spec it cannot render honestly, including a `ThrottleInterval` under 1s
  (launchd takes whole seconds, so it would render as `0` and disable throttling).
- `__launch` — the hidden subcommand launchd execs. Resolves secrets, builds a minimal explicit
  environment (`PATH`, `HOME`, declared `env`, resolved secrets — nothing inherited), and
  `syscall.Exec`s mcp-proxy with `--pass-environment`.
- `[infra] keychain` — optionally resolve secrets from a dedicated keychain rather than the
  default search list.
- Config file support — strict TOML at `$XDG_CONFIG_HOME/mcp-remote-bridge/config.toml`, with
  unknown keys rejected, all problems reported at once, secret references validated (a pasted
  value is refused, and the error does not echo it), and subdomain/port collisions caught.
- ADR 0005 — TOML library `pelletier/go-toml/v2`, and the project's dependency policy.
- `KeychainSecretSource` — resolves `keychain:<service>` references via `security -g`, with a
  round-trip test over every byte class (accents, tabs, newlines, PEM keys, emoji) and a named
  error for a missing secret.
- ADR 0004 — read keychain secrets with `security -g`, never `-w`.
- `ProbeMCPResponds` — the deep liveness probe, with an integration harness that runs a real
  mcp-proxy and requires the probe to go red when the MCP is killed.
- ADR 0003 — the liveness probe must carry data back from the MCP. Measured against mcp-proxy
  0.12.0: both `initialize` and `ping` are answered by the proxy, so neither detects a dead MCP;
  a dead MCP also returns HTTP 200, so the verdict must be read from the JSON-RPC body.
- ADR 0002 + `docs/SPEC-launcher.md` — the launcher is a hidden `__launch` subcommand of the
  binary, not a generated shell script; the environment handed to the MCP is constructed
  explicitly rather than inherited.
- `ValidateName` / `ValidateSubdomain` — strict charset validation for an entry name and
  subdomain (length 1..63, `a-z0-9-`, no leading/trailing hyphen). Invalid input is rejected,
  never sanitized: the name becomes a service label, a hostname component and a log path.
- `ProbeProxyListening` — the shallow health check, dialling `127.0.0.1:<port>` only.
- Maker/verifier loop scaffolding (`loop/`) and the project invariants in `CLAUDE.md`.
- Go module skeleton: `Entry`, `HealthReport` and the three seams (`ServiceManager`,
  `Exposer`, `SecretSource`) in `internal/bridge`, with stub implementations in
  `internal/{launchd,cloudflared,keychain}`. No behaviour yet.
- ADR 0001 — `doctor` must flag an exposed hostname with no access policy in front of it;
  promoted to a Milestone 2 requirement.
- Project bootstrapped with groundrules on 2026-08-20

### Changed
- `LaunchdManager.Remove` now waits for the service to actually go, rather than returning when
  `launchctl bootout` is accepted — the two are up to ~230ms apart.
- **The Exposer targets remotely-managed tunnels via the Cloudflare API**, not
  `cloudflared tunnel route dns` (ADR 0006). `[infra]` loses `tunnel` and gains `account_id`,
  `zone_id`, `tunnel_id` and `api_token`. **Breaking** for any config written against the
  earlier draft.
- `ServiceSpec.KeepAlive` is now a `KeepAlivePolicy` struct — launchd expresses this as a
  dictionary, which a bool could not carry.
- `HealthReport` check `mcp_initialize` renamed to `mcp_responds` — the old name asserted a
  handshake that proves nothing about the MCP.
- `docs/SPEC-primitive.md` corrected: its description of the deep probe was wrong.

### Deprecated

### Removed

### Fixed

### Security

<!--
## [0.1.0] - YYYY-MM-DD

### Added
- ...
-->
