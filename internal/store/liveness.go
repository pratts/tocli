package store

import "github.com/pratts/tocli/internal/process"

// currentBootID is a package variable (rather than calling process.BootID
// directly) purely so tests can substitute a fake boot session id to
// simulate a reboot without needing an actual one.
var currentBootID = process.BootID

// ReconcileLiveness checks whether tc's recorded background process is
// actually still running and, if it isn't, persists a corrected status.
// list, pause, and remove all go through this single function before
// trusting a stored "running" status, so a stale status left behind by an
// unclean death (kill -9, crash, OOM, machine reboot) gets corrected once,
// consistently, at the point of detection -- rather than each command
// re-deriving its own answer and only one of them happening to fix the
// file on disk.
//
// tc is mutated in place (Status/PID) when a correction is made, so
// callers can immediately act on tc.Status after calling this.
//
// Relationship to the per-torrent lock (process.AcquireLock, used by
// internal/engine.Run to prevent double-spawning into the same save path):
// this is a pid/boot_id *inference*, whereas the lock is a direct kernel
// guarantee -- it's unconditionally released the instant a process's file
// descriptors close, for any reason, including the unclean deaths this
// function is trying to detect indirectly. In other words, by the time
// ReconcileLiveness could ever observe a dead pid or a boot_id mismatch,
// that same event has already released the lock too; the two signals are
// never actually in a position to disagree (confirmed directly by
// TestLockAndReconcileLiveness_NeverContradict in internal/engine, rather
// than assumed). Were that premise ever violated -- e.g. an unreliable
// flock implementation on some network filesystem -- the lock should win,
// since it's the direct fact rather than the inference. This function does
// not consult the lock itself: replacing pid/boot_id liveness reporting
// with lock-based liveness checking is a reasonable future simplification,
// but out of scope here -- the lock's job today is narrowly to prevent
// double-spawning, not to replace this status-reporting logic.
func ReconcileLiveness(tc *TorrentConfig) error {
	if tc.Status != StatusRunning {
		return nil
	}

	alive, rebooted := checkAlive(tc)
	if alive {
		return nil
	}

	if rebooted {
		tc.Status = StatusInterrupted
	} else {
		tc.Status = StatusCrashed
	}
	tc.PID = 0
	return SaveTorrentConfig(tc)
}

// checkAlive determines whether tc's recorded process is still the one we
// spawned. PIDs get reused by the OS after a reboot, so a signal-0 hit on
// the "right" number doesn't by itself mean it's the same process: if the
// machine has rebooted since tc.BootID was recorded (rebooted=true), the
// pid check is skipped entirely -- performing it at that point would be
// actively misleading, since an unrelated process may have since reused
// tc.PID.
func checkAlive(tc *TorrentConfig) (alive, rebooted bool) {
	current, err := currentBootID()
	if err != nil {
		// Can't determine the current boot session (e.g. an unsupported
		// platform) -- fall back to the pid check alone rather than
		// assuming a reboot occurred.
		return process.IsAlive(tc.PID), false
	}
	if tc.BootID != "" && tc.BootID != current {
		return false, true
	}
	return process.IsAlive(tc.PID), false
}
