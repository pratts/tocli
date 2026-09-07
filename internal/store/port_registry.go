package store

import (
	"errors"
	"os"
	"path/filepath"
)

// PortRegistry is the on-disk shape of ~/.tocli/ports.json: which ports in
// the configured [PortRangeStart, PortRangeEnd] range are currently
// claimed, and which torrent id holds each one. It's the shared state that
// lets tocli's independent per-torrent processes coordinate a
// non-colliding BitTorrent listen port without a daemon holding anything
// in memory -- see internal/portpool, which owns the actual claim/release
// decisions; this file only handles the registry's storage, the same way
// every other on-disk state in this package (config.json, state.json) is
// handled here rather than by its caller.
type PortRegistry struct {
	// Ports maps a claimed port number to the id of the torrent holding it.
	Ports map[int]string `json:"ports"`
}

// portRegistryFileName and portRegistryLockFileName live at the root of
// ~/.tocli, not under any single torrent's directory (contrast
// store.LockPath), since the registry is shared across every torrent, not
// scoped to one.
const (
	portRegistryFileName     = "ports.json"
	portRegistryLockFileName = "ports.lock"
)

// PortRegistryPath returns ~/.tocli/ports.json.
func PortRegistryPath() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, portRegistryFileName), nil
}

// PortRegistryLockPath returns ~/.tocli/ports.lock: the advisory lock
// guarding read-check-claim-write access to ports.json. Use
// process.AcquireLockBlocking on it, not process.AcquireLock -- contention
// here means "another torrent's process is in the middle of its own brief
// claim/release", worth waiting a moment for, not "give up".
func PortRegistryLockPath() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, portRegistryLockFileName), nil
}

// LoadPortRegistry reads ~/.tocli/ports.json, returning an empty registry
// (not an error) if it doesn't exist yet -- the normal state before any
// torrent has ever claimed a port.
func LoadPortRegistry() (*PortRegistry, error) {
	path, err := PortRegistryPath()
	if err != nil {
		return nil, err
	}
	var reg PortRegistry
	if err := readJSON(path, &reg); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &PortRegistry{Ports: map[int]string{}}, nil
		}
		return nil, err
	}
	if reg.Ports == nil {
		reg.Ports = map[int]string{}
	}
	return &reg, nil
}

// SavePortRegistry writes reg to ~/.tocli/ports.json.
func SavePortRegistry(reg *PortRegistry) error {
	path, err := PortRegistryPath()
	if err != nil {
		return err
	}
	return writeJSONAtomic(path, reg)
}
