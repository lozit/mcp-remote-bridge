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
# archive, not the binary, is what gets submitted. Confirmed 2026-08-26:
# `stapler staple` on a SUCCESSFULLY NOTARISED binary exits 73.
#
# The check below is `codesign --test-requirement="=notarized"`, NOT
# `spctl --assess`. That distinction cost a wrong check that would have failed
# every release: spctl judges app bundles, and on a bare CLI it answers
# "rejected (the code is valid but does not seem to be an app)" whether the
# binary is notarised or not. Measured both ways on the same binary -
# codesign exits 0 notarised, 3 signed-but-not.
#
# NOTARY_WAIT is explicit because the default is not long enough and, worse,
# reports its expiry as `HTTPClientError.connectTimeout` - which reads as a
# network fault when nothing has failed at all: the upload succeeded and Apple
# is still processing.
#
# Measured 2026-08-25: a 7MB archive was still In Progress 56 minutes after
# submission, and `--wait` itself died twice with a request timeout against
# appstoreconnect.apple.com while `notarytool info` answered instantly
# throughout. So the two failures are different things and must not be read as
# one: `--wait` holds a long-lived connection that some environments kill,
# whereas `info` is a short request. A dead `--wait` says NOTHING about the
# submission.
#
# Nothing is ever lost when this expires. Recover by polling, not by waiting,
# and never by resubmitting:
#   xcrun notarytool info <id> --keychain-profile $(NOTARY_PROFILE)
# then re-run `make notarize` once it reports Accepted.
NOTARY_WAIT ?= 45m

# The ticket does not exist the instant Apple says Accepted - it has to reach
# the service `codesign` asks. Measured 2026-08-26: the check failed on an
# archive whose status was already Accepted, and passed on the same bytes about
# ninety seconds later. Without this window the release gate fails on a release
# that is perfectly fine, which is the way a gate gets ignored.
#
# Same shape as DNS propagation elsewhere in this project, handled the same way:
# retry a bounded number of times rather than sleeping a fixed guess.
TICKET_SETTLE_TRIES ?= 20
TICKET_SETTLE_INTERVAL ?= 15

# And the check has a hard limit worth stating, because it looks like a failure.
# Measured 2026-08-26 on an arm64 host, both archives Accepted by Apple:
# the arm64 binary verifies 5/5, the x86_64 one fails 5/5 - same command, same
# bytes, deterministic either way. The local ticket lookup does not resolve for
# a foreign architecture here.
#
# So a failed local check on the NON-native slice is not evidence of anything,
# and hard-failing on it would block a perfectly good release - which is how a
# gate gets switched off. It degrades to a message naming Apple's record as
# authoritative. On the native architecture it still fails hard, because there
# the check means what it says.

notarize:
	@test -d $(DIST) || { echo "run 'make dist' first"; exit 1; }
	@for zip in $(DIST)/*.zip; do \
		echo "notarising $$zip (up to $(NOTARY_WAIT); the upload is quick, Apple's queue is not)"; \
		xcrun notarytool submit "$$zip" --keychain-profile "$(NOTARY_PROFILE)" \
			--wait --timeout $(NOTARY_WAIT) || { \
			echo "submission did not complete. It may still be processing - check with:"; \
			echo "  xcrun notarytool history --keychain-profile $(NOTARY_PROFILE)"; \
			exit 1; }; \
	done
	@echo "checking the effect, not the submission:"
	@host=$$(uname -m); \
	for zip in $(DIST)/*.zip; do \
		rm -rf $(DIST)/.verify && mkdir -p $(DIST)/.verify && \
		unzip -q "$$zip" -d $(DIST)/.verify || exit 1; \
		arch=$$(file -b $(DIST)/.verify/$(BINARY) | awk '{print $$NF}'); \
		ok=0; \
		for attempt in $$(seq 1 $(TICKET_SETTLE_TRIES)); do \
			if codesign --verify --test-requirement="=notarized" \
					$(DIST)/.verify/$(BINARY) >/dev/null 2>&1; then \
				ok=1; break; \
			fi; \
			sleep $(TICKET_SETTLE_INTERVAL); \
		done; \
		if [ $$ok -eq 1 ]; then \
			echo "  $$zip: notarised (verified locally)"; \
		elif [ "$$arch" != "$$host" ]; then \
			echo "  $$zip: built for $$arch on a $$host host - cannot be verified here."; \
			echo "    Apple's record is authoritative; confirm it with:"; \
			echo "    xcrun notarytool history --keychain-profile $(NOTARY_PROFILE)"; \
		else \
			echo "  $$zip: NOT notarised - the release is not usable"; exit 1; \
		fi; \
	done
	@rm -rf $(DIST)/.verify

checksums:
	@cd $(DIST) && shasum -a 256 *.zip > SHA256SUMS && cat SHA256SUMS
