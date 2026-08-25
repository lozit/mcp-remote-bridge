<!-- generated-by: groundrules v1.10.0 -->
# Loop backlog

The tasks the autonomous loop is allowed to execute. **The loop reads only this file** — never
`PLAN.md`. Put here **only loop-safe tasks**: atomic, isolatable, and **verifiable** (each names an
acceptance test or check the verifier can re-run). Anything that needs a human decision, exploration, or
a cross-cutting refactor stays in `PLAN.md` as `[supervised]`.

> **Hand-filled for now.** Once `/groundrules:realize` lands it will partition an approved plan into
> `[loop]` vs `[supervised]` and populate this file for you. Until then, add tasks by hand using the
> shape below.

## How to write a loop-safe task

Each task is one `- [ ]` line (the loop checks it off to `- [x]` on a verified PASS). State, crisply:
1. **what** to build (one atomic unit — no "and also"),
2. **where** (the file/module),
3. the **acceptance test** to run and what "green" means (the verifier re-runs it; no test → not
   loop-safe, keep it `[supervised]` in `PLAN.md`).

A task the loop can't verify its way out of, or that hides a decision, will be parked in
`loop/blocked.md` for you to triage — by design. See `loop/README.md` for the full contract.

## Tasks

- [x] **Implement `ValidateName` and `ValidateSubdomain` in `internal/bridge/validate.go`.**
      Acceptance test: `go test ./internal/bridge/ -run TestValidate` → exit 0 = green.
      Behaviour: return `nil` for a valid string, a non-nil `error` describing the violation
      otherwise. Valid means all of: length 1..63; characters `a-z`, `0-9` and `-` only
      (lowercase, no unicode); neither the first nor the last character is `-`. Anything else
      is **rejected, never sanitized** — a sanitized name silently addresses something other
      than what the user wrote, and the name becomes a service label, a hostname component
      *and* a log path. Both functions enforce the same rules.
      Out of scope: do not change `Entry` or any other type; do not call these from
      `EnsureExposed` yet; do not touch any file other than `validate.go`.

- [x] **Implement `ProbeProxyListening` in `internal/bridge/probe.go`.**
      Acceptance test: `go test ./internal/bridge/ -run TestProbeProxy` → exit 0 = green.
      Behaviour: dial TCP `127.0.0.1:<port>` bounded by `ProxyDialTimeout`, then close the
      connection. Return a `Check` with `Name: CheckProxyListening`; `OK` true when the dial
      succeeds and false otherwise; `Err` nil on success and carrying the dial error on
      failure; `Detail` naming the dialled address `127.0.0.1:<port>` **in both cases** — a
      red result that does not say where it looked is not actionable.
      Out of scope: never dial `0.0.0.0` or the public hostname (loopback only — a security
      control, not a default); do not implement the other probes; do not change `Check`,
      `HealthReport` or `ProxyDialTimeout`.

- [x] **Implement `BuildPlist` in `internal/launchd/plist.go`.**
      Acceptance test: `go test ./internal/launchd/` → exit 0 = green.
      Behaviour: render a `bridge.ServiceSpec` as a launchd XML property list. It must contain
      **exactly** these seven keys, no others: `Label` (string), `ProgramArguments` (array of
      strings: `spec.Program` first, then `spec.Args`), `RunAtLoad` (boolean true), `KeepAlive`
      (a **dictionary** — `SuccessfulExit: false` when `spec.KeepAlive.OnFailure`, `Crashed:
      true` when `spec.KeepAlive.OnCrash`; omit the whole key when the policy is zero, so a
      service is never supervised without being asked), `ThrottleInterval` (integer seconds
      from `spec.ThrottleInterval`), `StandardOutPath` and `StandardErrorPath` (strings).
      XML metacharacters (`& < > " '`) in any value must be escaped so the document survives a
      round trip. Return an error rather than a document for a spec that cannot be rendered
      honestly: empty `Label`, empty `Program`, or a `Program` that is not an absolute path.
      Out of scope: **never add an `EnvironmentVariables` key** — the plist is world-readable
      and secrets are resolved at launch time by `__launch` (ADR 0002); do not rewrite or
      inject arguments; do not add a TOML/plist library (a dependency needs an ADR); do not
      touch `bridge.ServiceSpec`, `KeepAlivePolicy`, or any file outside
      `internal/launchd/plist.go`.

- [ ] **Implement `RetryCheck` in `internal/bridge/retry.go`.**
      Acceptance test: `go test ./internal/bridge/ -run TestRetryCheck` → exit 0 = green.
      Behaviour: call `probe`, and while it returns a failing `Check`, sleep for `interval` via
      the injected `sleep` function and call it again, until it passes or the accumulated wait
      would exceed `timeout`. Return the **last** `Check` the probe produced — never a synthetic
      timeout error: a red result must keep the probe's own `Name`, `Detail` and `Err`, or it
      stops saying where it looked. `probe` is called **at least once**, even when `timeout` is
      zero, since a wait that never probes reports on nothing. A probe that passes on the first
      call must not sleep at all.
      Out of scope: do not call `time.Sleep` directly (the injected `sleep` is what makes the
      test instant and deterministic); do not add backoff, jitter or a context parameter; do not
      change `Check`, `HealthReport`, or wire this into `EnsureExposed` yet — that is a separate,
      supervised step; do not touch any file other than `retry.go`.

- [ ] **Implement `ProbeHostnameResolves` in `internal/bridge/resolve.go`.**
      Acceptance test: `go test ./internal/bridge/ -run TestProbeHostnameResolves` → exit 0 =
      green.
      Behaviour: look `hostname` up in DNS with the standard library. Return a `Check` with
      `Name: CheckHostnameResolves`; `OK` true when the lookup returns at least one address and
      false otherwise; `Err` nil on success and carrying the lookup error on failure; `Detail`
      naming the hostname that was looked up **in both cases** — a red result that does not say
      what it looked up is not actionable. An empty hostname is a caller bug: fail with an error
      that says so, rather than reporting it as a name that does not resolve.
      Out of scope: do not resolve through the Exposer or any Cloudflare API — this is a plain
      DNS lookup; do not add a resolver-injection parameter (the tests need none); do not
      implement any other probe; do not change `Check` or `HealthReport`; do not touch any file
      other than `resolve.go`.
