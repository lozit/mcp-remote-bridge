package bridge

import "hash/fnv"

// The range auto-assigned ports are drawn from. Above the common service ports,
// below the ephemeral range macOS allocates from (49152+), so an auto-assigned
// port cannot collide with a port the kernel hands out to an outgoing
// connection.
const (
	autoPortBase = 20000
	autoPortSpan = 10000
)

// AutoPort derives a stable local port from an entry name.
//
// Stability is the requirement, not uniqueness-at-any-cost: the port ends up in
// the service definition, so a port that changed between runs would rewrite the
// plist and restart the service on every apply — and "run twice on a healthy
// entry is a no-op" is load-bearing rule 1.
//
// Derivation is therefore pure: same name, same port, no stored state, and the
// primitive needs no OS knowledge to work out what the port was last time.
//
// FNV-1a is used for its stability, not its distribution: it is specified and
// will not change between Go releases. A collision with an unrelated process is
// possible and is reported as an error telling the user to set `port`
// explicitly — never silently drifted to another port, which would reintroduce
// exactly the instability this avoids.
func AutoPort(name string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return autoPortBase + int(h.Sum32()%autoPortSpan)
}

// ResolvePort returns the port an entry should use: its explicit one, or the
// derived one when it declares none.
func ResolvePort(e Entry) int {
	if e.Port != 0 {
		return e.Port
	}
	return AutoPort(e.Name)
}
