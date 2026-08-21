<!-- generated-by: groundrules v1.10.0 -->
# 0006 — The Exposer targets remotely-managed tunnels, via the Cloudflare API

**Date**: 2026-08-21
**Status**: Accepted

## Context

`SPEC-primitive.md` specified `CloudflaredExposer` as "adds an ingress rule +
`cloudflared tunnel route dns` to a named, already-authenticated tunnel". Measured on the
target machine, 2026-08-21, that does not work — and the reason is not the command.

Cloudflare tunnels come in two flavours:

- **Locally-managed** — created with `cloudflared tunnel login`, which writes an origin
  certificate to `~/.cloudflared/cert.pem`. Ingress rules live in a local `config.yml`, and
  `cloudflared tunnel route dns` creates the DNS record.
- **Remotely-managed** — installed with `sudo cloudflared service install <token>`. The
  connector runs from a token; **its ingress configuration lives in Cloudflare, not on disk**.
  This is the path Cloudflare's dashboard hands you today, and the one the reference guide
  (`mcp-standardnotes/docs/remote-agent-bridge.md` §2.1–2.2) documents.

The target machine is remotely-managed:

```
$ ls ~/.cloudflared/
No such file or directory
$ pgrep -fl cloudflared
/opt/homebrew/bin/cloudflared tunnel run --token-file /Library/Application Support/com.cloudflare.cloudflared/token
$ cloudflared tunnel list
ERR Cannot determine default origin certificate path. No file cert.pem in [...]
Error locating origin cert: client didn't specify origincert path
```

Even `tunnel list` fails, so `tunnel route dns` never had a chance.

**The deeper error**: the spec assumed an ingress rule is something you *write locally*. On a
remotely-managed tunnel there is no local config to write — `cloudflared` started from a token
ignores one. So this is not a command to swap out; the whole model of the Exposer was wrong for
this machine.

## Decision

**The MVP `Exposer` targets remotely-managed tunnels, through the Cloudflare API.** `Ensure`
becomes two API calls:

1. **Ingress** — `PUT /accounts/{account_id}/cfd_tunnel/{tunnel_id}/configurations`, adding an
   ingress entry mapping `subdomain.domain` to `http://localhost:PORT`.
2. **DNS** — a proxied `CNAME` for `subdomain.domain` pointing at
   `{tunnel_id}.cfargotunnel.com`.

`http://` and not `https://` for the origin, per the reference guide: that URL is the target
*inside* the machine, on loopback. Edge-to-client traffic is HTTPS with a Cloudflare
certificate regardless. Requiring TLS on loopback would mean generating and trusting a
self-signed certificate for zero security gain.

**The `Exposer` interface does not change.** `Ensure(subdomain, domain, localPort)` /
`Remove(subdomain, domain)` describe the *intent*, and the intent is identical in both models.
That is the seam earning its keep: locally-managed becomes a second implementation later, not a
rewrite.

**Locally-managed is out of the MVP** — not from preference, but because it cannot be tested
here: there is no `cert.pem` on this machine, so any locally-managed code path would be written
against an assumption. Three separate spec errors in this project were caused by exactly that,
each found by measurement (ADR 0003, ADR 0004, the `ThrottleInterval` zero value). Writing a
fourth deliberately is not a trade worth making.

## New preconditions and configuration

The preconditions change, and `doctor` must check the new ones:

| Was | Becomes |
|---|---|
| `cloudflared` installed, tunnel created **and authenticated** (`cert.pem`) | `cloudflared` installed **as a service from a token**, connector running |
| — | A **Cloudflare API token**, held in the `SecretSource` |
| `[infra] tunnel` (a name) | `[infra] account_id`, `zone_id`, `tunnel_id` |

`tunnel_id` (a UUID) replaces the tunnel *name*: the API addresses tunnels by id, and the DNS
target is `{tunnel_id}.cfargotunnel.com`.

## The security consequence, stated plainly

**This is the first credential the tool holds on its own behalf.** Until now it only read the
user's secrets to hand them to an MCP; it owned nothing. A Cloudflare API token can modify a
zone's DNS.

