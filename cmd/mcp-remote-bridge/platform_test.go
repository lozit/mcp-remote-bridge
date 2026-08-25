package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The guard in platform_other.go is only worth having if it actually stops a
// non-darwin build, and that is not observable from a test compiled for darwin
// — the file is excluded here by its own build tag. So the test runs the
// compiler.
//
// Without this, the guard could be silently defeated by a stray edit to the
// build tag and nothing would notice until someone downloaded a binary that
// cannot work.
func TestTheBuildFailsOnANonDarwinTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("invokes the compiler")
	}

	cmd := exec.Command("go", "build", "-o", os.DevNull, ".")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatal("the tool built for linux; it would install there and fail at the first apply, " +
			"since launchctl and security do not exist (ADR 0009)")
	}
	// The message the user sees has to say WHY, not just that something is
	// undefined. The identifier carries it, so assert on the identifier.
	if !strings.Contains(string(out), "requires_macOS") {
		t.Errorf("the build failed without saying macOS is required:\n%s", out)
	}
}

// The counterpart: the guard must not be so broad that it breaks the real
// target. A test asserting only the failure would pass on a file that refuses
// every OS.
func TestTheBuildSucceedsOnDarwin(t *testing.T) {
	if testing.Short() {
		t.Skip("invokes the compiler")
	}

	cmd := exec.Command("go", "build", "-o", os.DevNull, ".")
	cmd.Env = append(os.Environ(), "GOOS=darwin")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the guard broke the supported target:\n%s", out)
	}
}
