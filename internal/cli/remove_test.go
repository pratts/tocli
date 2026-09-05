package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/pratts/tocli/internal/store"
)

// TestRemove_CompletedTorrentDoesNotSignal confirms that removing a torrent
// that isn't running (here: "completed") skips straight to directory
// cleanup instead of attempting to signal a pid that's known not to be
// running -- and returns no error doing so.
func TestRemove_CompletedTorrentDoesNotSignal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	called := false
	restore := terminateFunc
	terminateFunc = func(pid int, timeout time.Duration) error {
		called = true
		return nil
	}
	t.Cleanup(func() { terminateFunc = restore })

	tc := &store.TorrentConfig{
		ID:       "done00001",
		Name:     "finished-torrent",
		Status:   store.StatusCompleted,
		PID:      0,
		SavePath: t.TempDir(),
	}
	if err := store.InitTorrentDir(tc.ID); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	if err := store.SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save torrent config: %v", err)
	}

	out := new(bytes.Buffer)
	root := NewRootCmd()
	root.SetArgs([]string{"remove", tc.ID})
	root.SetOut(out)

	if err := root.Execute(); err != nil {
		t.Fatalf("remove returned an error: %v", err)
	}
	if called {
		t.Fatal("terminateFunc was called for a torrent that was already completed")
	}

	if _, err := store.TorrentDir(tc.ID); err != nil {
		t.Fatalf("torrent dir: %v", err)
	}
	if _, err := store.LoadTorrentConfig(tc.ID); err == nil {
		t.Fatal("expected the torrent's directory to have been removed")
	}
}

// TestRemove_StaleRunningStatusDoesNotSignal covers the case relevant to
// items 1/2: a config.json that still says "running" but whose process is
// actually dead (or predates a reboot) should also be reconciled before
// remove decides whether to signal anything.
func TestRemove_StaleRunningStatusDoesNotSignal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	called := false
	restore := terminateFunc
	terminateFunc = func(pid int, timeout time.Duration) error {
		called = true
		return nil
	}
	t.Cleanup(func() { terminateFunc = restore })

	const implausiblePID = 2147483647
	tc := &store.TorrentConfig{
		ID:       "stalerun01",
		Name:     "half-dead",
		Status:   store.StatusRunning,
		PID:      implausiblePID,
		SavePath: t.TempDir(),
	}
	if err := store.InitTorrentDir(tc.ID); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	if err := store.SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save torrent config: %v", err)
	}

	root := NewRootCmd()
	root.SetArgs([]string{"remove", tc.ID})
	root.SetOut(new(bytes.Buffer))

	if err := root.Execute(); err != nil {
		t.Fatalf("remove returned an error: %v", err)
	}
	if called {
		t.Fatal("terminateFunc was called for a torrent whose recorded pid was already dead")
	}
}
