package store

import "time"

// State is the live progress snapshot for a torrent: state.json under
// ~/.tocli/torrents/<id>/. It's overwritten every 1-2s by the download
// process (internal/engine.Run) and only ever read by the parent CLI
// (`list`), never written by it.
type State struct {
	Percent         float64   `json:"percent"`
	DownloadedBytes int64     `json:"downloaded_bytes"`
	TotalBytes      int64     `json:"total_bytes"`
	ActivePeers     int       `json:"active_peers"`
	TotalPeers      int       `json:"total_peers"`
	DownloadRateBps float64   `json:"download_rate_bps"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func LoadState(id string) (*State, error) {
	path, err := StatePath(id)
	if err != nil {
		return nil, err
	}
	var st State
	if err := readJSON(path, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func SaveState(id string, st *State) error {
	path, err := StatePath(id)
	if err != nil {
		return err
	}
	return writeJSONAtomic(path, st)
}
