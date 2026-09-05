// Package engine wraps github.com/anacrolix/torrent: resolving a torrent's
// metadata (from a file or magnet link) and running the actual download.
package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// MetadataTimeout bounds how long any caller in this package waits for a
// torrent's metadata to resolve (peer/DHT discovery, then the info dict
// exchange itself) before giving up. Without this, a magnet with a dead or
// unreachable swarm hangs whichever process is waiting on it forever --
// this used to be a real, reachable bug in three separate places (the
// plain `start` path here, the interactive preview, and the background
// download process), since none of them had any way to bound or cancel the
// wait. Callers construct their own context (context.WithTimeout(...,
// MetadataTimeout) for a hard deadline, or a cancellable one so a user
// action like pressing Esc can interrupt it sooner) and pass it in --
// this package only defines the shared default duration and the shared
// wait, not the cancellation policy itself.
const MetadataTimeout = 2 * time.Minute

// waitForInfo blocks until t's info is available or ctx is done, whichever
// happens first.
func waitForInfo(ctx context.Context, t *torrent.Torrent) error {
	select {
	case <-t.GotInfo():
		return nil
	case <-ctx.Done():
		return fmt.Errorf("waiting for torrent metadata: %w", ctx.Err())
	}
}

// FileSummary is one file inside a torrent, for display before download
// starts.
type FileSummary struct {
	Path   string
	Length int64
}

// ResolveMetainfo loads a torrent's metainfo, either straight from a local
// .torrent file or, for a magnet link, by connecting to the swarm just long
// enough to fetch the info dict from a peer. out receives human-readable
// status messages (e.g. "fetching metadata..."); pass nil to discard them.
// ctx bounds how long the magnet case will wait (see MetadataTimeout); it's
// unused for a local file, which needs no network round trip at all.
func ResolveMetainfo(ctx context.Context, source string, out io.Writer) (*metainfo.MetaInfo, error) {
	if isMagnet(source) {
		return fetchMagnetMetainfo(ctx, source, out)
	}
	mi, err := metainfo.LoadFromFile(source)
	if err != nil {
		return nil, fmt.Errorf("load torrent file %s: %w", source, err)
	}
	return mi, nil
}

// ListFiles returns the files described by info, in display form.
func ListFiles(info metainfo.Info) []FileSummary {
	files := info.UpvertedFiles()
	out := make([]FileSummary, 0, len(files))
	for _, f := range files {
		out = append(out, FileSummary{Path: f.DisplayPath(&info), Length: f.Length})
	}
	return out
}

func isMagnet(source string) bool {
	return strings.HasPrefix(source, "magnet:?")
}

// fetchMagnetMetainfo connects to the swarm just long enough to learn the
// torrent's name and file list, using a scratch data directory since no
// piece data is downloaded at this stage. The resulting metainfo is cached
// to disk by the caller so this network round trip only ever happens once
// per torrent, even across pause/resume.
func fetchMagnetMetainfo(ctx context.Context, uri string, out io.Writer) (*metainfo.MetaInfo, error) {
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = os.TempDir()
	client, err := torrent.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create scratch torrent client: %w", err)
	}
	defer client.Close()

	t, err := client.AddMagnet(uri)
	if err != nil {
		return nil, fmt.Errorf("add magnet: %w", err)
	}

	if out != nil {
		fmt.Fprintln(out, "fetching metadata...")
	}
	if err := waitForInfo(ctx, t); err != nil {
		return nil, err
	}

	mi := t.Metainfo()
	if len(mi.PieceLayers) == 0 {
		// Torrent.Metainfo() always allocates a PieceLayers map, even for
		// plain v1 torrents. A non-nil-but-empty map is misread downstream,
		// by addPieceLayersLocked, as "this is a v2 torrent missing its
		// piece roots" and errors out any multi-piece v1 file. Dropping the
		// map when it's empty keeps the cached metainfo usable for the
		// (much more common) v1-only torrents fetched over a magnet link.
		mi.PieceLayers = nil
	}
	return &mi, nil
}
