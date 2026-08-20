<!-- generated-by: groundrules v1.10.0 -->
# Spec — the launcher and the service definition

**Status**: draft · **Date**: 2026-08-20 · **Language**: Go 1.24

How a declared [`entry`](SPEC-primitive.md) becomes a running, supervised process — and where
rule 3 (the secret path) stops being a principle and becomes code.

The shape decision is [ADR 0002](decisions/0002-launcher-is-a-hidden-subcommand.md): the
launcher is a **hidden subcommand of the binary**, not a generated shell script.

## The chain

```
launchd
  └─ exec  mcp-remote-bridge __launch <name> --config <path>     ← the launcher
       │     resolves secrets via SecretSource, builds the environment
       └─ syscall.Exec  mcp-proxy --host 127.0.0.1 --port <P> --pass-environment -- <command> <args...>
            └─ spawns   <command> <args...>                       ← the stdio MCP
                        (inherits the environment, including the secrets)
```

`syscall.Exec` **replaces** the launcher process rather than forking, so launchd supervises
`mcp-proxy` directly: one process, an honest PID, no signal forwarding to get wrong.

## The launcher

### Contract

```
mcp-remote-bridge __launch <name> --config <path> --port <n>
```

Hidden from `--help` (it is not for humans), but it is still a public entry point of a public
binary, so it must be safe to run by hand.

Steps, in order:

1. Load the config at `<path>`; find the entry named `<name>`. Absent → exit non-zero,
   naming the entry and the config path.
2. **Resolve every referenced secret** through the `SecretSource`. Any failure → exit non-zero
   **before launching anything**, naming the *key* that failed. Never launch a proxy that will
   401 silently.
3. Build the environment (below).
4. `syscall.Exec` `mcp-proxy` with the arguments below.

### The environment is constructed, not inherited

The launcher does **not** pass `os.Environ()` through. It builds exactly:

| Source | Contents |
|---|---|
| Fixed | `PATH`, `HOME` — the only variables carried over from the ambient environment |
| Declared by the entry | any `env` the entry lists (plain, non-secret values) |
| Resolved | one variable per `secrets` entry: the declared name → the fetched value |

Everything else is dropped. What the MCP receives is a list written in the config, not
whatever launchd happened to hold. An MCP that silently depended on an inherited variable
**will break** — loudly, fixed by one config line. That is the intended trade.

### Reading a secret (`KeychainSecretSource`)

`security find-generic-password -g -s <service>`, parsing **stderr**. Not `-w`: measured, `-w`
prints a bare hex string for any value containing a non-printable-ASCII byte — an accent, a tab,
a newline, a backslash — and that output is **indistinguishable** from a value that literally is
that hex string. An accented password or a PEM key would reach the MCP corrupted, with nothing
logged, because the secret *was* found and *was* injected. See
[ADR 0004](decisions/0004-keychain-read-must-use-g-not-w.md).

A missing item is exit code **44** and must surface as a named "not found", never an empty
value.

**The write side has the mirror trap**: `security add-generic-password -w <value>` puts the
secret in `argv`. `set-secret` must not shell out that way — it is the CLI-level half of rule 3.

### What the launcher must never do

- **Never log a secret value.** Errors name the *key*, never the value. This includes the
  error path, which is the one people forget.
- **Never place a value in `argv`** — see the mcp-proxy note below.
- **Never write a value to disk**, including a temp file or a debug dump.

## Invoking mcp-proxy

```
mcp-proxy --host 127.0.0.1 --port <PORT> --pass-environment -- <command> <args...>
```

- **`--host 127.0.0.1` is passed explicitly**, even though it is mcp-proxy's default. Loopback
  binding is a security control here, and a control that relies on someone else's default is
  one upstream release away from being gone.
- **`--port <PORT>`** is always explicit; omitted, mcp-proxy picks a random port and the ingress
  rule would point at nothing.
- **`--pass-environment`** is how the secrets reach the MCP: mcp-proxy forwards its own
  environment to the process it spawns. Since the launcher constructed that environment, the
  forwarding is bounded.
- **`--`** separates the proxy's flags from the MCP's command, so an MCP argument can never be
  eaten as a proxy flag.

