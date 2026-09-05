package engine

import (
	"errors"
	"fmt"
	"os"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// PreviewSession holds a live torrent.Client + torrent.Torrent for the
// interactive add-flow: one client, created once, kept open for the
// entire preview (metadata resolution *and* live peer/seeder stats), and
// closed only once the user confirms or cancels.
//
// This replaces the old pattern of creating a scratch client just to fetch
// a magnet's metadata and discarding it immediately (see the plain,
// non-interactive path in metadata.go) -- wasteful in general, and doubly
// so for a preview screen that also wants live peer/seeder counts, which
// would otherwise mean either awkwardly reusing that throwaway client or
// spinning up a second one just to watch it.
//
// What this session does *not* do is eliminate the real download's own
// peer discovery. That boundary is unavoidable: this session runs in the
// interactive foreground process, which must be able to exit immediately
// once the user confirms (tocli's process-per-torrent design depends on
// that), and Go can't safely continue a running program in the background
// without an exec -- which starts a genuinely new process, and therefore a
// new torrent.Client. There is no supported way to hand a live
// anacrolix/torrent client's open connections across a process boundary
// (raw fork() without exec() is unsafe in a multi-threaded Go program, and
// exec() cannot transplant in-memory/socket state). What this session
// avoids is redundant work *within* the single foreground preview, not the
// fresh start any spawned child inherently needs.
type PreviewSession struct {
	Client  *torrent.Client
	Torrent *torrent.Torrent
	Info    metainfo.Info
}

// OpenPreview creates a client rooted at a scratch directory (nothing
// downloaded during a preview should land in a real save path before the
// user has confirmed anything) and adds source (a local .torrent file or a
// magnet link), waiting for its info to resolve. The returned session
// stays alive for the caller to poll live stats from via Stats, and must
// be closed exactly once via Close.
func OpenPreview(source string) (*PreviewSession, error) {
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = os.TempDir()
	client, err := torrent.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create torrent client: %w", err)
	}

	var t *torrent.Torrent
	if isMagnet(source) {
		t, err = client.AddMagnet(source)
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("add magnet: %w", err)
		}
	} else {
		mi, loadErr := metainfo.LoadFromFile(source)
		if loadErr != nil {
			client.Close()
			return nil, fmt.Errorf("load torrent file %s: %w", source, loadErr)
		}
		t, err = client.AddTorrent(mi)
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("add torrent: %w", err)
		}
	}

	<-t.GotInfo()

	return &PreviewSession{Client: client, Torrent: t, Info: *t.Info()}, nil
}

// Metainfo returns the resolved metainfo, ready to be cached to disk by
// StartTorrent. Applies the same PieceLayers fix as the plain magnet path
// (see metadata.go): Torrent.Metainfo() always allocates a PieceLayers
// map, even for plain v1 torrents, which downstream code misreads as "this
// is a v2 torrent missing its piece roots".
func (p *PreviewSession) Metainfo() *metainfo.MetaInfo {
	mi := p.Torrent.Metainfo()
	if len(mi.PieceLayers) == 0 {
		mi.PieceLayers = nil
	}
	return &mi
}

// Stats returns a live snapshot of peer/seeder counts.
func (p *PreviewSession) Stats() torrent.TorrentStats {
	return p.Torrent.Stats()
}

// Files returns the torrent's files for display.
func (p *PreviewSession) Files() []FileSummary {
	return ListFiles(p.Info)
}

// Close releases the session. Whether the caller confirmed or cancelled,
// nothing meaningful was ever written outside the scratch directory: this
// session never calls DownloadAll, so no piece data is requested and
// nothing lands in a real save path before StartTorrent (using a freshly
// spawned process) takes over.
func (p *PreviewSession) Close() error {
	return errors.Join(p.Client.Close()...)
}
