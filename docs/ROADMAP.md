<!-- generated-by: groundrules v1.10.0 -->
# Roadmap — mcp-remote-bridge

**Long-term** breakdown into deliverable milestones / increments.

> Distinct from `PLAN.md` (the **active** todo right now): the roadmap describes the
> trajectory, not the current task. Structural decisions go in `docs/decisions/`.

## Condensed vision

Turn a 489-line manual procedure into one verified command — and keep it plumbing, never a
gateway.

## Milestones

### Milestone 1 — The primitive

- **Goal**: `ensure_exposed` / `remove_exposed` working end to end for one hardcoded entry,
  behind the three seams.
- **Scope**: `LaunchdManager`, `CloudflaredExposer`, `KeychainSecretSource`, the generated
  launcher, `HealthReport` including the `initialize` deep probe.
- **Exit criteria**: a real local stdio MCP is reachable at a hostname; re-running is a
  no-op; killing the service and re-running repairs it; `remove` makes the hostname stop
  answering. No secret value in the plist, in `argv`, or in a log.
- **Status**: Upcoming
- **Spec**: [`SPEC-primitive.md`](SPEC-primitive.md)

### Milestone 2 — Config + CLI (the MVP)

- **Goal**: many entries, declaratively, reconciled by one command.
- **Scope**: TOML config at `$XDG_CONFIG_HOME/mcp-remote-bridge/config.toml`; Cobra
  commands `apply` / `remove` / `status` / `logs` / `restart` / `doctor` / `set-secret`;
  exit codes `0` / `1` / `2`.
- **Exit criteria**: a user of `mcp-standardnotes` replaces the 489-line guide with one
  config entry plus `apply`. `doctor` gives an actionable message for each missing
  precondition. `set-secret` accepts a value only from a masked prompt.
- **Status**: Upcoming
- **Spec**: [`SPEC-config-cli.md`](SPEC-config-cli.md)

### Milestone 3 — Ship it

- **Goal**: someone other than the author installs and uses it.
- **Scope**: tagged binaries, install instructions that work on a clean machine, a README
  that a stranger can follow, the Gatekeeper story settled.
- **Exit criteria**: a clean-machine install → `doctor` → `apply` → healthy, without
  reading the source.
- **Status**: Upcoming
- **Runbook**: [`../RELEASE.md`](../RELEASE.md)

### Milestone 4 — Linux

- **Goal**: the seams pay off. Open source implies Linux users.
- **Scope**: a `SystemdManager`, and a Linux `SecretSource` (libsecret, or a `600` env
  file) behind the unchanged interfaces.
- **Exit criteria**: the same config file works on Linux with no field changes; the
  primitive is untouched by the addition.
- **Status**: Upcoming
- **Note**: this milestone is the first real test of the three seams. Expect the interfaces
  to need adjusting — with one implementation each today, they are untested abstractions.

### Milestone 5 — More exposers, and Portals

- **Goal**: stop assuming Cloudflare.
- **Scope**: additional `Exposer` implementations (tailscale funnel, ngrok, a plain reverse
  proxy). Separately: Cloudflare Portals registration inside the primitive (CF API + token;
  `entry` grows a `portal` block).
- **Exit criteria**: an entry switches exposer by changing config, not code.
- **Status**: Upcoming

## Out of scope (for now)

Explicitly deferred, with the reason:

- **`watch` / daemon reconcile mode** — launchd's `KeepAlive` already restarts a dead proxy;
  a manual `apply` / `status` suffices until drift proves otherwise. Wanted.
- **Multiple config files / profiles** — no demonstrated need.
- **Anything gateway-shaped** — UI, filtering, skills, aggregation. Not deferred:
  **refused**. It belongs to an adopted gateway sitting on top, which would consume this
  primitive once for itself. This is the non-goal most likely to be eroded by a reasonable-
  sounding request; guard it.
