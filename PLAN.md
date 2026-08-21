<!-- generated-by: groundrules v1.10.0 -->
# PLAN — mcp-remote-bridge

**Active** plan/todo for the project. Maintained by Claude during work.

This file differs from the long-term roadmap: it describes what is happening **now**.
For the trajectory → [`docs/ROADMAP.md`](docs/ROADMAP.md).

**Where things stand**: the skeleton compiles. Both specs are written
([`docs/SPEC-primitive.md`](docs/SPEC-primitive.md),
[`docs/SPEC-config-cli.md`](docs/SPEC-config-cli.md)), the module exists, and the three
seams plus `Entry` / `HealthReport` are declared with stubs returning
`bridge.ErrNotImplemented`. **No behaviour yet** — every stub is a Milestone 1 task below.

## In progress

- [ ] (nothing yet — pick the first task from "Up next")

## Up next

**Decision to settle** — blocks nothing today, must land before Milestone 2 ships

- [ ] `[supervised]` — *Milestone 2 work, unblocked now that ADR 0001 is Accepted.* Implement
      the access-policy check: send `initialize` **without credentials**; a success proves the
      door is open → `apply` fails unless `--allow-public`. Ambiguous → warn. Never read a
      generic failure as "protected" — only a positive auth signature (302 to an IdP, a 403
      from Cloudflare) counts. Requires Cloudflare Access **service-token support** in
      the probe first, or a correctly guarded user gets a permanently red `status`.
      Decide where `--allow-public` is recorded: a CLI flag vanishes into shell history, the
      config file keeps the choice visible.

**Milestone 1 — the primitive** (see `docs/ROADMAP.md`)

- [ ] `[supervised]` — *`proxy_listening` is **done** (delivered by the loop, 2026-08-20);
      these three remain.* `hostname_resolves` and `hostname_responds` need real DNS/network (or an
      injected resolver, which is an unmade design decision); `service_loaded` delegates to
      `ServiceManager.Status`, so a loop would have to author its own fake — writer = maker.
      Remaining probes for `HealthReport`: `hostname_resolves`, `hostname_responds`,
      `service_loaded`.
      **`service_loaded` is one decision away from being loop-safe**: it is structurally clean
      (call `ServiceManager.Status`, map to a `Check`, testable against a fake), but what should
      it report when the service is `Loaded` but not `Running` — crashed, or throttled after a
      failure? Decide that and it can be looped.
- [x] Access service tokens + the `hostname_responds` probe — **implemented**. Still to verify:
      the two header names come from documentation, not measurement (the only such case in this
      codebase). The first live run against a guarded hostname is the verification.
- [ ] `[supervised]` — the access-policy check of ADR 0001: probe the hostname **without**
      credentials and read the verdict from the signature. Both signatures are now measured and
      recorded in the ADR; this is mostly wiring.
- [x] `LaunchdManager` implemented against measured launchctl behaviour: bootstrap is **not**
      idempotent (rc 5 = "already there", despite saying "Input/output error"), rc 3 and rc 113
      are answers rather than failures, and `Running` must come from the pid because `state` is
      transiently `xpcproxy` right after bootstrap (2026-08-21)
- [x] Loop run #2: `BuildPlist` delivered and verified. The verifier went past the frozen test
      and loaded the plist into real launchd (`launchctl bootstrap` + `print`), which is the check
      that actually proves launchd accepts it. Follow-up found by probing the zero value:
      `ThrottleInterval` 0 disabled throttling — now refused (2026-08-21)
- [x] Realize #2: plist generation partitioned to `[loop]` with a red acceptance test frozen in
      `internal/launchd/plist_test.go`; one PLAN task found already done, one absorbed into that
      test (2026-08-21)
- [x] `__launch` + `internal/launcher` — **done**, with mutation-verified argv invariants.
- [x] `KeychainSecretSource` — **done** (ADR 0004: `-g`, not `-w`), read **and** write.
- [x] `set-secret` — **done**. Masked prompt, value piped to `security` stdin, never in `argv`.
      Known limit, deliberate: writes to the default keychain only, because `security` cannot
      combine a named keychain with a stdin read.
