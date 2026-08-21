// Package keychain implements bridge.SecretSource against the macOS keychain.
//
// It is the only place in the codebase allowed to know about the security
// binary. Values it returns are never logged and never written to disk.
package keychain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Prefix marks a SecretSource key this implementation can resolve, as written
// in the config: secrets = { SN_EMAIL = "keychain:mcp-sn-email" }.
const Prefix = "keychain:"

// notFoundExitCode is what security(1) returns when the item does not exist.
const notFoundExitCode = 44

// ErrNotFound reports a referenced secret that is absent from the keychain.
//
// It is a distinct error on purpose: an absent secret must fail loudly at
// start, never become an empty value injected into the environment.
var ErrNotFound = errors.New("secret not found in the keychain")

// Source resolves secrets from the macOS keychain.
type Source struct {
	// Keychain is the keychain file to search. Empty means the user's default
	// search list. Tests set it to a throwaway keychain so they never touch the
	// real one.
	Keychain string
}

// New returns a keychain-backed SecretSource.
//
// keychain names a specific keychain file; empty means the user's default
// search list.
func New(keychain string) *Source { return &Source{Keychain: keychain} }

// Get resolves key to a secret value.
//
// key is the full reference from the config, including the "keychain:" prefix;
// the remainder is the generic-password service name.
//
// An absent key returns ErrNotFound rather than an empty string: silently
// launching an MCP with a blank credential is the failure mode this whole path
// exists to prevent.
func (s *Source) Get(key string) (string, error) {
	service, err := serviceFromKey(key)
	if err != nil {
		return "", err
	}

	// -g, not -w. -w prints a bare hex string for any value containing a
	// non-printable-ASCII byte, which is indistinguishable from a value that
	// literally is that hex string. See ADR 0004.
	args := []string{"find-generic-password", "-g", "-s", service}
	if s.Keychain != "" {
		args = append(args, s.Keychain)
	}
	cmd := exec.Command("security", args...)

	// -g writes the password to stderr. This buffer holds a secret: it is parsed
	// here and never returned, logged, or attached to an error.
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == notFoundExitCode {
			return "", fmt.Errorf("%q: %w", service, ErrNotFound)
		}
		return "", fmt.Errorf("reading %q from the keychain: %w", service, err)
	}

	value, err := parsePasswordLine(stderr.String())
	if err != nil {
		return "", fmt.Errorf("reading %q from the keychain: %w", service, err)
	}
	return value, nil
}

// Account is the account name written on items this tool creates, so a keychain
// entry is traceable to its origin.
const Account = "mcp-remote-bridge"

// Set stores value under key, replacing any existing item.
//
// The value is written through the process's STDIN, never as an argument.
// `security add-generic-password -w <value>` would place the secret in the
// child's argv, where any local `ps` can read it for the lifetime of the call —
// short, but the invariant says "no secret value on a command line" without an
// exception for duration.
//
// Measured 2026-08-21: `-w` with no value reads the password from stdin and
// asks for it TWICE (entry plus confirmation), so the value is written twice.
// Sending it once makes security report "passwords don't match", retry, read
// EOF twice, and silently store an EMPTY password — a failure that looks like
// success. That is why this is a measured contract and not an obvious one.
func (s *Source) Set(key, value string) error {
	service, err := serviceFromKey(key)
	if err != nil {
		return err
	}

	// security's own usage says: "Specify -w as the last option to be prompted."
	// Nothing may follow it — including the positional [keychain] argument, which
	// -w would otherwise swallow as the password, writing the keychain PATH into
	// the DEFAULT keychain while reporting success. Measured; it is a quiet
	// failure, so this refuses rather than falling back to -w <value>.
	if s.Keychain != "" {
		return fmt.Errorf(
			"cannot write to the keychain at %s: security(1) can target a specific keychain "+
				"or read the password from stdin, but not both, and passing the value as an "+
				"argument would expose it to any local `ps`. Store this secret in the default "+
				"keychain, or add it to %s by hand",
			s.Keychain, s.Keychain)
	}

	cmd := exec.Command("security", "add-generic-password", "-U", "-s", service, "-a", Account, "-w")
	cmd.Stdin = strings.NewReader(value + "\n" + value + "\n")

	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		// The output holds security's prompts, not the value, but it is not
		// worth the risk of quoting it: name the service and nothing else.
		return fmt.Errorf("storing %q in the keychain: %w", service, err)
	}

	// Verify the effect rather than trusting the write: an empty or mismatched
	// read here is exactly the silent-empty-password failure described above.
	stored, err := s.Get(key)
	if err != nil {
		return fmt.Errorf("storing %q appeared to succeed but it cannot be read back: %w", service, err)
	}
	if stored != value {
		return fmt.Errorf("storing %q appeared to succeed but the value read back differs", service)
	}
	return nil
}

// ValidateKey reports whether key is a well-formed keychain reference.
//
// Exported so a caller can check the reference BEFORE prompting for a value:
// asking someone to type a secret and only then rejecting the key wastes the
// one thing that is expensive to re-enter.
func ValidateKey(key string) error {
	_, err := serviceFromKey(key)
	return err
}

// serviceFromKey validates a reference and returns the service name in it.
func serviceFromKey(key string) (string, error) {
	service, ok := strings.CutPrefix(key, Prefix)
	if !ok {
		return "", fmt.Errorf("secret reference %q is not a keychain reference (expected %q prefix)", key, Prefix)
	}
	if service == "" {
		return "", fmt.Errorf("secret reference %q names no service", key)
	}
	return service, nil
}

// parsePasswordLine extracts the value from security -g output.
//
// Two forms, per ADR 0004:
//
//	password: "abc123"
//	password: 0x636166C3A9  "caf\303\251"
//
// The error it returns must never quote the input: that input is the secret.
func parsePasswordLine(out string) (string, error) {
	for line := range strings.SplitSeq(out, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "password:")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)

		if hexRun, ok := strings.CutPrefix(rest, "0x"); ok {
			if i := strings.IndexAny(hexRun, " \t"); i >= 0 {
				hexRun = hexRun[:i]
			}
			raw, err := hex.DecodeString(hexRun)
			if err != nil {
				return "", errors.New("the keychain returned a hex password that could not be decoded")
			}
			return string(raw), nil
		}

		// Quoted form. A value containing a quote is printed unescaped
		// (a"b -> password: "a"b"), so the value runs to the LAST quote, not the
		// next one. A backslash would have forced the hex form, so there is no
		// escape sequence to undo here.
		first := strings.Index(rest, `"`)
		last := strings.LastIndex(rest, `"`)
		if first < 0 || last <= first {
			return "", errors.New("the keychain returned a password line in an unrecognised format")
		}
		return rest[first+1 : last], nil
	}
	return "", errors.New("the keychain returned no password line")
}
