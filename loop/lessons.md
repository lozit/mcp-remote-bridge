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
- `plutil -lint` proves the document *parses*, not that launchd *accepts* it. What actually
  verified `BuildPlist` was `launchctl bootstrap gui/$UID <generated.plist>` followed by
  `launchctl print`: it showed `arguments` verbatim (including `a&b` and `two words`),
  `minimum runtime = 60` for `ThrottleInterval`, `successful exit => 0` / `after crash => 1`
  for the `KeepAlive` dict, and `runs = 1` for `RunAtLoad` — none of which a lint can tell
  you. Bootstrap into a throwaway label and `bootout` afterwards.
