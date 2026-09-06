package engine

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/anacrolix/torrent/metainfo"

	"github.com/pratts/tocli/internal/config"
	"github.com/pratts/tocli/internal/process"
	"github.com/pratts/tocli/internal/store"
)

// This file holds the orchestration steps behind start/pause/resume/remove:
// the sequence of store/process calls each command performs. It's the
// single place that sequence lives, called identically by the plain CLI
// commands and by every internal/tui model -- neither layer re-implements
// any of it; they only decide *when* to call it (after a Y/N prompt, after
// a TUI keypress) and how to render the result.

// ErrAlreadyTracked indicates StartTorrent found an existing torrent with
// the same id (same info hash). The caller can still use the returned
// *store.TorrentConfig -- it describes the already-tracked torrent -- to
// tell the user how to resume it, if appropriate, instead of duplicating it.
var ErrAlreadyTracked = errors.New("torrent is already tracked")

// terminateTimeout is how long RemoveTorrent waits for a graceful SIGTERM
// shutdown before escalating to SIGKILL.
const terminateTimeout = 5 * time.Second

// terminateFunc is a package variable, rather than calling process.Terminate
// directly, so tests can substitute a spy to confirm RemoveTorrent doesn't
// signal a torrent it already knows isn't running.
var terminateFunc = process.Terminate

// StartTorrent finalizes a resolved torrent into a tracked, running one:
// caches its metainfo, creates the save directory, writes the initial
// config.json, and spawns the detached background process that performs
// the actual download.
//
// If a torrent with the same id (info hash) is already tracked, StartTorrent
// changes nothing and returns that existing config alongside
// ErrAlreadyTracked (check with errors.Is) rather than duplicating it.
func StartTorrent(mi *metainfo.MetaInfo, source string, globalCfg config.Config) (*store.TorrentConfig, error) {
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return nil, fmt.Errorf("parse torrent info: %w", err)
	}

	id := store.DeriveID(mi)

	exists, err := store.Exists(id)
	if err != nil {
		return nil, fmt.Errorf("check existing torrent: %w", err)
	}
	if exists {
		existing, err := store.LoadTorrentConfig(id)
		if err != nil {
			return nil, fmt.Errorf("load existing torrent config: %w", err)
		}
		return existing, fmt.Errorf("torrent %s: %w", id, ErrAlreadyTracked)
	}

	if err := store.InitTorrentDir(id); err != nil {
		return nil, fmt.Errorf("create torrent directory: %w", err)
	}

	metainfoPath, err := store.MetainfoPath(id)
	if err != nil {
		return nil, err
	}
	// Cache the resolved .torrent bytes to disk so pause/resume (and any
	// future retry) never needs to re-fetch magnet metadata from the swarm.
	if err := writeMetainfoFile(metainfoPath, mi); err != nil {
		return nil, fmt.Errorf("cache metainfo: %w", err)
	}

	savePath := store.DefaultSavePath(globalCfg.BaseDownloadDir, info.BestName(), id)
	if err := os.MkdirAll(savePath, 0o755); err != nil {
		return nil, fmt.Errorf("create download directory: %w", err)
	}

	tc := &store.TorrentConfig{
		ID:       id,
		Name:     info.BestName(),
		InfoHash: mi.HashInfoBytes().HexString(),
		Source:   source,
		SavePath: savePath,
		Status:   store.StatusStopped, // flipped to running by SpawnAndTrack once the child is up
		AddedAt:  time.Now(),
	}
	if err := store.SaveTorrentConfig(tc); err != nil {
		return nil, fmt.Errorf("write torrent config: %w", err)
	}

	if err := SpawnAndTrack(tc); err != nil {
		return nil, err
	}
	return tc, nil
}

