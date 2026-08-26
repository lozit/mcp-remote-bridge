<!-- generated-by: groundrules v1.10.0 -->
# Glossary — mcp-remote-bridge

Domain vocabulary for the project. One entry per term, alphabetical order.

Keep definitions short and precise. The goal: a new developer (or Claude) quickly
understands the domain language.

---

## A

**`apply`** — the everyday CLI command. Reconciles **all** entries in the config to reality
(one `ensure_exposed` per entry). Idempotent. `apply <name>` does one. It never *removes*
an entry that vanished from the config — see **reconcile**.

## C

**`cloudflared`** — Cloudflare's tunnel daemon. The MVP's `Exposer` shells out to it. Its
installation and an authenticated **named tunnel** are **preconditions**, not something the
tool creates.

## D

**Deep probe** — shorthand for the `mcp_responds` check: a JSON-RPC call that must carry data
back from the MCP process, read from the response body. Distinguished from a shallow "is the
port listening?" check — and, less obviously, from `initialize` and `ping`, which mcp-proxy
answers itself and which therefore also miss a dead MCP behind a live proxy
([ADR 0003](decisions/0003-liveness-probe-must-carry-data.md)).

**`doctor`** — the CLI command that checks **preconditions** (is `cloudflared` installed, is
the tunnel authenticated, is `mcp-proxy` present, does the `SecretSource` answer) rather
than entries. Emits fix-it hints and changes nothing.

**Drift** — the gap between the config's desired state and the machine's actual state: a
service unloaded, a proxy dead, an ingress rule missing. What `ensure_exposed` repairs.

## E

**`ensure_exposed(entry)`** — the primitive's forward function. *Guarantees* a state rather
than creating one: repairs only what drifted, no-ops on a healthy entry. Returns a
**`HealthReport`**.

**Entry** — the declarative unit, one per MCP to expose: `name`, `command` + `args`,
`secrets` (references), `port` (auto-assigned if absent), `subdomain`, `tunnel`, `domain`.
A `[mcp.<name>]` table in the config maps one-to-one onto it.

**Exposer** — the seam that makes a local port reachable at a hostname (ingress rule + DNS
route). MVP implementation: `CloudflaredExposer`.

## G

**Gateway** — an MCP aggregation layer (metamcp, mcphub, MCPJungle…) with a UI, filtering,
skills. **Explicitly not what this project is.** A gateway would sit *on top* and consume
this primitive once for itself instead of once per MCP.

## H

**`HealthReport`** — the verdict for one entry, carrying the evidence behind it:
`proxy_listening`, `mcp_responds`, `hostname_responds`,
`service_loaded`, plus the derived `healthy: bool`. On failure it names *which* check failed
and where. It is a record of probes run, never a record of files written.

## I

**Ingress rule** — the tunnel-side mapping `subdomain.domain → localhost:PORT`. Added by
the `Exposer`, alongside its DNS route.

## L

**Launcher** — the small generated script the service actually execs. Its whole job is the
secret path: ask the `SecretSource` for each referenced secret *immediately before* `exec`,
inject the values into the process environment, exec the MCP. Exists so no secret has to
appear in the plist.

**`LaunchdManager`** — the MVP `ServiceManager`: writes
`~/Library/LaunchAgents/<label>.plist` and drives `launchctl bootstrap` / `bootout`.

## M

**`mcp-proxy`** — the piece that wraps a stdio MCP's stdin/stdout into HTTP on
`127.0.0.1:PORT`. A **precondition**: assumed present, not installed by this tool.

## P

**Precondition** — infrastructure the primitive **assumes and never creates**, because it is
too setup-variable to own: `cloudflared` installed with a named tunnel already created and
authenticated, and `mcp-proxy` available. `doctor` reports on them.

**Primitive** — the atomic operation `ensure_exposed` / `remove_exposed`. Everything else
(the CLI, the config, `status`, `doctor`) is a thin layer over it.

## R

**Reconcile** — the model the whole tool is built on: the config is *desired state*, and a
command makes reality match it — creating what is missing, repairing what drifted, leaving
what is already right. Load-bearing rule #1: *reconcile, not create*. It is what makes
re-running safe and `remove` the exact inverse.

**`remove_exposed(name)`** — the primitive's reverse function. Tears down cleanly. Always
explicit; never triggered by an `apply`, because an edit must never be silently destructive.

## S

**Seam** — one of the three interfaces the primitive talks to (`ServiceManager`, `Exposer`,
`SecretSource`). Each has exactly one implementation in the MVP, so OS / exposer /
secret-store variety is additive later rather than a rewrite.

**Secret reference** — a `SecretSource` key (e.g. `keychain:mcp-sn-email`) stored in the
config in place of a value. Load-bearing rule #3: the config, the service file and the
command line never see a secret value.

**`SecretSource`** — the seam that resolves a named secret **at launch time**. MVP
implementation: `KeychainSecretSource` (macOS keychain).

**`ServiceManager`** — the seam that keeps a process alive across login and reboot. MVP
implementation: `LaunchdManager`.

**`set-secret <key>`** — the CLI command that stores a secret in the keychain, read from a
**masked stdin prompt**. Never an argument, never the environment. It exists so nobody is
ever tempted to pass a secret on a command line.

**`status`** — probes all entries and prints the `HealthReport` table. Changes nothing.

**stdio MCP** — an MCP server that speaks over stdin/stdout, meant to run locally next to
the OS keychain with no network port. The thing this tool makes remotely reachable, and the
reason the problem exists at all.

## T

**Tunnel (remotely-managed)** — a `cloudflared` tunnel installed from a token
(`cloudflared service install <token>`), whose ingress configuration lives **in Cloudflare, not
on disk**. This is the MVP's only supported model, and the one the default install path
produces. Addressed by `tunnel_id` (a UUID) rather than by name, because that is how the API
addresses it and because the DNS target is `{tunnel_id}.cfargotunnel.com`. Referenced by
`[infra].tunnel_id`. See [ADR 0006](decisions/0006-exposer-targets-remotely-managed-tunnels.md).

**Tunnel (locally-managed)** — the other model: created with `cloudflared tunnel login`, which
writes `~/.cloudflared/cert.pem`, with ingress rules in a local `config.yml`. **Not supported in
the MVP** — deliberately, since it cannot be tested on the target machine.
