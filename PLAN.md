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

- [x] The access-policy check — **done**. `CheckAccessPolicy` + `enforceAccessPolicy`, verified
      live on both hostnames. `allow_public` lives in the config, not a CLI flag, so the decision
      stays visible in a reviewable file rather than in one person's shell history.

**Milestone 1 — the primitive** (see `docs/ROADMAP.md`)

- [x] `service_loaded` and `hostname_responds` — **done**, both in `Probe`. The
      Loaded-but-not-Running question resolved itself: `Running` is derived from the presence of
      a pid, and the check reports the last exit code so a caller learns why it died.
- [~] `hostname_resolves` — split into `loop/backlog.md` as a `[loop]` task, with a red acceptance
      test frozen in `internal/bridge/resolve_test.go`. The "injected resolver" concern was
      unfounded: `localhost` and the reserved `.invalid` TLD make it testable without one.
- [x] Access service tokens + the `hostname_responds` probe — **implemented**. Still to verify:
      the two header names come from documentation, not measurement (the only such case in this
      codebase). The first live run against a guarded hostname is the verification.
- [x] The access-policy check of ADR 0001 — **done and verified live**: `hermes-mcp` reports
      `guarded` (403 + `cf-access-aud`), `freestyle-mcp` reports `open` (unauthenticated
      `initialize` succeeded). `apply` refuses the second unless `allow_public = true`.
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
- [x] `TestEnsureExposedRepairsDrift` stabilised — 10/10. The flake was not in the test but in
      `LaunchdManager.Ensure`: exit 5 means "already there", which is also what comes back when a
      label is on its way OUT after a concurrent bootout, and then nothing ends up loaded.
      `Ensure` now verifies the effect instead of trusting the exit code.
- [x] Negative DNS assertion strengthened — a positive control in the same run, and the error must
      unwrap to `*net.DNSError{IsNotFound: true}` so a timeout or a refused resolver no longer
      passes for an absent name.
- [x] Realize #3: two tasks partitioned to `[loop]` (`RetryCheck`, `ProbeHostnameResolves`) with
      red acceptance tests frozen; six PLAN entries found already done and checked off rather than
      partitioned (2026-08-21)
- [x] **The complete walking skeleton runs** (2026-08-21). `apply` on a throwaway entry against
      the real tunnel: service loaded, proxy listening, `mcp_responds` and `hostname_responds`
      both reaching `tools/list` through Access with service tokens. `remove` restored the
      configuration exactly — no Access application, DNS record or launchd service left behind.
- [~] Waiting for a freshly published hostname — split into `loop/backlog.md` as `RetryCheck`,
      with a red acceptance test frozen in `internal/bridge/retry_test.go`.
- [x] `RetryCheck` wired in — **done**. Only the access-policy verdict waits, and only in
      `apply`: `status` stays a fast, side-effect-free read. `PolicyUnknown` turned out to be the
      natural retry condition — it already means "no answer, or an answer that proves nothing",
      while Guarded and Open are both conclusions that end the wait.
- [x] `[infra]` reshaped: `account_id`, `zone_id`, `tunnel_id`, `api_token` (validated as a
      secret reference), `tunnel` removed.
- [ ] `[supervised]` — **bound `ProbeHostnameResolves` with an explicit DNS timeout.** It is the
      only probe in the package not bounded by one (`ProxyDialTimeout`, `MCPProbeTimeout`), so it
      inherits whatever the system resolver decides — and `health.go` states the house rule: a probe
      that can hang is a probe that never reports. Shape (`net.Resolver` + `context.WithTimeout`, no
      new parameter) is mechanical; the *value* is the judgement call, which is why the loop was not
      allowed to guess it.

- [ ] `[supervised]` — `doctor` checks the preconditions (connector running, token present, never
      tested with a write). The command does not exist yet, and its shape is a design choice.
- [x] `ensure_exposed` / `remove_exposed` — **done for the local half**: the walking skeleton
      runs end to end against real launchd, real mcp-proxy and a real keychain, with the Exposer
      left out. Remaining: wire the Exposer in once the Cloudflare credentials exist.
- [x] `remove_exposed` verified through a real hostname (2026-08-21): the tunnel configuration
      came back byte-identical, with no Access application, DNS record or launchd service left.
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

## Next — ADR 0007, the tool owns the Access configuration

- [ ] A `setup` or `doctor --fix` path that creates the service token once, storing its secret
      through the `SecretSource`. Deliberately NOT in `ensure_exposed`: the token is shared across
      entries, so creating it per-entry would make one token per MCP. The API returns the secret
      at creation, which is what makes this worth automating at all.
- [x] The Exposer creates the Access application on the hostname, reusing the policy named by
      `[infra] access_policy_id`. Guards **before** publishing and unguards **last** — both
      orderings mutation-tested, since they are the security property, not the API calls.
- [ ] `doctor` detects a Portal MCP server recorded as needing no authentication while its
      origin is guarded: that combination is a dead end (the field only exists at creation), so
      the fix is delete-and-recreate and the message must say it outright.
- [ ] `doctor` reports the Portal's MCP server configuration as an outstanding manual step —
      `/access/mcp_servers` answers `Unable to authenticate request`, so the route exists but is
      closed to API tokens while Portals are Beta. Re-test when the Beta lifts.
- [x] Probing with credentials after applying — **done and proven live**: `hostname_responds`
      reached `tools/list` through Access on a throwaway entry.

## Operational — the maintainer's own infrastructure, not this codebase

- [x] **`freestyle-mcp.paranoid.foo` is guarded** (2026-08-21). Access application created via
      the API reusing the existing `any_valid_service_token` policy, service token created with
      its secret going straight to the keychain, and the Portal server recreated declaring
      header-based authentication. Verified by measurement: `403` + `cf-access-aud`, the same
      signature as `hermes-mcp`.
- [x] `mcp-standardnotes` checked: **not affected**. It is served by `hermes-mcp.paranoid.foo`
      (port 8080), which was already guarded — the hostname name is simply misleading.
- [x] End-to-end confirmed: the server had to be **re-ticked in the Portal's server selection**
      after the delete-and-recreate. `status: ready` described the server, not the service.
- [ ] `doctor` should flag an entry whose hostname is guarded and whose Portal server is absent
      from the selection — the state is invisible from every screen except a client's tool call.

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
