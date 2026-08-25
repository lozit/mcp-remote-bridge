- Judge a task on **its own** acceptance test, not on `go test ./...`: the loop's acceptance tests
  were committed deliberately red (commit ca696d2), so a whole-suite red was the expected state
  until the last backlog task landed. Verify a red is pre-existing (`git archive HEAD` into a
  scratch dir and re-run) before treating it as your regression. **Retirement is per batch, not once and for all**: the
  `ProbeProxyListening` batch went green, then 152ba09 froze a *new* red batch (`RetryCheck`,
  `ProbeHostnameResolves`). Before reading a red as your regression, check whether the failing test
  is a later, still-stubbed backlog task.
- A probe passes its acceptance test long before it is correct. `probe_test.go` is satisfiable by an
  implementation that dials every interface; what proved the loopback-only invariant was an
  out-of-test probe against a listener bound to a **LAN address only** (expect `OK=false`). Probe the
  inputs the frozen test does not cover — that is where a green-but-wrong diff hides.
- `plutil -lint` proves the document *parses*, not that launchd *accepts* it. What actually
  verified `BuildPlist` was `launchctl bootstrap gui/$UID <generated.plist>` followed by
  `launchctl print`: it showed `arguments` verbatim (including `a&b` and `two words`),
  `minimum runtime = 60` for `ThrottleInterval`, `successful exit => 0` / `after crash => 1`
  for the `KeepAlive` dict, and `runs = 1` for `RunAtLoad` — none of which a lint can tell
  you. Bootstrap into a throwaway label and `bootout` afterwards.
- `TestEnsureExposedRepairsDrift` is **flaky at roughly 1 in 10**, independently of any diff — it boots
  out a real launchd job and races its re-registration (a failing run takes ~30s against ~2.5s for a
  green one). Measured 1/20 at clean HEAD and 1/8 with an unrelated diff. Do not chase it as a
  regression: confirm with `-count=10` on a stashed tree first.
- A negative DNS assertion can be green for the wrong reason. `TestProbeHostnameResolvesFailsForANameThatDoesNot`
  passes just as happily when the resolver is unreachable as when `.invalid` genuinely NXDOMAINs — the
  probe would look correct on a machine where *nothing* resolves. What proved it was a control lookup
  out of test: `example.com`, `one.one.one.one` and `dns.google` all `OK=true` in the same run, and the
  `.invalid` error unwrapping to a `*net.DNSError` with `IsNotFound=true`. Pair every "must fail" fixture
  with a "must succeed" control against the same dependency.
