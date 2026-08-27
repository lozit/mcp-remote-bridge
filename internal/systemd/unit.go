// Package systemd drives per-user systemd services: units under
// ~/.config/systemd/user and `systemctl --user`.
//
// It is the Linux counterpart of internal/launchd, and the only place in the
// codebase allowed to know about systemd. User units rather than system ones,
// deliberately: it keeps the tool out of root, the same refusal it already
// makes for the tunnel connector. See ADR 0012.
package systemd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// BuildUnit renders spec as a systemd service unit.
//
// The unit file is world-readable, exactly as a launchd plist is, so it carries
// a name and paths and nothing else. In particular it has NO Environment= and
// no EnvironmentFile= holding values: secrets are resolved at launch by the
// __launch subcommand this unit invokes. See ADR 0002 and ADR 0012.
//
// A spec that cannot be rendered honestly — no label, no program, or a program
// that is not an absolute path — yields an error rather than a unit systemd
// would reject at load time.
func BuildUnit(spec bridge.ServiceSpec) ([]byte, error) {
	if spec.Label == "" {
		return nil, fmt.Errorf("service spec has no label")
	}
	if spec.Program == "" {
		return nil, fmt.Errorf("service spec %q has no program", spec.Label)
	}
	if !filepath.IsAbs(spec.Program) {
		return nil, fmt.Errorf("service spec %q program %q is not an absolute path",
			spec.Label, spec.Program)
	}

	var b strings.Builder

	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=%s\n", escape(spec.Label))
	// Without this the unit is started at boot before the network exists, fails,
	// and burns its restart budget before anything could have worked.
	b.WriteString("After=network-online.target\n")
	b.WriteString("Wants=network-online.target\n\n")

	b.WriteString("[Service]\n")
	b.WriteString("Type=simple\n")
	fmt.Fprintf(&b, "ExecStart=%s\n", execStart(spec))

	if path := spec.StdoutPath; path != "" {
		fmt.Fprintf(&b, "StandardOutput=append:%s\n", escape(path))
	}
	if path := spec.StderrPath; path != "" {
		fmt.Fprintf(&b, "StandardError=append:%s\n", escape(path))
	}

	// Restart semantics, mapped from the same policy launchd reads inverted.
	// launchd's KeepAlive says when to KEEP it alive; systemd's Restart says
	// when to RESTART. `always` covers both cases at once, so the two-flag
	// policy collapses — but only when both are set, and a policy asking for
	// neither must produce no supervision at all, never a default.
	switch {
	case spec.KeepAlive.OnFailure && spec.KeepAlive.OnCrash:
		b.WriteString("Restart=always\n")
	case spec.KeepAlive.OnFailure:
		// A non-zero exit. systemd calls that on-failure, which also covers a
		// signal — close enough that distinguishing them would be a fiction.
		b.WriteString("Restart=on-failure\n")
	case spec.KeepAlive.OnCrash:
		b.WriteString("Restart=on-abnormal\n")
	}

	if spec.ThrottleInterval > 0 {
		// systemd counts in seconds and rejects a fractional value here, so a
		// sub-second interval would render as 0 and disable throttling
		// silently — the same trap that was measured on launchd's
		// ThrottleInterval (2026-08-21). Refuse rather than round.
		if spec.ThrottleInterval < time.Second {
			return nil, fmt.Errorf("service spec %q has a throttle interval below one second (%s), "+
				"which would render as 0 and disable throttling", spec.Label, spec.ThrottleInterval)
		}
		fmt.Fprintf(&b, "RestartSec=%d\n", int(spec.ThrottleInterval.Seconds()))
	}

	b.WriteString("\n[Install]\n")
	b.WriteString("WantedBy=default.target\n")

	return []byte(b.String()), nil
}

// execStart renders the command line.
//
// systemd parses ExecStart itself, so an argument containing a space or a quote
// would be split or swallowed. Quoting is not cosmetic here: an unquoted path
// with a space silently becomes two arguments, and the service starts with the
// wrong command rather than failing.
func execStart(spec bridge.ServiceSpec) string {
	parts := make([]string, 0, len(spec.Args)+1)
	for _, arg := range append([]string{spec.Program}, spec.Args...) {
		parts = append(parts, quote(arg))
	}
	return strings.Join(parts, " ")
}

func quote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"'\\$%\n") {
		return s
	}
	// systemd's own escaping rules inside double quotes: backslash and double
	// quote need escaping, and a literal % must be doubled or it is read as a
	// specifier.
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`)
	return `"` + r.Replace(s) + `"`
}

// escape guards the single-line directives. A newline in a value would end the
// directive and let the rest be read as a new one — the injection this file has
// to prevent, since the label comes from user config.
func escape(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ", "%", "%%").Replace(s)
}

// UnitName is the file name for a label.
func UnitName(label string) string {
	return label + ".service"
}
