<!-- generated-by: groundrules v1.10.0 -->
# Release — mcp-remote-bridge

> Operational **runbook** for shipping. The CHANGELOG records *what* shipped; this file
> records *how* to ship safely. Update it whenever a release reveals a fragility (pair it
> with a `docs/LEARNINGS.md` entry).

> **Status: pre-code.** Nothing has shipped. The commands below are the intended procedure
> — replace each one with what actually worked the first time you run it, and delete this
> banner.

## TL;DR

```bash
# The exact commands of a normal release, in order:
go vet ./... && gofmt -l . && go test ./...   # must be green, same order as CI
git tag v0.1.0 && git push --follow-tags      # the tag is the trigger
# GoReleaser (via CI) builds the matrix, attaches binaries + checksums, updates the tap
```

## Environments

This is a CLI, not a service — there is no staging or production *of the tool*. The
"environments" are distribution channels.

| Channel | Trigger | Target | URL |
|---|---|---|---|
| Source | push on `main` | GitHub | https://github.com/lozit/mcp-remote-bridge |
| Tagged binaries | tag `vX.Y.Z` | GitHub Releases (darwin/arm64, darwin/amd64; linux later) | `/releases` |
| Homebrew | the same tag | `<fill in: tap repo — decide before v0.1>` | — |
| `go install` | the same tag | proxy.golang.org | `go install github.com/lozit/mcp-remote-bridge@latest` |

**Undecided before v0.1** — the release toolchain (GoReleaser vs. a hand-rolled build
matrix) and whether there is a Homebrew tap at all for the first release. → ADR.

## Pre-release checklist

- [ ] Quality suite green locally, **in the same order as CI**: `gofmt -l .` (empty output)
      → `go vet ./...` → `go test ./...`
- [ ] **The secret invariants have tests, and they pass** — no secret value in a generated
      plist, none in `argv`, none in a log; an absent referenced secret fails at start.
      These are the ones that must never regress silently (see `docs/SECURITY.md`).
- [ ] **A real end-to-end `apply` was run on a clean machine state**, not just unit tests —
      this tool's whole thesis is "verify the effect, never trust the write". Shipping it on
      the strength of mocks alone would be the exact failure it exists to prevent.
- [ ] `remove` verified as the exact inverse: after it, the hostname stops answering.
- [ ] `doctor` gives a useful message on each precondition being absent (uninstall /
      unauthenticate one deliberately, or fake it).
- [ ] CHANGELOG `[Unreleased]` section moved under the new version with today's date.
- [ ] `README.md` install instructions match what this release actually publishes.
- [ ] **Capture before shipping** (cf. `CLAUDE.md` → "Capture at checkpoints"): anything
      **decided** → an ADR, **learned / blocked** → `docs/LEARNINGS.md`, an **agent
      mistake/drift** → `docs/AGENT-EVALS.md`. A push/tag is the most reliable capture
      moment.

## Secrets & configuration

- **The tool's own secrets: none.** It ships no API key, no token, no endpoint.
- **The release pipeline's secrets** — a GitHub token for the release (the default
  `GITHUB_TOKEN` where possible), a tap-repo token if a Homebrew tap exists, and an Apple
  signing/notarization identity **if** notarized macOS binaries turn out to be needed.
  Names live here; values live in GitHub Actions secrets. `<fill in the names once set>`
- **macOS Gatekeeper** — an unsigned downloaded binary is quarantined and refused. Decide
  before v0.1 whether to notarize or to document the `xattr -d com.apple.quarantine`
  workaround. Homebrew installs sidestep this; direct downloads do not.

## Rollback

- **A bad release**: tags are immutable once published — never re-tag. Ship a new patch.
- **A truly broken binary**: mark the GitHub release as a pre-release (or delete the
  release, keeping the tag) so the "latest" pointer falls back, then patch.
- **Homebrew**: revert the formula commit in the tap; users get the prior version on the
  next `brew update`.
- **`go install`**: nothing to roll back — the proxy is immutable and users pin a version.
- **On the user's machine**: a bad version's damage is bounded by design — `remove <name>`
  is the exact inverse of `apply`, so recovery is a documented command, not a manual
  cleanup.

## Known fragilities

<!-- Feed this from real incidents — one line each, link the LEARNINGS entry: -->

- Tags are immutable once pushed: bump a new patch, never re-tag.
- **A green test suite is not a green release here.** Every seam is mocked in unit tests;
  the things that actually break (launchd semantics, DNS propagation, tunnel auth) are all
  on the other side of the mock. The end-to-end item in the checklist is the only real gate.
- **DNS propagation makes releases flaky to verify** — a fresh hostname can fail a probe
  purely on timing. Don't conclude "the release is broken" from one red `status`; wait and
  re-probe.
- `<fill in as you learn>`
