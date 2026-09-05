package process

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
