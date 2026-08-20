<!-- generated-by: groundrules v1.10.0 -->
# 0001 — `doctor` must flag an exposed hostname with no access policy

**Date**: 2026-08-20
**Status**: Accepted

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
`mcp_responds`, `hostname_resolves`, `hostname_responds`, `service_loaded` — goes *greener*
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

**Sub-decision, resolved 2026-08-20: refuse when certain, warn when ambiguous.**

- **Certain** — an unauthenticated MCP `initialize` **succeeds**. That is not a heuristic: the
  endpoint demonstrably serves anyone. `apply` **fails**, with `--allow-public` as the explicit
  override for a deliberately public MCP. The safe path becomes the default path, exactly as
  rule 3 does for secrets.
- **Ambiguous** — anything else. `doctor` and `status` warn; nothing is blocked.

The asymmetry is the point. Refusing on certainty costs a legitimately-public user one flag,
once. Refusing on ambiguity would block on a broken tunnel or an unpropagated DNS record —
turning a security control into an availability problem, which is how security controls get
switched off.

### How detection works

The check is **not a separate probe**. With an access policy in front of a hostname, an
unauthenticated request is redirected to the identity provider (302 to
`<team>.cloudflareaccess.com`) or refused (403). With no policy, it reaches the proxy and the
MCP answers. So the signal is the `initialize` **request**, sent twice:

| With credentials | Without credentials | Verdict |
|---|---|---|
| ✓ | ✗ *with an auth signature* | plumbing healthy, door guarded |
| ✓ | ✓ | plumbing healthy, **door open** → refuse |
| ✓ | ✗ *no auth signature* | ambiguous → warn |

**Never conclude "protected" from a generic failure.** A dead tunnel, an unpropagated DNS
record or a crashed proxy make the unauthenticated request fail exactly as a policy would, and
would then read as "secure". Only a **positive authentication signature** — a redirect to an
IdP, a 403 carrying Cloudflare's headers — may be read as "guarded". Absence of a response is
absence of evidence.

### Consequence the original draft missed

The probe must be able to **authenticate** (a Cloudflare Access service token). Without
that, a user who has done the right thing — a policy in front of their MCP — gets a permanently
red `status`, because the deep probe eats the 302. Access support is therefore not a
nice-to-have for Milestone 2; it is what makes the probe work at all for a correctly configured
user. Its absence would have punished exactly the users this ADR is written to protect.

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
- **Only the "door open" verdict is certain.** A successful unauthenticated `initialize`
  proves openness; nothing proves protection. "Guarded" is inferred from an auth signature and
  stays exposer-specific and best-effort — document it as such, never as a guarantee.
- **Adds scope to the MVP**, including Cloudflare Access service-token support in the probe.
- **`--allow-public` is a permanent escape hatch.** Once a user passes it, nothing warns again
  for that entry. Prefer recording it in the config over a bare CLI flag, so the choice is
  visible in the file rather than in one person's shell history.
- Couples `doctor` slightly to Cloudflare specifics, in a codebase built to avoid exactly
  that. Keep the coupling inside the `Exposer` implementation.

### Neutral
- Does not change the primitive's contract or any of the three seams.

## Notes

- Raised in `docs/SECURITY.md` → "Authentication" as an open question, then promoted here.
- **Still valid after [ADR 0003](0003-liveness-probe-must-carry-data.md)**, which found that
  `initialize` does not reach the MCP. That does not weaken this ADR: the access-policy check
  asks whether an unauthenticated HTTP request *crosses the policy*, not whether the MCP is
  alive. `initialize` answers that perfectly. Liveness needs a different call — two distinct
  questions over the same connection. The check formerly written `mcp_initialize` is now
  `mcp_responds`; the request this ADR relies on is still `initialize`.
- Milestone 2 in `docs/ROADMAP.md` carries the requirement.
- Resolved 2026-08-20. The sub-decision turned on a finding made while settling it: the check
  reuses the `initialize` request rather than adding a probe, which removes the
  "heuristic" objection that had argued for warn-only.
- Related: load-bearing rule 2 (`docs/SPEC-primitive.md`) — a check that cannot fail
  manufactures confidence; a green table over an open endpoint is the same defect wearing a
  different hat.
