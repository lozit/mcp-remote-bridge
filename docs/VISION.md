<!-- generated-by: groundrules v1.10.0 -->
# Vision — mcp-remote-bridge

> Synthesis of the project intent. Source: intake/INTENT.md (file docs/SPEC-primitive.md + docs/SPEC-config-cli.md + README.md). Update when intent evolves (rare; tactical decisions go in `docs/decisions/`).

## Goal

Make a local **stdio** MCP server reachable from a remote agent with one command —
replacing a manual procedure (for one such server, a 489-line guide:
`mcp-standardnotes/docs/remote-agent-bridge.md`) with a tool that executes it **and
verifies it**.

Success looks like: `mcp-remote-bridge apply` reconciles the machine to a declarative
config, and `status` proves each entry healthy all the way down to a real MCP
live answer from the MCP itself — not "the files were written". Run twice on a healthy entry →
no-op; run on a drifted one → it repairs only what drifted.

## Users / personas

- **Primary** — people already running local stdio MCP servers, starting with the users of
  `mcp-standardnotes`, `mcp-freestyle` and `mcp-nightscout`, who today follow the manual
  guide by hand.
- **Then** — anyone with a VPS-hosted agent (a Hermes instance, a Claude Agent SDK
  worker, …) that needs to reach an MCP running on a personal machine.
- **Adjacent** — operators of an MCP gateway (metamcp, mcphub, MCPJungle) who would
  consume this primitive **once for the gateway** instead of once per MCP.

Open source, and **not site-specific**: nothing assumes `paranoid.foo` — tunnel name,
domain, ports and secrets are all inputs.

## Constraints

- **Language: Go 1.24** — chosen for the artifact, not the fleet: a system CLI others
  install wants a single distributable binary, and the work (template a service file,
  shell out, probe a port) fits the stdlib.
- **MVP platform boundary**: macOS (launchd) + Cloudflare Tunnel (`cloudflared`) + macOS
  keychain — exactly one implementation per seam.
- **Preconditions are assumed, never created**: the exposer's tool installed with a
  **tunnel installed as a service from a token**, a Cloudflare API token in the keychain, and
  `mcp-proxy` available. The tool
  *adds hostnames to* a tunnel; it does not build one.
- **The secret path is non-negotiable**: no secret value in the config, in a service file,
  or on a command line — referenced by name, fetched from the keychain at launch, failing
  loudly when absent. (This rule exists because a real token leaked once: the simplest way
  to supply it was the exposing way.)
- **Health must be a real probe.** A health check that cannot fail is worse than none — it
  manufactures confidence.
- **Open source implies Linux users.** The three seams exist so Linux is additive later,
  not a rewrite.

## Out of scope for V1 (non-goals)

- **Not a gateway** — no UI, no filtering, no skills, no aggregation. Those belong to an
  *adopted* gateway sitting on top; aggregation specifically is Cloudflare Portals' or the
  gateway's job. The bridge is usable **with or without** one.
- **No Cloudflare Portals registration** inside the primitive — the Mac side is the real
  gain, and Portals is Beta-specific and moves rarely.
- **No Linux / systemd**, no non-keychain `SecretSource`, no tailscale-funnel / ngrok /
  plain-reverse-proxy `Exposer`.
- **No `watch` / daemon reconcile mode** — launchd's `KeepAlive` already restarts a dead
  proxy; a manual `apply` / `status` suffices until drift proves otherwise.
- **No multiple config files or profiles.**
- **`apply` never removes** an entry deleted from the config — an edit must never be
  silently destructive. `remove <name>` is always explicit.

## V1 acceptance criteria

- `ensure_exposed` and `remove_exposed` are **idempotent inverses**: a second `apply` on a
  healthy entry is a no-op; an `apply` on a drifted entry repairs only what drifted.
- `HealthReport` carries **checked facts**, not claims — `proxy_listening`,
  `mcp_responds`, `hostname_responds`, `service_loaded`, plus the
  single derived `healthy`. A red result names *which* check failed and where.
- The deep probe catches the subtle trap: **proxy up, MCP inside it dead**.
- **No secret value** appears in `config.toml`, in a launchd plist, in `argv`, in the
  environment of a shell, or in a log. `set-secret` reads from a **masked stdin prompt**
  only.
- A referenced-but-absent secret **fails at start, loudly** — never a proxy that 401s
  silently.
- Exit codes compose in scripts and CI: `0` all healthy · `1` a precondition failed
  (doctor territory) · `2` at least one entry unhealthy.
- `doctor` checks preconditions with fix-it hints and **changes nothing**; `status`
  changes nothing.
- **The reference case works end to end**: a user of `mcp-standardnotes` replaces the
  489-line guide with one config entry plus `apply`.

---

Further reading:
- `intake/` — raw upstream notes (specs, emails, brainstorms)
- `docs/decisions/` — structural decisions made during the project
- `docs/LEARNINGS.md` — non-trivial learnings
- `docs/ARCHITECTURE.md` (if present) — architecture snapshot
