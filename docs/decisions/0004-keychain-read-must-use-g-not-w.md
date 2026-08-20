<!-- generated-by: groundrules v1.10.0 -->
# 0004 — Read keychain secrets with `security -g`, never `-w`

**Date**: 2026-08-21
**Status**: Accepted

## Context

`KeychainSecretSource.Get` shells out to `security find-generic-password`. The obvious flag is
`-w`: it prints the password alone on stdout, which is exactly what a `SecretSource` wants.

Measured on macOS 2026-08-21, `-w` is **lossy and ambiguous**. It prints the value verbatim only
when every byte is printable ASCII. Any other byte — an accented character, a tab, a newline, a
backslash — makes it print a **bare hexadecimal string with no marker**:

| Stored value | `-w` output |
|---|---|
| `abc123` | `abc123` |
| `a b c` | `a b c` |
| `café` | `636166c3a9` |
| `a\nb` | `610a62` |
| `a\tb` | `610962` |
| `a\b` | `615c62` |

The failure is not that hex needs decoding. It is that **the output is ambiguous**: storing the
literal string `636166c3a9` and storing `café` both print `636166c3a9`. No amount of care at the
call site can tell them apart, because the information is gone.

Consequences if `-w` were used: a password containing an accent — not exotic — reaches the MCP
as the ASCII string `636166c3a9`. A multi-line secret (a PEM private key) is corrupted the same
way. The MCP then fails to authenticate for a reason that appears nowhere: the secret *was*
found, the launcher *did* inject it, and nothing logged the value (correctly). A silent,
unloggable corruption of exactly the data the project promises to handle carefully.

`-g` prints to **stderr** and disambiguates with a `0x` prefix:

```
password: "abc123"
password: 0x636166C3A9  "caf\303\251"
password: "636166c3a9"
```

## Decision

`KeychainSecretSource.Get` invokes `security find-generic-password -g`, reads **stderr**, and
parses the `password:` line:

- Starts with `0x` → decode the hex run as the value's bytes.
- Otherwise → the value is between the **first** and the **last** `"` on the line.

The last-quote rule is not decoration: a value containing `"` is printed unescaped
(`a"b` → `password: "a"b"`), so a naive "read to the next quote" truncates it. A value
containing a backslash is forced to hex, so the quoted form never carries an escape sequence
to unescape.

A missing key is `security`'s exit code **44**, which must be reported as a distinct, named
failure rather than an empty value.

## Alternatives considered

- **`-w`** — rejected: measured ambiguous, see above. It is the flag anyone will reach for
  first, which is why this ADR exists.
- **Decode `-w`'s output as hex when it looks like hex** — rejected: that is the ambiguity, not
  a fix. It corrupts a legitimate all-hex secret, and hex-shaped secrets (a hashed token, an
  API key) are common.
- **The Security framework via cgo** — rejected for the MVP. It removes the parsing entirely and
  is the honest long-term answer, but it costs cgo (cross-compilation, a heavier build) for a
  single-binary tool. Revisit if the parsing proves fragile.
- **Restrict secrets to printable ASCII and reject the rest at `set-secret`** — rejected. It
  makes the tool refuse legitimate credentials, and the refusal would arrive long after the user
  chose the password.

## Consequences

### Positive
- Any byte sequence round-trips, including PEM keys and accented passwords.
- A missing key is a named error, never an empty string silently injected into the environment.

### Negative / Tradeoffs
- **Parsing another tool's human-readable output**, which is not a contract and can change.
  Mitigated by the round-trip tests below; treat a format change as a spec change.
- The value transits **stderr** rather than stdout, so any code capturing stderr for logging
  must be certain it does not log this call. The `Get` implementation owns that buffer and must
  never hand it to a logger.

### Neutral
- No change to the `SecretSource` interface.

## Notes

- Verified against a throwaway keychain, never the user's default one. The tests do the same.
- The **write** side has a matching trap, handled in `SPEC-launcher.md`:
  `security add-generic-password -w <value>` puts the secret in `argv`. `set-secret` must not
  shell out that way.
