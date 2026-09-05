// Package config loads and saves tocli's global defaults from
// ~/.tocli/config.toml.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/pratts/tocli/internal/store"
)

const fileName = "config.toml"

// Config holds the global defaults applied to every torrent unless
// overridden per-torrent in a later pass. Zero values for the speed limits
// mean "unlimited"; zero for the port range means "let the OS pick".
type Config struct {
	BaseDownloadDir string `toml:"base_download_dir"`
	MaxUploadBps    int64  `toml:"max_upload_bps"`
	MaxDownloadBps  int64  `toml:"max_download_bps"`
	PortRangeStart  int    `toml:"port_range_start"`
	PortRangeEnd    int    `toml:"port_range_end"`
}

// Default returns tocli's out-of-the-box configuration.
func Default() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		// Home directory resolution failures surface for real when the
		// config is actually loaded/saved; here we just need some
		// reasonable fallback so Default() itself never errors.
		home = "."
	}
	return Config{
		BaseDownloadDir: filepath.Join(home, "Downloads", "torrents"),
		MaxUploadBps:    0,
		MaxDownloadBps:  0,
		PortRangeStart:  42069,
		PortRangeEnd:    42069,
	}
}

func path() (string, error) {
	root, err := store.Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, fileName), nil
}

// Load reads ~/.tocli/config.toml, creating it with defaults on first run.
func Load() (*Config, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		cfg := Default()
		if err := Save(&cfg); err != nil {
			return nil, fmt.Errorf("write default config: %w", err)
		}
		return &cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", p, err)
	}

	cfg := Default()
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", p, err)
	}
	return &cfg, nil
}

// Save writes cfg to ~/.tocli/config.toml.
func Save(cfg *Config) error {
	p, err := path()
	if err != nil {
		return err
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", p, err)
	}
	return nil
}
