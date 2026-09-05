package engine_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/pratts/tocli/internal/store"
)

// buildTocliBinary compiles the real tocli binary once per test run. These
// tests spawn genuine separate OS processes -- the actual mechanism being
// hardened -- rather than approximating the race with goroutines sharing
// one process's signal disposition, which would not faithfully exercise
// the same SIGTERM/SIGKILL semantics `pause`/`remove`/a real shutdown do.
var buildTocliBinary = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "tocli-bin-*")
	if err != nil {
		return "", err
	}
	binPath := filepath.Join(dir, "tocli")
	cmd := exec.Command("go", "build", "-o", binPath, "../../cmd/tocli")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build: %w: %s", err, out)
	}
	return binPath, nil
})

// setUpSyntheticTorrent writes a small multi-piece, tracker-less torrent
// (fully self-contained info dict, no magnet resolution needed) plus a
// pre-created config.json, so `__run <id>` can start downloading
// immediately without any network dependency: GotInfo() resolves
// instantly since the info is already on disk.
func setUpSyntheticTorrent(t *testing.T, id string) *store.TorrentConfig {
	t.Helper()

	dataDir := t.TempDir()
	dataFile := filepath.Join(dataDir, "payload.bin")
	if err := os.WriteFile(dataFile, bytes.Repeat([]byte{0xAB}, 256*1024), 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	info := metainfo.Info{PieceLength: 16 * 1024}
	if err := info.BuildFromFilePath(dataFile); err != nil {
		t.Fatalf("build info: %v", err)
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatalf("marshal info: %v", err)
	}
	mi := &metainfo.MetaInfo{InfoBytes: infoBytes}

	if err := store.InitTorrentDir(id); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	metainfoPath, err := store.MetainfoPath(id)
	if err != nil {
		t.Fatalf("metainfo path: %v", err)
	}
	f, err := os.Create(metainfoPath)
	if err != nil {
		t.Fatalf("create metainfo file: %v", err)
	}
	if err := mi.Write(f); err != nil {
		f.Close()
		t.Fatalf("write metainfo: %v", err)
	}
	f.Close()

	savePath := filepath.Join(t.TempDir(), "download")
	if err := os.MkdirAll(savePath, 0o755); err != nil {
		t.Fatalf("create save path: %v", err)
	}

	tc := &store.TorrentConfig{
		ID:       id,
		Name:     "payload.bin",
		InfoHash: mi.HashInfoBytes().HexString(),
		SavePath: savePath,
		Status:   store.StatusStopped,
	}
	if err := store.SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save torrent config: %v", err)
	}
	return tc
}

