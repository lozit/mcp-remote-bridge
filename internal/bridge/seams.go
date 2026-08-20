package bridge

import "time"

// The three seams. The primitive talks only to these interfaces, so OS,
// exposer and secret-store variety is additive later rather than a rewrite.
//
// They are declared here, in the consuming package, on purpose: implementations
// live in their own packages and satisfy these implicitly.
//
// Caveat worth remembering: with exactly one implementation each in the MVP,
// these are untested abstractions. Expect them to need adjusting when Linux
// arrives (Milestone 4) — that is the accepted cost of deferring.

// ServiceManager keeps a process alive across login and reboot.
type ServiceManager interface {
	// Ensure writes the service definition and loads it. It is idempotent.
	Ensure(label string, spec ServiceSpec) error
	// Remove unloads the service and deletes its definition.
	Remove(label string) error
	// Status reports what the service manager currently knows about the label.
	Status(label string) (ServiceState, error)
}

// Exposer makes a local port reachable at a public hostname.
type Exposer interface {
	// Ensure adds the ingress rule and its DNS route. It is idempotent.
	Ensure(subdomain, domain string, localPort int) error
	// Remove drops the ingress rule and its DNS route.
	Remove(subdomain, domain string) error
}

// SecretSource resolves a named secret at launch time.
//
// Get is called by the generated launcher immediately before exec, never at
// config-parse time and never at service-write time. The returned value must
// never be logged, written to disk, or placed on a command line.
type SecretSource interface {
	Get(key string) (string, error)
}

// ServiceSpec describes what the service manager should keep alive.
//
// Note what is absent: there is no environment map of secret values. The
// service file is world-readable, so it carries none. Program points at this
// binary and Args select the __launch subcommand, which resolves the entry's
// secret references through a SecretSource and injects them into the process
// environment immediately before exec. See docs/SPEC-launcher.md and ADR 0002.
type ServiceSpec struct {
	// Label is the service identifier (a launchd label, later a systemd unit name).
	Label string

	// Program is the absolute path of the mcp-remote-bridge binary, resolved at
	// apply time via os.Executable.
	//
	// It is absolute on purpose: the service outlives the shell that created it.
	// The consequence is that moving or uninstalling the binary breaks every
	// service, which is why doctor checks this path still exists.
	Program string

	// Args select the launcher: __launch <name> --config <path>.
	//
	// Everything here lands in a world-readable service file, so it carries a
	// name and a path and nothing else.
	Args []string

	// StdoutPath and StderrPath are the known log paths.
	StdoutPath string
	StderrPath string

	// KeepAlive asks the service manager to restart the program when it dies.
	//
	// launchd expresses this as a dictionary rather than a boolean
	// ({SuccessfulExit: false, Crashed: true}), so this is a struct: a bool
	// could not carry the distinction, and the working hand-built plists this
	// generator must match use the dictionary form.
	KeepAlive KeepAlivePolicy

	// ThrottleInterval bounds how fast a repeatedly-failing program is retried.
	//
	// It matters for the unrecoverable case: a secret deleted after apply makes
	// the launcher exit before starting the proxy, and KeepAlive would otherwise
	// spin on it. A slow, visible loop is diagnosable; a spin is noise.
	ThrottleInterval time.Duration
}

// KeepAlivePolicy says when a dead program should be restarted.
//
// The zero value restarts nothing, so a spec that forgets to set it does not
// silently get supervision it never asked for.
type KeepAlivePolicy struct {
	// OnFailure restarts the program when it exits non-zero
	// (launchd: KeepAlive/SuccessfulExit = false).
	OnFailure bool
	// OnCrash restarts the program when it is killed by a signal
	// (launchd: KeepAlive/Crashed = true).
	OnCrash bool
}

// ServiceState is what a ServiceManager knows about a label right now.
type ServiceState struct {
	// Loaded reports whether the service manager has the definition loaded.
	Loaded bool
	// Running reports whether a process is currently alive for it.
	Running bool
	// PID is the running process id, zero when not running.
	PID int
	// LastExitCode is the exit status of the last run, when the service
	// manager reports one.
	LastExitCode int
}
