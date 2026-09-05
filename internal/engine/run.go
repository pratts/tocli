package engine

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"golang.org/x/time/rate"

	"github.com/pratts/tocli/internal/config"
	"github.com/pratts/tocli/internal/process"
	"github.com/pratts/tocli/internal/store"
)

// stateWriteInterval is how often the download process refreshes
// state.json. 2s balances `list` feeling live against unnecessary disk I/O.
const stateWriteInterval = 2 * time.Second

// shutdownCloseTimeout bounds how long we wait for torrent.Client.Close()
// when reacting to SIGTERM. Close() isn't documented to return instantly --
// it tears down every peer connection and waits for that cleanup -- so
// against an unresponsive swarm it could in principle run long. systemd's
// default SIGTERM-to-SIGKILL grace period is 90s, but installs commonly
// tighten that to single digits, so we bound well under "a few seconds"
// rather than assuming the generous default.
const shutdownCloseTimeout = 3 * time.Second

// Run is the body of the `tocli __run <id>` background process: it loads
// the torrent's cached config and metainfo, downloads, periodically
// persists progress to state.json, and handles SIGTERM as a request to
// pause (shut the client down cleanly and record status "paused") rather
// than a hard kill. It returns once the torrent either completes or is
// paused.
func Run(id string) error {
	// Acquire the per-torrent lock before doing anything else -- before
	// even loading config.json. If another process (an old child that's
	// still alive, or a second resume racing this one) already holds it,
	// there is a real risk of two torrent.Client instances writing into
	// the same save path concurrently, which the pid/boot_id-based
	// liveness check alone can't rule out (see ReconcileLiveness). An OS
	// advisory lock closes that gap: it's released by the kernel the
	// instant the holder's file descriptors close, for any reason at all,
	// so "the lock is free" is a direct guarantee rather than an
	// inference.
	lockPath, err := store.LockPath(id)
	if err != nil {
		return fmt.Errorf("resolve lock path: %w", err)
	}
	release, err := process.AcquireLock(lockPath)
	if err != nil {
		if recordErr := recordLockFailure(id, err); recordErr != nil {
			log.Printf("record lock failure: %v", recordErr)
		}
		return fmt.Errorf("acquire lock for torrent %s: %w", id, err)
	}
	// Safety net for any early-return error path below (e.g. a failed
	// client.AddTorrent) that doesn't reach the explicit release() calls
	// further down. release is idempotent, so this is harmless on the
	// normal paths where we've already released explicitly.
	defer release()

	tc, err := store.LoadTorrentConfig(id)
	if err != nil {
		return fmt.Errorf("load torrent config: %w", err)
	}

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load global config: %w", err)
	}

	metainfoPath, err := store.MetainfoPath(id)
	if err != nil {
		return err
	}
	mi, err := metainfo.LoadFromFile(metainfoPath)
	if err != nil {
		return fmt.Errorf("load cached metainfo: %w", err)
	}

	clientCfg := torrent.NewDefaultClientConfig()
	clientCfg.DataDir = tc.SavePath
	if globalCfg.PortRangeStart > 0 {
		// TODO: once bandwidth scheduling lands, rotate across
		// [PortRangeStart, PortRangeEnd] instead of pinning the first port.
		clientCfg.ListenPort = globalCfg.PortRangeStart
	}
	if globalCfg.MaxDownloadBps > 0 {
		clientCfg.DownloadRateLimiter = rate.NewLimiter(rate.Limit(globalCfg.MaxDownloadBps), int(globalCfg.MaxDownloadBps))
	}
	if globalCfg.MaxUploadBps > 0 {
		clientCfg.UploadRateLimiter = rate.NewLimiter(rate.Limit(globalCfg.MaxUploadBps), int(globalCfg.MaxUploadBps))
	}

	client, err := torrent.NewClient(clientCfg)
	if err != nil {
		return fmt.Errorf("create torrent client: %w", err)
	}
	defer client.Close()

	t, err := client.AddTorrent(mi)
	if err != nil {
		return fmt.Errorf("add torrent: %w", err)
	}
	<-t.GotInfo()
	t.DownloadAll()

	tc.Status = store.StatusRunning
	tc.PID = os.Getpid()
	if err := store.SaveTorrentConfig(tc); err != nil {
		return fmt.Errorf("record running status: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	downloadDone := make(chan struct{})
	go func() {
		client.WaitAll()
		close(downloadDone)
	}()

	tracker := newRateTracker(t)
	writeState := func() {
		if err := store.SaveState(id, tracker.snapshot()); err != nil {
			// Logged to log.txt (stdout/stderr are redirected there by the
			// spawning process) rather than returned: a missed progress
			// write shouldn't abort an otherwise-healthy download.
			log.Printf("write state: %v", err)
		}
	}
	writeState()

	ticker := time.NewTicker(stateWriteInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			writeState()

		case <-downloadDone:
			writeState()
			tc.Status = store.StatusCompleted
			tc.PID = 0
			if err := store.SaveTorrentConfig(tc); err != nil {
				return fmt.Errorf("record completed status: %w", err)
			}
			// Release right here, at the same point config.json is marked
			// completed, rather than leaving it to the deferred release
			// when this function returns a moment later -- callers reading
			// config.json/the lock state should never see the two disagree.
			if err := release(); err != nil {
				log.Printf("release lock: %v", err)
			}
			return nil

		case <-sigCh:
			// SIGTERM is intentionally treated as both "the user ran
			// `tocli pause`" and "the system is shutting down" (e.g.
			// systemd/launchd send SIGTERM to processes before a reboot,
			// with a grace period before SIGKILL). The two are
			// indistinguishable to this process, and that's fine: the
			// correct response is identical either way -- stop cleanly,
			// record "paused" so `resume` picks it back up later, and exit
			// promptly rather than risk a hard SIGKILL mid-cleanup that
			// would leave config.json/state.json stuck on "running".
			//
			// anacrolix/torrent has no pause primitive, so "pause" means
			// closing the client (which stops all network activity and
			// releases the data directory cleanly) and exiting. Resume
			// just respawns this same process against the same cached
			// metainfo and save path; the library re-verifies pieces
			// already on disk against their hashes on the next start,
			// which is expected and cheap relative to re-downloading them.
			// Piece data itself is never at risk here regardless of how
			// this process ends: anacrolix/torrent writes each piece to
			// the data directory as soon as it's downloaded and hash
			// -verified, independent of a clean client shutdown.
			closeClientWithTimeout(client, shutdownCloseTimeout)
			writeState()
			tc.Status = store.StatusPaused
			tc.PID = 0
			if err := store.SaveTorrentConfig(tc); err != nil {
				return fmt.Errorf("record paused status: %w", err)
			}
			// Same reasoning as the downloadDone case: release right here,
			// at the point config.json is marked paused, not slightly
			// after via the deferred release.
			if err := release(); err != nil {
				log.Printf("release lock: %v", err)
			}
			return nil
		}
	}
}

