# Spec — the `expose` primitive

**Status**: draft · **Date**: 2026-08-20 · **Language**: Go 1.24

The atomic operation of mcp-remote-bridge: **make one local stdio MCP reachable
from outside**. Everything else (a CLI, an `--all` loop, `status`, `doctor`) is a
thin layer over this one function. The config file format and the CLI are a
separate, later spec.

## What this is, and who it is for

An **open-source** tool that automates the manual setup its users do today by hand.
The reference case is `mcp-standardnotes/docs/remote-agent-bridge.md` — a 489-line
guide walking a user through wrapping a local stdio MCP into an HTTP service and
exposing it through a tunnel so a remote agent (a VPS-hosted Hermes, a Claude Agent
SDK worker, …) can reach it. This tool *is* that guide, executed and verified.

Audience: people running local stdio MCP servers — first the users of these MCPs
(standardnotes, freestyle, nightscout), then anyone with the same need.

**It is the plumbing, not a gateway.** No UI, no filtering, no skills, no
aggregation — those are a gateway's job (an *adopted* one, sitting on top, which
would consume this primitive once for itself instead of N times per MCP).
Aggregation specifically is Cloudflare Portals' or the gateway's job, not this.
The bridge is usable **with or without** such a gateway; that is the point.

**Not site-specific.** Nothing assumes `paranoid.foo`. Tunnel name, domain, ports
and secrets are all inputs.

## The contract

Two idempotent functions:

```
ensure_exposed(entry) -> HealthReport      # reconcile: repair drift, never duplicate
remove_exposed(name)  -> HealthReport      # reconcile in reverse: tear down cleanly
```

`ensure_exposed` **guarantees** a state, it does not blindly *create* one. Run twice
on a healthy entry → no-op. Run on a drifted entry (service gone, hostname missing,
proxy dead) → it repairs only what drifted.

### The `entry` (declarative unit)

| field | role |
|---|---|
| `name` | unique id → service label, hostname, log path |
| `command` + `args` | how to launch the stdio MCP |
| `secrets` | **names** of secrets (a `SecretSource` key) — **never values** (see rule 3) |
| `port` | `127.0.0.1:PORT` for the proxy — **auto-assigned if absent**, explicit if given |
| `subdomain` | → `subdomain.domain` |
| `tunnel` / `domain` | reference to shared infra (tunnel name, domain) |

### Preconditions (assumed present, never created by the primitive)

Too setup-variable to own: the exposer's tool installed with a **named tunnel
already created** (for the MVP: `cloudflared` installed **as a service from a token** — the
remotely-managed model, see [ADR 0006](decisions/0006-exposer-targets-remotely-managed-tunnels.md));
plus a **Cloudflare API token** held in the `SecretSource`;
`mcp-proxy` available. The primitive **adds hostnames to** the tunnel; it does not
build the tunnel.

### Effects produced

The proxy bound to `127.0.0.1:PORT`; the service keeping it alive; the ingress rule
`subdomain.domain → localhost:PORT` plus its DNS route; logs at a known path.
**No secret written to disk in cleartext.**

## The three load-bearing rules

### 1. Reconcile, not create

State is *ensured*, not imperatively built. Detect what is missing or dead, repair
that, leave the rest. This is what makes re-running safe and `remove` the exact
inverse.

### 2. Verify the effect, never trust the write

`HealthReport` is **a real probe**, not "I wrote the files". It answers:

- is the proxy answering on `127.0.0.1:PORT`?
- does the hostname resolve and respond?
- **and the subtle trap**: the proxy can be up while the MCP inside it is dead.

A health check that cannot fail is worse than none — it manufactures confidence.

### The trap is deeper than it looks (measured, 2026-08-21)

An earlier draft of this spec answered the trap with "the probe goes all the way to a real MCP
`initialize` handshake". **That does not work**, and finding out required killing a real MCP
behind a real `mcp-proxy` rather than reasoning about it:

| Probe | MCP alive | MCP killed |
|---|---|---|
| TCP connect to the port | ✓ | ✓ |
| `initialize` | `result` | **`result`, complete, in 3 ms** |
| `ping` | `result {}` | **`result {}`** |
| `tools/list` | `result` | `error` |

`mcp-proxy` answers `initialize` **itself**, from the state negotiated at startup; the request
never reaches the child. `ping` — the method the protocol advertises for liveness — is served by
the proxy too, which makes it the more dangerous of the two: its name promises exactly what it
does not deliver.

And underneath both: `tools/list` against a dead MCP still returns **HTTP 200**. The failure is
in the JSON-RPC body. A probe reading the transport status is fooled a third time.

**Therefore**: the probe must be a call that *carries data back from the MCP process*, and its
verdict is read from the response **body**. See
[ADR 0003](decisions/0003-liveness-probe-must-carry-data.md) for the sequence and the caveats.

This is rule 2 applied to this spec's own first attempt at implementing rule 2. Treat it as the
worked example: a check is not verified because it sounds thorough.

