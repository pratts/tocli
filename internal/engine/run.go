package engine

import (
	"context"
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
	// Acquire the per-torrent lock before touching anything else: it's what
	// stops two processes from ever downloading into the same save path
	// concurrently (see process.AcquireLock for why this, not just the
	// pid/boot_id check, is what makes that guarantee real).
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
	// Idempotent safety net for any early-return path below that doesn't
	// reach one of the explicit release() calls further down.
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
	// Bounded: a magnet whose swarm never turns up the metadata must not
	// hang this process forever (see MetadataTimeout).
	infoCtx, cancel := context.WithTimeout(context.Background(), MetadataTimeout)
	err = waitForInfo(infoCtx, t)
	cancel()
	if err != nil {
		return fmt.Errorf("resolve torrent metadata: %w", err)
	}
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
			// Released here, not via the deferred release() below: callers
			// reading config.json and the lock state should never see the
			// two disagree about whether this torrent is still running.
			if err := release(); err != nil {
				log.Printf("release lock: %v", err)
			}
			return nil

		case <-sigCh:
			// SIGTERM means "pause" whether it came from `tocli pause` or
			// the system shutting down -- the two are indistinguishable
			// here, and the correct response is the same either way: stop
			// cleanly and exit promptly, since a slow shutdown risks a hard
			// SIGKILL that leaves config.json/state.json stuck on
			// "running". anacrolix/torrent has no pause primitive, so
			// "pause" is closing the client; resume just respawns against
			// the same cached metainfo, and already-downloaded pieces are
			// safe regardless (they're written to disk as soon as they're
			// verified, independent of a clean shutdown).
			closeClientWithTimeout(client, shutdownCloseTimeout)
			writeState()
			tc.Status = store.StatusPaused
			tc.PID = 0
			if err := store.SaveTorrentConfig(tc); err != nil {
				return fmt.Errorf("record paused status: %w", err)
			}
			if err := release(); err != nil { // see downloadDone case above
				log.Printf("release lock: %v", err)
			}
			return nil
		}
	}
}

// recordLockFailure runs when this process couldn't acquire the per-torrent
// lock. If config.json already says "running", that's a legitimate holder
// and it's left untouched; otherwise this is unexpected, so it's recorded
// as a distinct error status rather than left silent (otherwise `resume`
// would appear to do nothing).
func recordLockFailure(id string, lockErr error) error {
	tc, err := store.LoadTorrentConfig(id)
	if err != nil {
		return fmt.Errorf("load torrent config to record lock failure: %w", err)
	}
	if tc.Status == store.StatusRunning {
		return nil
	}

	tc.Status = store.StatusError
	tc.Message = fmt.Sprintf("failed to start: %v", lockErr)
	// tc.PID is deliberately left as-is: whatever holds the lock isn't
	// necessarily known here, so there's nothing accurate to record.
	return store.SaveTorrentConfig(tc)
}

// closeClientWithTimeout closes client without waiting indefinitely: a slow
// or hung Close() shouldn't turn into a hard SIGKILL that leaves
// config.json/state.json stuck on "running". The close still completes in
// the background if we stop waiting on it; harmless since this process is
// about to exit either way.
func closeClientWithTimeout(client *torrent.Client, timeout time.Duration) {
	runWithTimeout(timeout, func() { client.Close() })
}

// runWithTimeout runs fn in a goroutine and waits up to timeout for it,
// giving up (but not cancelling fn) if it takes longer. Split out from
// closeClientWithTimeout so this behavior is testable without a real
// torrent.Client.
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
