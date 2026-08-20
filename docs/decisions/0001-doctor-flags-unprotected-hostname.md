<!-- generated-by: groundrules v1.10.0 -->
# 0001 — `doctor` must flag an exposed hostname with no access policy

**Date**: 2026-08-20
**Status**: Proposed

## Context

The tool's entire purpose is to take something that had no network port and give it one.
The exposed MCPs are not neutral payloads:

- `mcp-nightscout` fronts **CGM data** — continuous glucose monitoring, i.e. health data.
- `mcp-standardnotes` fronts **private notes**.

The primitive deliberately does **not** add authentication: that is the exposer's job
(Cloudflare Access, a Portals policy). See `docs/SECURITY.md` → "the tunnel is the
perimeter".

The gap that follows: **a successful `apply` and a green `status` say nothing about whether
anything guards the hostname.** Every check in `HealthReport` — `proxy_listening`,
`mcp_initialize`, `hostname_resolves`, `hostname_responds`, `service_loaded` — goes *greener*
when the endpoint is wide open. A user who forgets the access policy gets an all-green table
confirming their private notes are reachable by anyone who knows the URL.

That is the loudest foot-gun the tool can hand someone, and it is one the tool is uniquely
positioned to notice: it knows the hostname, and it just proved the hostname answers.

This also sits badly with load-bearing rule 2. "Verify the effect, never trust the write"
was written against the failure mode of *manufactured confidence*; an all-green report on an
unprotected health-data endpoint is exactly that.

## Decision

`doctor` must detect an exposed hostname that answers **without any access policy in front
of it**, and surface it. This becomes a **Milestone 2 (MVP) requirement**, not a
post-MVP nice-to-have — shipping a v0.1 that can silently expose CGM data is not acceptable.

**Open sub-decision — warn or refuse?** This ADR commits to *at minimum a warning*. Whether
the tool goes further is unresolved:

- **Warn only** — `doctor` reports it loudly; `apply` proceeds. Respects the user's machine
  and the "plumbing, not a gateway" non-goal. Risk: warnings get skimmed, and this one is
  most likely to appear on the very first run when everything else is red anyway.
- **Refuse by default, `--allow-public` to override** — `apply` fails on an unprotected
  hostname unless the user says otherwise. Turns the safe path into the default path,
  matching how rule 3 treats secrets. Risk: breaks a legitimate use case (a deliberately
  public MCP), and edges toward the tool having an opinion about access control.

Resolve this sub-decision before Milestone 2 ships; amend this ADR with the outcome.

## Alternatives considered

- **Do nothing; document it in the README** — rejected. The information ("this hostname is
  unprotected") is available to the tool at probe time and nowhere else at that moment.
  Pushing it into prose that gets read once, before the mistake, is how the manual guide
  failed in the first place.
- **Add authentication in the tool** — rejected. It contradicts "plumbing, not a gateway",
  and a hand-rolled auth layer in front of an MCP would be worse than Cloudflare Access.
  The tool should *notice* the absence, not *fill* it.
- **Make it a `HealthReport` check (`hostname_protected`) folded into `healthy`** — rejected
  **for now**, and worth revisiting. It is the cleanest home conceptually, but it would make
  a legitimately-public MCP report unhealthy forever, and `healthy` currently means "the
  plumbing works", which is a useful, narrow meaning. `doctor` — which already checks
  *preconditions and posture* rather than entries — is the better fit.
- **Defer to post-MVP** — rejected. That is precisely the decision that leaves the first
  real users exposed.

## Consequences

### Positive
- The one failure mode with a real-world blast radius (health data, private notes) is
  caught by the tool that created it.
- `doctor` gains a clear identity: preconditions **and** posture, versus `status`'s "does
  the plumbing work".

### Negative / Tradeoffs
- **Detection is not clean.** "Is there a policy in front of this?" has no reliable
  general answer over HTTP. Probable approach: an unauthenticated request from outside the
  tunnel — if the MCP `initialize` handshake succeeds with no credentials, nothing is
  guarding it. That is a genuine signal but it is exposer-specific and heuristic, and a
  false *negative* here is dangerous. It must be documented as best-effort, never phrased
  as a guarantee.
- Adds scope to the MVP.
- Couples `doctor` slightly to Cloudflare specifics, in a codebase built to avoid exactly
  that. Keep the coupling inside the `Exposer` implementation.

### Neutral
- Does not change the primitive's contract or any of the three seams.

## Notes

- Raised in `docs/SECURITY.md` → "Authentication" as an open question, then promoted here.
- Milestone 2 in `docs/ROADMAP.md` carries the requirement.
- Related: load-bearing rule 2 (`docs/SPEC-primitive.md`) — a check that cannot fail
  manufactures confidence; a green table over an open endpoint is the same defect wearing a
  different hat.