func waitForStatus(t *testing.T, id string, want store.Status, timeout time.Duration) *store.TorrentConfig {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last *store.TorrentConfig
	var lastErr error
	for time.Now().Before(deadline) {
		tc, err := store.LoadTorrentConfig(id)
		if err == nil {
			last, lastErr = tc, nil
			if tc.Status == want {
				return tc
			}
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("torrent %s did not reach status %q within %s; last config: %+v (err %v)", id, want, timeout, last, lastErr)
	return nil
}

// waitForPID polls until config.json shows status "running" under
// specifically wantPID. Plain waitForStatus isn't precise enough after a
// SIGKILL: the killed process never gets to update its own "running"
// entry, so it's still sitting there under its old (now-dead) pid, and a
// generic "status == running" check would match that stale leftover
// instead of waiting for the new process's own fresh write.
func waitForPID(t *testing.T, id string, wantPID int, timeout time.Duration) *store.TorrentConfig {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last *store.TorrentConfig
	var lastErr error
	for time.Now().Before(deadline) {
		tc, err := store.LoadTorrentConfig(id)
		if err == nil {
			last, lastErr = tc, nil
			if tc.Status == store.StatusRunning && tc.PID == wantPID {
				return tc
			}
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("torrent %s did not reach status running under pid %d within %s; last config: %+v (err %v)", id, wantPID, timeout, last, lastErr)
	return nil
}

// TestLockConcurrency_OnlyOneChildProceeds spawns two `__run` processes
// against the same torrent id in quick succession -- the exact race a
// resume against a still-alive earlier child (or two racing resumes) would
// produce. Exactly one must win the lock and reach status "running" under
// its own real pid; the other must fail fast (exit non-zero, well before
// it could plausibly have created a torrent.Client and gotten anywhere
// near a status write).
func TestLockConcurrency_OnlyOneChildProceeds(t *testing.T) {
	binPath, err := buildTocliBinary()
	if err != nil {
		t.Fatalf("build tocli: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	id := "race0001aa"
	setUpSyntheticTorrent(t, id)

	spawn := func() *exec.Cmd {
		cmd := exec.Command(binPath, "__run", id)
		return cmd
	}

	first := spawn()
	second := spawn()
	if err := first.Start(); err != nil {
		t.Fatalf("start first child: %v", err)
	}
	if err := second.Start(); err != nil {
		t.Fatalf("start second child: %v", err)
	}

	type result struct {
		cmd      *exec.Cmd
		exitErr  error
		duration time.Duration
	}
	results := make(chan result, 2)
	start := time.Now()
	go func() { err := first.Wait(); results <- result{first, err, time.Since(start)} }()
	go func() { err := second.Wait(); results <- result{second, err, time.Since(start)} }()

	var loser result
	select {
	case loser = <-results:
	case <-time.After(10 * time.Second):
		t.Fatal("neither child exited within 10s; expected the lock loser to fail fast")
	}
	if loser.exitErr == nil {
		t.Fatal("expected the lock loser to exit non-zero (lock contention), got a clean exit")
	}
	// "Fails fast": well under the time it'd take to stand up a
	// torrent.Client (bind a socket, kick off DHT, etc.) -- this is the
	// externally observable proxy for "never created a torrent.Client",
	// which item 2's recordLockFailure tests already confirm by
	// construction (the lock check is the first thing Run does, with a
	// hard return on failure before any client-related code runs).
	if loser.duration > 2*time.Second {
		t.Errorf("lock loser took %s to exit, want it to fail near-instantly", loser.duration)
	}

	winner := first
	if loser.cmd == first {
		winner = second
	}

	tc := waitForStatus(t, id, store.StatusRunning, 5*time.Second)
	if tc.PID != winner.Process.Pid {
		t.Fatalf("config.json pid = %d, want the winner's actual pid %d", tc.PID, winner.Process.Pid)
	}

	// Clean up gracefully (mirrors `tocli pause`); also exercised in
	// TestLockConcurrency_LockFreedAfterChildExits below, so failures here
	// are just cleanup noise, not a second assertion of that property.
	//
	// Drain the winner's exit from the `results` channel -- the goroutine
	// that's already calling winner.Wait() (started above to race against
	// the loser) will deliver it there once the process exits. Calling
	// Wait() a second time directly on the same *exec.Cmd is invalid and
	// racy: its docs specifically say Wait must not be called more than
	// once.
	_ = winner.Process.Signal(syscall.SIGTERM)
	select {
	case <-results:
	case <-time.After(5 * time.Second):
		t.Fatal("winner did not exit within 5s of SIGTERM")
	}
}

// TestLockConcurrency_LockFreedAfterChildExits proves the property that
// makes this approach more robust than the pid/boot_id check alone: once a
// process holding the lock is gone -- whether via a graceful SIGTERM
// shutdown or a hard SIGKILL with no chance to run any handler -- the lock
// is free immediately, with no PID-reuse ambiguity and no reliance on
// boot_id, so a subsequent spawn succeeds right away.
func TestLockConcurrency_LockFreedAfterChildExits(t *testing.T) {
	binPath, err := buildTocliBinary()
	if err != nil {
		t.Fatalf("build tocli: %v", err)
	}

	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		t.Run(sig.String(), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			id := "free0001aa"
			setUpSyntheticTorrent(t, id)

			spawn := func() *exec.Cmd { return exec.Command(binPath, "__run", id) }

			first := spawn()
			if err := first.Start(); err != nil {
				t.Fatalf("start first child: %v", err)
			}
			waitForStatus(t, id, store.StatusRunning, 5*time.Second)

			if err := first.Process.Signal(sig); err != nil {
				t.Fatalf("signal first child: %v", err)
			}
			_ = first.Wait() // exit status is irrelevant for SIGKILL

			second := spawn()
			if err := second.Start(); err != nil {
				t.Fatalf("start second child: %v", err)
			}
			t.Cleanup(func() {
				_ = second.Process.Signal(syscall.SIGTERM)
				_ = second.Wait()
			})

			// wantPID (not a generic "status == running" check): after
			// SIGKILL, config.json is still sitting at "running" under
			// first's now-dead pid -- nothing ever ran to change that. We
			// specifically need the *second* process's own write to land,
			// which only happens if it actually acquired the lock.
			waitForPID(t, id, second.Process.Pid, 5*time.Second)
		})
	}
}