// recordLockFailure is called when this process couldn't acquire the
// per-torrent lock -- meaning another process is (or, anomalously, still
// appears to be) running it. It must never clobber the bookkeeping of a
// legitimately running instance: if the on-disk config already says
// "running", the lock is correctly held by that healthy instance and this
// failing attempt leaves config.json untouched (its failure is still
// visible via this process's non-zero exit, logged to log.txt). Only when
// our own bookkeeping didn't expect anyone to be running -- yet the lock
// says otherwise -- do we write a distinct error status, since that's a
// genuinely confusing situation the user needs visibility into (otherwise
// `resume` appears to silently do nothing).
func recordLockFailure(id string, lockErr error) error {
	tc, err := store.LoadTorrentConfig(id)
	if err != nil {
		// Can't safely say anything about a config we can't even load; the
		// failure is still visible in log.txt via the caller's returned
		// error.
		return fmt.Errorf("load torrent config to record lock failure: %w", err)
	}
	if tc.Status == store.StatusRunning {
		return nil
	}

	tc.Status = store.StatusError
	tc.Message = fmt.Sprintf("failed to start: %v", lockErr)
	// Deliberately not touching tc.PID: whatever holds the lock isn't
	// necessarily even known to us here, so there's nothing accurate to
	// record, and leaving it as-is avoids fabricating a pid.
	return store.SaveTorrentConfig(tc)
}

// closeClientWithTimeout closes client but doesn't wait indefinitely for
// it: if Close() hasn't returned within timeout, we give up waiting and let
// the caller move on to persisting status itself, so a slow or hung
// Close() can't turn into a hard SIGKILL that leaves config.json/state.json
// stuck on "running". The close still completes in the background even if
// we stop waiting on it; that's harmless since this process is about to
// exit either way.
func closeClientWithTimeout(client *torrent.Client, timeout time.Duration) {
	runWithTimeout(timeout, func() { client.Close() })
}

// runWithTimeout runs fn in a goroutine and waits up to timeout for it to
// finish. If it doesn't finish in time, runWithTimeout gives up waiting
// (logging that it did) rather than blocking the caller indefinitely; fn
// keeps running in its goroutine regardless. Split out from
// closeClientWithTimeout so the timeout behavior itself is testable without
// a real torrent.Client.
func runWithTimeout(timeout time.Duration, fn func()) {
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		log.Printf("operation did not finish within %s; continuing without waiting for it", timeout)
	}
}
