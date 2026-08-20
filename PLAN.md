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

- [ ] `[supervised]` — *it is a decision; a loop would guess and commit.* **Resolve [ADR 0001](docs/decisions/0001-doctor-flags-unprotected-hostname.md)**:
      `doctor` flagging an exposed hostname with no access policy in front of it is a
      Milestone 2 requirement (at minimum a warning). The open sub-decision is **warn only**
      vs **refuse by default with `--allow-public`**. Also open: how detection actually works
      — probably an unauthenticated `initialize` from outside the tunnel, which is heuristic
      and where a false *negative* is the dangerous direction.

**Milestone 1 — the primitive** (see `docs/ROADMAP.md`)

- [ ] `[supervised]` — *`proxy_listening` was split out to the loop backlog; these three
      remain.* `hostname_resolves` and `hostname_responds` need real DNS/network (or an
      injected resolver, which is an unmade design decision); `service_loaded` delegates to
      `ServiceManager.Status`, so a loop would have to author its own fake — writer = maker.
      Remaining probes for `HealthReport`: `hostname_resolves`, `hostname_responds`,
      `service_loaded`
- [ ] `[supervised]` — *embedded decision: neither the MCP transport (SSE vs streamable
      HTTP) nor the protocol version is fixed by the spec.* **The `mcp_initialize` deep
      probe** — a real MCP `initialize` handshake through the hostname. Write this one
      *before* the happy path; it is what makes every later "it works" claim mean something
- [ ] `[supervised]` — *compound, and both halves fail the bar: the keychain lookup has a
      real machine side effect, and the launcher's shape (shell, injection mechanism) is an
      open design decision.* `KeychainSecretSource` + the generated launcher (fetch at
      `exec` time, inject into the environment). **Do this before anything needs a secret**
      — rule 3
- [ ] `[supervised]` — *`bootstrap`/`bootout`/`Status` drive real launchd state. Plist
      generation alone would be loop-safe (a pure function, testable against its output),
      but its `Program` field points at the launcher above, which does not exist yet — the
      unresolved dependency is the embedded decision. Revisit once the launcher is decided.*
      `LaunchdManager` — plist generation, `bootstrap` / `bootout`, `Status`
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
