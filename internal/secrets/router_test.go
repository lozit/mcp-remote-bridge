package secrets

import (
	"errors"
	"strings"
	"testing"
)

type stub struct {
	prefix string
	got    string
}

func (s *stub) Prefix() string { return s.prefix }
func (s *stub) Get(name string) (string, error) {
	s.got = name
	if name == "absent" {
		return "", errors.New("no such secret")
	}
	return "the-value", nil
}

func TestTheReferenceChoosesTheBackend(t *testing.T) {
	mac := &stub{prefix: "keychain:"}
	linux := &stub{prefix: "systemd-creds:"}
	r := NewRouter(mac, linux)

	if _, err := r.Get("systemd-creds:cf-api-token"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if linux.got != "cf-api-token" {
		t.Errorf("the Linux backend received %q, want the name without its prefix", linux.got)
	}
	if mac.got != "" {
		t.Errorf("the macOS backend was consulted for a Linux reference (%q)", mac.got)
	}
}

// The message a reader gets here decides whether they spend a minute or an
// afternoon: a Linux reference on a macOS build is not a typo, it is a config
// written for another machine.
func TestAReferenceForAnotherPlatformSaysSo(t *testing.T) {
	r := NewRouter(&stub{prefix: "keychain:"})

	_, err := r.Get("systemd-creds:cf-api-token")
	if err == nil {
		t.Fatal("a Linux reference resolved on a build that has no Linux backend")
	}
	if !strings.Contains(err.Error(), "Linux") {
		t.Errorf("the error does not say which platform the reference belongs to: %v", err)
	}
	if !strings.Contains(err.Error(), "another machine") {
		t.Errorf("the error reads like a typo rather than a wrong-machine config: %v", err)
	}
}

func TestAnUnknownPrefixListsWhatIsServed(t *testing.T) {
	r := NewRouter(&stub{prefix: "keychain:"})

	_, err := r.Get("vault:secret/data/token")
	if err == nil {
		t.Fatal("an unknown prefix resolved")
	}
	if !strings.Contains(err.Error(), "keychain:") {
		t.Errorf("the error does not say what this build can serve: %v", err)
	}
}

// Never guess. A bare name would have to be interpreted by platform, and the
// wrong guess produces a service that starts and then fails to authenticate —
// the silent 401 rule 3 exists to prevent.
func TestABareNameIsRefusedRatherThanInferred(t *testing.T) {
	r := NewRouter(&stub{prefix: "keychain:"})

	if _, err := r.Get("cf-api-token"); err == nil {
		t.Fatal("a reference with no prefix was resolved by inference")
	}
}

func TestAPrefixWithNothingAfterItIsRefused(t *testing.T) {
	r := NewRouter(&stub{prefix: "keychain:"})

	if _, err := r.Get("keychain:"); err == nil {
		t.Fatal("a reference naming no secret was accepted")
	}
}

func TestAnEmptyReferenceIsRefused(t *testing.T) {
	r := NewRouter(&stub{prefix: "keychain:"})

	if _, err := r.Get(""); err == nil {
		t.Fatal("an empty reference was accepted")
	}
}

// Validate must reject exactly what Get would, or checking a reference before
// prompting for its value would let a bad one through to the prompt.
func TestValidateAgreesWithGet(t *testing.T) {
	r := NewRouter(&stub{prefix: "keychain:"})

	for _, key := range []string{"", "cf-api-token", "keychain:", "vault:x", "systemd-creds:x"} {
		_, getErr := r.Get(key)
		validateErr := r.Validate(key)
		if (getErr == nil) != (validateErr == nil) {
			t.Errorf("Validate and Get disagree on %q: validate=%v get=%v", key, validateErr, getErr)
		}
	}

	if err := r.Validate("keychain:cf-api-token"); err != nil {
		t.Errorf("Validate rejected a reference Get resolves: %v", err)
	}
}

// The backend's own failure must reach the caller unchanged: "no such secret"
// and "wrong machine" send the reader to different places.
func TestABackendFailureIsNotSwallowed(t *testing.T) {
	r := NewRouter(&stub{prefix: "keychain:"})

	_, err := r.Get("keychain:absent")
	if err == nil {
		t.Fatal("a backend failure was swallowed")
	}
	if !strings.Contains(err.Error(), "no such secret") {
		t.Errorf("the backend's own error was replaced: %v", err)
	}
}

// Every prefix a backend answers for must be listed in knownPrefixes, or the
// wrong-machine message silently degrades to "unknown prefix" for it.
func TestEveryKnownPrefixIsAccountedFor(t *testing.T) {
	for _, prefix := range []string{"keychain:", "systemd-creds:", "secret-tool:"} {
		if _, ok := knownPrefixes[prefix]; !ok {
			t.Errorf("%q is served somewhere but absent from knownPrefixes, so a build without "+
				"it would call it a typo", prefix)
		}
	}
	for prefix := range knownPrefixes {
		if !strings.HasSuffix(prefix, ":") {
			t.Errorf("%q has no colon, so CutPrefix would match a longer name", prefix)
		}
	}
}