- [x] Config parser — **done** (ADR 0005). Strict TOML, every problem reported at once, secret
      references validated, subdomain/port collisions rejected.
- [x] `__launch` — **done**. Resolves secrets, builds a minimal explicit environment,
      `syscall.Exec`s mcp-proxy with `--pass-environment`. Proven end to end: the secret reaches
      the MCP's environment **and** appears in no process `argv` (checked with `ps`).
- [x] The plist half of the secret path — **absorbed into the frozen acceptance test** for
      plist generation. The useful assertion turned out not to be "no secret in the plist"
      (`ServiceSpec` carries none, so that check could never fail) but **exactly these seven
      keys and no others**: it is what catches a future `EnvironmentVariables` section, which is
      the natural place someone would put credentials in a world-readable file.
- [x] `LaunchdManager` — **done**. `Ensure` reconciles (no-op when unchanged, repairs a changed
      definition, reloads after an external unload), `Remove` is its idempotent inverse, `Status`
      reports an unknown label as not-loaded. Tested against real launchctl.
- [x] Plist generation — **done** by the loop, then hardened: a `ThrottleInterval` under 1s is
      now refused, since it rendered as `0` and disabled throttling (2026-08-21).
- [x] `CloudflaredExposer` — **implemented**, tested against a fixture captured from the real
      tunnel configuration, with mutation checks on the two costliest errors (appending after
      the catch-all, and dropping `warp-routing`).
- [x] Exposer exercised against the **real Cloudflare API** (2026-08-21): a throwaway hostname
      added, re-ensured (no write — confirmed by the `version` counter not moving), then removed,
      with the configuration byte-identical before and after and no DNS residue. All three seams
      have now touched reality.
- [ ] `[supervised]` — the **complete** walking skeleton: `EnsureExposed` with the Exposer wired
      in, verified through the public hostname rather than loopback. Needs the `mcp_responds`
      probe to carry Access credentials first if a policy guards the hostname.
- [ ] `[supervised]` — config: `[infra]` gains `account_id`, `zone_id`, `tunnel_id`, `api_token`
      and loses `tunnel`; the parser must validate `api_token` as a secret reference like any
      other. `doctor` checks the new preconditions (connector running, token present — never
      tested with a write).
- [x] `ensure_exposed` / `remove_exposed` — **done for the local half**: the walking skeleton
      runs end to end against real launchd, real mcp-proxy and a real keychain, with the Exposer
      left out. Remaining: wire the Exposer in once the Cloudflare credentials exist.
- [ ] `[supervised]` — `remove_exposed` through a **real hostname**: verify the public name
      stops answering, not just the local port. Needs the tunnel.
- [ ] `[supervised]` — *the task **is** the oracle. A loop writing these would author its
      own grading criteria (writer = maker) and the back pressure disappears. These are the
      tests to write by hand, or in a reflection session — never in the loop.* Tests for the
      four secret invariants in `docs/SECURITY.md` (no value in the plist, in `argv`, in a
      log; absent secret fails at start)
- [x] Input validation: split into `loop/backlog.md` as a `[loop]` task, with a red
      acceptance test frozen in `internal/bridge/validate_test.go` (2026-08-20). Reject a
      `name` / `subdomain` containing `/`, `..`, or anything outside a strict charset — they
      become a launchd label, a hostname *and* a log path
- [x] Assert the proxy binds `127.0.0.1` only, never `0.0.0.0` — **done**:
      `TestBuildBindsLoopbackExplicitly` in `internal/launcher/launcher_test.go` checks both the
      explicit `--host 127.0.0.1` and that no argument mentions `0.0.0.0`. The reason it was
      deferred (we did not yet own the code passing the bind address) went away with `__launch`.