> **Never use `mcp-proxy -e KEY VALUE`.** It is the most prominent example in mcp-proxy's own
> `--help`, and it puts each value in `argv`, where `ps` exposes it to every local account.
> It is the exact shape of the trap rule 3 exists to close: the simplest way to supply the
> secret is the exposing way.

**Verified 2026-08-21** (mcp-proxy 0.12.0): the streamable-HTTP endpoint is **`/mcp`**, and a
single `POST` there completes an `initialize`. `/sse` also exists but needs a long-lived stream
plus a separate POST. `GET /` is a 404. The probe uses `/mcp`; the ingress rule routes the whole
hostname, so it carries no path constraint.

Beware what that `initialize` does **not** prove — see
[ADR 0003](decisions/0003-liveness-probe-must-carry-data.md).

## The service definition (launchd)

`ServiceSpec` → `~/Library/LaunchAgents/<label>.plist`, loaded with `launchctl bootstrap`.

| Field | Value |
|---|---|
| `Label` | derived from the entry name (see the naming rules) |
| `ProgramArguments` | the absolute path of the `mcp-remote-bridge` binary, then `__launch`, `<name>`, `--config`, `<path>` |
| `RunAtLoad` | true |
| `KeepAlive` | a **dictionary**, not a boolean: `{SuccessfulExit: false, Crashed: true}` — restart when the program exits non-zero or crashes |
| `ThrottleInterval` | ≥ 60s, so an unrecoverable failure fails slowly rather than spinning. **Below 1s is refused**: launchd takes whole seconds, so anything smaller renders as `0`, and `0` disables throttling — the zero value must not be the dangerous one |
| `StandardOutPath` / `StandardErrorPath` | the entry's **own** log paths — one file per entry |

> **Measured against a working hand-built setup** (`~/Library/LaunchAgents/com.mcpstandardnotes.proxy.plist`,
> 2026-08-21) rather than derived from documentation. That plist is the reference this generator
> must match. Two deliberate differences from it:
>
> - It uses `ThrottleInterval: 10`. We use ≥ 60: with `SuccessfulExit: false`, a launcher that
>   exits non-zero on a missing secret is restarted forever, and 10s makes that a spin. Slower is
>   more diagnosable.
> - It points `StandardOutPath` and `StandardErrorPath` of **several entries at one shared file**,
>   so their output interleaves. One log per entry is a concrete gain of the tool, not just
>   automation of the status quo.

**What that reference plist also demonstrates**: it is `-rw-r--r--` (world-readable) and carries
`-e SN_EMAIL <value>` in `ProgramArguments`. The secret is exposed twice over — in a readable
file and in `argv`. This is the failure rule 3 exists to close, observed in production on the
author's own machine.

**The plist is world-readable, and it contains no secret** — only a name and a config path.
That is the whole reason the launcher exists.

**The binary path is resolved at `apply` time** via `os.Executable()` and written absolute.
Consequence: moving or uninstalling the binary breaks every service. `doctor` checks the
recorded path still exists.

## Failure modes handled explicitly

**A referenced secret is missing.** Two gates, on purpose:

1. **At `apply`** — `ensure_exposed` resolves every reference *before* writing the service. A
   missing secret fails the apply loudly; nothing is written, nothing is loaded. This is the
   primary gate, and it is where rule 3's "fail loudly at start" lives.
2. **At launch** — the secret may have been deleted since. The launcher exits non-zero naming
   the key, before starting the proxy.

Because `KeepAlive` is true, case 2 makes launchd retry; `ThrottleInterval` keeps that to a
slow, visible loop rather than a spin. `status` reports the service as loaded but
`proxy_listening` red, and the log names the missing key — a diagnosable state, not a mystery.

**The binary moved or was uninstalled.** Every service fails at exec. `doctor` reports it
against the recorded path.

**Port already in use.** Detected at `apply`, before writing the service.

**The MCP crashes on boot** while mcp-proxy stays up. Invisible to every check except the
`mcp_responds` deep probe — and *only* by it, since the proxy answers `initialize` and `ping`
on its own. This is the trap that motivates the whole probe design.

## Deferred

**See [`ROADMAP.md`](ROADMAP.md)** — the consolidated source for post-MVP scope. In short: a
`SystemdManager` writing a unit file instead of a plist arrives with Milestone 4, behind the
unchanged `ServiceManager` interface, and the launcher subcommand is reused as-is (its only
OS-specific dependency is the `SecretSource` behind it).
