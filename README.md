# mcp-remote-bridge

> **What** — a small Go tool that makes a local **stdio** MCP server reachable from a remote agent, by automating the wrap-in-a-service + expose-through-a-tunnel setup people do today by hand.
> **For** — anyone running local stdio MCP servers (starting with the users of `mcp-standardnotes`, `mcp-freestyle`, `mcp-nightscout`) who wants a VPS-hosted agent to reach them.
> **Deployed** — not deployed — a local CLI, distributed as a single binary.
> **Run** — no code yet; see status below.

## Status

**Pre-code.** The design is specified, not built:
[docs/SPEC-primitive.md](docs/SPEC-primitive.md) fixes the atomic operation — make
one stdio MCP reachable — and the three seams (`ServiceManager`, `Exposer`,
`SecretSource`) behind which the MVP ships one implementation each.

## Why

A stdio MCP server is meant to run locally, next to your OS keychain, with no network
port. To let a **remote** agent use it, you have to wrap its stdio into HTTP, keep
that alive, and expose it through an authenticated tunnel. Today that is a manual
procedure — for one such server it is a **489-line guide**
(`mcp-standardnotes/docs/remote-agent-bridge.md`). This tool *is* that guide,
executed and **verified**: it does not trust that it wrote the files, it probes all
the way to a real MCP `initialize` handshake before reporting success.

## Scope

Plumbing, **not a gateway**. No UI, no filtering, no skills, no aggregation — those
belong to an *adopted* gateway (metamcp, mcphub, MCPJungle…) that would sit on top
and consume this tool once for itself instead of once per MCP. The bridge is usable
**with or without** such a gateway.

Nothing is site-specific: tunnel name, domain, ports and secrets are all inputs.

**MVP**: macOS (launchd) + Cloudflare Tunnel (`cloudflared`) + macOS keychain.
Linux (systemd), other tunnels, and Cloudflare Portals registration are wanted and
deferred — see the spec's Deferred section.

## Design commitments

- **Reconcile, not create** — `ensure` repairs drift and is safe to re-run; `remove`
  is its exact inverse.
- **Verify the effect, never trust the write** — health is a real probe (proxy up,
  hostname responding, and the MCP `initialize` succeeding), not "files written".
- **Secrets never touch config, service files, or a command line** — referenced by
  name, fetched from the keychain at launch, failing loudly if absent.

## License

[MIT](LICENSE)
