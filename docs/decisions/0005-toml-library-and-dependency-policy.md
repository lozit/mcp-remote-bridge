<!-- generated-by: groundrules v1.10.0 -->
# 0005 — TOML library: `pelletier/go-toml/v2`, and the dependency policy

**Date**: 2026-08-21
**Status**: Accepted

## Context

`__launch` cannot be written without loading the config, and `apply` needs it too, so the config
parser moved onto the critical path earlier than the milestone order suggested.

TOML was already chosen in [`SPEC-config-cli.md`](../SPEC-config-cli.md) (over YAML's footguns
and JSON's lack of comments). This ADR is about **which library**, and about the fact that this
is the project's **first dependency** — `go.mod` has been empty until now, and `CLAUDE.md` makes
adding one require an ADR.

### Why this config file deserves a strict parser

`config.toml` is hand-edited, and its fields have security consequences:

- A typo in `subdomain` publishes the MCP at a **different hostname** than intended.
- A typo in `secrets` means a referenced secret is not found — caught loudly — but a typo in the
  *key name* silently drops the variable, and the MCP starts without a credential.
- A typo in `port` or `command` fails visibly. These are the easy ones.

A parser that silently ignores an unrecognised key turns a one-character mistake into a wrong
hostname. So the deciding criterion is **whether unknown keys are rejected**, not speed.

### Measured, 2026-08-21

Both candidates have **zero external dependencies** — stdlib only — so the usual argument against
adding a dependency does not apply here.

| | `BurntSushi/toml` v1.6.0 | `pelletier/go-toml/v2` v2.4.3 |
|---|---|---|
| External deps | none | none |
| Unknown key | **no error**; must inspect `MetaData.Undecoded()` by hand | `DisallowUnknownFields()` → error |
| Error quality | `toml: line 3 (last key "mcp.sn.command"): …` | positioned, with a caret under the offending field |

The pelletier message for a typo:

```
4| subdomian = "oops"
 | ~~~~~~~~~ unknown field
```

BurntSushi can catch the same typo, but only through opt-in code that a future contributor can
forget to write. pelletier makes strictness a decoder setting: forgetting it is a visible
omission at the call site rather than an absent check somewhere else.

## Decision

**`github.com/pelletier/go-toml/v2`**, with `DisallowUnknownFields()` always on.

**Dependency policy**, stated here since this is the first one:

1. A new dependency needs an ADR. This is that rule's first application, not an exception to it.
2. Prefer zero-transitive-dependency libraries. Both candidates qualified; a candidate that did
   not would have needed to justify each transitive edge.
3. The stdlib is the default. A dependency is justified by a format or protocol we would
   otherwise implement ourselves badly — TOML parsing qualifies; a helper that saves ten lines
   does not.
4. Pin the version and record why in the ADR, so a bump is a decision rather than a drift.

## Alternatives considered

- **`BurntSushi/toml`** — rejected on the strictness ergonomics above. It is the older, very
  widely used option and would have been a defensible choice; the deciding factor is that its
  strict check is something you must remember to write.
- **Hand-written TOML parser** — rejected. TOML 1.0 has enough surface (datetimes, inline tables,
  multi-line strings, escapes) that a hand-rolled parser would be a bug source in the component
  that decides which hostname gets published.
- **JSON, from the stdlib, no dependency at all** — rejected, and this is the tempting one. It
  would keep `go.mod` empty. But `SPEC-config-cli.md` chose TOML precisely because the file is
  meant to be **commented and hand-edited**, and JSON has no comments. Trading the config file's
  usability for an empty `go.mod` is optimising the wrong thing.

## Consequences

### Positive
- A hand-edited config is checked, and a typo is reported with its line and a caret.
- Still a single static binary with no cgo and no transitive dependencies.

### Negative / Tradeoffs
- `go.mod` is no longer empty. The "stdlib-first" claim in the README now means "one dependency,
  argued for" — keep it honest.
- Strict mode makes the config **forward-incompatible**: a file written for a newer version, with
  a field this build does not know, is rejected rather than partly read. That is the intended
  trade for a security-relevant file, but it means adding a field is a breaking change for older
  binaries, and needs a line in the CHANGELOG.
- One more upstream to track for security advisories.

### Neutral
- No change to any seam interface. The parser lives in its own package and yields
  `bridge.Entry` values.

## Notes

- Cobra, already chosen for the CLI in `SPEC-config-cli.md`, will be the second dependency and
  does carry transitive deps. It needs its own ADR when it lands — this one does not pre-approve
  it.
