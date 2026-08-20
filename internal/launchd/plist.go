package launchd

import (
	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// BuildPlist renders spec as a launchd property list.
//
// The plist is world-readable, so it carries a name and paths and nothing else.
// In particular it has NO environment section: the secrets are resolved at
// launch time by the __launch subcommand this plist invokes. See ADR 0002.
func BuildPlist(spec bridge.ServiceSpec) ([]byte, error) {
	// STUB — the loop's maker replaces this body. A syntactically valid but
	// empty plist, so plist_test.go fails on its behavioural assertions rather
	// than on a parse error.
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict/>
</plist>
`), nil
}