Therefore:

- The token is a `SecretSource` reference like any other — never a config value, never in
  `argv`, resolved when used. Rule 3 covers it, and it is the most important thing rule 3 now
  covers.
- **Least privilege is a documented requirement, not a suggestion**: `Zone:DNS:Edit` on the one
  zone, plus `Account:Cloudflare Tunnel:Edit`. **Never a Global API Key**, which is
  account-wide and cannot be scoped.
- `doctor` should report the token's presence, never its value, and should not "test" it with a
  write.

## Alternatives considered

- **Keep `cloudflared tunnel route dns`** — rejected: measured, it fails on this machine, and it
  cannot work for any token-installed connector.
- **Support both models in the MVP** — rejected. It doubles the Exposer's code and tests, and
  the locally-managed half would be unverifiable here. Wanted later, behind the same interface.
- **Shell out to `cloudflared` with `TUNNEL_ORIGIN_CERT`** — rejected: it requires obtaining a
  `cert.pem`, which means `cloudflared tunnel login` — an interactive browser flow, and one that
  converts the tunnel management model. The tool must not change how the user's tunnel is
  managed.
- **Have the user add hostnames in the dashboard by hand** — rejected: that is the manual
  procedure this project exists to replace, and it is precisely the step the reference guide
  spends its length on.

## Consequences

### Positive
- Works on the machine the project was written for, and on the default install path Cloudflare
  gives people today.
- No local tunnel config to write, so `Ensure` and `Remove` become symmetric API operations,
  which suits the reconcile model: read the current configuration, add or drop one entry, put it
  back.

### Negative / Tradeoffs
- **The tool now holds a powerful credential.** See above.
- **A read-modify-write on the tunnel configuration is a lost-update risk**: `PUT
  .../configurations` replaces the whole ingress list. Two concurrent `apply` runs, or an edit
  made in the dashboard between the read and the write, silently drops entries. The
  implementation must re-read immediately before writing and preserve entries it did not create;
  a future ADR may need optimistic concurrency if Cloudflare exposes an ETag.
- **Network dependency in `Ensure`**, so failures are now partly other people's outages.
  `HealthReport` should distinguish "the API refused us" from "the hostname is not answering".
- Locally-managed users are unsupported until the second implementation lands.

### Neutral
- The `Exposer` interface is untouched.

## The configuration shape, as measured (2026-08-21)

```json
"ingress": [
  {"service": "http://localhost:8080", "hostname": "a.example.com"},
  {"service": "http://localhost:8081", "hostname": "b.example.com", "originRequest": {}},
  {"service": "http_status:404"}
],
"warp-routing": {"enabled": false}
```

Three properties the read-modify-write must respect, none of them guessable:

1. **The catch-all is the entry with no `hostname`, and it must stay last.** A rule appended
   after it is never reached, because it matches everything.
2. **Entries are not uniform.** `originRequest` is present on one and absent on the other, so
   only the field being corrected may be touched. Parsing into a typed struct would silently
   drop what the struct does not know.
3. **`warp-routing` lives in the same `config` object.** A `PUT` sending only `ingress` erases
   it.

There is also a `version` counter in the response, and it is useful for more than concurrency:
it increments once per write, so it is an **independent measure of whether a run wrote
anything**. A live round trip took it from 2 to 4 — one `Ensure`, one `Remove`, and a repeated
`Ensure` that correctly wrote nothing. That is rule 1 confirmed from Cloudflare's side rather
than from our own code.

Whether it can serve as a compare-and-swap token is still unknown; until it is, the mitigation
stays "read immediately before writing".

## Notes

- Measured 2026-08-21 on the target machine. Reproduction: `cloudflared tunnel list` with no
  `~/.cloudflared/cert.pem`.
- Reference: `mcp-standardnotes/docs/remote-agent-bridge.md` §2.1–2.2 — the manual procedure
  this replaces, which documents the token/dashboard model.
- The `http://localhost:PORT` origin choice is explained in that guide, and repeated here so the
  reasoning survives without it.
