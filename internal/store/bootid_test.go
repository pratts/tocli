package store

import (
	"os"
	"testing"
)

// TestReconcileLiveness_BootMismatchSkipsPidCheck simulates a reboot
// without one: it injects a fake "current" boot id that differs from what
// was recorded, on a torrent whose pid belongs to the *test process itself*
// (guaranteed alive). If the pid check were consulted at all, this torrent
// would incorrectly come back "alive". It must not be: a boot id mismatch
// has to short-circuit before the pid is ever looked at.
func TestReconcileLiveness_BootMismatchSkipsPidCheck(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	restore := currentBootID
	currentBootID = func() (string, error) { return "boot-session-after-reboot", nil }
	t.Cleanup(func() { currentBootID = restore })

	tc := &TorrentConfig{
		ID:     "reboot001",
		Status: StatusRunning,
		PID:    os.Getpid(), // deliberately alive; proves the pid path was skipped
		BootID: "boot-session-before-reboot",
	}
	if err := InitTorrentDir(tc.ID); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	if err := SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save torrent config: %v", err)
	}

	if err := ReconcileLiveness(tc); err != nil {
		t.Fatalf("ReconcileLiveness: %v", err)
	}
	if tc.Status != StatusInterrupted {
		t.Fatalf("status = %q, want %q (boot id mismatch should mark interrupted, not fall through to a pid check)", tc.Status, StatusInterrupted)
	}
	if tc.PID != 0 {
		t.Fatalf("pid = %d, want 0", tc.PID)
	}

	reloaded, err := LoadTorrentConfig(tc.ID)
	if err != nil {
		t.Fatalf("reload torrent config: %v", err)
	}
	if reloaded.Status != StatusInterrupted {
		t.Fatalf("persisted status = %q, want %q", reloaded.Status, StatusInterrupted)
	}
}

// TestReconcileLiveness_BootMatchStillChecksPid is the control case: when
// the boot id matches, a dead pid is still detected normally (as
// "crashed", not "interrupted").
func TestReconcileLiveness_BootMatchStillChecksPid(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	restore := currentBootID
	currentBootID = func() (string, error) { return "same-boot-session", nil }
	t.Cleanup(func() { currentBootID = restore })

	const implausiblePID = 2147483647
	tc := &TorrentConfig{
		ID:     "sameboot1",
		Status: StatusRunning,
		PID:    implausiblePID,
		BootID: "same-boot-session",
	}
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
}
