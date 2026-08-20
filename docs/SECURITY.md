<!-- generated-by: groundrules v1.10.0 -->
# Security & Compliance — mcp-remote-bridge

**Living** document of security and compliance (GDPR / privacy) choices.

For the **why** behind structural decisions → see `docs/decisions/`.
The normative rule is rule 3 of [`SPEC-primitive.md`](SPEC-primitive.md); this file is the
operational reading of it.

> **Status: pre-code.** These are commitments the implementation must satisfy, not
> properties yet observed in code. Turn each into a test.

## Threat model, briefly

This tool's whole purpose is to **take something that had no network port and give it
one**. That is the risk it introduces, and it is deliberate. Two consequences:

1. **The tunnel is the perimeter.** An exposed MCP is reachable by anyone who can reach the
   hostname. Authentication in front of it is the tunnel's job (Cloudflare Access / a
   Portals policy), not this tool's — but the tool must never make it *easy* to expose
   something with no policy in front of it.
2. **Secrets move from "local only" to "on a machine with a public hostname".** Hence rule
   3 below, which is stricter than it would need to be for a purely local tool.

## Authentication

- **Of the tool itself**: none — a local CLI run by the machine's own user.
- **Of the exposed endpoint**: delegated to the exposer. For the MVP, Cloudflare Tunnel
  plus whatever access policy the user has on the hostname (Cloudflare Access, a Portals
  policy). **The tool does not add authentication and must not pretend to.**
- **Of the tunnel to Cloudflare**: `cloudflared`'s own credentials, a **precondition** —
  the tunnel is created and authenticated before the tool runs, and the tool never handles
  those credentials.
- ⚠️ **An unprotected hostname is refused, not just reported — a Milestone 2 requirement.**
  Every `HealthReport` check goes *greener* when the endpoint is wide open, so nothing else in
  the tool catches this. The check reuses the probe's `initialize` request rather than adding a
  probe: sent **without credentials**, a success proves the door is open and `apply` fails
  (`--allow-public` overrides); an ambiguous result warns.
  **Never read a generic failure as "protected"** — a dead tunnel or an unpropagated DNS record
  fails exactly like a policy would. Only a positive authentication signature (a redirect to an
  IdP, a 403 from Cloudflare) may be read as guarded. Absence of a response is absence of
  evidence.
  → [ADR 0001](decisions/0001-doctor-flags-unprotected-hostname.md) (Accepted).

## Authorization / access control

- **Model**: none inside the tool. It is a single-user local CLI; there are no roles, no
  multi-tenancy, no resource matrix.
- **On the filesystem**: everything it writes lives under the invoking user's account
  (`~/Library/LaunchAgents/`, the config dir, the log path) with default user ownership.
- **Per-MCP access control** is the exposer's / gateway's concern, deliberately out of
  scope (see the "not a gateway" non-goal).

## Personal data (GDPR / privacy)

**The tool collects, stores and transmits no personal data of its own.** No telemetry, no
analytics, no phone-home, no crash reporting. Nothing leaves the machine except what the
user's own MCP traffic sends through their own tunnel.

- **Personal data collected**: none by the tool.
- **Legal basis / retention / user rights**: not applicable — there is no processing and
  no controller.
- **Processors / transfers outside the EU**: none introduced by the tool. Note that a user
  choosing Cloudflare Tunnel routes their own MCP traffic through Cloudflare; that is the
  user's choice of exposer, disclosed here rather than owned here.
- **Caveat worth stating**: some MCPs this tool exposes *do* handle personal or health data
  (`mcp-nightscout` is CGM data; `mcp-standardnotes` is private notes). The tool makes them
  network-reachable. That raises the stakes on the exposer's access policy — see the open
  question above.

## Secrets and configuration

**Rule 3 — the through-line of the project.** A real token leaked once because the simplest
way to supply it was the exposing way. The safe path is built first, before anything asks
for a secret.

