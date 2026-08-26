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
git tag v0.1.0 && git push --follow-tags   # the tag is what VERSION comes from
make release                              # check -> build+sign -> notarise -> checksums
gh release create v0.1.0 dist/*.zip dist/SHA256SUMS
```

## Environments

This is a CLI, not a service — there is no staging or production *of the tool*. The
"environments" are distribution channels.

| Channel | Trigger | Target | URL |
|---|---|---|---|
| Source | push on `main` | GitHub | https://github.com/lozit/mcp-remote-bridge |
| Tagged binaries | tag `vX.Y.Z` | GitHub Releases (darwin/arm64, darwin/amd64 — **darwin only**) | `/releases` |
| Homebrew | — | **not for v0.1** (ADR 0009) | — |
| `go install` | the same tag | proxy.golang.org | `go install github.com/lozit/mcp-remote-bridge@latest` |

**Settled by [ADR 0009](docs/decisions/0009-release-is-hand-rolled-and-darwin-only.md)**: the
release is hand-rolled (`make release`), darwin-only, and there is no Homebrew tap in v0.1.

**There is no linux binary, and that is not an omission.** The tool compiles for linux but
cannot run there — it drives `launchctl` and `/usr/bin/security`. A compile-time guard
(`cmd/mcp-remote-bridge/platform_other.go`) turns that into an error naming the reason, rather
than an `exec: not found` at the user's first `apply`.

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
  `GITHUB_TOKEN` where possible) and the Apple notarisation credentials. The latter live in a
  local `notarytool` keychain profile, created once:
  `xcrun notarytool store-credentials mcp-remote-bridge --apple-id <id> --team-id <team>`
  (override the name with `NOTARY_PROFILE=`). Releases are cut from a macOS host, so these
  stay out of CI for now.
- **macOS Gatekeeper — notarised, not worked around.** Teaching users `xattr -d
  com.apple.quarantine` teaches them to disarm the check that protects them; the Developer ID
  answers it properly. `make notarize` submits each archive and then **verifies the effect** on
  the extracted binary, because a successful submission is not proof that the artefact you are
  about to publish is notarised. See the next two bullets for which check, and which not.
- **The ticket is not stapled, and cannot be.** Stapling needs a bundle (`.app`, `.dmg`,
  `.pkg`) to write the ticket into; a bare executable has nowhere to put it. Gatekeeper
  resolves the ticket online on first run instead. Consequence to accept: the very first
  launch on a fresh machine needs network. **Confirmed 2026-08-26**: `stapler staple` exits
  **73** on a binary Apple had just accepted — so this is the format, not a missing ticket.
- **Verify notarisation with `codesign`, never with `spctl`.** `spctl --assess --type exec`
  judges app bundles; on a bare CLI it answers `rejected (the code is valid but does not seem
  to be an app)` whether or not the binary is notarised, so a release gate built on it fails
  every time and teaches you to ignore it. The check that discriminates:
  `codesign --verify --test-requirement="=notarized" <binary>` — exit **0** notarised, exit
  **3** signed but not. Both measured on the same binary.
- **The local check becomes true on its own schedule, and you cannot predict when.** Measured
  across one release on 2026-08-26: ninety seconds in one case; still failing after five
  minutes in another; and an archive that failed 5/5 for more than two hours verified cleanly
  later with no intervention. Apple's `Accepted` is immediate and authoritative — the local
  lookup lags it.
  *An earlier version of this file blamed the architecture, having watched an arm64 binary pass
  while an x86_64 one failed on the same host. That was wrong: both verify here now. It was
  propagation, and the two had simply been submitted at different moments.*
  So `make notarize` reports a failed local check and points at Apple's record rather than
  failing the release — hard-failing on a lagging ticket blocks good releases, which is exactly
  how a release gate ends up disabled.
- **A stuck submission is a real thing; resubmit rather than wait.** One archive sat at
  `In Progress` for 2h56 while an identical-pipeline archive submitted afterwards was Accepted
  in about four minutes. The second submission is what diagnosed it: without a control there is
  no way to tell a stuck upload from a slow queue. Resubmitting is free and idempotent.
- **Code signing is not only about Gatekeeper: it is about the keychain prompt.** macOS grants
  keychain access to a binary by IDENTITY, and an unsigned binary's identity changes with its
  contents. So an unsigned release makes every user re-authorise access on **every update**, not
  once — the tool reads a secret on every `apply`. Observed in development, where each rebuild
  triggers a fresh prompt, and where the symptom in code is a **network timeout** rather than an
  error, because the call blocks on a dialog the process cannot see.
  Signing with a stable identity fixes it; an ad-hoc signature does not, since it is derived from
  the contents.
  **Resolved for development** (2026-08-25): `make build` signs with the Developer ID when one is
  available. Verified — two rebuilds, no prompt, 1s per call against 30s unsigned. The same
  identity is what a notarised release needs, so this is not a development-only workaround.

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
