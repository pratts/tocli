// Package portpool assigns each running torrent a distinct BitTorrent
// listen port from the range configured in config.toml
// (PortRangeStart..PortRangeEnd), so that concurrently-running torrents --
// each one a fully independent OS process, per tocli's process-per-torrent
// design -- never collide trying to bind the same port.
//
// There's no daemon holding shared in-memory state to arbitrate this, so
// coordination happens the same way everything else in tocli does:
// through a lock-protected file on disk (store.PortRegistry), guarded by
// the same flock-based primitive (process.AcquireLockBlocking) that
// internal/process already uses elsewhere, just in its blocking form
// rather than the per-torrent spawn lock's non-blocking one -- contention
// here is brief, expected coordination between processes doing a quick
// read-check-claim-write, not "someone else already owns this, give up".
package portpool

import (
	"fmt"

	"github.com/pratts/tocli/internal/config"
	"github.com/pratts/tocli/internal/process"
	"github.com/pratts/tocli/internal/store"
)

// Claim assigns id the lowest available port in [cfg.PortRangeStart,
// cfg.PortRangeEnd], recording the claim in the shared port registry so no
// other concurrently-starting torrent can claim the same one. Assignment
// is always lowest-available, not an arbitrary free port, so a range a
// user has forwarded on their router gets used predictably from the
// bottom up rather than scattered across it.
//
// If cfg.PortRangeStart is 0, no range is configured at all (see
// config.Config's doc comment: zero means "let the OS pick") -- Claim
// returns (0, false, nil) immediately without touching the registry,
// matching that existing behavior exactly.
//
// If the range is configured but every port in it is already claimed by
// another running torrent, Claim returns (0, true, nil): ephemeral is
// true, telling the caller to fall back to an OS-assigned port (and warn
// about why) rather than fail to start the torrent.
func Claim(id string, cfg config.Config) (port int, ephemeral bool, err error) {
	if cfg.PortRangeStart <= 0 {
		return 0, false, nil
	}

	release, reg, err := lockAndLoadRegistry()
	if err != nil {
		return 0, false, err
	}
	defer release()

	for p := cfg.PortRangeStart; p <= cfg.PortRangeEnd; p++ {
		if _, taken := reg.Ports[p]; taken {
			continue
		}
		reg.Ports[p] = id
		if err := store.SavePortRegistry(reg); err != nil {
			return 0, false, fmt.Errorf("save port registry: %w", err)
		}
		return p, false, nil
	}
	// Every port in the configured range is already claimed by another
	// running torrent -- the caller falls back to an OS-assigned port.
	return 0, true, nil
}

// Release frees id's claimed port, if any, so a subsequently-started
// torrent can claim it. Safe to call even if id never held a claim (it ran
// on an ephemeral fallback port, or no range is configured at all) -- a
// no-op in that case, not an error.
func Release(id string) error {
	release, reg, err := lockAndLoadRegistry()
	if err != nil {
		return err
	}
	defer release()

	changed := false
	for p, heldBy := range reg.Ports {
		if heldBy == id {
			delete(reg.Ports, p)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := store.SavePortRegistry(reg); err != nil {
		return fmt.Errorf("save port registry: %w", err)
	}
	return nil
}

// lockAndLoadRegistry acquires the registry's lock (waiting for it, not
// failing on contention -- see process.AcquireLockBlocking) and loads its
// current contents. Callers must call the returned release once done,
// before the registry can reflect their change to any other process.
func lockAndLoadRegistry() (release func() error, reg *store.PortRegistry, err error) {
	lockPath, err := store.PortRegistryLockPath()
	if err != nil {
		return nil, nil, err
	}
	release, err = process.AcquireLockBlocking(lockPath)
	if err != nil {
		return nil, nil, fmt.Errorf("lock port registry: %w", err)
	}

	reg, err = store.LoadPortRegistry()
	if err != nil {
		release()
		return nil, nil, err
	}
	return release, reg, nil
}
