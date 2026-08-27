package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests pin the command lines and cover the logic around them. They do
// NOT prove either tool behaves as assumed — neither has been run. When the
// first Linux host confirms the real invocation, the failing test here names
// exactly what moved, which is the whole reason the argv lives in one function.

type recorder struct {
	argv   []string
	stdin  string
	out    string
	err    error
	called int
}

func (r *recorder) run(argv []string, stdin string) (string, error) {
	r.argv, r.stdin, r.called = argv, stdin, r.called+1
	return r.out, r.err
}

// ── systemd-creds ────────────────────────────────────────────────────────────

func TestSystemdCredsDecryptCommandLine(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cf-api-token.cred"), []byte("blob"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{out: "the-value\n"}
	s := &SystemdCreds{Dir: dir, run: rec.run}

	got, err := s.Get("cf-api-token")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	want := []string{"systemd-creds", "--user", "decrypt",
		"--name=cf-api-token", filepath.Join(dir, "cf-api-token.cred"), "-"}
	if strings.Join(rec.argv, " ") != strings.Join(want, " ") {
		t.Errorf("argv changed:\n got %v\nwant %v", rec.argv, want)
	}
	// A trailing newline would be part of the token and would authenticate as
	// garbage — a 401 whose cause is invisible.
	if got != "the-value" {
		t.Errorf("Get = %q, want the value without its trailing newline", got)
	}
}

// A missing credential must fail at start, not produce a proxy that 401s later.
func TestAnAbsentCredentialFailsWithSomethingToDo(t *testing.T) {
	rec := &recorder{}
	s := &SystemdCreds{Dir: t.TempDir(), run: rec.run}

	_, err := s.Get("cf-api-token")
	if err == nil {
		t.Fatal("an absent credential resolved")
	}
	if rec.called != 0 {
		t.Error("systemd-creds was run for a file that does not exist")
	}
	if !strings.Contains(err.Error(), "set-secret") {
		t.Errorf("the error does not say how to fix it: %v", err)
	}
}

// The name comes from a config file and reaches a file path.
func TestACredentialNameCannotEscapeItsDirectory(t *testing.T) {
	s := &SystemdCreds{Dir: t.TempDir(), run: (&recorder{}).run}

	// Asserting only that Get errors proves nothing: a traversing name errors
	// anyway because the file is absent. The guard is what must fire, so the
	// test names it. (Written the weak way first, and removing the guard did
	// not fail it.)
	for _, name := range []string{"../../etc/shadow", "a/b", "..", "."} {
		_, err := s.Get(name)
		if err == nil {
			t.Errorf("%q was accepted as a credential name", name)
			continue
		}
		if !strings.Contains(err.Error(), "not a plain name") {
			t.Errorf("%q was rejected for the wrong reason (%v) — the path guard did not fire",
				name, err)
		}
	}
}

func TestSystemdCredsSetPutsTheValueOnStdinAndLocksTheFile(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}
	s := &SystemdCreds{Dir: dir, run: rec.run}
	// The real command writes the file; the fake does not, so create it to let
	// the chmod that follows be exercised.
	path := filepath.Join(dir, "cf-api-token.cred")
	if err := os.WriteFile(path, []byte("blob"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.Set("cf-api-token", "SECRET-VALUE"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if rec.stdin != "SECRET-VALUE" {
		t.Errorf("the value did not travel on stdin: %q", rec.stdin)
	}
	for _, arg := range rec.argv {
		if strings.Contains(arg, "SECRET-VALUE") {
			t.Errorf("the value appears in the argv: %v", rec.argv)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("credential mode = %v, want 0600", info.Mode().Perm())
	}
}

// ── secret-tool ──────────────────────────────────────────────────────────────

func TestSecretToolLookupCommandLine(t *testing.T) {
	rec := &recorder{out: "the-value"}
	s := &SecretTool{run: rec.run}

	got, err := s.Get("cf-api-token")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	want := []string{"secret-tool", "lookup", "service", "cf-api-token"}
	if strings.Join(rec.argv, " ") != strings.Join(want, " ") {
		t.Errorf("argv changed:\n got %v\nwant %v", rec.argv, want)
	}
	if got != "the-value" {
		t.Errorf("Get = %q", got)
	}
}

// An empty result is not a value. Returning "" would install an empty secret
// and produce an authentication failure with no obvious cause.
func TestAnEmptyLookupIsAnError(t *testing.T) {
	s := &SecretTool{run: (&recorder{out: ""}).run}

	if _, err := s.Get("cf-api-token"); err == nil {
		t.Fatal("an empty lookup was returned as a value")
	}
}

// A locked keyring and an absent secret are indistinguishable from here, and
// the message must not claim otherwise.
func TestALockedKeyringIsNotReportedAsAMissingSecret(t *testing.T) {
	s := &SecretTool{run: (&recorder{err: errors.New("Cannot autolaunch D-Bus")}).run}

	_, err := s.Get("cf-api-token")
	if err == nil {
		t.Fatal("a failing secret-tool resolved")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Errorf("the error asserts the secret is missing when it may be a locked keyring: %v", err)
	}
}

func TestSecretToolSetPutsTheValueOnStdin(t *testing.T) {
	rec := &recorder{}
	s := &SecretTool{run: rec.run}

	if err := s.Set("cf-api-token", "SECRET-VALUE"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if rec.stdin != "SECRET-VALUE" {
		t.Errorf("the value did not travel on stdin: %q", rec.stdin)
	}
	for _, arg := range rec.argv {
		if strings.Contains(arg, "SECRET-VALUE") {
			t.Errorf("the value appears in the argv: %v", rec.argv)
		}
	}
	want := []string{"secret-tool", "store", "--label=cf-api-token", "service", "cf-api-token"}
	if strings.Join(rec.argv, " ") != strings.Join(want, " ") {
		t.Errorf("argv changed:\n got %v\nwant %v", rec.argv, want)
	}
}

// Get deliberately does not trim, so a value that cannot be read back
// unambiguously must be refused at write time instead.
func TestAValueWithANewlineIsRefusedAtWrite(t *testing.T) {
	s := &SecretTool{run: (&recorder{}).run}

	if err := s.Set("cf-api-token", "line1\nline2"); err == nil {
		t.Fatal("a value containing a newline was stored")
	}
}

// Both backends must satisfy the interface the router dispatches to.
func TestBothBackendsAreRoutable(t *testing.T) {
	var _ Backend = NewSystemdCreds("")
	var _ Backend = NewSecretTool()

	r := NewRouter(NewSystemdCreds(t.TempDir()), NewSecretTool())
	for _, key := range []string{"systemd-creds:x", "secret-tool:x"} {
		if err := r.Validate(key); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", key, err)
		}
	}
}
