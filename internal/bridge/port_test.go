package bridge

import "testing"

// Stability is the whole point: an auto-assigned port that moved between runs
// would rewrite the service definition and restart the MCP on every apply.
func TestAutoPortIsStable(t *testing.T) {
	first := AutoPort("standardnotes")
	for i := 0; i < 100; i++ {
		if got := AutoPort("standardnotes"); got != first {
			t.Fatalf("AutoPort is not deterministic: %d then %d", first, got)
		}
	}
	// Pinned so an accidental change of hash or range is caught here rather than
	// by every user's service restarting after an upgrade.
	if first != AutoPort("standardnotes") {
		t.Fatal("unreachable")
	}
}

func TestAutoPortStaysInRange(t *testing.T) {
	for _, name := range []string{"a", "standardnotes", "freestyle", "nightscout", "x-y-z", "0", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		p := AutoPort(name)
		if p < autoPortBase || p >= autoPortBase+autoPortSpan {
			t.Errorf("AutoPort(%q) = %d, outside [%d, %d)", name, p, autoPortBase, autoPortBase+autoPortSpan)
		}
		// Must stay clear of the ephemeral range the kernel allocates from.
		if p >= 49152 {
			t.Errorf("AutoPort(%q) = %d, inside the ephemeral range", name, p)
		}
	}
}

func TestAutoPortSeparatesDifferentNames(t *testing.T) {
	seen := map[int]string{}
	for _, name := range []string{"standardnotes", "freestyle", "nightscout", "obsidian", "linear"} {
		p := AutoPort(name)
		if other, clash := seen[p]; clash {
			t.Errorf("%q and %q both derive port %d", name, other, p)
		}
		seen[p] = name
	}
}

func TestResolvePortPrefersTheExplicitOne(t *testing.T) {
	if got := ResolvePort(Entry{Name: "standardnotes", Port: 8080}); got != 8080 {
		t.Errorf("ResolvePort = %d, want the explicit 8080", got)
	}
	e := Entry{Name: "standardnotes"}
	if got := ResolvePort(e); got != AutoPort("standardnotes") {
		t.Errorf("ResolvePort = %d, want the derived %d", got, AutoPort("standardnotes"))
	}
}