// SpawnAndTrack launches the background `__run` process for tc and records
// its pid and boot id, mutating tc in place. Shared by StartTorrent and
// ResumeTorrent (and their TUI equivalents).
func SpawnAndTrack(tc *store.TorrentConfig) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve tocli executable path: %w", err)
	}
	logPath, err := store.LogPath(tc.ID)
	if err != nil {
		return err
	}

	pid, err := process.SpawnDetached(exe, []string{"__run", tc.ID}, logPath)
	if err != nil {
		return fmt.Errorf("spawn background process: %w", err)
	}

	bootID, err := process.BootID()
	if err != nil {
		// Not fatal: on a platform without boot-id support, liveness
		// checks just fall back to a plain pid probe (see
		// store.ReconcileLiveness), which is what we'd have done anyway
		// before boot-id tracking existed.
		bootID = ""
	}

	tc.PID = pid
	tc.BootID = bootID
	tc.Status = store.StatusRunning
	if err := store.SaveTorrentConfig(tc); err != nil {
		return fmt.Errorf("record spawned pid: %w", err)
	}
	return nil
}

func writeMetainfoFile(path string, mi *metainfo.MetaInfo) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := mi.Write(f); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// PauseOutcome reports what PauseTorrent actually did, so callers can
// render an accurate message without re-deriving it.
type PauseOutcome struct {
	// Config is tc after liveness reconciliation -- its Status reflects
	// reality (e.g. corrected to "crashed") even when Signaled is false.
	Config *store.TorrentConfig
	// Signaled is true only if a SIGTERM was actually sent.
	Signaled bool
}

// PauseTorrent reconciles id's liveness and, if it's genuinely running,
// sends it SIGTERM. internal/engine.Run's own signal handler is responsible
// for closing the torrent.Client cleanly and marking status "paused" once
// it's done; PauseTorrent itself doesn't wait for that. If the torrent
// isn't running (already paused, completed, or its status was just
// corrected by reconciliation), PauseTorrent does nothing and reports that
// via Signaled=false rather than erroring.
func PauseTorrent(id string) (PauseOutcome, error) {
	tc, err := store.LoadTorrentConfig(id)
	if err != nil {
		return PauseOutcome{}, fmt.Errorf("load torrent %s: %w", id, err)
	}

	// Reconcile before trusting a stored "running" status: the process may
	// have died without going through pause (kill -9, crash, OOM), leaving
	// config.json stale. Signaling a dead pid would otherwise either error
	// outright or, worse, silently hit an unrelated process that has since
	// reused the same pid.
	if err := store.ReconcileLiveness(tc); err != nil {
		return PauseOutcome{}, fmt.Errorf("check torrent %s liveness: %w", id, err)
	}

	if tc.Status != store.StatusRunning {
		return PauseOutcome{Config: tc}, nil
	}

	// SIGTERM only, no escalation: internal/engine.Run's own handler
	// closes the torrent.Client cleanly and marks status "paused". Forceful
	// SIGKILL is reserved for RemoveTorrent, where the directory needs to
	// be gone right away.
	if err := process.Signal(tc.PID, syscall.SIGTERM); err != nil {
		return PauseOutcome{Config: tc}, fmt.Errorf("signal torrent %s: %w", id, err)
	}
	return PauseOutcome{Config: tc, Signaled: true}, nil
}

// ResumeOutcome reports what ResumeTorrent actually did.
type ResumeOutcome struct {
	Config *store.TorrentConfig
	// AlreadyRunning is true if the torrent turned out to already be
	// running (nothing was spawned).
	AlreadyRunning bool
}

// ResumeTorrent reconciles id's liveness, checks it's in a resumable
// status, and respawns it from its cached metainfo -- no network
// resolution needed, unlike StartTorrent.
func ResumeTorrent(id string) (ResumeOutcome, error) {
	tc, err := store.LoadTorrentConfig(id)
	if err != nil {
		return ResumeOutcome{}, fmt.Errorf("load torrent %s: %w", id, err)
	}

	// Reconcile first: a status of "running" might be stale (process
	// crashed, or the machine rebooted since it was recorded). This flips
	// tc.Status to "crashed"/"interrupted" in that case, both of which are
	// resumable, so the switch below doesn't need to special-case them.
	if err := store.ReconcileLiveness(tc); err != nil {
		return ResumeOutcome{}, fmt.Errorf("check torrent %s liveness: %w", id, err)
	}

	switch {
	case tc.Status == store.StatusRunning:
		return ResumeOutcome{Config: tc, AlreadyRunning: true}, nil
	case tc.Status.IsResumable():
		// resumable
	default:
		return ResumeOutcome{}, fmt.Errorf("torrent %s cannot be resumed from status %q", id, tc.Status)
	}

	metainfoPath, err := store.MetainfoPath(id)
	if err != nil {
		return ResumeOutcome{}, err
	}
	if _, err := os.Stat(metainfoPath); err != nil {
		return ResumeOutcome{}, fmt.Errorf("cached metainfo missing for %s: %w", id, err)
	}

	if err := SpawnAndTrack(tc); err != nil {
		return ResumeOutcome{}, err
	}
	return ResumeOutcome{Config: tc}, nil
}