### 3. The secret path — never in config, never in the service file

- The declarative `entry` carries secret **references** (a `SecretSource` key), never
  a value.
- The service file (a launchd plist, later a systemd unit) carries **no** secret in
  cleartext — those files are readable.
- The secret is fetched **at launch time**: the launcher asks the `SecretSource`
  immediately before `exec`, injects it into the process environment, and the value transits
  neither the config, nor the service file, nor a command line. The launcher is a hidden
  subcommand of the binary, not a generated script — see
  [ADR 0002](decisions/0002-launcher-is-a-hidden-subcommand.md) and
  [`SPEC-launcher.md`](SPEC-launcher.md) for the mechanics, including why
  `mcp-proxy -e KEY VALUE` is never used.
- A referenced secret that is **absent** makes the primitive **fail loudly at start**
  — it does not launch a proxy that will 401 silently.

> This rule is the through-line of the days around its writing: a real token leaked
> once because the simplest way to supply it was the exposing way. The safe path is
> built here, before anything asks for a secret.

## `HealthReport`

Per entry, a verdict with the evidence behind it: `proxy_listening`, `mcp_responds`
(the deep probe — see ADR 0003; **not** `mcp_initialize`, which proves nothing),
`hostname_resolves`, `hostname_responds`, `service_loaded` — each a checked fact, plus the
single derived `healthy: bool`. On failure it names *which* check failed and where, so a red
result is actionable.

## Failure modes handled explicitly

Port already in use · tunnel not running / not authenticated · referenced secret
missing → fail clearly · MCP crashes on boot (proxy up, MCP dead) → caught by the
`mcp_responds` probe, and **only** by it · hostname added but DNS not yet propagated → wait and
verify.

## Three seams, one implementation each in the MVP

The primitive talks only to interfaces; each has exactly one implementation now, so
the MVP stays small while the OS/exposer/secret-store variety is additive later.

```go
type ServiceManager interface {           // keep a process alive across login/reboot
    Ensure(label string, spec ServiceSpec) error   // write + load
    Remove(label string) error                     // unload + delete
    Status(label string) (ServiceState, error)
}

type Exposer interface {                   // make a local port reachable at a hostname
    Ensure(subdomain, domain string, localPort int) error  // ingress + DNS route
    Remove(subdomain, domain string) error
}

type SecretSource interface {              // resolve a named secret at launch time
    Get(key string) (string, error)        // never logged, never written to disk
}
```

MVP implementations:

- `LaunchdManager` — `~/Library/LaunchAgents/<label>.plist`, `launchctl bootstrap` /
  `bootout`.
- `CloudflaredExposer` — adds an ingress entry and a proxied `CNAME` **through the Cloudflare
  API**, to a remotely-managed tunnel addressed by id
  ([ADR 0006](decisions/0006-exposer-targets-remotely-managed-tunnels.md)).
  Not `cloudflared tunnel route dns`: that needs `~/.cloudflared/cert.pem`, which a
  token-installed connector does not have. More fundamentally, a remotely-managed tunnel keeps
  its ingress configuration **in Cloudflare, not on disk**, so there is no local rule to write.
- `KeychainSecretSource` — macOS keychain lookup at launch, via the generated
  launcher.

The primitive never touches `launchctl`, `cloudflared`, or `security` directly.

## Resolved decisions

1. **Language: Go** — a distributable single binary (`brew`, download-and-run, no
   runtime) is the right story for a system CLI others install; and the work is
   orchestration (template a service file, shell out, probe a port) that fits Go's
   stdlib. Chosen for the artifact, not the fleet.
2. **MVP boundary**: macOS + cloudflared + keychain; a config listing entries;
   `ensure` / `remove` / `status`; the `initialize` probe. Nothing else.
3. ~~**Portals registration is out** of the primitive at first~~ — **partly reopened by
   [ADR 0007](decisions/0007-the-tool-owns-the-access-configuration.md)**. The decision assumed
   hostnames and Portal were alternatives; measured, the Portal reaches each MCP *through* the
   tunnel hostname (`HTTP URL = https://<host>/sse`), so one is the origin of the other. The tool
   now owns the Access application guarding that hostname. Configuring the Portal's own MCP
   server stays out — its API route is closed to tokens while the feature is Beta.
4. **Port auto-assigned if absent**, explicit if given.

## Deferred (wanted, out of the MVP)

**See [`ROADMAP.md`](ROADMAP.md)** — it is the single consolidated source for post-MVP
scope. Keeping a second list here would drift from it.

In short: Linux (`SystemdManager` + a Linux `SecretSource`) is Milestone 4; Portals inside
the primitive and other exposers (tailscale funnel, ngrok, a plain reverse proxy) are
Milestone 5.

What is **no longer** deferred: the CLI and the declarative config file listing entries —
the layer above this primitive. They are specified in
[`SPEC-config-cli.md`](SPEC-config-cli.md) and scheduled as Milestone 2.
