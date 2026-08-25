package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// printReport renders one entry's HealthReport.
//
// It prints the checks that actually ran, and nothing else. A check that did not
// run is absent rather than shown as passing — an empty line would read as a
// pass, and the report may only claim what it probed.
func printReport(w io.Writer, r bridge.HealthReport) {
	mark := "FAIL"
	if r.Healthy() {
		mark = "ok"
	}
	fmt.Fprintf(w, "%-4s %s\n", mark, r.Entry)

	for _, c := range r.Checks {
		symbol := "x"
		if c.OK {
			symbol = "v"
		}
		fmt.Fprintf(w, "       %s %-18s %s\n", symbol, c.Name, c.Detail)
		if c.Err != nil {
			// Indented under its check: a red result is only useful with the
			// reason next to it.
			for _, line := range strings.Split(c.Err.Error(), "\n") {
				fmt.Fprintf(w, "         %s\n", line)
			}
		}
	}
}

// exitCodeFor turns reports into the documented exit codes, so the tool
// composes in scripts: 0 all healthy, 2 at least one entry unhealthy.
//
// A green exit means the probes passed, not that a write succeeded.
func exitCodeFor(reports []bridge.HealthReport) int {
	for _, r := range reports {
		if !r.Healthy() {
			return exitUnhealthy
		}
	}
	return exitOK
}
