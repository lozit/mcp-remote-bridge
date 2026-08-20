<!-- generated-by: groundrules v1.10.0 -->
# Changelog

All notable changes to this project are documented in this file.

Format inspired by [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
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
