<!-- generated-by: groundrules v1.10.0 -->
# Learnings — mcp-remote-bridge

Rules learned from corrections and non-trivial discoveries during the project. Reverse-chronological order (newest at the top). **Re-read at session start.**

One entry = one **actionable rule**, not a journal note. Each entry has:
- a title that states the rule (imperative or "X: do Y");
- **Why** — the story behind it: what happened, what it cost (a revert, a lost CI cycle, a confused user…);
- **When to apply** — the concrete trigger conditions, so the rule fires at the right moment instead of being remembered too late.

Include the minimal code snippet / command when it is the fix.

---

## An acceptance test constrains less than it looks — probe outside it before calling a task done

**Why**: `internal/bridge/probe_test.go` was written to freeze the spec of
`ProbeProxyListening`, *including* the loopback-only invariant ("never `0.0.0.0`, never the
public hostname"). It does not actually prove that. All three of its cases dial a listener
that is itself bound to `127.0.0.1`, so an implementation dialling **every** interface would
have passed all three green. The gap was caught by the loop's verifier probing *outside* the
frozen test — a listener bound to a LAN address only, expecting `OK=false` — not by the test
that was supposed to be the gate.

Cost: nothing this time, because the verifier looked further than its instructions required.
That is luck, not process.

**When to apply**: whenever a test is the acceptance gate for a **security-shaped invariant**
— a bind address, an auth requirement, path containment, a charset that keeps input out of a
path. Ask what the test would still *accept*, not only what it rejects. A frozen acceptance
test is a floor, never a proof.

Corollary when writing one: prefer a case that fails on a **wrong** implementation, not only
on a **missing** one. `ValidateName("../../etc/passwd")` is that kind of case; a probe test
whose every fixture satisfies the invariant by construction is not.

<!-- Example:

## Palette changes: one mock screen first, then propagate

**Why**: a new primary color was propagated to all 7 prototypes before the user
saw it in context. Verdict: "revert it all" — one full commit/push/deploy cycle lost.

**When to apply**: any *substitutive* visual change (primary color, font, layout
overhaul). Apply on ONE representative screen, get a visual validation, then
propagate. Additive changes (a new utility class) are lower-risk.

-->
