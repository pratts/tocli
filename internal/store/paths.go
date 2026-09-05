// Package store manages tocli's on-disk state under ~/.tocli: the cached
// metainfo, per-torrent config, and live progress state that the CLI
// (parent) and the background download process (child) use to communicate
// without either one holding the other's data in memory.
package store

import (
	"fmt"
	"os"
	"path/filepath"
)

const rootDirName = ".tocli"

// Root returns ~/.tocli, creating it and its torrents subdirectory if they
// don't exist yet.
func Root() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	root := filepath.Join(home, rootDirName)
	if err := os.MkdirAll(TorrentsDirFrom(root), 0o755); err != nil {
		return "", fmt.Errorf("create tocli root %s: %w", root, err)
	}
	return root, nil
}

// TorrentsDirFrom joins the torrents subdirectory onto an already-resolved
// root, without touching the filesystem. Exists mainly so Root can create
// both directories in a single MkdirAll call.
func TorrentsDirFrom(root string) string {
	return filepath.Join(root, "torrents")
}

// TorrentsDir returns ~/.tocli/torrents.
func TorrentsDir() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return TorrentsDirFrom(root), nil
}

// TorrentDir returns ~/.tocli/torrents/<id>. This is the single choke point
// every other per-torrent path helper (ConfigPath, StatePath, MetainfoPath,
// LockPath, LogPath) goes through, so validating id here -- rejecting
// anything that isn't a well-formed id, e.g. containing ".." -- protects
// all of them at once against a malformed or malicious id being used to
// escape ~/.tocli/torrents.
func TorrentDir(id string) (string, error) {
	if err := ValidateID(id); err != nil {
		return "", err
	}
	dir, err := TorrentsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id), nil
}

// InitTorrentDir creates ~/.tocli/torrents/<id>.
func InitTorrentDir(id string) error {
	dir, err := TorrentDir(id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create torrent directory %s: %w", dir, err)
	}
	return nil
}

// Exists reports whether a torrent directory has already been created for
// id, which is how `start` detects that a torrent (by info hash) is already
// tracked instead of adding a duplicate.
func Exists(id string) (bool, error) {
	dir, err := TorrentDir(id)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(dir)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat torrent directory %s: %w", dir, err)
}

func MetainfoPath(id string) (string, error) {
	dir, err := TorrentDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "metainfo.torrent"), nil
}

func ConfigPath(id string) (string, error) {
	dir, err := TorrentDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// LockPath returns the per-torrent advisory lock file used to prevent two
// processes (e.g. an old still-running child and a racing `resume`) from
// ever downloading into the same save path concurrently. See
// process.AcquireLock.
func LockPath(id string) (string, error) {
	dir, err := TorrentDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lock"), nil
}

func StatePath(id string) (string, error) {
	dir, err := TorrentDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

func LogPath(id string) (string, error) {
	dir, err := TorrentDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "log.txt"), nil
}

// ListIDs returns the ids of all tracked torrents (the names of the
// directories under ~/.tocli/torrents).
func ListIDs() ([]string, error) {
	dir, err := TorrentsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read torrents directory %s: %w", dir, err)
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}
