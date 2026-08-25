# Build, sign, and the quality suite.
#
# Signing matters more than it looks: macOS grants keychain access to a binary
# by IDENTITY, and an unsigned binary's identity changes with its contents. So
# an unsigned build re-prompts for keychain access on every rebuild, and an
# unsigned release would re-prompt every user on every update — the tool reads a
# secret on every apply. See RELEASE.md.
#
# Signing is OPTIONAL here: with no identity available the build still produces
# a working binary, so CI and Linux need nothing. Set CODESIGN_IDENTITY, or let
# the Makefile find a Developer ID automatically.

BINARY := mcp-remote-bridge
PKG    := ./cmd/mcp-remote-bridge
DIST   := dist

# git describe, so a binary can always be traced back to a commit. --dirty is
# load-bearing: `release` refuses a dirty tree rather than shipping a version
# string that names a commit the artefact does not match.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

# darwin only, and deliberately: the tool drives launchctl and the macOS
# keychain, so any other target would install and then fail at first use.
# cmd/mcp-remote-bridge/platform_other.go makes that a compile error. ADR 0009.
ARCHES := arm64 amd64

# The notarytool credentials profile, created once with:
#   xcrun notarytool store-credentials <name> --apple-id <id> --team-id <team>
NOTARY_PROFILE ?= mcp-remote-bridge

# The first Developer ID Application identity, if any. An ad-hoc signature is
# deliberately NOT used as a fallback: it is derived from the contents, so it
# changes with every build and fixes nothing.
CODESIGN_IDENTITY ?= $(shell security find-identity -v -p codesigning 2>/dev/null \
	| grep -m1 'Developer ID Application' \
	| sed -E 's/.*"(.+)".*/\1/')

.PHONY: build sign check test lint fmt clean release dist notarize checksums

build:
	go build -ldflags '$(LDFLAGS)' -o $(BINARY) $(PKG)
ifeq ($(strip $(CODESIGN_IDENTITY)),)
	@echo "note: no Developer ID found; the binary is unsigned and macOS will"
	@echo "      re-ask for keychain access after each rebuild (see RELEASE.md)"
else
	@codesign --force --options runtime --sign "$(CODESIGN_IDENTITY)" $(BINARY) \
		&& echo "signed with: $(CODESIGN_IDENTITY)"
endif

# sign an existing binary without rebuilding
sign:
	@test -n "$(strip $(CODESIGN_IDENTITY))" || { echo "no signing identity available"; exit 1; }
	codesign --force --options runtime --sign "$(CODESIGN_IDENTITY)" $(BINARY)
	codesign --verify --verbose $(BINARY)

# CI's order, so a green local run means the same thing as a green CI run
check: fmt lint test

fmt:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

lint:
	go vet ./...

test:
	go test ./...

clean:
	rm -rf $(BINARY) $(DIST)

# ---------------------------------------------------------------------------
# Release. See RELEASE.md for the runbook and ADR 0009 for why this is shell
# rather than GoReleaser: the matrix is two darwin targets, and the one hard
# step - notarisation - needs a macOS host and Apple credentials either way.
# ---------------------------------------------------------------------------

release: check dist notarize checksums
	@echo
	@echo "$(VERSION) is built, signed, notarised and checksummed in $(DIST)/."
	@echo "Publish with:  gh release create $(VERSION) $(DIST)/*.zip $(DIST)/SHA256SUMS"

dist:
	@test -n "$(strip $(CODESIGN_IDENTITY))" || { \
		echo "no Developer ID: an unsigned release re-prompts every user for keychain"; \
		echo "access on every update (see RELEASE.md). Refusing."; exit 1; }
	@case "$(VERSION)" in *-dirty) \
		echo "the tree is dirty; $(VERSION) would name a commit this build does not match"; \
		exit 1;; esac
	@rm -rf $(DIST) && mkdir -p $(DIST)
	@for arch in $(ARCHES); do \
		echo "building darwin/$$arch"; \
		GOOS=darwin GOARCH=$$arch CGO_ENABLED=0 \
			go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY) $(PKG) || exit 1; \
		codesign --force --options runtime --timestamp \
			--sign "$(CODESIGN_IDENTITY)" $(DIST)/$(BINARY) || exit 1; \
		codesign --verify --strict $(DIST)/$(BINARY) || exit 1; \
		(cd $(DIST) && zip -q $(BINARY)_$(VERSION)_darwin_$$arch.zip $(BINARY)) || exit 1; \
		rm -f $(DIST)/$(BINARY); \
	done
	@echo "signed: $(CODESIGN_IDENTITY)"

# Notarisation is per-archive. The ticket cannot be stapled to a bare
# executable - there is no bundle to staple it into - so Gatekeeper resolves it
# online on first run. That is the accepted shape for a CLI, and it is why the
# archive, not the binary, is what gets submitted.
notarize:
	@test -d $(DIST) || { echo "run 'make dist' first"; exit 1; }
	@for zip in $(DIST)/*.zip; do \
		echo "notarising $$zip"; \
		xcrun notarytool submit "$$zip" \
			--keychain-profile "$(NOTARY_PROFILE)" --wait || exit 1; \
	done
	@echo "checking the effect, not the submission:"
	@for zip in $(DIST)/*.zip; do \
		rm -rf $(DIST)/.verify && mkdir -p $(DIST)/.verify && \
		unzip -q "$$zip" -d $(DIST)/.verify && \
		spctl --assess --type exec -vv $(DIST)/.verify/$(BINARY) 2>&1 \
			| grep -q "accepted" \
			&& echo "  $$zip: Gatekeeper accepts it" \
			|| { echo "  $$zip: Gatekeeper REJECTS it - the release is not usable"; exit 1; }; \
	done
	@rm -rf $(DIST)/.verify

checksums:
	@cd $(DIST) && shasum -a 256 *.zip > SHA256SUMS && cat SHA256SUMS
