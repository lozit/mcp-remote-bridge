# mcp-remote-bridge

> **What** — a small Go tool that makes a local **stdio** MCP server reachable from a remote agent, by automating the wrap-in-a-service + expose-through-a-tunnel setup people do today by hand.
> **For** — anyone running local stdio MCP servers (starting with the users of `mcp-standardnotes`, `mcp-freestyle`, `mcp-nightscout`) who wants a VPS-hosted agent to reach them.
> **Deployed** — not deployed — a local CLI, distributed as a single binary.
> **Run** — `make build`, then `mcp-remote-bridge apply`. See below.

## Status

**v0.2.0.** The MVP runs end to end against a real tunnel: `apply` publishes a local stdio MCP
behind a guarded hostname and probes it all the way to an MCP `tools/list`.

## Install

**macOS only** — the tool drives `launchctl` and the macOS keychain. Non-darwin builds fail at
compile time rather than at your first `apply`.

```sh
brew install --cask lozit/tap/mcp-remote-bridge
```

Or download the archive for your architecture from the
[latest release](https://github.com/lozit/mcp-remote-bridge/releases/latest), check it against
the published `SHA256SUMS`, and put the binary on your `PATH`.

Either way the binary is signed with a Developer ID and notarised by Apple, so no `xattr`
incantation is needed. One consequence to know: the notarisation ticket cannot be stapled to a
bare executable — there is no bundle to hold it — so **the first launch on a fresh machine needs
network** for Gatekeeper to check the ticket online. Afterwards it is cached. Each release also
publishes a notarisation receipt beside its archives, naming the Apple submission id.

Building from source stays supported:

```
make build                       # compiles and code-signs when a Developer ID is present
./mcp-remote-bridge doctor       # check the preconditions
./mcp-remote-bridge setup        # create the Access service token, once
./mcp-remote-bridge apply        # reconcile the machine to the config
./mcp-remote-bridge status       # probe everything, change nothing
```

Also: `remove <name>`, `logs <name>`, `restart <name>`.

Exit codes compose in scripts: `0` all healthy, `1` a precondition failed, `2` at least one
entry unhealthy. **A green exit means the probes passed, not that a file was written.**

## What it assumes and does not create

- `mcp-proxy` on `PATH`, and `cloudflared` installed **as a service from a tunnel token** (the
  remotely-managed model — see [ADR 0006](docs/decisions/0006-exposer-targets-remotely-managed-tunnels.md))
- a Cloudflare API token, scoped to `Zone:DNS:Edit`, `Account:Cloudflare Tunnel:Edit` and Access
  edit rights, stored with `set-secret`
- if an MCP Portal fronts your hostnames, its server entries stay manual: that API is closed to
  tokens while Portals are Beta, and header authentication must be declared **at creation** —
  it cannot be added afterwards

`doctor` reports all of this, and every failure carries what to do about it.

### What it does for you

- a launchd service that keeps the proxy alive, with per-entry logs
- the ingress rule and a proxied `CNAME`, inserted without disturbing anything already on the
  tunnel
- **a Cloudflare Access application in front of the hostname, created before the hostname is
  published** — and `apply` refuses an entry proven to answer without authentication, unless the
  config says `allow_public`
- secrets resolved at launch, never written to the config, the service file, or a command line

## Why

A stdio MCP server is meant to run locally, next to your OS keychain, with no network
port. To let a **remote** agent use it, you have to wrap its stdio into HTTP, keep
that alive, and expose it through an authenticated tunnel. Today that is a manual
procedure — for one such server it is a **489-line guide**
(`mcp-standardnotes/docs/remote-agent-bridge.md`). This tool *is* that guide,
executed and **verified**: it does not trust that it wrote the files, it probes all
the way to a live answer from the MCP itself before reporting success — a call that has to
carry data back, because the proxy answers `initialize` and even `ping` on its own while the
MCP behind it is dead ([ADR 0003](docs/decisions/0003-liveness-probe-must-carry-data.md)).

## Scope

Plumbing, **not a gateway**. No UI, no filtering, no skills, no aggregation — those
belong to an *adopted* gateway (metamcp, mcphub, MCPJungle…) that would sit on top
and consume this tool once for itself instead of once per MCP. The bridge is usable
**with or without** such a gateway.

Nothing is site-specific: tunnel name, domain, ports and secrets are all inputs.

**MVP**: macOS (launchd) + Cloudflare Tunnel (`cloudflared`) + macOS keychain.
Linux (systemd), other tunnels, and Cloudflare Portals registration are wanted and
deferred — see the spec's Deferred section.

## Configuration

`$XDG_CONFIG_HOME/mcp-remote-bridge/config.toml`, overridable with `--config`. It carries secret
**references**, never values, so it can live in a dotfiles repo.

```toml
[infra]
domain               = "example.com"
account_id           = "..."
zone_id              = "..."
tunnel_id            = "..."                      # a UUID: the API addresses tunnels by id
api_token            = "keychain:cf-api-token"
access_policy_id     = "..."                      # an existing policy to guard each hostname
access_client_id     = "....access"
access_client_secret = "keychain:cf-access-secret"

[mcp.standardnotes]
command   = "mcp-standardnotes"
subdomain = "sn-mcp"                              # -> sn-mcp.example.com
secrets   = { SN_EMAIL = "keychain:mcp-sn-email" }
```

See [docs/SPEC-config-cli.md](docs/SPEC-config-cli.md) for every field.

## Design commitments

- **Reconcile, not create** — `ensure` repairs drift and is safe to re-run; `remove`
  is its exact inverse.
- **Verify the effect, never trust the write** — health is a real probe (proxy up,
  hostname responding, and the MCP `initialize` succeeding), not "files written".
- **Secrets never touch config, service files, or a command line** — referenced by
  name, fetched from the keychain at launch, failing loudly if absent.

## License

[MIT](LICENSE)
