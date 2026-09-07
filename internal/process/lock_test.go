package process

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireLock_ExclusiveAndReleasable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")

	release1, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}

	if _, err := AcquireLock(path); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("second AcquireLock error = %v, want ErrLockHeld", err)
	}

	if err := release1(); err != nil {
		t.Fatalf("release: %v", err)
	}

	release2, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock after release: %v", err)
	}
	if err := release2(); err != nil {
		t.Fatalf("release2: %v", err)
	}
}

func TestAcquireLock_ReleaseIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")

	release, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("first release: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("second release (should be a no-op, not an error): %v", err)
	}
}

// TestAcquireLockBlocking_WaitsForRelease confirms the actual difference
// from AcquireLock: contention doesn't fail immediately, it waits until
// the current holder releases.
func TestAcquireLockBlocking_WaitsForRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")

	release1, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}

	const holdTime = 200 * time.Millisecond
	go func() {
		time.Sleep(holdTime)
		_ = release1()
	}()

	start := time.Now()
	release2, err := AcquireLockBlocking(path)
	if err != nil {
		t.Fatalf("AcquireLockBlocking: %v", err)
	}
	defer release2()

	if waited := time.Since(start); waited < holdTime {
		t.Fatalf("AcquireLockBlocking returned after %s, want it to have waited at least %s for the first holder to release", waited, holdTime)
	}
}

func TestAcquireLock_CreatesFileIfMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "does-not-exist-yet", "lock")

	// The lock file's parent directory must already exist (mirrors real
	// usage: store.InitTorrentDir always creates the torrent directory
	// before anything tries to lock inside it), but the lock file itself
	// should be created on demand.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	release, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer release()
}
