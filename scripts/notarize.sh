#!/bin/bash
# Notarise one archive, prove the effect on the binary inside it, and write a
# receipt beside it.
#
# GoReleaser's OSS edition has no Developer ID notarisation of its own, so this
# runs as a `signs` hook over each archive. See ADR 0011.
#
# The receipt is not decoration. GoReleaser ALWAYS registers a signature
# artifact for a `signs` entry — `output: false` does not stop it — and then
# fails the publish if that file does not exist. So the hook must produce
# something. Rather than an empty file to satisfy a tool, it writes what a
# reader would actually want months later: which submission id notarised this
# exact archive, when, and whether the ticket was visible locally at the time.
#
# Two facts, each measured the hard way and recorded in RELEASE.md, shape this:
#
#   - `notarytool submit --wait` reports its own expiry as a connection error,
#     which reads as a network fault when nothing has failed at all. A dead
#     waiter says nothing about the submission: poll with `info`, never
#     resubmit blindly.
#   - The ticket does not exist the instant Apple says Accepted. Verification
#     retries within a bounded window rather than sleeping a fixed guess, and a
#     ticket that has not landed is not a bad release.
#
# The check is `codesign --test-requirement="=notarized"`, NOT `spctl`: spctl
# judges app bundles and answers "the code is valid but does not seem to be an
# app" for a bare CLI, notarised or not.
set -euo pipefail

ARCHIVE="${1:?usage: notarize.sh <archive.zip> [receipt]}"
RECEIPT="${2:-$ARCHIVE.notarization.txt}"
PROFILE="${NOTARY_PROFILE:-mcp-remote-bridge}"
WAIT="${NOTARY_WAIT:-45m}"
TRIES="${TICKET_SETTLE_TRIES:-20}"
INTERVAL="${TICKET_SETTLE_INTERVAL:-15}"

# Testing the pipeline SHAPE — artefacts registered once, cask written, upload
# working — should not cost ten minutes of Apple's queue per archive. This
# skips the submission and says so in the receipt as well as on screen, so a
# skipped notarisation can never be mistaken for a successful one.
if [ "${NOTARIZE_SKIP:-}" = "1" ]; then
  echo "  !! NOTARISATION SKIPPED for $ARCHIVE (NOTARIZE_SKIP=1) — not publishable"
  printf 'NOT NOTARISED — skipped via NOTARIZE_SKIP=1.\nThis archive must not be published.\n' > "$RECEIPT"
  exit 0
fi

echo "notarising $ARCHIVE (up to $WAIT; the upload is quick, Apple's queue is not)"
submit_log="$(mktemp)"
if ! xcrun notarytool submit "$ARCHIVE" --keychain-profile "$PROFILE" \
      --wait --timeout "$WAIT" 2>&1 | tee "$submit_log"; then
  echo "the wait did not complete. The upload may well have succeeded — check with:" >&2
  echo "  xcrun notarytool history --keychain-profile $PROFILE" >&2
  echo "then re-run. Do not resubmit on a timeout you have not diagnosed." >&2
  exit 1
fi

submission_id="$(awk '/^  id: /{print $2; exit}' "$submit_log")"
status="$(awk '/^  status: /{print $2, $3; exit}' "$submit_log" | xargs)"
rm -f "$submit_log"

if [ "$status" != "Accepted" ]; then
  echo "Apple did not accept $ARCHIVE (status: ${status:-unknown}, id: ${submission_id:-unknown})" >&2
  echo "  xcrun notarytool log ${submission_id:-<id>} --keychain-profile $PROFILE" >&2
  exit 1
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
unzip -q "$ARCHIVE" -d "$work"

verified="no — ticket not visible locally yet"
for _ in $(seq 1 "$TRIES"); do
  if codesign --verify --test-requirement="=notarized" "$work/mcp-remote-bridge" >/dev/null 2>&1; then
    verified="yes"
    break
  fi
  sleep "$INTERVAL"
done

{
  echo "archive:         $(basename "$ARCHIVE")"
  echo "sha256:          $(shasum -a 256 "$ARCHIVE" | awk '{print $1}')"
  echo "submission id:   $submission_id"
  echo "apple status:    $status"
  echo "verified locally: $verified"
  echo
  echo "The ticket is not stapled: stapling needs a bundle to write it into, and"
  echo "a bare executable has none (stapler exits 73). Gatekeeper resolves it"
  echo "online instead, so a first launch on a fresh machine needs network."
  echo
  echo "Re-check at any time:"
  echo "  xcrun notarytool info $submission_id --keychain-profile $PROFILE"
} > "$RECEIPT"

if [ "$verified" = "yes" ]; then
  echo "  $(basename "$ARCHIVE"): notarised (verified locally)"
else
  # Not a failed release: the ticket lands on its own schedule, measured
  # anywhere from ninety seconds to over two hours. Apple accepted it, which is
  # what counts, and the receipt records both facts.
  echo "  $(basename "$ARCHIVE"): accepted by Apple; ticket not visible locally yet"
fi
