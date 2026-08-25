package bridge

import (
	"errors"
	"net/http"
	"time"
)

// ErrNotImplemented marks a stub that has not been built yet.
//
// It exists so the skeleton compiles and can be wired end to end before any
// real behaviour lands. Every occurrence is a Milestone 1 task in PLAN.md.
var ErrNotImplemented = errors.New("not implemented")

// Bridge is the primitive. It owns no OS knowledge of its own — everything it
// does goes through the three seams.
type Bridge struct {
	Services ServiceManager
	Exposer  Exposer
	Secrets  SecretSource

	// BinaryPath is the absolute path of this binary, resolved at apply time via
	// os.Executable and written into the service definition. The service outlives
	// the shell that created it, so a relative path has nothing to resolve
	// against.
	BinaryPath string

	// HTTPClientForTest overrides the client used for hostname probes.
	//
	// Only tests set it: it is what lets them exercise a five-minute settle
	// budget against a local server, in milliseconds.
	HTTPClientForTest *http.Client

	// ThrottleInterval overrides how long launchd waits before restarting a
	// repeatedly-failing service. Zero means DefaultThrottleInterval.
	//
	// Tests set it low: after a bootout followed by a bootstrap, launchd applies
	// this interval before starting the program at all, so a production value of
	// a minute makes an integration test wait a minute for a service that is
	// already loaded.
	ThrottleInterval time.Duration

	// Sleep is how the Bridge waits. Nil means time.Sleep; tests inject a
	// recorder so a five-minute budget costs nothing to exercise.
	Sleep func(time.Duration)

	// Warn receives messages the caller should see but that do not stop a run —
	// notably an access policy that could not be confirmed. Nil discards them.
	Warn func(string)

	// ConfigPath is the absolute path the launcher reloads the entry from.
	ConfigPath string

	// LogDir is where per-entry proxy logs go — one file each, so two MCPs'
	// output never interleaves.
	LogDir string

	// ProxyPath is the absolute path of mcp-proxy, resolved by the caller at
	// apply time.
	//
	// It is resolved here rather than looked up in the launcher because under
	// launchd the PATH is minimal — measured as /usr/bin:/bin:/usr/sbin:/sbin —
	// so a PATH lookup at launch time fails for anything installed in
	// ~/.local/bin or /opt/homebrew/bin. Resolving at apply time, where the
	// user's own PATH applies, and writing the absolute path into the service
	// definition keeps the plist free of an environment section.
	ProxyPath string
}

// New returns a Bridge wired to the given seam implementations.
func New(services ServiceManager, exposer Exposer, secrets SecretSource) *Bridge {
	return &Bridge{Services: services, Exposer: exposer, Secrets: secrets}
}
