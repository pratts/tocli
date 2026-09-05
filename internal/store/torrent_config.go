package store

import "time"

// Status is the lifecycle state of a tracked torrent, as recorded in
// config.json.
type Status string

const (
	StatusRunning   Status = "running"
	StatusPaused    Status = "paused"
	StatusStopped   Status = "stopped"
	StatusCompleted Status = "completed"
	StatusError     Status = "error"
	// StatusCrashed marks a torrent whose recorded pid died without going
	// through the normal pause/complete paths (kill -9, OOM, panic) while
	// the machine stayed up. See StatusInterrupted for the reboot case.
	StatusCrashed Status = "crashed"
	// StatusInterrupted marks a torrent whose process was running before
	// the machine rebooted. It's kept distinct from StatusCrashed because
	// the pid couldn't even be meaningfully checked (see boot_id in
	// ReconcileLiveness) -- diagnostically useful, though both are treated
	// identically as "resumable" everywhere else.
	StatusInterrupted Status = "interrupted"
)

// IsResumable reports whether a torrent in this status can be resumed
// directly from its cached metainfo without re-resolving the source.
// StatusError is included: today the only way a torrent lands there is a
// failed lock acquisition (see internal/engine.Run), which is inherently
// transient -- once whatever holds the lock finishes, a later resume can
// succeed.
func (s Status) IsResumable() bool {
	switch s {
	case StatusPaused, StatusStopped, StatusCrashed, StatusInterrupted, StatusError:
		return true
	default:
		return false
	}
}

// TorrentConfig is the static and control information for one tracked
// torrent: config.json under ~/.tocli/torrents/<id>/. It's written by the
// parent CLI (start/resume/remove) and by the child download process
// (internal/engine.Run, on status transitions).
type TorrentConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	InfoHash string `json:"info_hash"`
	Source   string `json:"source"` // original .torrent path or magnet URI, kept for reference only
	SavePath string `json:"save_path"`
	Status   Status `json:"status"`
	// Message is a human-readable explanation for a non-obvious status.
	// Currently only set for StatusError (a failed lock acquisition); empty
	// otherwise.
	Message string `json:"message,omitempty"`
	PID     int    `json:"pid"`
	// BootID is the machine's boot session id (see process.BootID) at the
	// moment this process was spawned. A mismatch against the current boot
	// id means the machine has rebooted since, so PID can no longer be
	// trusted -- it may have been reassigned to an unrelated process.
	BootID  string    `json:"boot_id"`
	AddedAt time.Time `json:"added_at"`
}

func LoadTorrentConfig(id string) (*TorrentConfig, error) {
	path, err := ConfigPath(id)
	if err != nil {
		return nil, err
	}
	var cfg TorrentConfig
	if err := readJSON(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveTorrentConfig(cfg *TorrentConfig) error {
	path, err := ConfigPath(cfg.ID)
	if err != nil {
		return err
	}
	return writeJSONAtomic(path, cfg)
}
