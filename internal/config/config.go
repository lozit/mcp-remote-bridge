// Package config loads the declarative config file and turns it into entries
// the primitive can act on.
//
// It is deliberately strict: this file is hand-edited and its fields decide
// which hostname gets published, so an unrecognised key is an error rather than
// something quietly ignored. See ADR 0005.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/lozit/mcp-remote-bridge/internal/bridge"
	"github.com/lozit/mcp-remote-bridge/internal/keychain"
)

// File is the on-disk shape of the config.
type File struct {
	Infra Infra          `toml:"infra"`
	MCP   map[string]MCP `toml:"mcp"`
}

// Infra is the shared infrastructure every entry references.
//
// The tunnel is a precondition: it is assumed to exist as a remotely-managed
// tunnel, installed from a token. This tool adds hostnames to it through the
// Cloudflare API — there is no local ingress file to write. See ADR 0006.
type Infra struct {
	Domain string `toml:"domain"`

	// AccountID, ZoneID and TunnelID address the tunnel through the API.
	// TunnelID is a UUID rather than a name.
	AccountID string `toml:"account_id"`
	ZoneID    string `toml:"zone_id"`
	TunnelID  string `toml:"tunnel_id"`

	// APIToken is a SecretSource reference, validated like any other secret
	// reference: the config never holds the value.
	APIToken string `toml:"api_token"`

	// AccessClientID and AccessClientSecret authenticate probes to a hostname
	// guarded by a Cloudflare Access policy. Optional: an unguarded hostname
	// needs neither.
	//
	// The id is an identifier and sits here in clear; the secret is a reference,
	// validated like every other.
	AccessClientID     string `toml:"access_client_id"`
	AccessClientSecret string `toml:"access_client_secret"`

	// AccessPolicyID names an existing Access policy to attach to the
	// applications this tool creates in front of its hostnames (ADR 0007).
	//
	// An existing policy is reused rather than authored: the account already has
	// one that works, and a policy invented here could lock out a working MCP.
	// Empty means the tool publishes hostnames without guarding them — which the
	// access-policy check will then refuse, per ADR 0001.
	AccessPolicyID string `toml:"access_policy_id"`

	// Keychain optionally names a specific keychain file to resolve secrets
	// from. Empty means the user's default search list.
	//
	// A dedicated keychain for MCP credentials is a reasonable setup: it can be
	// locked independently of the login keychain, so the blast radius of an
	// unlocked session is smaller.
	Keychain string `toml:"keychain"`
}

// MCP is one [mcp.<name>] table.
type MCP struct {
	Command   string            `toml:"command"`
	Args      []string          `toml:"args"`
	Subdomain string            `toml:"subdomain"`
	Port      int               `toml:"port"`
	Env       map[string]string `toml:"env"`

	// AllowPublic acknowledges an entry deliberately served without
	// authentication. Without it, apply refuses an entry proven to be open.
	AllowPublic bool              `toml:"allow_public"`
	Secrets     map[string]string `toml:"secrets"`
}

// knownSecretPrefixes are the SecretSource schemes a reference may name.
//
// A secrets value that matches none of these is rejected rather than resolved:
// the most likely reason for a bare value is that someone pasted the secret
// itself, and rule 3 says the config never holds one.
var knownSecretPrefixes = []string{keychain.Prefix}

// DefaultPath is $XDG_CONFIG_HOME/mcp-remote-bridge/config.toml, falling back to
// ~/.config when XDG_CONFIG_HOME is unset.
func DefaultPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locating the config directory: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "mcp-remote-bridge", "config.toml"), nil
}

// Load reads and validates the config at path.
//
// Every problem it can see is reported, not just the first: a hand-edited file
// with three typos should take one round trip to fix, not three.
func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the config: %w", err)
	}
	return Parse(raw, path)
}

// Parse validates config bytes. path is used only in messages.
func Parse(raw []byte, path string) (*File, error) {
	var f File
	dec := toml.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) {
			return nil, fmt.Errorf("%s: unrecognised key — a typo here can publish an MCP at the wrong hostname:\n%s", path, strict.String())
		}
		var de *toml.DecodeError
		if errors.As(err, &de) {
			return nil, fmt.Errorf("%s:\n%s", path, de.String())
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if problems := f.validate(); len(problems) > 0 {
		return nil, fmt.Errorf("%s is not usable:\n  - %s", path, strings.Join(problems, "\n  - "))
	}
	return &f, nil
}

