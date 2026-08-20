package launchd

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"time"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
)

// plistHeader is the XML prologue every launchd property list opens with.
const plistHeader = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`

// BuildPlist renders spec as a launchd property list.
//
// The plist is world-readable, so it carries a name and paths and nothing else.
// In particular it has NO environment section: the secrets are resolved at
// launch time by the __launch subcommand this plist invokes. See ADR 0002.
//
// It emits exactly seven keys — Label, ProgramArguments, RunAtLoad, KeepAlive,
// ThrottleInterval, StandardOutPath, StandardErrorPath — and KeepAlive only
// when the policy asks for something, so a service is never supervised without
// being asked.
//
// A spec that cannot be rendered honestly (no label, no program, or a program
// that is not an absolute path) yields an error rather than a document launchd
// would reject at load time.
func BuildPlist(spec bridge.ServiceSpec) ([]byte, error) {
	if spec.Label == "" {
		return nil, fmt.Errorf("service spec has no label")
	}
	if spec.Program == "" {
		return nil, fmt.Errorf("service spec %q has no program", spec.Label)
	}
	if !filepath.IsAbs(spec.Program) {
		// The service outlives the shell that created it, so there is no working
		// directory to resolve a relative path against.
		return nil, fmt.Errorf("service spec %q has a relative program path %q: an absolute path is required", spec.Label, spec.Program)
	}

	var b bytes.Buffer
	b.WriteString(plistHeader)

	writeString(&b, "Label", spec.Label)

	// argv[0] is the program itself, then the spec's arguments verbatim: the
	// generator reproduces what it was given, it does not rewrite or inject.
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, arg := range append([]string{spec.Program}, spec.Args...) {
		b.WriteString("\t\t<string>")
		escape(&b, arg)
		b.WriteString("</string>\n")
	}
	b.WriteString("\t</array>\n")

	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")

	if spec.KeepAlive.OnFailure || spec.KeepAlive.OnCrash {
		b.WriteString("\t<key>KeepAlive</key>\n\t<dict>\n")
		if spec.KeepAlive.OnFailure {
			// SuccessfulExit=false reads as "keep alive unless it exited zero".
			b.WriteString("\t\t<key>SuccessfulExit</key>\n\t\t<false/>\n")
		}
		if spec.KeepAlive.OnCrash {
			b.WriteString("\t\t<key>Crashed</key>\n\t\t<true/>\n")
		}
		b.WriteString("\t</dict>\n")
	}

	// Seconds as an integer — launchd rejects a string here.
	fmt.Fprintf(&b, "\t<key>ThrottleInterval</key>\n\t<integer>%d</integer>\n", spec.ThrottleInterval/time.Second)

	writeString(&b, "StandardOutPath", spec.StdoutPath)
	writeString(&b, "StandardErrorPath", spec.StderrPath)

	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes(), nil
}

// writeString emits one <key>/<string> pair, both escaped.
func writeString(b *bytes.Buffer, key, value string) {
	b.WriteString("\t<key>")
	escape(b, key)
	b.WriteString("</key>\n\t<string>")
	escape(b, value)
	b.WriteString("</string>\n")
}

// escape writes s as XML character data.
//
// xml.EscapeText is the stdlib's own escaper and is deliberately conservative:
// besides the five metacharacters it also encodes the control characters a raw
// path could carry, which is what makes the document survive a round trip.
func escape(b *bytes.Buffer, s string) {
	// EscapeText only ever fails when the underlying writer does, and a
	// bytes.Buffer does not.
	_ = xml.EscapeText(b, []byte(s))
}
