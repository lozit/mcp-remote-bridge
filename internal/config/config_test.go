package config

import (
	"strings"
	"testing"
)

const valid = `
[infra]
tunnel = "mac-mcp-bridge"
domain = "example.com"

[mcp.standardnotes]
command   = "mcp-standardnotes"
subdomain = "sn-mcp"
secrets   = { SN_EMAIL = "keychain:mcp-sn-email" }

[mcp.freestyle]
command   = "mcp-freestyle"
subdomain = "freestyle-mcp"
port      = 8081
`

func TestParseAcceptsAValidConfig(t *testing.T) {
	f, err := Parse([]byte(valid), "test.toml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	entries := f.Entries()
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	// Sorted, so a run is reproducible and a diff of `status` output is stable.
	if entries[0].Name != "freestyle" || entries[1].Name != "standardnotes" {
		t.Errorf("entries are not in a stable sorted order: %q, %q", entries[0].Name, entries[1].Name)
	}
	// [infra] is spread onto every entry so the primitive never reads the file.
	for _, e := range entries {
		if e.Tunnel != "mac-mcp-bridge" || e.Domain != "example.com" {
			t.Errorf("%s did not inherit [infra]: tunnel=%q domain=%q", e.Name, e.Tunnel, e.Domain)
		}
	}
	if got := entries[1].Hostname(); got != "sn-mcp.example.com" {
		t.Errorf("Hostname() = %q", got)
	}
	if entries[0].Port != 8081 {
		t.Errorf("explicit port lost: %d", entries[0].Port)
	}
	if entries[1].Port != 0 {
		t.Errorf("omitted port should stay 0 (auto-assign), got %d", entries[1].Port)
	}
}

// The load-bearing one: the config carries references, never values.
func TestParseRejectsASecretValueInPlaceOfAReference(t *testing.T) {
	const pasted = `
[infra]
tunnel = "t"
domain = "example.com"
[mcp.sn]
command   = "x"
subdomain = "sn"
secrets   = { SN_PASSWORD = "hunter2-actual-password" }
`
	_, err := Parse([]byte(pasted), "test.toml")
	if err == nil {
		t.Fatal("a pasted secret value was accepted as a reference")
	}
	// The error must not echo the value: doing so writes it to the terminal
	// and the shell's scrollback, which is the leak we are preventing.
	if strings.Contains(err.Error(), "hunter2-actual-password") {
		t.Errorf("the error echoed the secret value: %v", err)
	}
	if !strings.Contains(err.Error(), "SN_PASSWORD") {
		t.Errorf("the error should name the variable so it is fixable: %v", err)
	}
}

// A typo must not be silently ignored: it would publish at a wrong hostname.
func TestParseRejectsAnUnknownKey(t *testing.T) {
	const typo = `
[infra]
tunnel = "t"
domain = "example.com"
[mcp.sn]
command   = "x"
subdomian = "sn"
`
	_, err := Parse([]byte(typo), "test.toml")
	if err == nil {
		t.Fatal("an unrecognised key was ignored")
	}
	if !strings.Contains(err.Error(), "subdomian") {
		t.Errorf("the error should point at the offending key: %v", err)
	}
}

func TestParseReportsEveryProblemAtOnce(t *testing.T) {
	const messy = `
[mcp.BAD_NAME]
command = ""
subdomain = "Not-Valid"
port = 99999
`
	_, err := Parse([]byte(messy), "test.toml")
	if err == nil {
		t.Fatal("an unusable config was accepted")
	}
	msg := err.Error()
	for _, want := range []string{"tunnel is required", "domain is required", "command is required", "port 99999"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
}

func TestParseRejectsCollisions(t *testing.T) {
	const clash = `
[infra]
tunnel = "t"
domain = "example.com"
[mcp.a]
command = "x"
subdomain = "same"
port = 9000
[mcp.b]
command = "y"
subdomain = "same"
port = 9000
`
	_, err := Parse([]byte(clash), "test.toml")
	if err == nil {
		t.Fatal("two entries claiming one subdomain and one port were accepted")
	}
	for _, want := range []string{"both claim subdomain", "both claim port"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in: %v", want, err)
		}
	}
}

func TestParseRejectsAVariableInBothEnvAndSecrets(t *testing.T) {
	const both = `
[infra]
tunnel = "t"
domain = "example.com"
[mcp.sn]
command   = "x"
subdomain = "sn"
secrets   = { TOKEN = "keychain:tok" }
env       = { TOKEN = "plain" }
`
	if _, err := Parse([]byte(both), "test.toml"); err == nil {
		t.Fatal("a variable set in both env and secrets was accepted")
	}
}

func TestEntryLooksUpByName(t *testing.T) {
	f, err := Parse([]byte(valid), "test.toml")
	if err != nil {
		t.Fatal(err)
	}
	e, err := f.Entry("standardnotes")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if e.Secrets["SN_EMAIL"] != "keychain:mcp-sn-email" {
		t.Errorf("secret reference lost: %q", e.Secrets["SN_EMAIL"])
	}
	if _, err := f.Entry("nope"); err == nil {
		t.Error("Entry returned no error for an absent name")
	}
}
