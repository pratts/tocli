package store

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/anacrolix/torrent/metainfo"
)

// idLength is how many hex characters of the info hash to use as a
// torrent's id. 10 hex chars (40 bits) is more than enough to avoid
// collisions across the handful of torrents a single user tracks, while
// staying short enough to type on the command line.
const idLength = 10

// validID matches a well-formed id: letters and digits only, 1-32 of them.
// This is deliberately looser than "exactly idLength lowercase hex chars"
// (which is all DeriveID ever produces) so it doesn't need to change if the
// derivation scheme ever does -- what actually matters here is defensive,
// not descriptive: no "/", "\", ".", or other characters that could turn an
// id into a path traversal once it's joined into a filesystem path.
var validID = regexp.MustCompile(`^[a-zA-Z0-9]{1,32}$`)

// ValidateID reports an error if id isn't safe to use as a filesystem path
// component. Every function that turns an id into a path under
// ~/.tocli/torrents (TorrentDir and everything built on it) calls this
// first: id ultimately comes from a CLI argument (`tocli pause <id>`,
// `tocli __run <id>`), and without this check a value like "../../etc"
// would let path/filepath.Join walk straight out of ~/.tocli/torrents.
func ValidateID(id string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("invalid torrent id %q", id)
	}
	return nil
}

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
