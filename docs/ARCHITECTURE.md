<!-- generated-by: groundrules v1.10.0 -->
# Architecture — mcp-remote-bridge

**Living** snapshot of the current architecture. Updated as the structure evolves.

For the **why** behind choices → see `docs/decisions/`.
For the normative contracts → [`SPEC-primitive.md`](SPEC-primitive.md) and
[`SPEC-config-cli.md`](SPEC-config-cli.md). This file is the *map*; the specs are the
territory.

> **Status: pre-code.** Everything below is the intended structure, not an observed one.
> Correct it against reality as soon as code lands.

## Overview

Three layers, each thinner than the one below it:

```
  CLI  (cobra)          apply · remove · status · logs · restart · doctor · set-secret
    |                   load config -> loop over entries -> report. No logic of its own.
    v
  primitive             ensure_exposed(entry) -> HealthReport
    |                   remove_exposed(name)  -> HealthReport
    |                   Reconciles. Talks ONLY to the three interfaces below.
    v
  seams                 ServiceManager | Exposer | SecretSource
                        One implementation each in the MVP.
```

The arrow only points down. The primitive never shells out; the CLI never touches an
implementation.

## Stack

**Go 1.24** — distributed as a single static binary (`brew`, download-and-run, no runtime).
Stdlib-first (one dependency, argued in ADR 0005); [Cobra](https://github.com/spf13/cobra) for command structure, TOML for the
config file.

- The primitive talks **only to interfaces** (`ServiceManager`, `Exposer`, `SecretSource`).
  Shelling out to `launchctl`, `cloudflared`, or `security` happens **inside an
  implementation**, never in the primitive or the CLI.
- The CLI owns no logic beyond load → loop → report; every real action is a primitive call.
- `gofmt` + `go vet` clean. Wrap errors with `%w`; never swallow one.
- Readability > cleverness. No premature abstractions. No comments paraphrasing code —
  reserve them for non-obvious "why".

## Components

### The `expose` primitive

The atomic operation: make **one** local stdio MCP reachable from outside. Two idempotent
functions that are exact inverses:

- `ensure_exposed(entry) -> HealthReport` — **guarantees** a state, does not blindly create
  one. Detects what is missing or dead, repairs that, leaves the rest.
- `remove_exposed(name) -> HealthReport` — tears the same state down cleanly.

It consumes an `entry` (`name`, `command`+`args`, `secrets` *by reference*, `port`,
`subdomain`, `tunnel`, `domain`) and produces: a proxy bound to `127.0.0.1:PORT`, a service
keeping it alive, an ingress rule `subdomain.domain → localhost:PORT` with its DNS route,
and logs at a known path.

### `HealthReport` — the verification component

Not a status struct: a **record of probes actually run**. `proxy_listening`,
`mcp_responds`, `hostname_resolves`, `hostname_responds`, `service_loaded`, plus the
single derived `healthy: bool`.

`mcp_responds` is the load-bearing one. The proxy can be listening while the MCP inside it is
dead, so the probe must be a JSON-RPC call that **carries data back from the MCP process**, with
the verdict read from the response body — never the HTTP status, which is `200` even for a dead
MCP.

It is not `initialize`, and not `ping`: measured against mcp-proxy 0.12.0, the proxy answers
both itself and neither can fail when the MCP is dead. See
[ADR 0003](decisions/0003-liveness-probe-must-carry-data.md).

### The three seams

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

MVP implementations — one each, so the MVP stays small while OS / exposer / secret-store
variety is purely additive later:

| Seam | MVP implementation | Mechanism |
|---|---|---|
| `ServiceManager` | `LaunchdManager` | `~/Library/LaunchAgents/<label>.plist`, `launchctl bootstrap` / `bootout` |
| `Exposer` | `CloudflaredExposer` | ingress entry + proxied `CNAME` via the **Cloudflare API**, on a remotely-managed tunnel addressed by id ([ADR 0006](decisions/0006-exposer-targets-remotely-managed-tunnels.md)) |
| `SecretSource` | `KeychainSecretSource` | macOS keychain lookup **at launch**, via the generated launcher |

### The CLI

Cobra subcommands over the primitive. `apply` (all or one) · `remove <name>` · `status` ·
`logs <name>` · `restart <name>` · `doctor` · `set-secret <key>`.

`doctor` is the odd one out: it checks **preconditions**, not entries, and changes nothing.

### The config

`$XDG_CONFIG_HOME/mcp-remote-bridge/config.toml`, overridable with `--config`. TOML.
`[infra]` supplies the shared `tunnel` / `domain`; each `[mcp.<name>]` table maps
one-to-one onto the primitive's `entry`.

**It is committable.** It carries secret *references* only, so it can live in a dotfiles
repo without leaking anything.

## Main flows

### 1. `apply` — the everyday flow

Load config → for each entry, `ensure_exposed`:
resolve port (auto-assign if absent) → ensure the proxy service (generate the launcher,
write the plist, `bootstrap`) → ensure the ingress rule + DNS route → **probe**, including
the `mcp_responds` deep probe → return the `HealthReport`.
Print the table; exit `0` / `1` / `2`.

Already healthy → every step is a no-op and only the probe runs. Drifted → only the drifted
step acts.

### 2. The secret path — launch time, never write time

`set-secret <key>` reads a value from a **masked stdin prompt** and stores it in the
keychain. Nothing else ever handles the value.

At launch, the generated launcher asks the `SecretSource` for the referenced key
**immediately before `exec`**, injects it into the process environment, and execs. The
value transits neither the config, nor the plist, nor a command line. A referenced secret
that is absent **fails loudly at start** rather than launching a proxy that will 401
silently.

### 3. `remove` — the exact inverse

`bootout` + delete the plist → drop the ingress rule and the DNS route → re-probe to
confirm the teardown. Never triggered by an `apply`; always explicit.

## Environments

- **Local (the only one)** — the user's macOS machine. This is a CLI, not a deployed
  service; there is no staging or production of *this tool*. What it configures is the
  user's own machine.
- **Distribution** — see [`../RELEASE.md`](../RELEASE.md).

## Points of attention

- **Preconditions are assumed, never created.** `cloudflared` installed with a named tunnel
  installed as a service from a token, with a Cloudflare API token in the `SecretSource`;
  `mcp-proxy` available. The tool *adds hostnames to* a
  tunnel; it does not build one. `doctor` is the mitigation.
- **Failure modes to handle explicitly** — port already in use · tunnel not running or not
  authenticated · referenced secret missing (fail clearly) · MCP crashes on boot with the
  proxy still up (caught only by the `initialize` probe) · hostname added but DNS not yet
  propagated (wait and verify).
- **The temptation to grow into a gateway.** Every request for filtering, aggregation or a
  UI belongs above this layer, in an adopted gateway. Say no here.
- **Single-implementation seams are unexercised seams.** With one implementation each, the
  interfaces are untested abstractions until Linux arrives. Expect them to need adjusting
  then — that is the cost of deferring, and it is accepted.
