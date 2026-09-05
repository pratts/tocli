package process

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
)

// ErrLockHeld indicates AcquireLock failed specifically because another
// process already holds the lock, as opposed to some other I/O error (e.g.
// permission denied, disk full).
var ErrLockHeld = errors.New("lock already held by another process")

// AcquireLock takes an exclusive, non-blocking advisory lock (flock) on the
// file at path, creating it if it doesn't exist.
//
// This is a small, deliberately isolated seam (rather than calling
// syscall.Flock directly from internal/engine) so a Windows implementation
// (LockFileEx, or similar) can be dropped in here later without touching
// any caller.
//
// Unlike a pid recorded in a file, a kernel-held flock is released
// automatically the instant the holding process's file descriptors close --
// for any reason, including a clean exit, a crash, or being killed -9 --
// with no "was that pid reused since?" ambiguity to reason about.
//
// On success, release unlocks and closes the underlying file descriptor;
// the caller must hold onto it for as long as the lock should be held, and
// is free to call it more than once (idempotent) or not at all (the lock is
// still released when the process exits and the kernel closes its fds). On
// failure, err is non-nil (wrapping ErrLockHeld when contended) and there
// is nothing to release.
func AcquireLock(path string) (release func() error, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", path, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%s: %w", path, ErrLockHeld)
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}

	var once sync.Once
	release = func() error {
		var releaseErr error
		once.Do(func() {
			// Explicit LOCK_UN followed by Close, rather than relying on
			// Close alone to release it implicitly: this makes the release
			// point observable/orderable relative to other work a caller
			// does around it (e.g. writing config.json status), instead of
			// being an incidental side effect of garbage collection or
			// process exit.
			if unlockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); unlockErr != nil {
				releaseErr = fmt.Errorf("unlock %s: %w", path, unlockErr)
			}
			if closeErr := f.Close(); closeErr != nil && releaseErr == nil {
				releaseErr = fmt.Errorf("close lock file %s: %w", path, closeErr)
			}
		})
		return releaseErr
	}
	return release, nil
}
