package engine_test

import (
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/pratts/tocli/internal/process"
	"github.com/pratts/tocli/internal/store"
)

// TestLockAndReconcileLiveness_NeverContradict confirms, rather than
// assumes, the premise behind item 4: an OS advisory lock is tied to a
// process's open file descriptors, so it is released unconditionally the
// instant that process dies -- for any reason, including a hard SIGKILL
// that gives it no chance to run any cleanup handler at all. That means
// store.ReconcileLiveness (a pid/boot_id inference, working from
// config.json) and the lock (a direct kernel fact) can never actually
// disagree in practice: by the time ReconcileLiveness could ever observe
// "the pid is dead", the very same death has already released the lock.
//
// If that premise were ever violated (e.g. a filesystem whose flock
// semantics are unreliable, such as some NFS configurations), the lock
// should be treated as authoritative, since it's a direct guarantee rather
// than an inference -- but this test exists so that's a confirmed fact
// about this codebase's environment, not an assumption.
func TestLockAndReconcileLiveness_NeverContradict(t *testing.T) {
	binPath, err := buildTocliBinary()
	if err != nil {
		t.Fatalf("build tocli: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	id := "agree0001a"
	setUpSyntheticTorrent(t, id)

	cmd := exec.Command(binPath, "__run", id)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	waitForStatus(t, id, store.StatusRunning, 5*time.Second)

	tc, err := store.LoadTorrentConfig(id)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if tc.PID != cmd.Process.Pid {
		t.Fatalf("config pid = %d, want the child's actual pid %d", tc.PID, cmd.Process.Pid)
	}

	lockPath, err := store.LockPath(id)
	if err != nil {
		t.Fatalf("lock path: %v", err)
	}

	// Sanity check on the premise: while the child is genuinely alive, the
	// lock really is held.
	if _, err := process.AcquireLock(lockPath); !errors.Is(err, process.ErrLockHeld) {
		t.Fatalf("expected the lock to be held while the child is alive, got: %v", err)
	}

	// Hard-kill it: no SIGTERM handler runs, no cleanup, config.json is
	// never touched by the dying process. This is the worst case for the
	// pid/boot_id approach (it's left claiming "running" forever), and
	// exactly the scenario item 4 asks about.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill child: %v", err)
	}
	_ = cmd.Wait()

	// Signal #1: ReconcileLiveness, working only from the stale
	// config.json (still says "running" under the now-dead pid, since
	// nothing ever ran to change it).
	if err := store.ReconcileLiveness(tc); err != nil {
		t.Fatalf("ReconcileLiveness: %v", err)
	}
	if tc.Status != store.StatusCrashed {
		t.Fatalf("ReconcileLiveness status = %q, want %q", tc.Status, store.StatusCrashed)
	}

	// Signal #2: the lock, checked completely independently of
	// config.json. Both signals must agree: the torrent is not actually
	// running.
	release, err := process.AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("expected the lock to be free once its holder was killed, got: %v", err)
	}
	_ = release()
}