// RemoveOutcome reports what RemoveTorrent actually did, distinguishing the
// "downloaded files were already gone" case from a real deletion failure so
// callers can word their message accordingly.
type RemoveOutcome struct {
	// DataAlreadyGone is true if withData was requested but the save path
	// didn't exist any more (e.g. the user moved or deleted it themselves).
	// This is not an error: the tracking entry is still removed normally.
	DataAlreadyGone bool
	// DataRemoveErr, if non-nil, means withData was requested, the save
	// path did exist, but deleting it failed for a real reason (e.g.
	// permission denied). The tracking entry is still removed regardless
	// -- see RemoveTorrent's doc comment for why nothing is left orphaned
	// in ~/.tocli even in this case.
	DataRemoveErr error
	// ConfigUnreadable is true if config.json itself couldn't be loaded or
	// parsed at all (e.g. a torn write during spawn, or manual/external
	// corruption), so RemoveTorrent fell back to deleting
	// ~/.tocli/torrents/<id> outright instead of the normal logic -- see
	// removeWithUnreadableConfig.
	ConfigUnreadable bool
}

// RemoveTorrent stops id if it's running (reconciling liveness first, the
// same as PauseTorrent, so it never signals a pid it already knows is
// dead or unrelated), then removes ~/.tocli/torrents/<id>. If withData is
// set, it also removes the downloaded files at the torrent's save path.
//
// A missing save path (already moved or deleted by the user) is not
// treated as a failure: RemoveOutcome.DataAlreadyGone is set and the
// tracking entry is removed normally. A real deletion failure (e.g.
// permission denied) is captured in RemoveOutcome.DataRemoveErr and also
// returned as err -- but the tracking entry removal still happens first,
// so nothing is left orphaned in ~/.tocli either way: at worst, the
// downloaded files remain on disk needing manual cleanup, which the
// returned error surfaces clearly.
//
// If config.json can't even be loaded/parsed, RemoveTorrent can't reach any
// of the above -- see removeWithUnreadableConfig for that fallback.
func RemoveTorrent(id string, withData bool) (RemoveOutcome, error) {
	tc, err := store.LoadTorrentConfig(id)
	if err != nil {
		return removeWithUnreadableConfig(id)
	}

	// Reconcile before deciding whether there's anything to signal: a
	// stored "running" status might be stale (the process crashed, or the
	// machine rebooted since), in which case there's nothing live to stop.
	// Skipping straight to cleanup avoids signaling a pid that's already
	// known dead -- or, worse, an unrelated process that has since reused
	// it after a reboot.
	if err := store.ReconcileLiveness(tc); err != nil {
		return RemoveOutcome{}, fmt.Errorf("check torrent %s liveness: %w", id, err)
	}

	if tc.Status == store.StatusRunning {
		if err := terminateFunc(tc.PID, terminateTimeout); err != nil {
			return RemoveOutcome{}, fmt.Errorf("stop torrent %s: %w", id, err)
		}
	}

	var outcome RemoveOutcome
	if withData {
		if _, statErr := os.Stat(tc.SavePath); statErr != nil {
			if os.IsNotExist(statErr) {
				outcome.DataAlreadyGone = true
			} else {
				outcome.DataRemoveErr = fmt.Errorf("check downloaded data at %s: %w", tc.SavePath, statErr)
			}
		} else if err := os.RemoveAll(tc.SavePath); err != nil {
			outcome.DataRemoveErr = fmt.Errorf("remove downloaded data at %s: %w", tc.SavePath, err)
		}
	}

	dir, err := store.TorrentDir(id)
	if err != nil {
		return outcome, err
	}
	if err := os.RemoveAll(dir); err != nil {
		return outcome, fmt.Errorf("remove torrent directory: %w", err)
	}

	if outcome.DataRemoveErr != nil {
		return outcome, outcome.DataRemoveErr
	}
	return outcome, nil
}

