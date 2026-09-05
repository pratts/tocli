package engine

import (
	"os"
	"testing"
	"time"

	"github.com/pratts/tocli/internal/store"
)

// TestRemoveTorrent_CompletedTorrentDoesNotSignal confirms that removing a
// torrent that isn't running (here: "completed") skips straight to
// directory cleanup instead of attempting to signal a pid that's known not
// to be running -- and returns no error doing so.
func TestRemoveTorrent_CompletedTorrentDoesNotSignal(t *testing.T) {
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

	if _, err := RemoveTorrent(tc.ID, false); err != nil {
		t.Fatalf("RemoveTorrent returned an error: %v", err)
	}
	if called {
		t.Fatal("terminateFunc was called for a torrent that was already completed")
	}
	if _, err := store.LoadTorrentConfig(tc.ID); err == nil {
		t.Fatal("expected the torrent's directory to have been removed")
	}
}

// TestRemoveTorrent_StaleRunningStatusDoesNotSignal covers the case
// relevant to earlier liveness hardening: a config.json that still says
// "running" but whose process is actually dead (or predates a reboot)
// should also be reconciled before remove decides whether to signal
// anything.
func TestRemoveTorrent_StaleRunningStatusDoesNotSignal(t *testing.T) {
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

	if _, err := RemoveTorrent(tc.ID, false); err != nil {
		t.Fatalf("RemoveTorrent returned an error: %v", err)
	}
	if called {
		t.Fatal("terminateFunc was called for a torrent whose recorded pid was already dead")
	}
}

// TestRemoveTorrent_MissingSavePathIsNotAnError covers the item-4 fix:
// requesting --with-data removal when the save path has already been
// moved or deleted by the user must not be treated as a failure.
func TestRemoveTorrent_MissingSavePathIsNotAnError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tc := &store.TorrentConfig{
		ID:     "gonepath01",
		Name:   "moved-away",
		Status: store.StatusCompleted,
		// SavePath deliberately points at a directory that doesn't exist.
		SavePath: t.TempDir() + "/does-not-exist",
	}
	if err := store.InitTorrentDir(tc.ID); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	if err := store.SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save torrent config: %v", err)
	}

	outcome, err := RemoveTorrent(tc.ID, true)
	if err != nil {
		t.Fatalf("RemoveTorrent returned an error for an already-missing save path: %v", err)
	}
	if !outcome.DataAlreadyGone {
		t.Fatal("expected DataAlreadyGone to be true")
	}
	if outcome.DataRemoveErr != nil {
		t.Fatalf("expected no DataRemoveErr, got: %v", outcome.DataRemoveErr)
	}
	if _, err := store.LoadTorrentConfig(tc.ID); err == nil {
		t.Fatal("expected the torrent's tracking entry to have been removed")
	}
}

// TestRemoveTorrent_DataRemovalFailureStillRemovesTrackingEntry confirms
// that a real (non-"already gone") data-removal failure still results in
// the tracking entry being removed -- nothing is left orphaned in
// ~/.tocli, only the downloaded files remain, clearly reported via the
// returned error.
func TestRemoveTorrent_DataRemovalFailureStillRemovesTrackingEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// A file (not a directory) as the save "path" makes os.RemoveAll fail
	// in a way that isn't "does not exist": RemoveAll on a plain file whose
	// parent can't be traversed as a directory returns a real error here
	// because we treat SavePath as something that should contain entries;
	// simplest reliable trigger is a save path nested under a file, since
	// mkdir/removal beneath a non-directory reliably fails with ENOTDIR.
	base := t.TempDir()
	blocker := base + "/blocker"
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("create blocker file: %v", err)
	}

	tc := &store.TorrentConfig{
		ID:       "dataerr001",
		Name:     "cant-delete",
		Status:   store.StatusCompleted,
		SavePath: blocker + "/nested", // blocker is a file, not a dir: ENOTDIR
	}
	if err := store.InitTorrentDir(tc.ID); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	if err := store.SaveTorrentConfig(tc); err != nil {
		t.Fatalf("save torrent config: %v", err)
	}

	outcome, err := RemoveTorrent(tc.ID, true)
	if err == nil {
		t.Fatal("expected an error for a genuine data-removal failure")
	}
	if outcome.DataRemoveErr == nil {
		t.Fatal("expected DataRemoveErr to be set")
	}
	if outcome.DataAlreadyGone {
		t.Fatal("DataAlreadyGone should not be set for a real removal failure")
	}
	if _, err := store.LoadTorrentConfig(tc.ID); err == nil {
		t.Fatal("expected the tracking entry to still be removed despite the data-removal failure")
	}
}
