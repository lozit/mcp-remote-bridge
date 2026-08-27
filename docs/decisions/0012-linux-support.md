# 0012 — Linux support: systemd, and a pluggable secret source

**Date**: 2026-08-27
**Status**: Accepted

## Context

The tool is darwin-only, deliberately and enforced at compile time
([ADR 0009](0009-release-is-hand-rolled-and-darwin-only.md)). Extending it to Linux is now
wanted: the maintainer has a Linux machine, and an MCP server that must be reachable from a
remote agent has no reason to be on a Mac.

**The architecture anticipated this.** The primitive talks only to `ServiceManager`,
`SecretSource` and `Exposer`. Two facts, checked rather than assumed:

- **`Exposer` needs no work at all.** It speaks only to the Cloudflare API
  ([ADR 0006](0006-exposer-targets-remotely-managed-tunnels.md)); nothing in it is
  OS-specific. The whole network half of the tool is already portable.
- **`ServiceSpec` maps onto a systemd unit with no contract change**: `Label` → unit name,
  `Program`+`Args` → `ExecStart`, `StdoutPath`/`StderrPath` → `StandardOutput=append:`,
  `KeepAlive` → `Restart=`, `ThrottleInterval` → `RestartSec`. So the seam interfaces are
  untouched, which matters because changing them would itself require an ADR.

What is genuinely undecided is **where secrets live on Linux**. macOS has one obvious answer.
Linux has four, and they disagree about the case that matters most.

## Decision

**Add a `ServiceManager` for systemd and make `SecretSource` selectable, defaulting per
environment rather than per OS.**

### systemd: user units, with lingering as a checked precondition

Units go in `~/.config/systemd/user/`, driven by `systemctl --user` — the direct analogue of
today's per-user `LaunchAgents`, and it keeps the tool free of root. This project already
refuses root once, for the tunnel connector, for the same reason: a tool that handles secrets
and asks for root is a different trust contract.

**`doctor` must check `loginctl show-user --property=Linger`.** Without lingering, a user
service dies when the last session ends — so a service installed over SSH stops the moment the
connection closes, and comes back "healthy" the next time anyone logs in. That is a failure that
disappears when observed, which is precisely the class this tool exists to make visible.

### Secrets: two backends, chosen by what the machine can actually do

Both are needed, because both machines are in scope and they exclude each other:

- **`systemd-creds`** — the default on a headless host. Encrypted to the machine (TPM or host
  key), readable by the service at start with no session, no agent, no unlocked keyring. This is
  the only option that works on a box that boots unattended.
- **`secret-tool`** (libsecret) — the desktop answer, and the closest mirror of the macOS
  keychain in ergonomics. It requires a session bus and an unlocked keyring, so a service
  started at boot would fail to read its secrets.

`doctor` reports which backend is in use and why, and refuses to guess silently: choosing the
wrong one produces a service that starts and then fails to authenticate, which is the exact
"proxy that 401s silently" that rule 3 exists to prevent.

**The reference syntax carries the backend**: `keychain:name` stays macOS, and Linux gets
`systemd-creds:name` and `secret-tool:name`. A reference that names its source can be read on
the wrong machine and fail clearly, instead of resolving to something unintended.

### The invariants do not move

Rule 3 holds identically: `LoadCredential=` / `systemd-creds` pass a *path*, never a value, and
`secret-tool` is read immediately before `exec`. **No secret in a unit file** — unit files are
world-readable exactly as plists are. The proxy still binds `127.0.0.1` only.

## Alternatives considered

- **libsecret alone**: rejected. It is the obvious desktop choice and useless headless, which is
  the likelier deployment for something serving an MCP to a remote agent.
- **A `0600` file**: rejected as a *default*, though it is what a bare `--secrets-file` would be.
  It is the weakest option and the one hardest to argue for in a tool whose reason to exist
  includes never letting a secret sit readable.
- **`pass` / GPG**: works headless, but needs `gpg-agent` and an unlocked key — an agent
  lifecycle problem traded for a keyring lifecycle problem, plus a heavier dependency.
- **System units instead of user units**: rejected. Root, for the reason above.
- **A single cross-platform secret abstraction (a Go keyring library)**: rejected on the
  dependency policy ([ADR 0005](0005-toml-library-and-dependency-policy.md)) and because it
  would hide exactly the distinction that matters — headless versus session — behind a uniform
  API that silently picks wrong.

## Consequences

### Positive
- The seams are validated by a second implementation, which is the only real test that an
  abstraction was worth having.
- `go install` becomes meaningful on Linux: no signing, no notarisation, no cask.

### Negative / Tradeoffs
- Two secret backends to test and document, where macOS has one.
- The release matrix stops being two darwin targets, which was the whole argument of ADR 0009
  against GoReleaser — that argument is now moot rather than wrong, and
  [ADR 0011](0011-goreleaser-and-a-homebrew-tap.md) already put GoReleaser in place.
- The compile guard relaxes from `!darwin` to `!darwin && !linux`, and its test must follow.
- **Nothing here can be verified from a Mac.** The systemd manager, lingering, and both secret
  backends all need a real Linux host to be believed — unit tests would prove only that the
  mocks agree with themselves, which this project treats as no proof at all.

### Neutral
- `doctor` grows per-OS checks. It already reports presence rather than validity, so the shape
  is unchanged.

## Notes

- Not decided here, and deliberately: whether Linux ships in the same release as macOS or on its
  own cadence, and whether a `.deb`/`.rpm` is worth it over a tarball. Those follow once the
  thing runs.
