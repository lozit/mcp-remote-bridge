# Spec — the config file and the CLI

**Status**: draft · **Date**: 2026-08-20 · **Language**: Go 1.24

The layer above [the `expose` primitive](SPEC-primitive.md). The primitive exposes
*one* MCP; this layer is a **declarative config** listing many, and a **CLI** that
reconciles the machine to it. The CLI owns no logic of its own beyond load → loop →
report; every real action is a primitive call.

## The config file

- **Location**: `$XDG_CONFIG_HOME/mcp-remote-bridge/config.toml`
  (`~/.config/mcp-remote-bridge/config.toml`), overridable with `--config`.
- **Format**: TOML — typed, comment-able, human-edited; the Go/CLI-config default.
- **It is committable / shareable.** It carries **no secret values** — only
  references (rule 3 of the primitive). A user can put this file in a dotfiles repo
  without leaking anything.

```toml
# shared infrastructure — assumed already set up (see preconditions)
[infra]
tunnel = "mac-mcp-bridge"        # a cloudflared tunnel, already created + authenticated
domain = "example.com"

# one table per MCP to expose
[mcp.standardnotes]
command   = "mcp-standardnotes"
subdomain = "sn-mcp"             # -> sn-mcp.example.com
# port omitted -> auto-assigned
secrets   = { SN_EMAIL = "keychain:mcp-sn-email" }   # NAME -> SecretSource key, never a value

[mcp.freestyle]
command   = "mcp-freestyle"
subdomain = "freestyle-mcp"
port      = 8081                  # explicit, if you want it stable
secrets   = { LIBRELINKUP_EMAIL = "keychain:mcp-freestyle-email" }
```

`[mcp.<name>]` maps one-to-one onto the primitive's `entry`. `[infra]` supplies the
shared `tunnel`/`domain` so each entry does not repeat them.

## The CLI

`mcp-remote-bridge <command>`. Every command loads the config, then acts through the
primitive.

| command | what it does |
|---|---|
| `apply` | reconcile **all** entries to the config (calls `ensure_exposed` per entry); the everyday command. Idempotent. |
| `apply <name>` | reconcile one entry. |
| `remove <name>` | tear one entry down (`remove_exposed`). |
| `status` | probe **all** entries, print the `HealthReport` table (proxy / initialize / hostname / service). |
| `logs <name>` | tail that entry's proxy log. |
| `restart <name>` | bounce one entry's service, then re-probe. |
| `doctor` | check **preconditions**, not entries: is `cloudflared` installed, is the named tunnel authenticated, is `mcp-proxy` present, does the SecretSource answer. Fix-it hints, no changes. |
| `set-secret <key>` | store a secret in the keychain under `<key>`, read from a **masked stdin prompt** — never an argument, never the environment. The safe path, so no one is ever tempted to pass a secret on a command line. |

### Reconcile model

`apply` is the whole philosophy: the config is desired state, `apply` makes reality
match it — creating what is missing, repairing what drifted, leaving what is already
right. An entry deleted from the config is **not** removed by `apply` (that would make
an edit destructive); `remove <name>` is always explicit. `status` never changes
anything.

### Exit codes (so it composes in scripts and CI)

`0` all healthy · `1` a precondition failed (doctor territory) · `2` at least one
entry unhealthy after the command. A green exit means the probes passed, not that a
file was written — the primitive's "verify the effect" rule surfaced all the way up.

## Decided (reversible)

- **TOML** for config — over YAML (footguns) and JSON (no comments).
- **`apply` reconciles all; `remove` is explicit** — an edit is never silently
  destructive.
- **`set-secret` with masked stdin → keychain** — the CLI-level half of the primitive's
  secret rule; the value never appears in `argv`, the environment, or a file.
- **Cobra** for command structure — the Go CLI standard (subcommands, `--help`, shell
  completion). A dependency, but the ergonomics are worth it for a tool others install.

## Deferred (out of the MVP)

**See [`ROADMAP.md`](ROADMAP.md)** — it is the single consolidated source for post-MVP
scope. Keeping a second list here would drift from it.

In short: `watch` / daemon reconcile mode and multiple config files / profiles are out of
scope for now (launchd's `KeepAlive` already restarts a dead proxy, so a manual `apply` /
`status` suffices until drift proves otherwise); a generated CF Portals entry per MCP is
Milestone 5, gated on the primitive's Portals seam; a non-keychain `SecretSource` and a
non-launchd `ServiceManager` arrive with Linux in Milestone 4.

## Not deferred — a `doctor` requirement for this MVP

`doctor` must flag an exposed hostname that answers with **no access policy in front of
it**, at minimum as a warning. The MCPs this tool exposes carry CGM data and private notes,
and every check in `HealthReport` goes *greener* when the endpoint is wide open — a green
table over an unprotected hostname is manufactured confidence, the defect load-bearing rule
2 exists to prevent.

Warn-only versus refuse-with-`--allow-public` is unresolved; see
[ADR 0001](decisions/0001-doctor-flags-unprotected-hostname.md), to be settled before this
layer ships.
