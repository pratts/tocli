package engine_test

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pratts/tocli/internal/engine"
	"github.com/pratts/tocli/internal/store"
)

// truncateConfig corrupts id's config.json the same way a torn write would:
// cutting a valid file off partway through. Same technique as
// internal/cli's TestList_CorruptConfigDegradesGracefully.
func truncateConfig(t *testing.T, id string) {
	t.Helper()

	configPath, err := store.ConfigPath(id)
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if len(data) < 10 {
		t.Fatalf("config too short to meaningfully truncate: %d bytes", len(data))
	}
	if err := os.WriteFile(configPath, data[:len(data)/2], 0o644); err != nil {
		t.Fatalf("truncate config: %v", err)
	}
}

// TestRemoveTorrent_CorruptConfigFallsBackToDeletingDirectory covers the
// fix: a config.json that fails to parse must not make RemoveTorrent fail
// outright. It should fall back to deleting the whole tracking directory,
// and -- since there's no readable save path to distinguish them -- do so
// identically whether "remove from list only" or "remove with data" was
// requested.
func TestRemoveTorrent_CorruptConfigFallsBackToDeletingDirectory(t *testing.T) {
	for _, withData := range []bool{false, true} {
		name := "list-only"
		if withData {
			name = "with-data"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			id := "corruptcfg"

			if err := store.InitTorrentDir(id); err != nil {
				t.Fatalf("init torrent dir: %v", err)
			}
			tc := &store.TorrentConfig{
				ID:       id,
				Name:     "torn-write",
				Status:   store.StatusStopped,
				SavePath: t.TempDir(),
			}
			if err := store.SaveTorrentConfig(tc); err != nil {
				t.Fatalf("save torrent config: %v", err)
			}
			truncateConfig(t, id)

			dir, err := store.TorrentDir(id)
			if err != nil {
				t.Fatalf("torrent dir: %v", err)
			}

			outcome, err := engine.RemoveTorrent(id, withData)
			if err != nil {
				t.Fatalf("RemoveTorrent returned an error for a corrupt config: %v", err)
			}
			if !outcome.ConfigUnreadable {
				t.Fatal("expected outcome.ConfigUnreadable to be true")
			}
			if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
				t.Fatalf("expected torrent directory %s to be gone, stat err = %v", dir, statErr)
			}

			got := engine.DescribeRemove(id, outcome, err)
			if !strings.Contains(got, "unreadable") {
				t.Fatalf("DescribeRemove message = %q, want it to mention the config being unreadable", got)
			}
		})
	}
}

// TestRemoveTorrent_CorruptConfigWithLockHeldRefusesToDelete covers the
// other half of the fix: if a real process is still holding the per-torrent
// lock -- the only liveness signal left once config.json can't be read --
// RemoveTorrent must refuse to delete anything and return a clear error,
// rather than deleting out from under a still-running download. Uses a real
// child process (like TestLockConcurrency_OnlyOneChildProceeds) rather than
// an in-process lock acquisition, since that's the actual condition this
// guards against.
func TestRemoveTorrent_CorruptConfigWithLockHeldRefusesToDelete(t *testing.T) {
	binPath, err := buildTocliBinary()
	if err != nil {
		t.Fatalf("build tocli: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	id := "heldlock01"
	setUpSyntheticTorrent(t, id)

	child := exec.Command(binPath, "__run", id)
	if err := child.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	t.Cleanup(func() {
		_ = child.Process.Signal(syscall.SIGKILL)
		_ = child.Wait()
	})

	waitForStatus(t, id, store.StatusRunning, 5*time.Second)

	// Corrupt config.json out from under the still-running child -- exactly
	// the torn-write scenario this fallback exists for, except this time
	// something is genuinely still alive and holding the lock.
	truncateConfig(t, id)

	dir, err := store.TorrentDir(id)
	if err != nil {
		t.Fatalf("torrent dir: %v", err)
	}

	outcome, removeErr := engine.RemoveTorrent(id, false)
	if removeErr == nil {
		t.Fatal("expected RemoveTorrent to refuse while the lock is still held")
	}
	if !outcome.ConfigUnreadable {
		t.Fatal("expected outcome.ConfigUnreadable to be true")
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Fatalf("expected torrent directory to still exist, stat err = %v", statErr)
	}

	got := engine.DescribeRemove(id, outcome, removeErr)
	if !strings.Contains(got, "still appears to be running") {
		t.Fatalf("DescribeRemove message = %q, want it to mention the torrent still running", got)
	}
}
