package engine_test

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/pratts/tocli/internal/config"
	"github.com/pratts/tocli/internal/store"
)

// syncBuffer is a bytes.Buffer safe for the concurrent write-while-reading
// pattern a live child process's stderr capture needs here: os/exec copies
// into it from a background goroutine for as long as the child is alive,
// while the test reads it from the main goroutine once a condition (e.g.
// "reached status running") is observed -- a plain bytes.Buffer would be a
// data race under -race for that combination.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestPortPool_ConcurrentTorrentsGetDistinctPorts spawns several real
// `__run` processes (genuine separate OS processes -- see
// buildTocliBinary's comment for why goroutines wouldn't faithfully
// exercise this) against a port range exactly as large as the number of
// torrents, and confirms every one lands on a distinct port within it with
// no collision -- proving the actual property under test (the shared
// registry file never double-assigns a port under real concurrent
// startup) by reading that registry back, rather than depending on
// platform-specific tools like lsof/netstat to observe the OS-level bind.
func TestPortPool_ConcurrentTorrentsGetDistinctPorts(t *testing.T) {
	binPath, err := buildTocliBinary()
	if err != nil {
		t.Fatalf("build tocli: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	cfg := config.Default()
	const n = 3
	cfg.PortRangeStart = 41000
	cfg.PortRangeEnd = cfg.PortRangeStart + n - 1
	if err := config.Save(&cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	ids := make([]string, n)
	children := make([]*exec.Cmd, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("concport%02d", i)
		ids[i] = id
		setUpSyntheticTorrent(t, id)

		child := exec.Command(binPath, "__run", id)
		if err := child.Start(); err != nil {
			t.Fatalf("start child %s: %v", id, err)
		}
		children[i] = child
	}
	t.Cleanup(func() {
		for _, c := range children {
			_ = c.Process.Signal(syscall.SIGTERM)
			_ = c.Wait()
		}
	})

	for _, id := range ids {
		waitForStatus(t, id, store.StatusRunning, 5*time.Second)
	}

	reg, err := store.LoadPortRegistry()
	if err != nil {
		t.Fatalf("load port registry: %v", err)
	}
	if len(reg.Ports) != n {
		t.Fatalf("expected %d claimed ports, got %d: %+v", n, len(reg.Ports), reg.Ports)
	}

	seenIDs := make(map[string]bool, n)
	for port, id := range reg.Ports {
		if port < cfg.PortRangeStart || port > cfg.PortRangeEnd {
			t.Errorf("claimed port %d outside configured range [%d-%d]", port, cfg.PortRangeStart, cfg.PortRangeEnd)
		}
		if seenIDs[id] {
			t.Errorf("torrent %s appears to hold more than one port: %+v", id, reg.Ports)
		}
		seenIDs[id] = true
	}
	for _, id := range ids {
		if !seenIDs[id] {
			t.Errorf("torrent %s never claimed a port: %+v", id, reg.Ports)
		}
	}
}

// TestPortPool_FreedPortBecomesAvailableToNextTorrent confirms a port
// released by its holder exiting (here, a graceful SIGTERM/pause) is
// genuinely available again -- not left marked claimed -- by spawning a
// second real torrent into a range with exactly one port and confirming it
// gets that same port back.
func TestPortPool_FreedPortBecomesAvailableToNextTorrent(t *testing.T) {
	binPath, err := buildTocliBinary()
	if err != nil {
		t.Fatalf("build tocli: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	cfg := config.Default()
	cfg.PortRangeStart = 41200
	cfg.PortRangeEnd = 41200
	if err := config.Save(&cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	id1 := "freedport1"
	setUpSyntheticTorrent(t, id1)
	first := exec.Command(binPath, "__run", id1)
	if err := first.Start(); err != nil {
		t.Fatalf("start first child: %v", err)
	}
	waitForStatus(t, id1, store.StatusRunning, 5*time.Second)

	reg, err := store.LoadPortRegistry()
	if err != nil {
		t.Fatalf("load port registry: %v", err)
	}
	if reg.Ports[cfg.PortRangeStart] != id1 {
		t.Fatalf("expected port %d claimed by %s, registry: %+v", cfg.PortRangeStart, id1, reg.Ports)
	}

	// Graceful stop (mirrors `tocli pause`): Run's SIGTERM path releases
	// the port as one of its last actions before exiting, same as it
	// already does for the per-torrent lock.
	if err := first.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal first child: %v", err)
	}
	if err := first.Wait(); err != nil {
		t.Fatalf("first child did not exit cleanly after SIGTERM: %v", err)
	}

	id2 := "freedport2"
	setUpSyntheticTorrent(t, id2)
	second := exec.Command(binPath, "__run", id2)
	if err := second.Start(); err != nil {
		t.Fatalf("start second child: %v", err)
	}
	t.Cleanup(func() {
		_ = second.Process.Signal(syscall.SIGTERM)
		_ = second.Wait()
	})
	waitForStatus(t, id2, store.StatusRunning, 5*time.Second)

	reg, err = store.LoadPortRegistry()
	if err != nil {
		t.Fatalf("load port registry: %v", err)
	}
	if reg.Ports[cfg.PortRangeStart] != id2 {
		t.Fatalf("expected freed port %d to be reclaimed by %s, registry: %+v", cfg.PortRangeStart, id2, reg.Ports)
	}
}

// TestPortPool_RangeExhaustionFallsBackToEphemeralPort confirms that a
// second torrent starting once the (single-port) range is already fully
// claimed still reaches "running" -- exhaustion must change which port it
// listens on, not stop it from starting -- and that the exhaustion is
// logged clearly to its log.txt.
func TestPortPool_RangeExhaustionFallsBackToEphemeralPort(t *testing.T) {
	binPath, err := buildTocliBinary()
	if err != nil {
		t.Fatalf("build tocli: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	cfg := config.Default()
	cfg.PortRangeStart = 41300
	cfg.PortRangeEnd = 41300
	if err := config.Save(&cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	id1 := "exhaust01"
	setUpSyntheticTorrent(t, id1)
	first := exec.Command(binPath, "__run", id1)
	if err := first.Start(); err != nil {
		t.Fatalf("start first child: %v", err)
	}
	t.Cleanup(func() {
		_ = first.Process.Signal(syscall.SIGTERM)
		_ = first.Wait()
	})
	waitForStatus(t, id1, store.StatusRunning, 5*time.Second)

	id2 := "exhaust02"
	setUpSyntheticTorrent(t, id2)
	second := exec.Command(binPath, "__run", id2)
	// Run's log.Printf output only lands in log.txt when spawned via
	// process.SpawnDetached (as `tocli start`/`resume` do); this test
	// invokes `__run` directly, like the other tests in this file, so it
	// captures the child's own stderr instead to observe the warning.
	var stderr syncBuffer
	second.Stderr = &stderr
	if err := second.Start(); err != nil {
		t.Fatalf("start second child: %v", err)
	}
	t.Cleanup(func() {
		_ = second.Process.Signal(syscall.SIGTERM)
		_ = second.Wait()
	})

	// Must still reach "running": exhaustion is a fallback, not a failure
	// to start.
	waitForStatus(t, id2, store.StatusRunning, 5*time.Second)

	got := stderr.String()
	if !strings.Contains(got, "port range") || !strings.Contains(got, "exhausted") {
		t.Fatalf("expected an exhaustion warning on stderr, got:\n%s", got)
	}

	// The exhausted torrent must not have claimed the single port -- it's
	// still (and only) held by the first torrent.
	reg, err := store.LoadPortRegistry()
	if err != nil {
		t.Fatalf("load port registry: %v", err)
	}
	if len(reg.Ports) != 1 || reg.Ports[cfg.PortRangeStart] != id1 {
		t.Fatalf("expected only %s to hold the single configured port, registry: %+v", id1, reg.Ports)
	}
}
