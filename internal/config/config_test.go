package config

import (
	"strings"
	"testing"
)

const valid = `
[infra]
domain     = "example.com"
account_id = "acc123"
zone_id    = "zone456"
tunnel_id  = "0e5f-uuid"
api_token  = "keychain:cf-api-token"

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
		if e.TunnelID != "0e5f-uuid" || e.Domain != "example.com" {
			t.Errorf("%s did not inherit [infra]: tunnel_id=%q domain=%q", e.Name, e.TunnelID, e.Domain)
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
domain     = "example.com"
account_id = "a"
zone_id    = "z"
tunnel_id  = "u"
api_token  = "keychain:cf"
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
domain     = "example.com"
account_id = "a"
zone_id    = "z"
tunnel_id  = "u"
api_token  = "keychain:cf"
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
	for _, want := range []string{"domain is required", "account_id is required", "api_token is required", "command is required", "port 99999"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
}

func TestParseRejectsCollisions(t *testing.T) {
	const clash = `
[infra]
domain     = "example.com"
account_id = "a"
zone_id    = "z"
tunnel_id  = "u"
api_token  = "keychain:cf"
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
domain     = "example.com"
account_id = "a"
zone_id    = "z"
tunnel_id  = "u"
api_token  = "keychain:cf"
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

// The API token is the most powerful credential in the system — it can modify
// the zone's DNS — so it goes through the same reference check as any other
// secret, and the error must not echo a pasted value.
func TestParseRejectsAPastedAPIToken(t *testing.T) {
	const pasted = `
[infra]
domain     = "example.com"
account_id = "a"
zone_id    = "z"
tunnel_id  = "u"
api_token  = "v1.0-actual-cloudflare-token-value"
[mcp.sn]
command   = "x"
subdomain = "sn"
`
	_, err := Parse([]byte(pasted), "test.toml")
	if err == nil {
		t.Fatal("a pasted Cloudflare API token was accepted as a reference")
	}
	if strings.Contains(err.Error(), "v1.0-actual-cloudflare-token-value") {
		t.Errorf("the error echoed the token value: %v", err)
	}
	if !strings.Contains(err.Error(), "api_token") {
		t.Errorf("the error should name the field: %v", err)
	}
}

// A tunnel name where an id belongs is a likely migration mistake, so the
// absence of each id is reported by name rather than as one vague failure.
func TestParseNamesEachMissingInfraField(t *testing.T) {
	const bare = `
[infra]
domain = "example.com"
[mcp.sn]
command   = "x"
subdomain = "sn"
`
	_, err := Parse([]byte(bare), "test.toml")
	if err == nil {
		t.Fatal("a config with no tunnel identifiers was accepted")
	}
	for _, want := range []string{"account_id is required", "zone_id is required", "tunnel_id is required", "api_token is required"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%s", want, err)
		}
	}
}
