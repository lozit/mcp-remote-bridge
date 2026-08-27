// Package secrets routes a secret reference to the backend that can resolve it.
//
// A reference carries its own backend — "keychain:name", "systemd-creds:name",
// "secret-tool:name" — rather than being interpreted according to whatever
// machine happens to be reading it. That is the whole point: a config file is
// meant to live in a dotfiles repo and be read on several machines, and a
// reference that only says "name" would silently resolve to something
// different on each. See ADR 0012.
package secrets

import (
	"fmt"
	"sort"
	"strings"
)

// Backend resolves references carrying one prefix.
type Backend interface {
	// Prefix is the reference prefix this backend answers for, including the
	// colon — "keychain:".
	Prefix() string
	// Get resolves the part after the prefix.
	Get(name string) (string, error)
}

// knownPrefixes names every backend the project has, with the platform it
// belongs to — including those NOT compiled into this binary.
//
// Without this, a macOS binary reading a Linux reference could only say
// "unknown prefix", which reads like a typo. It is not a typo: it is a config
// written for another machine, and saying so is the difference between a
// minute and an afternoon.
var knownPrefixes = map[string]string{
	"keychain:":      "macOS",
	"systemd-creds:": "Linux",
	"secret-tool:":   "Linux",
}

// Router dispatches a reference to the backend whose prefix it carries.
//
// It never guesses. A reference with no prefix, or one this binary cannot
// serve, is an error — choosing a backend by inference would produce a service
// that starts and then fails to authenticate, which is the silent 401 rule 3
// exists to prevent.
type Router struct {
	backends map[string]Backend
}

// NewRouter returns a Router serving the given backends.
func NewRouter(backends ...Backend) *Router {
	r := &Router{backends: make(map[string]Backend, len(backends))}
	for _, b := range backends {
		r.backends[b.Prefix()] = b
	}
	return r
}

// Get resolves a full reference, prefix included.
func (r *Router) Get(key string) (string, error) {
	backend, err := r.backendFor(key)
	if err != nil {
		return "", err
	}
	name := strings.TrimPrefix(key, backend.Prefix())
	return backend.Get(name)
}

// Validate reports whether a reference could be resolved by this binary,
// without resolving it.
//
// Checking the reference before prompting for a value matters: asking someone
// to paste a token and only then rejecting the key wastes the one input that
// is annoying to produce twice.
func (r *Router) Validate(key string) error {
	_, err := r.backendFor(key)
	return err
}

func (r *Router) backendFor(key string) (Backend, error) {
	if key == "" {
		return nil, fmt.Errorf("empty secret reference")
	}

	for prefix, backend := range r.backends {
		if name, ok := strings.CutPrefix(key, prefix); ok {
			if name == "" {
				return nil, fmt.Errorf("secret reference %q names nothing after %q", key, prefix)
			}
			return backend, nil
		}
	}

	// A known prefix this binary does not serve is a different problem from an
	// unknown one, and deserves a different sentence.
	for prefix, platform := range knownPrefixes {
		if strings.HasPrefix(key, prefix) {
			return nil, fmt.Errorf(
				"secret reference %q is a %s reference, which this build cannot resolve; "+
					"this config was written for another machine", key, platform)
		}
	}

	return nil, fmt.Errorf("secret reference %q carries no known backend prefix (this build serves: %s)",
		key, strings.Join(r.prefixes(), ", "))
}

// Prefixes lists what this Router can serve, sorted — for doctor, and for the
// error above.
func (r *Router) Prefixes() []string { return r.prefixes() }

func (r *Router) prefixes() []string {
	out := make([]string, 0, len(r.backends))
	for prefix := range r.backends {
		out = append(out, prefix)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return []string{"none"}
	}
	return out
}
