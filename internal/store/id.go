package store

import (
	"path/filepath"
	"strings"

	"github.com/anacrolix/torrent/metainfo"
)

// idLength is how many hex characters of the info hash to use as a
// torrent's id. 10 hex chars (40 bits) is more than enough to avoid
// collisions across the handful of torrents a single user tracks, while
// staying short enough to type on the command line.
const idLength = 10

// DeriveID returns a short, stable identifier for a torrent, derived from
// its info hash. Deriving from the hash (rather than, say, a counter or
// timestamp) means re-adding the same torrent always resolves to the same
// id, so `start` can detect it's already tracked instead of duplicating it.
func DeriveID(mi *metainfo.MetaInfo) string {
	hex := mi.HashInfoBytes().HexString()
	return hex[:idLength]
}

// Sanitize replaces path separators in a torrent's display name so it can
// be used as a filesystem path component.
func Sanitize(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_")
	return replacer.Replace(name)
}

// DefaultSavePath builds the directory a torrent's files are downloaded
// into. The id suffix guarantees uniqueness even when two different
// torrents share a display name, which sanitize(name) alone can't.
func DefaultSavePath(baseDir, name, id string) string {
	return filepath.Join(baseDir, Sanitize(name)+"-"+id)
}
