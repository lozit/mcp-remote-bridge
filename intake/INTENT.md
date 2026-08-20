<!-- generated-by: groundrules v1.10.0 -->
# Intake / Intent — mcp-remote-bridge

Raw upstream content (paste, email excerpt, call transcript, PO doc, etc.) describing the project intent.

This file is the **raw source**. The structured synthesis (goal, users, constraints, non-goals, acceptance) lives in `docs/VISION.md`.

---

> **Note on this file.** The upstream brief for this project is not a pasted email — it
> is the repo's own pre-code design writing, authored before any Go file existed. Rather
> than duplicate ~400 lines here (which would drift the moment the specs are edited), this
> file records the framing verbatim and **points at** the two specs, which remain the
> source of truth.

## Source documents

- [`../docs/SPEC-primitive.md`](../docs/SPEC-primitive.md) — the `expose` primitive: the
  contract, the three load-bearing rules, `HealthReport`, the three seams, resolved
  decisions, deferred scope.
- [`../docs/SPEC-config-cli.md`](../docs/SPEC-config-cli.md) — the config file format and
  the CLI: `apply` / `remove` / `status` / `logs` / `restart` / `doctor` / `set-secret`,
  the reconcile model, exit codes.
- [`../README.md`](../README.md) — the public framing, reproduced below.

## The framing, as written

**What** — a small Go tool that makes a local **stdio** MCP server reachable from a remote
agent, by automating the wrap-in-a-service + expose-through-a-tunnel setup people do today
by hand.

**Why** — a stdio MCP server is meant to run locally, next to your OS keychain, with no
network port. To let a **remote** agent use it, you have to wrap its stdio into HTTP, keep
that alive, and expose it through an authenticated tunnel. Today that is a manual
procedure — for one such server it is a **489-line guide**
(`mcp-standardnotes/docs/remote-agent-bridge.md`). This tool *is* that guide, executed and
**verified**: it does not trust that it wrote the files, it probes all the way to a real
MCP `initialize` handshake before reporting success.

**Scope** — plumbing, **not a gateway**. No UI, no filtering, no skills, no aggregation —
those belong to an *adopted* gateway (metamcp, mcphub, MCPJungle…) that would sit on top
and consume this tool once for itself instead of once per MCP. Nothing is site-specific:
tunnel name, domain, ports and secrets are all inputs.

**MVP** — macOS (launchd) + Cloudflare Tunnel (`cloudflared`) + macOS keychain. Linux
(systemd), other tunnels, and Cloudflare Portals registration are wanted and deferred.

## The three design commitments

1. **Reconcile, not create** — `ensure` repairs drift and is safe to re-run; `remove` is
   its exact inverse.
2. **Verify the effect, never trust the write** — health is a real probe (proxy up,
   hostname responding, and the MCP `initialize` succeeding), not "files written".
3. **Secrets never touch config, service files, or a command line** — referenced by name,
   fetched from the keychain at launch, failing loudly if absent.

> On rule 3, from the spec: *"This rule is the through-line of the days around its
> writing: a real token leaked once because the simplest way to supply it was the exposing
> way. The safe path is built here, before anything asks for a secret."*
