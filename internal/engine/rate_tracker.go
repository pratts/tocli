package engine

import (
	"time"

	"github.com/anacrolix/torrent"

	"github.com/pratts/tocli/internal/store"
)

// rateTracker computes an instantaneous download rate by diffing
// Torrent.BytesCompleted() between snapshots, since the library doesn't
// expose a ready-made bytes-per-second figure.
type rateTracker struct {
	t         *torrent.Torrent
	lastBytes int64
	lastTime  time.Time
}

func newRateTracker(t *torrent.Torrent) *rateTracker {
	return &rateTracker{t: t, lastTime: time.Now()}
}

func (r *rateTracker) snapshot() *store.State {
	now := time.Now()
	completed := r.t.BytesCompleted()

	var bps float64
	if elapsed := now.Sub(r.lastTime).Seconds(); elapsed > 0 {
		bps = float64(completed-r.lastBytes) / elapsed
	}
	r.lastBytes = completed
	r.lastTime = now

	total := r.t.Length()
	var percent float64
	if total > 0 {
		percent = float64(completed) / float64(total) * 100
	}

	stats := r.t.Stats()
	return &store.State{
		Percent:         percent,
		DownloadedBytes: completed,
		TotalBytes:      total,
		ActivePeers:     stats.ActivePeers,
		TotalPeers:      stats.TotalPeers,
		DownloadRateBps: bps,
		UpdatedAt:       now,
	}
}
