package bridge

import "errors"

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
