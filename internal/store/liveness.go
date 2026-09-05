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
// consistently, at the point of detection.
//
// tc is mutated in place (Status/PID) when a correction is made, so
// callers can immediately act on tc.Status after calling this.
//
// This is a pid/boot_id *inference*, distinct from the per-torrent advisory
// lock (process.AcquireLock) that actually prevents double-spawning: the
// two are verified to never disagree in practice (see
// TestLockAndReconcileLiveness_NeverContradict in internal/engine), since a
// dead pid/rebooted machine has already released the lock by the time this
// could observe either. If that premise were ever violated, the lock would
// be the one to trust -- it's a kernel guarantee, this is an inference.
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
