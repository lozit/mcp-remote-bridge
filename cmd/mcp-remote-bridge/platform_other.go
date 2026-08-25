//go:build !darwin

// This file is compiled on every OS except macOS, and fails on purpose.
//
// The tool builds cleanly for linux — nothing in the type system stops it — but
// it cannot run there: internal/launchd shells out to launchctl and
// internal/keychain to /usr/bin/security. Without this file the failure surfaces
// at the user's first `apply`, as `exec: "launchctl": executable file not
// found`, which reads like a broken install rather than an unsupported OS.
//
// Failing at compile time is the same rule the health report follows: surface
// the truth at the earliest moment it is knowable. See ADR 0009.
package main

const _ = mcp_remote_bridge_requires_macOS_it_drives_launchd_and_the_keychain
