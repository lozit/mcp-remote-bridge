# The service token the estate no longer has — are `access_client_id` / `access_client_secret` still required?

> From the operator's cockpit, a measurement of 2026-09-07 triaged on 2026-09-24. Recorded as
> received; the answer belongs in `docs/` or an ADR, not here.

## What was measured

Filling the `FILL_ME` fields of the operator's own `config.toml` (kept in his dotfiles, commit
`e47d158`): *Service credentials* is empty on the Cloudflare side, by a decision of 2026-08-28 —
the estate guards its hostnames with an IP-bypass policy plus one-time codes by e-mail, and holds
**no** Access service token. `access_client_id` and `access_client_secret` therefore cannot be
filled. The four other identifiers are written.

## The question for the project

Does the bridge still need a service token anywhere — `doctor`, a *Service Auth* policy, the
final `tools/list` probe of `apply` — or can these two fields become **optional**, the guarding
policy being whatever the operator already runs? Concretely:

- if the probe needs to pass Access, it can do so from a bypassed IP without a token; the config
  should say so rather than demand a value that no longer exists;
- if `setup` "creates the Access service token, once" (README), that step is the one that
  contradicts an estate that decided against service tokens — make it opt-in, or make the
  hostname guard pluggable (service token *or* an existing policy id).

**Do not mint a token to fill a field.** The right outcome is a config that validates without the
two fields when `access_policy_id` names a policy that already admits the caller, and `doctor`
reporting which mode is in effect.

## How we will know it is done

`mcp-remote-bridge doctor` and `apply` run green on a config with no `access_client_*` fields and
an existing policy; the README's config example shows both modes; the change is in `CHANGELOG.md`.
