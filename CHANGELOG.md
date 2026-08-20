<!-- generated-by: groundrules v1.10.0 -->
# Changelog

All notable changes to this project are documented in this file.

Format inspired by [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
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

### Deprecated

### Removed

### Fixed

### Security

<!--
## [0.1.0] - YYYY-MM-DD

### Added
- ...
-->
