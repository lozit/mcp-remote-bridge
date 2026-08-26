#!/bin/bash
# Notarise one archive, then prove the effect on the binary inside it.
#
# GoReleaser's OSS edition has no Developer ID notarisation of its own, so this
# runs as a `signs` hook over each archive. See ADR 0011.
#
# Two facts, each measured the hard way and recorded in RELEASE.md, shape this:
#
#   - `notarytool submit --wait` reports its own expiry as a connection error,
#     which reads as a network fault when nothing has failed at all. A dead
#     waiter says nothing about the submission: poll with `info`, never
#     resubmit blindly.
#   - The ticket does not exist the instant Apple says Accepted. Verification
#     retries within a bounded window rather than sleeping a fixed guess.
#
# The check is `codesign --test-requirement="=notarized"`, NOT `spctl`: spctl
# judges app bundles and answers "the code is valid but does not seem to be an
# app" for a bare CLI, notarised or not.
set -euo pipefail

ARCHIVE="${1:?usage: notarize.sh <archive.zip>}"
PROFILE="${NOTARY_PROFILE:-mcp-remote-bridge}"
WAIT="${NOTARY_WAIT:-45m}"
TRIES="${TICKET_SETTLE_TRIES:-20}"
INTERVAL="${TICKET_SETTLE_INTERVAL:-15}"

# Testing the pipeline SHAPE - artefacts registered once, cask written, upload
# working - should not cost ten minutes of Apple queue per archive. This skips
# the submission and says so loudly, so a skipped notarisation can never be
# mistaken for a successful one.
if [ "${NOTARIZE_SKIP:-}" = "1" ]; then
  echo "  !! NOTARISATION SKIPPED for $ARCHIVE (NOTARIZE_SKIP=1) - not publishable"
  exit 0
fi

echo "notarising $ARCHIVE (up to $WAIT; the upload is quick, Apple's queue is not)"
if ! xcrun notarytool submit "$ARCHIVE" --keychain-profile "$PROFILE" --wait --timeout "$WAIT"; then
  echo "the wait did not complete. The upload may well have succeeded - check with:" >&2
  echo "  xcrun notarytool history --keychain-profile $PROFILE" >&2
  echo "then re-run. Do not resubmit on a timeout you have not diagnosed." >&2
  exit 1
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
unzip -q "$ARCHIVE" -d "$work"

for _ in $(seq 1 "$TRIES"); do
  if codesign --verify --test-requirement="=notarized" "$work/mcp-remote-bridge" >/dev/null 2>&1; then
    echo "  $ARCHIVE: notarised (verified locally)"
    exit 0
  fi
  sleep "$INTERVAL"
done

# Not a failed release: the ticket lands on its own schedule, measured anywhere
# from ninety seconds to over two hours. Apple's record is what counts.
echo "  $ARCHIVE: ticket not visible locally yet - usually propagation, not a bad build."
echo "    Apple's record is authoritative: xcrun notarytool history --keychain-profile $PROFILE"
