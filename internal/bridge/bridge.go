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
}

// New returns a Bridge wired to the given seam implementations.
func New(services ServiceManager, exposer Exposer, secrets SecretSource) *Bridge {
	return &Bridge{Services: services, Exposer: exposer, Secrets: secrets}
}

// EnsureExposed guarantees that entry is reachable from outside, and returns a
// probed HealthReport.
//
// Load-bearing rule 1: it reconciles rather than creates. Run twice on a
// healthy entry it is a no-op; run on a drifted entry it repairs only what
// drifted. It never duplicates.
//
// A referenced secret that is absent makes this fail loudly here, at start,
// rather than launching a proxy that will 401 silently.
func (b *Bridge) EnsureExposed(e Entry) (HealthReport, error) {
	return HealthReport{Entry: e.Name}, ErrNotImplemented
}

// RemoveExposed tears down the entry named name, and returns a probed
// HealthReport confirming the teardown.
//
// It is the exact inverse of EnsureExposed. It is always explicit: reconciling
// a config never triggers it, because an edit must not be silently destructive.
func (b *Bridge) RemoveExposed(name string) (HealthReport, error) {
	return HealthReport{Entry: name}, ErrNotImplemented
}

// Probe runs the health checks for entry without changing anything.
func (b *Bridge) Probe(e Entry) (HealthReport, error) {
	return HealthReport{Entry: e.Name}, ErrNotImplemented
}
