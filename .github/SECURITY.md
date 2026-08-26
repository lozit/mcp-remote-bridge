# Security policy

## Reporting a vulnerability

**Please do not open a public issue for a security problem.**

Use GitHub's private reporting — the **Report a vulnerability** button under this repository's
[Security tab](https://github.com/lozit/mcp-remote-bridge/security/advisories/new). It creates a
private advisory only you and the maintainer can read, and it is the preferred channel because
the discussion, the fix and the disclosure all stay in one place.

If that is unavailable to you, email **guillaume.ferrari@protonmail.com** with `mcp-remote-bridge`
in the subject.

Expect an acknowledgement within **7 days**. This is a personal project maintained by one person,
not a product with an on-call rotation — the honest commitment is that reports are read and
answered, not that they are fixed within a fixed window. You will be told what is being done and
when, and credited in the advisory unless you ask otherwise.

## What is in scope

This tool wraps a local stdio MCP server into a supervised service and publishes it through a
tunnel. Its whole reason to exist is to get three things right, so a failure of any of them is a
vulnerability and not a bug report:

- **A secret reaching somewhere it can be read** — a config file, a launchd plist (they are
  world-readable), a command line, a process environment written to disk, or a log. Secrets are
  meant to travel as references and be resolved at launch, immediately before `exec`.
- **The proxy listening anywhere but `127.0.0.1`.** Binding the loopback is a security control
  here, not a default.
- **A published hostname reachable without credentials**, or the tool reporting a hostname as
  guarded when it is not. The health report is meant to record probes actually run; a check that
  cannot fail is worse than none, because it manufactures confidence.

Also in scope: privilege escalation through the generated service, and anything that makes the
tool destroy or overwrite infrastructure it did not create.

## What is not in scope

- **Cloudflare, `cloudflared`, `mcp-proxy`, and the MCP servers you point this at.** Report those
  to their own maintainers. If the flaw is in how *this* tool drives them, it is in scope.
- **The configuration you write.** Publishing an MCP server that exposes dangerous tools is
  working as designed; this tool is plumbing, and it does not filter what passes through it.
- Anything requiring an attacker who already has local access to your account. At that point they
  can read the keychain directly.

## Supported versions

Pre-1.0: only the latest tagged release is supported. There are no backports.

## Related

[`docs/SECURITY.md`](../docs/SECURITY.md) is the *design* document — the operational reading of
the secret-path rule, and the invariants the implementation must satisfy. It is not a reporting
policy, and it is not the place to send a vulnerability.
