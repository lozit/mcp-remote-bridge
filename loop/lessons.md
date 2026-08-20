- Judge a task on **its own** acceptance test, not on `go test ./...`: the loop's remaining
  acceptance tests are committed deliberately red (commit ca696d2), so a whole-suite red is the
  expected state until the last backlog task lands. Verify a red is pre-existing (`git archive HEAD`
  into a scratch dir and re-run) before treating it as your regression.
