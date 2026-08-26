#!/bin/bash
# Cut a release with GoReleaser, from a machine that holds the Developer ID.
#
# Never run this in CI: the signing key stays on a machine the maintainer
# controls (ADR 0011). Pass --dry to build, sign and archive without publishing
# or notarising.
set -euo pipefail
cd "$(dirname "$0")/.."

CODESIGN_IDENTITY="${CODESIGN_IDENTITY:-$(security find-identity -v -p codesigning 2>/dev/null \
  | grep -m1 'Developer ID Application' | sed -E 's/.*"(.+)".*/\1/')}"
export CODESIGN_IDENTITY

if [ -z "$CODESIGN_IDENTITY" ]; then
  echo "no Developer ID: an unsigned release makes every user re-authorise keychain" >&2
  echo "access on every update, because macOS grants it by binary identity. Refusing." >&2
  exit 1
fi
echo "signing identity: $CODESIGN_IDENTITY"

if [ "${1:-}" = "--dry" ]; then
  exec goreleaser release --snapshot --clean --skip=publish,sign
fi
exec goreleaser release --clean
