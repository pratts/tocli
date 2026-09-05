package store

import (
	"os"
	"testing"
)

func TestReconcileLiveness_AlivePidUnchanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tc := &TorrentConfig{ID: "alive001", Status: StatusRunning, PID: os.Getpid()}
	if err := InitTorrentDir(tc.ID); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	if err := SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save torrent config: %v", err)
	}

	if err := ReconcileLiveness(tc); err != nil {
		t.Fatalf("ReconcileLiveness: %v", err)
	}
	if tc.Status != StatusRunning {
		t.Fatalf("status = %q, want %q (own pid is definitely alive)", tc.Status, StatusRunning)
	}
}

func TestReconcileLiveness_DeadPidMarkedCrashed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const implausiblePID = 2147483647 // max int32; not a real pid on any supported OS
	tc := &TorrentConfig{ID: "dead0001", Status: StatusRunning, PID: implausiblePID}
	if err := InitTorrentDir(tc.ID); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	if err := SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save torrent config: %v", err)
	}

	if err := ReconcileLiveness(tc); err != nil {
		t.Fatalf("ReconcileLiveness: %v", err)
	}
	if tc.Status != StatusCrashed {
		t.Fatalf("status = %q, want %q", tc.Status, StatusCrashed)
	}
	if tc.PID != 0 {
		t.Fatalf("pid = %d, want 0", tc.PID)
	}

	// The correction must be persisted, not just reflected in the in-memory
	// struct.
	reloaded, err := LoadTorrentConfig(tc.ID)
	if err != nil {
		t.Fatalf("reload torrent config: %v", err)
	}
	if reloaded.Status != StatusCrashed {
		t.Fatalf("persisted status = %q, want %q", reloaded.Status, StatusCrashed)
	}
}

func TestReconcileLiveness_NonRunningStatusIgnored(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tc := &TorrentConfig{ID: "paused01", Status: StatusPaused, PID: 0}
	if err := InitTorrentDir(tc.ID); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	if err := SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save torrent config: %v", err)
	}

	if err := ReconcileLiveness(tc); err != nil {
		t.Fatalf("ReconcileLiveness: %v", err)
	}
	if tc.Status != StatusPaused {
		t.Fatalf("status = %q, want unchanged %q", tc.Status, StatusPaused)
	}
}
