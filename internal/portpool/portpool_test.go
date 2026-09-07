package portpool

import (
	"testing"

	"github.com/pratts/tocli/internal/config"
	"github.com/pratts/tocli/internal/store"
)

// TestClaim_NoRangeConfiguredReturnsNoPort covers config.Config's
// documented zero-value convention (PortRangeStart 0 means "let the OS
// pick"): Claim must not touch the registry at all in that case.
func TestClaim_NoRangeConfiguredReturnsNoPort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	port, ephemeral, err := Claim("torrentA", config.Config{})
	if err != nil {
		t.Fatalf("Claim returned an error: %v", err)
	}
	if port != 0 || ephemeral {
		t.Fatalf("Claim(no range) = (%d, %v), want (0, false)", port, ephemeral)
	}

	reg, err := store.LoadPortRegistry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if len(reg.Ports) != 0 {
		t.Fatalf("expected an untouched registry, got %+v", reg.Ports)
	}
}

// TestClaim_SequentialLowestAvailable confirms assignment fills the
// configured range from the bottom up, in order -- not an arbitrary free
// port -- since that's the behavior that makes a user's router
// port-forwarding of a specific range actually work as expected.
func TestClaim_SequentialLowestAvailable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := config.Config{PortRangeStart: 6000, PortRangeEnd: 6002}

	want := []int{6000, 6001, 6002}
	for i, id := range []string{"t1", "t2", "t3"} {
		port, ephemeral, err := Claim(id, cfg)
		if err != nil {
			t.Fatalf("Claim(%s): %v", id, err)
		}
		if ephemeral {
			t.Fatalf("Claim(%s) unexpectedly ephemeral", id)
		}
		if port != want[i] {
			t.Fatalf("Claim(%s) = port %d, want %d", id, port, want[i])
		}
	}
}

// TestClaim_RangeExhaustionReturnsEphemeral confirms Claim reports
// exhaustion distinctly (ephemeral=true, no error) rather than either
// erroring or silently reusing an already-claimed port.
func TestClaim_RangeExhaustionReturnsEphemeral(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := config.Config{PortRangeStart: 7000, PortRangeEnd: 7000}

	port, ephemeral, err := Claim("first", cfg)
	if err != nil || ephemeral || port != 7000 {
		t.Fatalf("first Claim = (%d, %v, %v), want (7000, false, nil)", port, ephemeral, err)
	}

	port, ephemeral, err = Claim("second", cfg)
	if err != nil {
		t.Fatalf("second Claim returned an error: %v", err)
	}
	if !ephemeral || port != 0 {
		t.Fatalf("second Claim = (%d, %v), want (0, true) once the range is exhausted", port, ephemeral)
	}
}

// TestReleaseThenClaim_FreedPortIsReclaimedFirst confirms a released port
// becomes genuinely available again -- lowest-available means it's handed
// out before any higher, never-yet-claimed port in the same range, rather
// than staying marked claimed or being skipped.
func TestReleaseThenClaim_FreedPortIsReclaimedFirst(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := config.Config{PortRangeStart: 8000, PortRangeEnd: 8001}

	if port, _, err := Claim("holder", cfg); err != nil || port != 8000 {
		t.Fatalf("Claim(holder) = (%d, %v), want (8000, nil)", port, err)
	}

	if err := Release("holder"); err != nil {
		t.Fatalf("Release(holder): %v", err)
	}

	reg, err := store.LoadPortRegistry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if _, stillClaimed := reg.Ports[8000]; stillClaimed {
		t.Fatalf("expected port 8000 to be freed, registry: %+v", reg.Ports)
	}

	port, ephemeral, err := Claim("next", cfg)
	if err != nil {
		t.Fatalf("Claim(next): %v", err)
	}
	if ephemeral || port != 8000 {
		t.Fatalf("Claim(next) = (%d, %v), want the freed port 8000 reclaimed first", port, ephemeral)
	}
}

// TestRelease_UnclaimedIDIsANoOp confirms Release never errors for a
// torrent that never held a claim -- the ephemeral-fallback and
// no-range-configured cases in internal/engine.Run both call it
// unconditionally-safe-to-call-anyway in some configurations.
func TestRelease_UnclaimedIDIsANoOp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := Release("never-claimed-anything"); err != nil {
		t.Fatalf("Release on an id that never claimed a port returned an error: %v", err)
	}
}