func (f *File) validate() []string {
	var problems []string

	if f.Infra.Domain == "" {
		problems = append(problems, "[infra] domain is required")
	}
	for _, req := range []struct{ name, value string }{
		{"account_id", f.Infra.AccountID},
		{"zone_id", f.Infra.ZoneID},
		{"tunnel_id", f.Infra.TunnelID},
	} {
		if req.value == "" {
			problems = append(problems, fmt.Sprintf("[infra] %s is required (the Cloudflare API addresses tunnels by id, not by name)", req.name))
		}
	}
	// Both or neither: a half-configured service token is sent, rejected, and
	// read as "the MCP is down" — a misleading red rather than a visible
	// misconfiguration.
	if (f.Infra.AccessClientID == "") != (f.Infra.AccessClientSecret == "") {
		problems = append(problems, "[infra] access_client_id and access_client_secret must be set together, or neither")
	}
	if f.Infra.AccessClientSecret != "" && !isSecretReference(f.Infra.AccessClientSecret) {
		problems = append(problems, fmt.Sprintf(
			"[infra] access_client_secret is not a secret reference (expected one of %s)",
			strings.Join(knownSecretPrefixes, ", ")))
	}
	if f.Infra.APIToken == "" {
		problems = append(problems, "[infra] api_token is required (a secret reference to a Cloudflare API token)")
	} else if !isSecretReference(f.Infra.APIToken) {
		// Never echoes the value: the likely cause is a pasted token, and this
		// one can modify the zone's DNS.
		problems = append(problems, fmt.Sprintf(
			"[infra] api_token is not a secret reference (expected one of %s). "+
				"The config holds references, never values — store the token with `set-secret` and reference it here",
			strings.Join(knownSecretPrefixes, ", ")))
	}
	if len(f.MCP) == 0 {
		problems = append(problems, "no [mcp.<name>] entry: there is nothing to expose")
	}

	seenSubdomain := map[string]string{}
	seenPort := map[int]string{}

	for _, name := range sortedKeys(f.MCP) {
		e := f.MCP[name]
		at := fmt.Sprintf("[mcp.%s]", name)

		if err := bridge.ValidateName(name); err != nil {
			problems = append(problems, fmt.Sprintf("%s %v", at, err))
		}
		if e.Command == "" {
			problems = append(problems, fmt.Sprintf("%s command is required", at))
		}
		if e.Subdomain == "" {
			problems = append(problems, fmt.Sprintf("%s subdomain is required", at))
		} else if err := bridge.ValidateSubdomain(e.Subdomain); err != nil {
			problems = append(problems, fmt.Sprintf("%s %v", at, err))
		}
		if e.Port < 0 || e.Port > 65535 {
			problems = append(problems, fmt.Sprintf("%s port %d is out of range (1-65535, or omit it to auto-assign)", at, e.Port))
		}

		// Two entries at one hostname, or on one port, is drift the reconciler
		// cannot resolve: whichever runs last wins, silently.
		if e.Subdomain != "" {
			if other, dup := seenSubdomain[e.Subdomain]; dup {
				problems = append(problems, fmt.Sprintf("%s and [mcp.%s] both claim subdomain %q", at, other, e.Subdomain))
			} else {
				seenSubdomain[e.Subdomain] = name
			}
		}
		if e.Port != 0 {
			if other, dup := seenPort[e.Port]; dup {
				problems = append(problems, fmt.Sprintf("%s and [mcp.%s] both claim port %d", at, other, e.Port))
			} else {
				seenPort[e.Port] = name
			}
		}

		for _, varName := range sortedKeys(e.Secrets) {
			ref := e.Secrets[varName]
			if !isSecretReference(ref) {
				// Deliberately does NOT echo the value: if this really is a pasted
				// secret, repeating it in an error message would write it to the
				// terminal and the shell's scrollback.
				problems = append(problems, fmt.Sprintf(
					"%s secrets.%s is not a secret reference (expected one of %s). "+
						"The config holds references, never values — store the value with `set-secret` and reference it here",
					at, varName, strings.Join(knownSecretPrefixes, ", ")))
			}
		}
		for _, varName := range sortedKeys(e.Env) {
			if _, clash := e.Secrets[varName]; clash {
				problems = append(problems, fmt.Sprintf("%s %s is set in both env and secrets", at, varName))
			}
		}
	}
	return problems
}

func isSecretReference(ref string) bool {
	for _, p := range knownSecretPrefixes {
		if rest, ok := strings.CutPrefix(ref, p); ok && rest != "" {
			return true
		}
	}
	return false
}

// Entries returns the config as primitive entries, in a stable order.
func (f *File) Entries() []bridge.Entry {
	out := make([]bridge.Entry, 0, len(f.MCP))
	for _, name := range sortedKeys(f.MCP) {
		e := f.MCP[name]
		out = append(out, bridge.Entry{
			Name:        name,
			Command:     e.Command,
			Args:        e.Args,
			Env:         e.Env,
			Secrets:     e.Secrets,
			Port:        e.Port,
			Subdomain:   e.Subdomain,
			AllowPublic: e.AllowPublic,
			Domain:      f.Infra.Domain,
			AccountID:   f.Infra.AccountID,
			ZoneID:      f.Infra.ZoneID,
			TunnelID:    f.Infra.TunnelID,
			APIToken:    f.Infra.APIToken,

			AccessClientID:     f.Infra.AccessClientID,
			AccessClientSecret: f.Infra.AccessClientSecret,
			AccessPolicyID:     f.Infra.AccessPolicyID,
		})
	}
	return out
}

// Entry returns the single named entry, for the launcher.
func (f *File) Entry(name string) (bridge.Entry, error) {
	for _, e := range f.Entries() {
		if e.Name == name {
			return e, nil
		}
	}
	return bridge.Entry{}, fmt.Errorf("no [mcp.%s] entry in the config", name)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
