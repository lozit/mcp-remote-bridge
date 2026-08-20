<!-- generated-by: groundrules v1.10.0 -->
# 0003 — The liveness probe must carry data back from the MCP

**Date**: 2026-08-21
**Status**: Accepted

## Context

[`SPEC-primitive.md`](../SPEC-primitive.md) named the subtle trap — *the proxy can be up while
the MCP inside it is dead* — and answered it with a probe going "all the way to a real MCP
`initialize` handshake, not just the port is listening".

Measured against `mcp-proxy 0.12.0` on 2026-08-21, that answer does not work. A minimal stdio
MCP was wrapped by `mcp-proxy --host 127.0.0.1 --port 9125`, its child process was killed with
`kill -9` (the proxy left running), and the same requests were replayed:

| Probe | MCP alive | MCP killed | Discriminates? |
|---|---|---|---|
| TCP connect to `127.0.0.1:PORT` | ✓ | ✓ | no |
| `POST /mcp` → `initialize` | `result` | **`result`, complete, in 3 ms** | **no** |
| `POST /mcp` → `ping` | `result {}` | **`result {}`** | **no** |
| `POST /mcp` → `tools/list` | `result` | `error {code:0, message:""}` | **yes** |

`mcp-proxy` answers `initialize` from the state it negotiated at startup — the request never
reaches the child. Ten seconds after the process died, the handshake still reported
`serverInfo.name = "fake-mcp"`.

Worse, **`ping` does not discriminate either**. The method the MCP protocol defines for
liveness is served by the proxy. It is the more dangerous of the two, because its name promises
exactly what it does not deliver: a reviewer reading `ping` in a health check has no reason to
doubt it.

**A third trap sits underneath both**: `tools/list` against a dead MCP returns **HTTP 200**. The
failure lives in the JSON-RPC body, not the transport status. A probe checking the HTTP code is
fooled a third time.

The spec was therefore about to specify the exact defect its own rule 2 forbids — a health check
that cannot fail, manufacturing confidence.

## Decision

**The liveness probe is a JSON-RPC call that must carry data back from the MCP process, and its
verdict is read from the response body, never from the HTTP status.**

Concretely, against `POST http://127.0.0.1:<port>/mcp`:

1. `initialize` — establishes the session, and yields the `mcp-session-id` header. **Its success
   proves nothing about the MCP** and must never be reported as if it did.
2. `notifications/initialized` — required by the protocol; answers `202`.
3. **`tools/list`** with the `mcp-session-id` header — the probe. **Green only when the body
   contains a `result` member; red whenever it contains an `error` member**, whatever the HTTP
   status.

The check is renamed `mcp_initialize` → **`mcp_responds`**. Keeping the old name would preserve
the exact confusion that caused this: a name asserting a handshake, while the thing being
verified is that the MCP answered.

## Alternatives considered

- **Keep `initialize`** — rejected: measured, it does not reach the MCP.
- **`ping`** — rejected: measured, it does not reach the MCP either. Tempting because the
  protocol advertises it for liveness; that is precisely what makes it a trap.
- **Trust the HTTP status** — rejected: `200` on a dead MCP.
- **Watch the child process instead of probing** (`ServiceState.PID` and its children) —
  rejected as the primary check. It proves a process exists, not that it answers, which is the
  same class of error one level down. Useful as *evidence in a failure message*, not as the
  verdict.
- **Call an actual tool** — rejected: side effects. `tools/list` is the cheapest call that still
  has to traverse to the MCP.

## Consequences

### Positive
- The probe can actually fail, which is the only property that makes a green result worth
  anything.
- The three stacked traps (open port, proxy-answered handshake, HTTP 200) are documented with
  the measurement that exposed each.

### Negative / Tradeoffs
- **`tools/list` assumes the MCP exposes tools.** An MCP declaring only `resources` or `prompts`
  may legitimately answer an error, which this probe would read as dead. Measured detail:
  `mcp-proxy` answers `-32601 Method not found` for a capability the server did not declare at
  `initialize`, so **the capabilities from the `initialize` response tell us which list method
  to call**. Implement it that way — pick `tools/list`, `resources/list` or `prompts/list` from
  the declared capabilities — and fail loudly if the MCP declares none.
- **The probe is now a three-request sequence with session state**, not one call. Slower, more
  code, more to get wrong.
- **It is coupled to mcp-proxy's observed behaviour**, which is not contractual and can change
  between versions. Mitigation: this ADR records the measurement and its date; re-run it when
  bumping the pinned mcp-proxy version, and treat a change as a spec change.
- The error returned for a dead MCP is empty (`code: 0, message: ""`), so it carries no
  diagnosis. `Detail` must say "the MCP did not answer", not relay an empty string.

### Neutral
- No change to the three seam interfaces.

## Notes

- Measured on macOS with `mcp-proxy 0.12.0`, 2026-08-21. Reproduction: wrap any stdio MCP, kill
  the child by PID (**not** `pkill -f <script>` — the proxy's own argv contains the script path,
  so that kills both and proves nothing), then replay the three methods.
- The endpoint is `/mcp` (streamable HTTP). `/sse` also exists but would need a long-lived
  stream plus a separate POST — more moving parts for the same answer.
- Related: rule 2 in `SPEC-primitive.md`, which this ADR is an application of, against the
  spec's own first attempt at implementing it.
