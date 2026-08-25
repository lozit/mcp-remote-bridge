<!-- generated-by: groundrules v1.10.0 -->
# 0008 — Cobra for the CLI

**Date**: 2026-08-25
**Status**: Accepted

## Context

The CLI has seven commands — `apply`, `status`, `remove`, `doctor`, `setup`, `logs`, `restart` —
parsed by hand: a `switch` on `os.Args[1]` and a shared `parseEntryArgs` reading `[name]
[--config path]`. About fifty lines.

[ADR 0005](0005-toml-library-and-dependency-policy.md) chose the project's first dependency and
set the policy: an ADR per dependency, prefer zero transitive deps, stdlib by default. It closed
with "Cobra … will be the second dependency and does carry transitive deps. It needs its own ADR
— this one does not pre-approve it." This is that ADR.

`SPEC-config-cli.md` had already named Cobra, but that was written before any parsing existed.
The measurement now available did not support it on volume alone.

### Measured, 2026-08-25

| | Hand-rolled | With Cobra v1.10.2 |
|---|---|---|
| Modules | 1 | 4 (`cobra`, `pflag`, `mousetrap`) |
| Binary | 9.0 MB | ~11 MB |
| Parsing code | ~50 lines | 0 |

**Fifty working lines do not justify tripling the module count.** If volume were the argument,
the honest answer would be to defer.

## Decision

**Adopt Cobra**, for what cannot reasonably be hand-written rather than for what can:

- **Shell completion.** A tool whose commands take an *entry name* from a config file is exactly
  the case where completion matters — `apply <tab>` listing the entries is a real ergonomic
  gain, and implementing that by hand for bash, zsh and fish is a project of its own.
- **Per-command `--help`.** Seven commands with different arguments already exceed what one
  usage block explains well, and this project's whole thesis is that confusion, not length, is
  the cost of a manual procedure — a claim that would sit badly next to a CLI whose only help is
  a wall of text.

Migrating now is also cheaper than later: seven commands is the point where the shape is settled
but the surface is still small.

## Alternatives considered

- **Defer, with criteria** — the honest default, and rejected only because the reasons above are
  present *today* rather than anticipated. Had the gain been "fewer lines", deferring would have
  won.
- **Hand-write per-command `--help`** — keeps one dependency, but produces exactly the code that
  grows until it costs more than the library, and never gets completion.
- **A smaller flag library** (`urfave/cli`, stdlib `flag` with subcommands) — `flag` has no
  subcommand model, so the switch stays and only the flag parsing moves. The smaller libraries
  trade Cobra's ubiquity for a marginal size saving on a 9 MB binary.

## Consequences

### Positive
- Completion and structured help, neither of which is worth hand-writing.
- Conventional structure: contributors know Cobra, and command files stop being a bespoke
  dispatch table.

### Negative / Tradeoffs
- **The module count triples**, from one dependency to four. The policy in ADR 0005 says to
  prefer zero transitive deps; this is the first exception, taken deliberately and recorded as
  such rather than quietly.
- **~2 MB on the binary**, ~22% — irrelevant for a developer tool installed once, and worth
  restating only so nobody rediscovers it as a surprise.
- **Three more upstreams to track** for security advisories.
- Cobra's defaults are opinionated (it will happily add a `completion` command and suggest
  corrections). Those are conveniences here, but they are behaviour the tool did not write.

### Neutral
- No change to any seam interface, to the primitive, or to the config format. This is the outer
  layer only — the CLI still owns no logic beyond load → loop → report.

## Notes

- Pinned to `v1.10.2`, per ADR 0005's policy of pinning and recording why.
- The exit codes stay as `SPEC-config-cli.md` defines them: `0` all healthy, `1` a precondition
  failed, `2` at least one entry unhealthy. Cobra's own error handling must not override them —
  a command that fails a precondition must still exit `1`, not Cobra's default.
