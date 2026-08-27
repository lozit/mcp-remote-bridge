package secrets

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// The two Linux backends. They exist because the machines in scope disagree:
// a headless host has no session bus and no unlocked keyring, so libsecret is
// useless there; a desktop has both, and systemd-creds is heavier than it needs
// to be. See ADR 0012.
//
// ─────────────────────────────────────────────────────────────────────────────
// UNVERIFIED, and deliberately marked as such.
//
// Neither command line below has ever been RUN. They are written from
// knowledge of these tools, not from measurement, and this project has just
// recorded what that is worth (docs/AGENT-EVALS.md, 2026-08-27: "the agent
// verified with an instrument that could not see the fault").
//
// Each backend therefore keeps its argv in ONE function, pinned by a test, so
// that confirming it on a real Linux host is a one-line change with a test that
// says exactly what moved. Until that happens, treat green tests here as
// evidence about the surrounding logic and about nothing else.
// ─────────────────────────────────────────────────────────────────────────────

// runner executes a command with a value on stdin, returning stdout.
// Injectable so the tests can pin the argv without a Linux host.
type runner func(argv []string, stdin string) (string, error)

func execRunner(argv []string, stdin string) (string, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	// Detach from the controlling terminal, so a backend that decides to prompt
	// blocks visibly instead of silently stealing the terminal — the failure
	// measured on `security` earlier in this project, where a prompt the
	// process could not see surfaced as a timeout.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		// The value never reaches an error message: stderr from these tools
		// does not contain it, but saying so here is what stops someone adding
		// stdout to the message later.
		return "", fmt.Errorf("%s: %w (%s)", argv[0], err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

// ── systemd-creds ────────────────────────────────────────────────────────────

// SystemdCreds resolves "systemd-creds:name" references.
//
// The default on a headless host: credentials are encrypted to the machine (TPM
// or host key) and readable by the service at start with no session, no agent
// and no unlocked keyring. It is the only one of the two that works on a box
// that boots unattended.
type SystemdCreds struct {
	// Dir holds the encrypted blobs. Empty means
	// $XDG_CONFIG_HOME/mcp-remote-bridge/credentials.
	Dir string

	run runner
}

func NewSystemdCreds(dir string) *SystemdCreds {
	return &SystemdCreds{Dir: dir, run: execRunner}
}

func (s *SystemdCreds) Prefix() string { return "systemd-creds:" }

func (s *SystemdCreds) dir() (string, error) {
	if s.Dir != "" {
		return s.Dir, nil
	}
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "mcp-remote-bridge", "credentials"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating the credentials directory: %w", err)
	}
	return filepath.Join(home, ".config", "mcp-remote-bridge", "credentials"), nil
}

func (s *SystemdCreds) path(name string) (string, error) {
	dir, err := s.dir()
	if err != nil {
		return "", err
	}
	// The name reaches a file path, so it must not be able to leave the
	// directory. A reference is user input from a config file.
	if name != filepath.Base(name) || name == "." || name == ".." {
		return "", fmt.Errorf("credential name %q is not a plain name", name)
	}
	return filepath.Join(dir, name+".cred"), nil
}

// decryptArgv is the one place the systemd-creds command line lives. UNVERIFIED.
func decryptArgv(name, path string) []string {
	return []string{"systemd-creds", "--user", "decrypt", "--name=" + name, path, "-"}
}

func (s *SystemdCreds) Get(name string) (string, error) {
	path, err := s.path(name)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		// Fail loudly at start rather than let a proxy come up and 401 later.
		return "", fmt.Errorf("no credential %q at %s: store it with `mcp-remote-bridge set-secret systemd-creds:%s`",
			name, path, name)
	}

	out, err := s.run(decryptArgv(name, path), "")
	if err != nil {
		return "", fmt.Errorf("decrypting credential %q: %w", name, err)
	}
	// systemd-creds writes the value verbatim; a trailing newline would be part
	// of the token and authenticate as garbage.
	return strings.TrimRight(out, "\n"), nil
}

// encryptArgv is the one place the systemd-creds command line lives. UNVERIFIED.
func encryptArgv(name, path string) []string {
	return []string{"systemd-creds", "--user", "encrypt", "--name=" + name, "-", path}
}

// Set stores a value, reading it from stdin so it never appears in an argv.
func (s *SystemdCreds) Set(name, value string) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating the credentials directory: %w", err)
	}
	if _, err := s.run(encryptArgv(name, path), value); err != nil {
		return fmt.Errorf("encrypting credential %q: %w", name, err)
	}
	// The blob is encrypted to the machine, but 0600 anyway: an encrypted
	// secret readable by everyone is still a secret handed to an attacker who
	// only needs the machine.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("restricting %s: %w", path, err)
	}
	return nil
}

// ── secret-tool (libsecret) ──────────────────────────────────────────────────

// SecretTool resolves "secret-tool:name" references.
//
// The desktop answer, and the closest mirror of the macOS keychain. It needs a
// session bus and an unlocked keyring, so a service started at boot would fail
// to read its secrets — which is exactly why it is not the default.
type SecretTool struct {
	// Attribute is the lookup attribute. Empty means "service", matching what
	// the macOS keychain calls it.
	Attribute string

	run runner
}

func NewSecretTool() *SecretTool { return &SecretTool{run: execRunner} }

func (s *SecretTool) Prefix() string { return "secret-tool:" }

func (s *SecretTool) attribute() string {
	if s.Attribute != "" {
		return s.Attribute
	}
	return "service"
}

// lookupArgv is the one place the secret-tool command line lives. UNVERIFIED.
func (s *SecretTool) lookupArgv(name string) []string {
	return []string{"secret-tool", "lookup", s.attribute(), name}
}

func (s *SecretTool) Get(name string) (string, error) {
	out, err := s.run(s.lookupArgv(name), "")
	if err != nil {
		return "", fmt.Errorf("looking up %q in the keyring: %w "+
			"(a locked or absent keyring reports the same as a missing secret)", name, err)
	}
	if out == "" {
		return "", fmt.Errorf("the keyring holds no value for %q", name)
	}
	// secret-tool prints the value without a trailing newline, but a value
	// STORED with one would come back with it. Trimming here would corrupt a
	// secret that legitimately ends in a newline — so it is not trimmed, and
	// `set-secret` is what refuses to store one.
	return out, nil
}

// storeArgv is the one place the secret-tool command line lives. UNVERIFIED.
func (s *SecretTool) storeArgv(name string) []string {
	return []string{"secret-tool", "store", "--label=" + name, s.attribute(), name}
}

// Set stores a value, reading it from stdin so it never appears in an argv.
func (s *SecretTool) Set(name, value string) error {
	if strings.Contains(value, "\n") {
		return fmt.Errorf("refusing to store a value containing a newline: it cannot be read back unambiguously")
	}
	if _, err := s.run(s.storeArgv(name), value); err != nil {
		return fmt.Errorf("storing %q in the keyring: %w", name, err)
	}
	return nil
}
