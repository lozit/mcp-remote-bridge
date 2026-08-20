<!-- generated-by: groundrules v1.10.0 -->
# Learnings — mcp-remote-bridge

Rules learned from corrections and non-trivial discoveries during the project. Reverse-chronological order (newest at the top). **Re-read at session start.**

One entry = one **actionable rule**, not a journal note. Each entry has:
- a title that states the rule (imperative or "X: do Y");
- **Why** — the story behind it: what happened, what it cost (a revert, a lost CI cycle, a confused user…);
- **When to apply** — the concrete trigger conditions, so the rule fires at the right moment instead of being remembered too late.

Include the minimal code snippet / command when it is the fix.

---

## Verify an external tool's behaviour by killing something, not by reading its docs

**Why**: `SPEC-primitive.md` named the trap "the proxy can be up while the MCP inside it is
dead" and answered it with a probe driving "a real MCP `initialize` handshake". Measured
against `mcp-proxy 0.12.0` — wrap an MCP, `kill -9` the child, replay the requests — the
answer was wrong: **the proxy answers `initialize` itself**, from the state negotiated at
startup, and still reported `serverInfo.name` ten seconds after the process died.

`ping` — the method the MCP protocol advertises *for liveness* — is answered by the proxy too.
It is the worse trap: its name disarms the reader, so it would have shipped unquestioned.
Underneath both, `tools/list` against a dead MCP returns **HTTP 200**; the failure is only in
the JSON-RPC body.

Cost: nothing, because the check happened before implementation. Had it not, the project would
have shipped the exact defect its own rule 2 forbids — a health check that cannot fail — while
its README promised the opposite. Three layers of plausible-sounding verification, all
non-verifying.

**When to apply**: before specifying **any** check that depends on an external process
answering — a proxy, a sidecar, a gateway, a connection pool. Do not reason about what the
layer "should" forward. Break the thing on purpose and watch: kill the child, stop the
upstream, revoke the token, then run the check and require it to go red. A check never observed
failing is a check that has not been verified.

Two traps met in the doing, worth stealing:
- `pkill -f <script>` killed the proxy too, because the proxy's own `argv` contains the script
  path. Kill the child by PID.
- The first run of a negative test needs a **positive control**. "It failed when I broke it" is
  worthless without "it passed when I hadn't."

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
