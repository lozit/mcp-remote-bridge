- Judge a task on **its own** acceptance test, not on `go test ./...`: the loop's acceptance tests
  were committed deliberately red (commit ca696d2), so a whole-suite red was the expected state
  until the last backlog task landed. Verify a red is pre-existing (`git archive HEAD` into a
  scratch dir and re-run) before treating it as your regression. **Retired as of the
  `ProbeProxyListening` task** — the frozen batch is now fully green, so from here a red suite is a
  real regression until proven otherwise.
- A probe passes its acceptance test long before it is correct. `probe_test.go` is satisfiable by an
  implementation that dials every interface; what proved the loopback-only invariant was an
  out-of-test probe against a listener bound to a **LAN address only** (expect `OK=false`). Probe the
  inputs the frozen test does not cover — that is where a green-but-wrong diff hides.
