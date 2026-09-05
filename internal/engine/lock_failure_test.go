package engine

import (
	"strings"
	"testing"

	"github.com/pratts/tocli/internal/process"
	"github.com/pratts/tocli/internal/store"
)

// TestRecordLockFailure_DoesNotClobberRunningInstance is item 2's core
// requirement: a child that lost the lock race must never overwrite the
// bookkeeping of the instance that legitimately holds it.
func TestRecordLockFailure_DoesNotClobberRunningInstance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	id := "winner001"
	if err := store.InitTorrentDir(id); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	winner := &store.TorrentConfig{ID: id, Name: "the real one", Status: store.StatusRunning, PID: 4242}
	if err := store.SaveTorrentConfig(winner); err != nil {
		t.Fatalf("save winner config: %v", err)
	}

	if err := recordLockFailure(id, process.ErrLockHeld); err != nil {
		t.Fatalf("recordLockFailure: %v", err)
	}

	got, err := store.LoadTorrentConfig(id)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if got.Status != store.StatusRunning {
		t.Fatalf("status = %q, want unchanged %q (must not clobber the legitimate holder)", got.Status, store.StatusRunning)
	}
	if got.PID != 4242 {
		t.Fatalf("pid = %d, want unchanged 4242", got.PID)
	}
	if got.Message != "" {
		t.Fatalf("message = %q, want empty (nothing to say about a healthy running instance)", got.Message)
	}
}

// TestRecordLockFailure_SurfacesUnexpectedHolder covers the anomalous case:
// our own bookkeeping didn't expect anyone to be running (e.g. "stopped"),
// yet the lock is held by something -- this must be surfaced distinctly
// rather than silently doing nothing.
func TestRecordLockFailure_SurfacesUnexpectedHolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	id := "anomaly01"
	if err := store.InitTorrentDir(id); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	tc := &store.TorrentConfig{ID: id, Name: "unexpected", Status: store.StatusStopped, PID: 0}
	if err := store.SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save config: %v", err)
	}

	if err := recordLockFailure(id, process.ErrLockHeld); err != nil {
		t.Fatalf("recordLockFailure: %v", err)
	}

	got, err := store.LoadTorrentConfig(id)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if got.Status != store.StatusError {
		t.Fatalf("status = %q, want %q", got.Status, store.StatusError)
	}
	if got.Message == "" || !strings.Contains(got.Message, "held") {
		t.Fatalf("message = %q, want it to explain the lock conflict", got.Message)
	}
	if got.PID != 0 {
		t.Fatalf("pid = %d, want 0 (nothing accurate to record)", got.PID)
	}
}