## Ideas — to triage

Raw ideas, captured before they're lost (e.g. via `/groundrules:idea`). Not yet vetted.
Each gets triaged later → a **decision** (ADR), a **build** (PRD), a **milestone**
(ROADMAP), or dropped.

- [ ] Release toolchain: GoReleaser vs. a hand-rolled build matrix → ADR before Milestone 3
- [ ] macOS Gatekeeper: notarize, or document the `xattr` workaround? → ADR
- [ ] Homebrew tap — is there one for v0.1, and where does it live?
- [ ] Security contact / GitHub private advisory, before the repo gets any users

## Waiting / blocked

- [ ] Nothing blocked.

## Recently done

- [x] Config file + parser landed early: `__launch` and `apply` both needed it, so it moved off
      Milestone 2 onto the critical path. First dependency of the project, argued in ADR 0005
      together with the dependency policy (2026-08-21)
- [x] `mcp_responds` probe implemented, with an integration harness that runs a **real**
      mcp-proxy over a real stdio MCP (the test binary re-execs itself as the fixture), kills
      the MCP by PID and requires the probe to go red. Verified by mutation: reverting the probe
      to the spec's original initialize-only form makes that test fail. `ServiceSpec.KeepAlive`
      became a `KeepAlivePolicy` struct to match the real plists (2026-08-21)
- [x] Launcher specified: [ADR 0002](docs/decisions/0002-launcher-is-a-hidden-subcommand.md)
      (hidden `__launch` subcommand, not a generated shell script) and
      [`docs/SPEC-launcher.md`](docs/SPEC-launcher.md). Found while specifying it that
      `mcp-proxy -e KEY VALUE` puts secrets in `argv` — never used; `--pass-environment` is the
      only safe channel. `ServiceSpec` and `Entry` aligned on the spec (2026-08-20)
- [x] ADR 0001 resolved and Accepted: refuse on certainty, warn on ambiguity. Settling it
      surfaced that the check reuses the `initialize` request rather than adding a probe,
      and that the probe must authenticate or a correctly guarded user gets a red `status`
      (2026-08-20)
- [x] Loop run #1: both backlog tasks delivered and verified — `ValidateName` /
      `ValidateSubdomain` and `ProbeProxyListening`. Natural stop at iteration 2/4
      (`DONE: backlog empty`), no `blocked.md`, acceptance tests untouched, 32 tests green.
      Captured: a `docs/LEARNINGS.md` entry (an acceptance test constrains less than it looks)
      and a `docs/AGENT-EVALS.md` entry (attribution drift in a headless run) (2026-08-20)
- [x] Realize: partitioned Milestone 1 into 2 `[loop]` tasks (`loop/backlog.md`) and 11
      `[supervised]`, with red acceptance tests frozen in `internal/bridge/validate_test.go`
      and `internal/bridge/probe_test.go` (2026-08-20)
- [x] Compiling skeleton: `go mod init`, package layout (`cmd/` + `internal/{bridge,launchd,cloudflared,keychain}`),
      the three interfaces, `Entry` / `HealthReport` / `ServiceSpec` / `ServiceState`, and
      stubs returning `bridge.ErrNotImplemented`. `gofmt` / `go vet` / `go test` green (2026-08-20)
- [x] Project bootstrapped (2026-08-20)
- [x] `docs/SPEC-config-cli.md` — the config file and the CLI layer specified (commit 02010dc)
- [x] `docs/SPEC-primitive.md` — the `expose` primitive specified, project opened (commit 9065ca2)

---

**Convention**: Claude updates this file at the start/end of each session. Completed tasks
stay in "Recently done" for ~1 week then are archived (deleted or moved to CHANGELOG).

**Status vocabulary**: `[ ]` to do · `[~]` delivered, in review / awaiting validation ·
`[x]` done & validated. Annotate reverts and key commits inline (e.g.
`reverted (commit abc123)`) — intermediate states are information, don't erase them.
