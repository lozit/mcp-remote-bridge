# 0009 — The release is hand-rolled, and darwin-only

**Date**: 2026-08-25
**Status**: Accepted

## Context

`RELEASE.md` has carried two open questions since it was written: the release
toolchain (GoReleaser or a hand-rolled build), and whether there is a Homebrew
tap at all for the first release. Both were parked until there was something to
release. There is now.

Two facts settle more of it than the toolchain comparison does.

**The tool is darwin-only in practice, and does not say so.** It compiles
cleanly for `linux/amd64` today — measured — but `internal/launchd` shells out
to `launchctl` and `internal/keychain` to `/usr/bin/security`. A Linux binary
would build, install, and then fail at the first `apply` with an `exec: not
found`. There is no `runtime.GOOS` guard anywhere in the tree. So the release
matrix is not a matrix: it is `darwin/arm64` and `darwin/amd64`, and shipping a
third target would be shipping something that cannot work.

**The hard step is notarisation, and nothing removes it.** ADR-adjacent work
already established why signing matters here (`Makefile`, `RELEASE.md`): macOS
grants keychain access by binary identity, so an unsigned build re-prompts on
every rebuild, and an unsigned release would re-prompt every user on every
update — for a tool that reads a secret on every `apply`. Notarisation is the
same identity applied to distribution. It requires `xcrun notarytool` on a
macOS runner with App Store Connect credentials, whatever else is in the
pipeline.

That reframes the toolchain question. GoReleaser's centre of gravity is the
cross-platform matrix, and this project does not have one. It would sit
*around* the notarisation step rather than solve it.

## Decision

Release from a `macos-latest` GitHub Actions runner, driven by a hand-rolled
`make release`: build both darwin architectures, sign each with the Developer
ID, submit to `notarytool --wait`, staple, checksum, and create the GitHub
release. No GoReleaser.

**Homebrew: not for v0.1.** Ship notarised archives and their checksums. A tap
is a second repository and a second release path to keep correct; it earns its
place once there are users asking for it, not before.

**And add the `GOOS` guard the code is missing**, so a non-darwin build fails at
compile time with a sentence that says why, rather than at first use with a
missing executable. Failing honestly at the earliest possible moment is the same
rule as rule 2, applied to the build.

## Alternatives considered

- **GoReleaser**: the standard answer, and it would genuinely give checksums,
  the GitHub release, and a tap update for free. Rejected because its main
  value is the matrix this project does not have, while the step that is
  actually hard — notarisation on a macOS runner with Apple credentials — stays
  exactly as hard. It would add a tool to keep current in exchange for
  automating the easy half.
- **Ship a Linux binary too, unguarded**: rejected. It builds, so it looks
  supported. A binary that installs and then cannot work is worse than one that
  is absent, because the failure surfaces at the user's first real use rather
  than at download.
- **A Homebrew tap in v0.1**: rejected for now, not on principle. It is also the
  cleanest answer to Gatekeeper for direct downloads, which is an argument to
  revisit it — but notarisation answers that too, and without a second repo.
- **`xattr -d com.apple.quarantine` in the README**: rejected. Teaching users to
  strip quarantine is teaching them to disarm the check that protects them, to
  work around a problem a Developer ID now solves properly.

## Consequences

### Positive
- One fewer tool in the release path, and the release script is readable in one
  screen — it is shell doing exactly the five things the release does.
- The notarisation step is written explicitly rather than configured, so when it
  fails (it will: credentials expire, `notarytool` messages are terse) the thing
  to debug is visible.
- No unusable Linux artefact, and a compile-time error for anyone who tries.

### Negative / Tradeoffs
- Checksums, the GitHub release, and the archive layout are now ours to get
  right. GoReleaser has been getting them right for years.
- If the tool ever grows a `systemd` `ServiceManager`, this decision inverts:
  the matrix becomes real and GoReleaser becomes the right answer. Revisit then
  rather than pre-building for it.
- No `brew install` at v0.1 — a real cost to first-run experience, accepted for
  one release.

### Neutral
- The Apple credentials become a CI secret. That is a new secret to hold, but it
  is App Store Connect, not a keychain, and it is outside the secret path this
  project's rule 3 governs.

## Notes

- `RELEASE.md` — the runbook this ADR resolves the open questions of.
- `Makefile` — signing already lands here; `release` joins `build` and `sign`.
- Measured 2026-08-25: `GOOS=linux GOARCH=amd64 go build ./cmd/mcp-remote-bridge`
  succeeds, which is what makes the guard necessary rather than theoretical.
