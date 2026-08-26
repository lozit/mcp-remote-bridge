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

# GoReleaser picks its forge from whichever token it finds in the environment,
# and a GITLAB_TOKEN sitting there from another project wins. That is not a
# preference it announces loudly: the first run of this script reported
# "using token from $GITLAB_TOKEN" while publishing a GitHub project. Pin the
# forge by clearing the others and supplying GitHub's explicitly.
unset GITLAB_TOKEN GITEA_TOKEN
GITHUB_TOKEN="${GITHUB_TOKEN:-$(gh auth token 2>/dev/null || true)}"
export GITHUB_TOKEN
if [ -z "$GITHUB_TOKEN" ]; then
  echo "no GitHub token: run 'gh auth login' or set GITHUB_TOKEN" >&2
  exit 1
fi

if [ "${1:-}" = "--dry" ]; then
  exec goreleaser release --snapshot --clean --skip=publish,sign
fi
exec goreleaser release --clean
