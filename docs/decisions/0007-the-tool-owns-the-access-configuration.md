<!-- generated-by: groundrules v1.10.0 -->
# 0007 — The tool owns the Access configuration, not just the plumbing

**Date**: 2026-08-21
**Status**: Accepted

## Context

The scope drawn in `SPEC-primitive.md` was: wrap a stdio MCP into a service, publish a hostname,
probe it. Guarding that hostname was left to the user as a precondition.

[ADR 0001](0001-doctor-flags-unprotected-hostname.md) then made `apply` **refuse** an entry whose
hostname answers without authentication. Both decisions are individually defensible and together
they produce something indefensible: **the tool refuses what it does not know how to fix.** It
creates the hostname, detects that it is open, and stops — handing back a half-built state and a
list of dashboard clicks.

That list is not short. Guarding one MCP by hand, measured on 2026-08-21:

1. create a service token — its secret is shown **once**, never again
2. create a Self-hosted Access application on the tunnel hostname
3. attach a policy to it
4. set header-based authentication on the MCP server **inside the Portal**

Steps 2 and 4 both present a field labelled *authentication*, in two different sections of the
dashboard, and they mean different things: one is who may reach the hostname, the other is what
the Portal presents when it reaches it. Step 4's field **does not appear** until step 2 is done,
because the Portal only offers to authenticate once the origin starts refusing it — so the order
is discoverable only by breaking something first.

The maintainer, who built this infrastructure by hand and knows it, spent twenty minutes stuck on
it. That is the measure of the problem, and it is a better argument than the 489-line guide the
project was founded on: the guide was long, this is *confusing*, which is worse.

## What the API allows, measured

| Operation | Endpoint | Verdict |
|---|---|---|
| Create a service token | `POST /accounts/{id}/access/service_tokens` | **works** — and returns the `client_secret`, which the dashboard shows only once |
| Create an Access application | `POST /accounts/{id}/access/apps` | **works** (`type: self_hosted`, `domain`, `policies`) |
| Reuse an existing policy | `GET /accounts/{id}/access/policies` | **works** — the existing policies are `any_valid_service_token`, so a new token is accepted with no policy change |
| Configure the Portal's MCP server | `/accounts/{id}/access/mcp_servers` | **refused**: `Unable to authenticate request` — the route exists (it is not `Could not route`), but no token permission covers it. MCP Portals are Beta. |

The secret-returning behaviour of the first row is worth stating on its own: **the API path is
strictly better than the dashboard path**, because the dashboard's one-time display is what makes
a lost secret unrecoverable.

## Decision

**The tool takes ownership of guarding the hostnames it creates.** `ensure_exposed` gains the
Access configuration as part of publishing an entry, rather than as a precondition the user must
satisfy first:

- ensure a service token exists, creating one if needed and storing its secret through the
  existing `SecretSource` — never through the terminal
- ensure an Access application exists on the entry's hostname, with a policy accepting that
  service token
- **the Portal's MCP server configuration remains manual**, until Cloudflare opens that route to
  API tokens. `doctor` reports it as an outstanding step rather than the tool pretending it is
  done.

This makes ADR 0001's refusal coherent: the tool refuses an open hostname because it can now
close it, not instead of closing it.

**The `Exposer` interface does not change.** Guarding a hostname is part of exposing it, which is
what `Ensure(subdomain, domain, port)` already means. The seam contract stays; only the
implementation grows. (`SPEC-primitive.md`'s three seams are unchanged, so no separate ADR.)

## Alternatives considered

- **Leave it as a precondition and document it well** — rejected. That is what the reference
  guide already does, and it is the failure mode: its line 258 calls an unauthenticated origin
  *"that's fine"*, and line 429 offers the open hostname as *"simpler but less safe"*. Automating
  a guide that documents the unsafe path as acceptable would reproduce the hole at scale, once
  per `apply`.
- **Drop ADR 0001's refusal instead** — rejected, and it is the tempting symmetry. It would
  restore coherence by lowering the bar: the tool would publish unguarded hostnames silently,
  which is exactly what happened here in production.
- **Only warn, and print the dashboard steps** — rejected as the end state, though it is what the
  Portal step must remain for now. A warning that always fires becomes a warning nobody reads.
- **Require the user to pre-create a service token** — rejected. It is the step whose secret is
  unrecoverable, so it is precisely the one worth automating.

## Consequences

### Positive
- Publishing an MCP becomes one command that ends in a guarded, probed hostname.
- The service-token secret never transits a terminal or a clipboard.
- Reusing existing policies (`any_valid_service_token`) means the tool adds no policy of its own
  to an account that already has six identical ones.

### Negative / Tradeoffs
- **The API token needs more permissions**: `Access: Apps and Policies → Edit` and
  `Access: Service Tokens → Edit`, on top of DNS and Tunnel. The credential the tool holds gets
  strictly more powerful, and `docs/SECURITY.md` must say so. Least privilege still applies —
  scoped to one account and one zone — but the blast radius of a leak grows from "DNS and tunnel
  routing" to "who may reach these applications".
- **The tool can now break access**, not just fail to create it. A wrong policy locks out a
  working MCP. Mitigation: reuse the account's existing policy rather than authoring one, and
  probe with credentials after applying — a green `hostname_responds` is the proof the
  configuration works, not the fact that the API accepted it.
- **The Portal step stays manual**, so the flow is not yet one command. Being honest about that
  in `doctor` matters more than appearing complete.
- More surface tied to Cloudflare specifics, in a codebase built around swappable exposers. It
  lives inside the `CloudflaredExposer`, where that knowledge belongs.

### Neutral
- No change to the three seam interfaces.

## Notes

- Reopens the "Portals registration is out of the primitive at first" line in
  `SPEC-primitive.md` § Resolved decisions. That decision was taken before it was known that the
  target infrastructure depends on a Portal structurally: the Portal reaches each MCP **through
  the tunnel hostname** (`HTTP URL = https://<host>/sse`), so hostnames and Portal are not
  alternatives — one is the origin of the other.
- The measurements behind this ADR are in `docs/LEARNINGS.md` and in ADR 0001.
