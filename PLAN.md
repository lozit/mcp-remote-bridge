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
      `service_loaded`
- [ ] `[supervised]` — *remaining on the probe: attach Cloudflare Access credentials (the
      `decorate` hook exists and is unused), and exercise it through a real hostname rather than
      loopback.* The `mcp_responds` probe itself is **done**.
- [x] `__launch` + `internal/launcher` — **done**, with mutation-verified argv invariants.
- [x] `KeychainSecretSource` — **done** (ADR 0004: `-g`, not `-w`).
- [x] Config parser — **done** (ADR 0005). Strict TOML, every problem reported at once, secret
      references validated, subdomain/port collisions rejected.
- [x] `__launch` — **done**. Resolves secrets, builds a minimal explicit environment,
      `syscall.Exec`s mcp-proxy with `--pass-environment`. Proven end to end: the secret reaches
      the MCP's environment **and** appears in no process `argv` (checked with `ps`).
- [ ] `[supervised]` — the plist half of the secret path: assert no secret value appears in a
      generated plist. Arrives with plist generation.
- [ ] `[supervised]` — *`bootstrap`/`bootout`/`Status` drive real launchd state.*
      `LaunchdManager` — `bootstrap` / `bootout`, `Status`
- [ ] **Plist generation is now loop-safe** — the dependency that blocked it is resolved:
      `Program` is the binary's absolute path and `Args` are `__launch <name> --config <path>`
      ([`docs/SPEC-launcher.md`](docs/SPEC-launcher.md) fixes every key). A pure
      `ServiceSpec` → XML function, testable against its output, including the invariant that
      no secret appears in it. **Candidate for the next `/groundrules:realize`** — it needs a
      red acceptance test first.
- [ ] `[supervised]` — *real network and DNS side effects on a shared tunnel; not a
      bounded blast radius.* `CloudflaredExposer` — ingress rule + `cloudflared tunnel route
      dns`
- [ ] `[supervised]` — *cross-cutting: orchestrates all three seams.* `ensure_exposed` —
      the reconcile loop: detect what drifted, repair only that
- [ ] `[supervised]` — *cross-cutting, and its acceptance is a real teardown observed on a
      real hostname.* `remove_exposed` — verify it is the *exact* inverse (hostname stops
      answering)
- [ ] `[supervised]` — *the task **is** the oracle. A loop writing these would author its
      own grading criteria (writer = maker) and the back pressure disappears. These are the
      tests to write by hand, or in a reflection session — never in the loop.* Tests for the
      four secret invariants in `docs/SECURITY.md` (no value in the plist, in `argv`, in a
      log; absent secret fails at start)
- [x] Input validation: split into `loop/backlog.md` as a `[loop]` task, with a red
      acceptance test frozen in `internal/bridge/validate_test.go` (2026-08-20). Reject a
      `name` / `subdomain` containing `/`, `..`, or anything outside a strict charset — they
      become a launchd label, a hostname *and* a log path
- [ ] `[supervised]` — *same as above: the task is the oracle. Also premature — we do not
      yet own the code that passes a bind address to `mcp-proxy`; that arrives with the
      launcher.* Assert the proxy binds `127.0.0.1` only, never `0.0.0.0` — as a test, since
      it is a security control rather than a default

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
