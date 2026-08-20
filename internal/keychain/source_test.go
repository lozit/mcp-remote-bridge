package keychain

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Unit tests for the parser. The round-trip tests below are what prove the
// parser matches what security(1) actually emits — these only pin the shapes.
func TestParsePasswordLine(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{"plain", `password: "abc123"`, "abc123"},
		{"empty value", `password: ""`, ""},
		{"value containing a quote runs to the last quote", `password: "a"b"`, `a"b`},
		{"hex form", "password: 0x636166C3A9  \"caf\\303\\251\"", "café"},
		{"hex form with newline", "password: 0x610A62  \"a\\012b\"", "a\nb"},
		{"a value that looks like hex is not decoded", `password: "636166c3a9"`, "636166c3a9"},
		{"password line among attribute lines", "keychain: \"x\"\nclass: \"genp\"\npassword: \"abc\"", "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePasswordLine(tt.out)
			if err != nil {
				t.Fatalf("parsePasswordLine: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// The error must never carry the secret, since the input it parses IS the secret.
func TestParsePasswordLineErrorsDoNotLeakInput(t *testing.T) {
	for _, bad := range []string{"", "no password line here", "password: 0xZZZZ  \"junk\"", "password: unquoted"} {
		_, err := parsePasswordLine(bad)
		if err == nil {
			continue
		}
		if strings.Contains(err.Error(), bad) && bad != "" {
			t.Errorf("error quoted its input: %v", err)
		}
	}
}

func TestGetRejectsANonKeychainReference(t *testing.T) {
	s := &Source{}
	for _, key := range []string{"vault:thing", "mcp-sn-email", "", "keychain:"} {
		if _, err := s.Get(key); err == nil {
			t.Errorf("Get(%q) accepted a reference it cannot resolve", key)
		}
	}
}

// --- round trip against a real, throwaway keychain ---

// newTestKeychain creates a keychain in the test's temp dir and deletes it on
// cleanup. It never touches the user's default keychain: every command below is
// given this file explicitly.
func newTestKeychain(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: creates a real keychain")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security(1) not on PATH")
	}

	path := filepath.Join(t.TempDir(), "mcp-remote-bridge-test.keychain")
	if out, err := exec.Command("security", "create-keychain", "-p", "testpass", path).CombinedOutput(); err != nil {
		t.Fatalf("create-keychain: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("security", "delete-keychain", path).Run()
	})
	if out, err := exec.Command("security", "unlock-keychain", "-p", "testpass", path).CombinedOutput(); err != nil {
		t.Fatalf("unlock-keychain: %v: %s", err, out)
	}
	return path
}

func store(t *testing.T, keychain, service, value string) {
	t.Helper()
	out, err := exec.Command("security", "add-generic-password",
		"-U", "-s", service, "-a", "test", "-w", value, keychain).CombinedOutput()
	if err != nil {
		t.Fatalf("add-generic-password: %v: %s", err, out)
	}
}

// The test that matters: every one of these byte sequences must come back
// exactly as stored. The non-ASCII cases are the ones `security -w` corrupts
// into a bare hex string (ADR 0004).
func TestGetRoundTripsEveryByteSequence(t *testing.T) {
	kc := newTestKeychain(t)
	s := &Source{Keychain: kc}

	tests := []struct {
		name  string
		value string
	}{
		{"plain ascii", "abc123"},
		{"an email", "someone@example.com"},
		{"spaces", "a b c"},
		{"a jwt-shaped token", "eyJhbGc.eyJzdWIi.SflKxwRJ"},
		{"an accented password", "café-au-lait"}, // -w returns hex
		{"a tab", "a\tb"},                        // -w returns hex
		{"a newline", "line1\nline2"},            // -w returns hex
		{"a PEM-shaped key", "-----BEGIN KEY-----\nabc\n-----END KEY-----"},
		{"a backslash", `a\b`}, // -w returns hex
		{"shell metacharacters", "a b\"c$(x)`y`|z;&"},
		{"a value that is itself hex", "636166c3a9"}, // must NOT be decoded
		{"emoji", "pass🔐word"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := "mcp-remote-bridge-test-" + strings.ReplaceAll(tt.name, " ", "-")
			store(t, kc, service, tt.value)

			got, err := s.Get(Prefix + service)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got != tt.value {
				t.Errorf("round trip corrupted the secret:\n got %q (% x)\nwant %q (% x)", got, got, tt.value, tt.value)
			}
		})
	}
}

// An absent secret must be a named failure, never an empty string.
func TestGetReportsAMissingSecret(t *testing.T) {
	kc := newTestKeychain(t)
	s := &Source{Keychain: kc}

	got, err := s.Get(Prefix + "mcp-remote-bridge-definitely-absent")

	if err == nil {
		t.Fatalf("Get returned no error for an absent secret (value %q) — a blank credential would reach the MCP", got)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error does not wrap ErrNotFound, so callers cannot tell 'absent' from 'broken': %v", err)
	}
	if got != "" {
		t.Errorf("Get returned a value alongside its error: %q", got)
	}
}