// removeWithUnreadableConfig is RemoveTorrent's fallback for when
// config.json itself can't be loaded or parsed (e.g. a torn write during
// spawn, or manual/external corruption). Without a readable config there's
// no reliable way to know where the actual downloaded data lives, so the
// "remove from list only" vs. "remove with data" distinction has nothing
// left to distinguish between -- both collapse into the same action here:
// delete ~/.tocli/torrents/<id> outright. Any downloaded data elsewhere on
// disk is left untouched simply because there's no way to find it any
// more, not as a deliberate policy choice.
//
// It also means there's no pid to read for a liveness check or a graceful
// stop the normal way (store.ReconcileLiveness, process.Terminate). The
// per-torrent advisory lock file is still readable and lockable without a
// readable config, though -- it's the same one engine.Run acquires on
// startup, and per the earlier lock-file hardening work, the more reliable
// liveness signal anyway (a kernel guarantee, not an inference). So it's
// what guards this fallback instead: if something still holds it, nothing
// is deleted.
func removeWithUnreadableConfig(id string) (RemoveOutcome, error) {
	outcome := RemoveOutcome{ConfigUnreadable: true}

	lockPath, err := store.LockPath(id)
	if err != nil {
		return outcome, err
	}
	release, err := process.AcquireLock(lockPath)
	if err != nil {
		if errors.Is(err, process.ErrLockHeld) {
			return outcome, fmt.Errorf("torrent %s still appears to be running; cannot determine its PID from the corrupt config to stop it -- you may need to find and kill the process manually", id)
		}
		return outcome, fmt.Errorf("check torrent %s liveness: %w", id, err)
	}
	if err := release(); err != nil {
		return outcome, fmt.Errorf("release lock for torrent %s: %w", id, err)
	}

	dir, err := store.TorrentDir(id)
	if err != nil {
		return outcome, err
	}
	if err := os.RemoveAll(dir); err != nil {
		return outcome, fmt.Errorf("remove torrent directory: %w", err)
	}
	return outcome, nil
}

// DescribeRemove renders a RemoveTorrent result as a single user-facing
// message, distinguishing its possible outcomes: the corrupt-config
// fallback (ConfigUnreadable set -- see removeWithUnreadableConfig), a
// genuine failure (err set, DataRemoveErr not -- e.g. the torrent couldn't
// be stopped, or the tracking directory itself couldn't be removed), a
// tracking entry removed despite a data-removal failure (DataRemoveErr set
// -- nothing orphaned in ~/.tocli, but the files need manual cleanup), and
// the two success cases. This is the single place that wording lives,
// called identically by the plain CLI, the standalone remove picker, and
// the dashboard's inline remove, so the three never drift out of sync.
func DescribeRemove(id string, outcome RemoveOutcome, err error) string {
	switch {
	case outcome.ConfigUnreadable && err != nil:
		// err is already a complete, id-specific sentence (e.g. "torrent
		// %s still appears to be running; ..."); wrapping it further would
		// just repeat itself.
		return err.Error()
	case outcome.ConfigUnreadable:
		return fmt.Sprintf("config for torrent %s was unreadable; removed its tracking directory entirely", id)
	case outcome.DataRemoveErr != nil:
		return fmt.Sprintf("removed torrent %s tracking entry, but failed to remove downloaded files: %v", id, outcome.DataRemoveErr)
	case err != nil:
		return fmt.Sprintf("failed to remove torrent %s: %v", id, err)
	case outcome.DataAlreadyGone:
		return fmt.Sprintf("removed torrent %s -- downloaded files were already moved or deleted, removed tocli's tracking entry only", id)
	default:
		return fmt.Sprintf("removed torrent %s", id)
	}
}
