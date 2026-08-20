<!-- generated-by: groundrules v1.10.0 -->
# 0002 — The launcher is a hidden subcommand, not a generated shell script

**Date**: 2026-08-20
**Status**: Accepted

## Context

Rule 3 of [`SPEC-primitive.md`](../SPEC-primitive.md) says the secret is fetched **at launch
time**, immediately before `exec`, and transits neither the config, nor the service file, nor
a command line. Something has to do that fetching. That something is the *launcher*: what
launchd actually execs, which resolves the referenced secrets and then starts `mcp-proxy`.

Its shape was left open by the spec. It is load-bearing: the launcher is the one place in the
system that holds a secret *value*, so its failure modes are the project's failure modes.

Two constraints discovered while settling this, both from `mcp-proxy --help`:

- **`mcp-proxy -e KEY VALUE` puts the value in `argv`.** On a shared machine `ps` shows the
  argv of other users' processes; this would leak every secret to any local account. It is the
  most prominent example in mcp-proxy's own help output, which is exactly the "the simplest way
  to supply it was the exposing way" trap rule 3 was written against. **We never use `-e`.**
- The only safe channel is **`--pass-environment`**, which forwards the proxy's own environment
  to the spawned MCP. So the launcher must place the secrets in *its own* environment and exec.

## Decision

**The launcher is a hidden subcommand of the binary**: launchd execs
`mcp-remote-bridge __launch <name> --config <path>`. It resolves secrets through the same
`SecretSource` implementation the rest of the tool uses, builds a **minimal, explicit**
environment, and `syscall.Exec`s `mcp-proxy` — replacing itself, so launchd supervises the
proxy directly with no intermediate process.

**The environment is constructed, not inherited.** The launcher does not pass `os.Environ()`
through. It builds: `PATH`, `HOME`, any variables the entry explicitly declares, plus the
resolved secrets. What the MCP receives is a list written in the config, not whatever launchd
happened to hold.

## Alternatives considered

- **A generated shell script** (`~/.../<name>-launcher.sh` calling `security find-generic-password`
  then `exec mcp-proxy`) — rejected. It is more debuggable by a human, and it would keep the
  service working after the binary is uninstalled. But it makes the secret transit a shell
  command substitution, which brings quoting into the trust path: a value containing a quote,
  a backtick or `$(…)` turns generation into an injection surface. Shell tracing (`set -x`, a
  `BASH_XTRACEFD` inherited from an environment we do not control) prints the value to the log.
  And it would require a second, shell-based implementation of `SecretSource`, so the Go one
  would no longer be the single place secrets are read. A debuggability gain is not worth a
  second code path around the one rule the project treats as non-negotiable.
- **`mcp-proxy -e KEY VALUE`** — rejected outright, see above. It is the direct violation.
- **`--pass-environment` with the full inherited environment** — rejected as the default. It is
  the easy answer and it works, but it forwards launchd's environment to the MCP and, through
  any MCP bug, potentially onward. An explicit list makes the blast radius readable.
- **Fork-and-wait instead of `exec`** — rejected. It leaves a supervising process that adds
  nothing, and forces us to forward signals correctly to avoid a proxy that outlives its
  supervisor.

## Consequences

### Positive
- **One implementation of the secret path**, in Go, testable, with no shell in the trust path.
- No generated file to write, template, or keep in sync — one fewer artifact for `remove` to
  clean up and for drift to accumulate in.
- `syscall.Exec` means launchd's PID *is* the proxy's PID, so `ServiceState.PID` is honest.
- The plist's `ProgramArguments` contain only a name and a config path — nothing secret, which
  matters because the plist is world-readable.

### Negative / Tradeoffs
- **The service now depends on the binary's path.** Uninstalling or moving
  `mcp-remote-bridge` breaks every service. Resolve the absolute path via `os.Executable()` at
  `apply` time and write *that* into the plist; `doctor` should check it still exists. A
  Homebrew upgrade keeps the path; a manual `mv` does not.
- **`__launch` is a public entry point of a public binary**, even hidden from `--help`. It must
  therefore be safe to run by hand: it reads only the named entry, and it must not print a
  secret on any path, including its error path.
- **An explicit environment will break some MCP** that silently depended on an inherited
  variable. That is the intended trade: the failure is loud and fixed by one config line,
  rather than silent and unbounded.

### Neutral
- Does not change any of the three seam interfaces.

## Notes

- The mechanics live in [`../SPEC-launcher.md`](../SPEC-launcher.md).
- Related: rule 3 in `SPEC-primitive.md`; the secret invariants table in `docs/SECURITY.md`;
  [ADR 0001](0001-doctor-flags-unprotected-hostname.md), which similarly turned on a detail of
  what an external tool does by default.
