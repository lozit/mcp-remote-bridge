# 0011 — GoReleaser and a Homebrew tap

**Date**: 2026-08-26
**Status**: Accepted
**Supersedes**: the toolchain and tap parts of
[ADR 0009](0009-release-is-hand-rolled-and-darwin-only.md). Everything else in 0009 stands —
darwin-only, notarised, released from a machine the maintainer controls.

## Context

ADR 0009 rejected GoReleaser and deferred the Homebrew tap, for a reason that was sound at the
time: the matrix is two darwin targets, so GoReleaser's centre of gravity did not apply, and it
would sit around the genuinely hard step — notarisation — rather than solve it.

**A tap is now wanted.** That changes the arithmetic rather than the reasoning. Publishing to a
tap means, on every release, computing the archive checksums and committing a formula to a second
repository. `make release` does not do that, and hand-writing it means hand-writing the part of a
release most likely to be wrong in a way nobody notices until `brew install` fails for someone
else. Updating a tap is exactly what GoReleaser is good at.

So the decision reverses on new information, not on a re-reading of the old argument.

Two things were re-examined while scoping it, and both must survive the change.

**Notarisation stays.** Homebrew installs by `curl` and sets no quarantine attribute, so a
tap-installed binary needs no notarisation. But the tap *adds* a channel; it does not replace the
GitHub Release archives, which are downloaded in a browser and *are* quarantined. Dropping
notarisation would break the direct-download path while the argument for dropping it only ever
covered the tap.

**Signing is not optional for this tool.** Go ad-hoc-signs darwin/arm64 binaries, so an unsigned
build does run. But an ad-hoc signature is derived from the contents and therefore changes with
every version, and macOS grants keychain access **by binary identity**. This tool reads a secret
on every `apply`. An ad-hoc-signed release would make every user re-authorise on every update —
measured in this project at 30s per call against 1s signed, surfacing as a *network timeout*
rather than an error. Signing with a stable Developer ID is a functional requirement here, not a
trust gesture.

## Decision

Release with **GoReleaser, run locally on the maintainer's laptop**: cross-build both darwin
architectures, sign each with the Developer ID, notarise, publish the GitHub release, and update
the `homebrew-tap` formula.

**On the laptop, not the always-on Mac mini.** The mini is a hardened, minimal MCP appliance;
installing a Go toolchain, GoReleaser and the signing key on it would remix trust tiers that were
deliberately separated. Cutting a release is a deliberate act, so always-on availability buys
nothing.

**A CI job builds and tests on `macos-latest` for every tag** — the one real risk the local
release does not cover is "it only compiles on my machine". It does not sign, notarise or
publish.

**No build-provenance attestation for now.** It would attest an *unsigned* artefact, because
signing changes the file and is not even deterministic — measured 2026-08-26: two builds of the
same source hash identically, but two signings of that same binary produce two different file
hashes *and* two different `CDHash` values. So a badge would appear beside an archive whose
checksum it does not cover, which manufactures confidence rather than establishing it.

## Alternatives considered

- **Keep `make release` and hand-write the tap update**: rejected. It is the part of the release
  whose breakage is discovered by someone else, on their machine, at `brew install` time.
- **Sign inside CI**: rejected, unchanged from 0009. The Developer ID private key would live in a
  third party's secrets and be decrypted on every build, for a tool whose entire purpose is
  handling secrets and tunnels.
- **Release from the Mac mini over SSH**: rejected. It is a remote signing service without any of
  a real one's controls — no approval step, no audit log, no rate limit — and it puts the signing
  key on the appliance.
- **Attest the unsigned build, reconciled by reproducibility**: rejected, and worth recording
  *why* so it is not proposed again. It creates two build paths — one attested, one published —
  and defines their outputs as legitimately different. Drift between them is then undetectable,
  because the mismatch that would signal it is the expected state.

## Consequences

### Positive
- `brew install` becomes the first-run path, which is the one people actually take.
- The checksum-and-formula step, the easiest to get quietly wrong, stops being hand-written.
- One build path. What is signed is what is published is what is in the tap.

### Negative / Tradeoffs
- A dependency in the release path, to keep current. 0009's objection to that is not withdrawn;
  it is outweighed by the tap.
- A second repository (`homebrew-tap`) whose failures are asynchronous — a broken formula is
  discovered by a user, not by the release.
- GoReleaser does not sign Mach-O with a Developer ID natively; signing and notarisation stay
  ours, wired in as hooks. The hard part remains exactly as hard as 0009 said it was.

### Neutral
- `make build` and `make check` are unchanged. `make release` is superseded by GoReleaser but the
  measurements in its comments — the notarisation bounds, the propagation lag, the
  `codesign --test-requirement` check — carry over and must not be lost with it.

## The rule for later

When users and a tap justify provenance, do **not** resurrect a two-path build reconciled by
reproducibility. Either sign *inside* CI with a dedicated, independently revocable certificate and
a protected environment, or attest exactly the signed artefact (build once → sign → attest).
One path, always.

## Notes

- Measured 2026-08-26: `go build -trimpath` is reproducible (identical hashes across runs);
  `codesign` is not (different file hash *and* `CDHash` on repeated signings of one input).
- The Developer ID private key cannot be re-downloaded from Apple. It must be backed up as an
  encrypted `.p12`, or a dead laptop means the loss of the ability to sign at all.
