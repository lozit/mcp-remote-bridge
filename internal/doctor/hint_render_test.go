package doctor

import (
	"strings"
	"testing"
)

// The connector hint is multi-line, which Render was never asked to do before:
// it indents only the first line. Assert the shape a reader actually sees, so a
// future edit cannot quietly produce a ragged block.
func TestTheConnectorHintPrintsTheCommand(t *testing.T) {
	deps := healthyDeps()
	deps.ProcessMatches = func(string) bool { return false }

	out := Render(Run(fullConfig(), deps))

	if !strings.Contains(out, "cloudflared service install") {
		t.Errorf("the hint does not print the command to run:\n%s", out)
	}
	if !strings.Contains(out, "several") {
		t.Error("the hint should say a tunnel accepts several connectors: without it, " +
			"moving a tunnel between machines looks like it needs a downtime window")
	}
	// Every line of the block must share one left edge. Render owns that, not
	// the hint text: this failed first time round because the hint carried its
	// own spacing and the two disagreed.
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.Contains(line, "cloudflared service install") ||
			strings.Contains(line, "connectors at once") {
			if !strings.HasPrefix(line, "        ") {
				t.Errorf("a continuation line does not align with the hint block: %q", line)
			}
		}
	}
	t.Log("\n" + out)
}
