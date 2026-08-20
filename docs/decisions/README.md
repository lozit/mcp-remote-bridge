<!-- generated-by: groundrules v1.10.0 -->
# Architecture Decisions (ADR)

This folder contains the project's **Architecture Decision Records**: each structural decision made during the project is recorded in a file.

## Format

Inspired by [Michael Nygard](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions). See `0000-template.md`.

## Naming convention

`NNNN-title-kebab.md` where NNNN is a 4-digit incremental integer.

Examples:
- `0001-database-choice.md`
- `0002-auth-pattern.md`

## When to create an ADR

When a decision:
- has a **long-term impact** on the architecture
- is **hard to reverse**
- has **explicit tradeoffs** worth documenting
- might be **revisited later** (better to freeze the context now)

No ADR needed for trivial choices or implementation details.

## Index

| # | Title | Status | Date |
|---|---|---|---|
| 0000 | Template | — | — |
| [0001](0001-doctor-flags-unprotected-hostname.md) | `doctor` must flag an exposed hostname with no access policy | Accepted | 2026-08-20 |
| [0002](0002-launcher-is-a-hidden-subcommand.md) | The launcher is a hidden subcommand, not a generated shell script | Accepted | 2026-08-20 |
| [0003](0003-liveness-probe-must-carry-data.md) | The liveness probe must carry data back from the MCP | Accepted | 2026-08-21 |
| [0004](0004-keychain-read-must-use-g-not-w.md) | Read keychain secrets with `security -g`, never `-w` | Accepted | 2026-08-21 |
| [0005](0005-toml-library-and-dependency-policy.md) | TOML library: `pelletier/go-toml/v2`, and the dependency policy | Accepted | 2026-08-21 |
