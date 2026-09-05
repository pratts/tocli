// Package engine wraps github.com/anacrolix/torrent: resolving a torrent's
// metadata (from a file or magnet link) and running the actual download.
package engine

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

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
func ResolveMetainfo(source string, out io.Writer) (*metainfo.MetaInfo, error) {
	if isMagnet(source) {
		return fetchMagnetMetainfo(source, out)
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
func fetchMagnetMetainfo(uri string, out io.Writer) (*metainfo.MetaInfo, error) {
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
	<-t.GotInfo()

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