Four hard invariants:

| Invariant | Enforced where |
|---|---|
| The config carries secret **references** (a `SecretSource` key), never values | config parsing — reject a value-shaped secret field |
| The service file (launchd plist, later a systemd unit) carries **no** cleartext secret — those files are world-readable | plist generation |
| The value is fetched **at launch time** by the launcher, immediately before `exec`, and injected into the process environment | the launcher (`__launch`, [ADR 0002](decisions/0002-launcher-is-a-hidden-subcommand.md)) |
| The proxy is invoked with `--pass-environment`, **never** `mcp-proxy -e KEY VALUE` — the latter puts the value in `argv`, where `ps` exposes it to every local account | launcher argument construction |
| The environment handed to the MCP is **constructed, not inherited**: `PATH`, `HOME`, the entry's declared variables, the resolved secrets, nothing else | the launcher |
| A referenced-but-absent secret **fails loudly at start** — never a proxy that 401s silently | `ensure_exposed` |

Consequently the value transits **neither the config, nor the service file, nor a command
line, nor a shell's environment**.

- **Reading**: `security find-generic-password -g`, never `-w` — `-w`'s output is lossy and
  ambiguous for any non-printable-ASCII byte, which would corrupt an accented password or a PEM
  key silently ([ADR 0004](decisions/0004-keychain-read-must-use-g-not-w.md)). The `-g` output
  carries the secret on **stderr**: whatever buffer captures it must never reach a logger.
- **Where secrets live**: the macOS keychain (MVP). Linux gets libsecret or a `600` env
  file behind the same `SecretSource` interface — deferred.
- **How a secret gets in**: `mcp-remote-bridge set-secret <key>`, reading from a **masked
  stdin prompt**. Never an argument (it would land in `argv` and shell history), never the
  environment.
- **`config.toml` is committable** — that is a designed property, not an accident. A user
  can put it in a dotfiles repo without leaking anything.
- **NEVER commit a secret.** See `.gitignore`.

### Things that must never appear in a log

Secret values, obviously — but also the full `argv` of the launched MCP, and any HTTP
request body from the `initialize` probe. Log the *shape* of a failure, not its payload.

The launcher's **error path** is the one people forget: a missing or unreadable secret is
reported by *key name*, never by value, and never by dumping the environment it had built.

## Attack surface and controls

- **Untrusted input — the config file.** User-authored, but `command`/`args` become an
  `exec` and `subdomain` becomes a DNS name. Validate: `name` and `subdomain` against a
  strict charset (they become a launchd label, a hostname and a log path); `port` in range;
  no shell interpolation anywhere — build `exec` argument vectors, never a command string.
- **Untrusted input — the network.** Whatever reaches the hostname reaches `mcp-proxy` and
  then the MCP's stdin. Not this tool's parser, but this tool is what put it in reach.
- **Path handling.** The log path and plist path derive from `name`. A `name` containing
  `/` or `..` must be rejected, not sanitized.
- **Encryption in transit**: TLS terminated by Cloudflare; the tunnel's own encrypted
  transport onward. The proxy itself binds **`127.0.0.1` only** — never `0.0.0.0`. That
  binding is a security control, not a default; assert it in a test.
- **Encryption at rest**: the keychain's. Nothing else is stored.
- **Logging and audit**: proxy logs at a known path, user-owned. No central audit trail.

## Incident / disclosure

- **A leaked secret** → `set-secret <key>` with a fresh value (rotate at the source first),
  then `restart <name>`. Because the value only ever lived in the keychain, rotation is the
  whole remediation — there is no config, plist, log or shell history to scrub. *That is
  the point of rule 3.*
- **An unintended exposure** → `remove <name>` tears down the ingress rule and DNS route;
  re-probe to confirm the hostname stops answering.
- **Reporting a vulnerability in this tool** → `<fill in: security contact / GitHub private
  advisory>`. Set this up before the first public release.
