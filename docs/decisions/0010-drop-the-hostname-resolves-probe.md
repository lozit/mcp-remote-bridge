# 0010 — Drop the `hostname_resolves` probe

**Date**: 2026-08-26
**Status**: Accepted

## Context

The v0.1.0 release check surfaced that `ProbeHostnameResolves` is a tautology on a DNS zone
carrying a wildcard record. Measured on `paranoid.foo`: `relcheck-mcp`,
`ceci-nexiste-pas-du-tout-42` and `xyzzy-quux` all resolved to the same address before anything
was published. The probe would report *resolves* for a hostname the tool has never created —
evidence of nothing, which is the shape rule 2 forbids: *a check that cannot fail is worse than
none, because it manufactures confidence.*

Two facts found while scoping the fix change what the decision is about.

**It is not wired in.** `Bridge.Probe` produces `service_loaded`, `proxy_listening`,
`mcp_responds` and `hostname_responds`. `ProbeHostnameResolves` has no caller outside its own
test. It was built by the autonomous loop from a backlog task, tested, and never connected. So
nothing shipped is misleading, and this is a decision about whether to *add* a check, not about
repairing one in use.

**Its stated purpose is already served.** The probe exists "to separate two failures that
otherwise look alike: a name that does not resolve at all, and one that resolves but does not
answer". But `probeHostname` passes the deep probe's error through unchanged, and a name with no
record surfaces there as `no such host` — plainly distinct from a connection or HTTP failure.
The separation the probe was written to provide is already in the report.

## Decision

Delete `ProbeHostnameResolves`, `CheckHostnameResolves` and their tests. The health report keeps
the four checks it actually runs.

The design that *would* make the check honest is recorded below, so that if a future need
appears it is not re-derived from scratch.

## Alternatives considered

- **Wire it in as it stands**: rejected outright. It would introduce the exact failure mode this
  project exists to prevent, and it would do so on the maintainer's own zone.
- **Discriminate with a control lookup** — resolve a name under the same parent that certainly
  has no record, and treat the target as *not published* when it answers with the same addresses
  as that control. This is the honest version: it stays plain DNS, needs no credentials, and
  turns the tautology into a check that can fail. It is the technique already used elsewhere in
  this project (a negative control in the same run). Rejected **for now** on cost, not on
  correctness: it needs the parent domain passed in — a signature change to a probe nothing
  calls — to buy a distinction the deep probe's error already makes. Worth revisiting the day
  `hostname_resolves` earns a caller.
- **Ask the Cloudflare API whether the record exists**: rejected. It is authoritative, but it
  asks the same control plane that wrote the record whether the write happened, which is close
  to trusting the write; and it would make a health probe depend on credentials and on the
  Exposer's seam.
- **Detect the record by its CNAME to `<tunnel-id>.cfargotunnel.com`**: does not work. Cloudflare
  proxied records answer with edge A records, so the CNAME is not visible from outside.

## Consequences

### Positive
- No check in the report that cannot fail.
- Less code, and one fewer probe to keep honest. The four remaining checks are all wired,
  exercised end to end against a real tunnel, and load-bearing.
- `DNSLookupTimeout` goes with it — a bound on a lookup nothing performs.

### Negative / Tradeoffs
- Loses a *cheap* pre-check. `hostname_responds` retries against a settle window, so a hostname
  that simply does not exist costs more to diagnose than one DNS lookup would. Accepted: it is
  seconds on a failure path, and the error still names the cause.
- Deletes work the loop produced and a frozen acceptance test with it. That is the right
  outcome — the test froze a specification that turned out to be wrong on wildcard zones — but
  it is worth stating plainly rather than quietly dropping the file.

### Neutral
- Re-adding it later is cheap, and this ADR carries the design to use.

## Notes

- Measured 2026-08-26 during the v0.1.0 release check; recorded in `docs/LEARNINGS.md` as
  *"A wildcard DNS record makes `hostname_resolves` a check that cannot fail"*.
- The CHANGELOG for 0.1.0 lists the wildcard behaviour as a known limitation. If this ADR is
  accepted, that entry should say the probe was removed rather than fixed.
