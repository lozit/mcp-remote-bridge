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

# The first Developer ID Application identity, if any. An ad-hoc signature is
# deliberately NOT used as a fallback: it is derived from the contents, so it
# changes with every build and fixes nothing.
CODESIGN_IDENTITY ?= $(shell security find-identity -v -p codesigning 2>/dev/null \
	| grep -m1 'Developer ID Application' \
	| sed -E 's/.*"(.+)".*/\1/')

.PHONY: build sign check test lint fmt clean

build:
	go build -o $(BINARY) $(PKG)
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
	rm -f $(BINARY)
