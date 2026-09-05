package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/pratts/tocli/internal/store"
)

// TestList_CorruptConfigDegradesGracefully simulates a torn write (e.g. the
// parent process dying mid-write to config.json, or a disk full mid-write)
// by truncating a valid config.json partway through. `list` must skip the
// broken entry and keep reporting the healthy ones, rather than erroring
// out or panicking for the whole command.
func TestList_CorruptConfigDegradesGracefully(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	healthy := &store.TorrentConfig{ID: "healthy01", Name: "ok-torrent", Status: store.StatusPaused}
	if err := store.InitTorrentDir(healthy.ID); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	if err := store.SaveTorrentConfig(healthy); err != nil {
		t.Fatalf("save healthy torrent config: %v", err)
	}

	broken := &store.TorrentConfig{ID: "broken001", Name: "torn-write", Status: store.StatusRunning}
	if err := store.InitTorrentDir(broken.ID); err != nil {
		t.Fatalf("init torrent dir: %v", err)
	}
	if err := store.SaveTorrentConfig(broken); err != nil {
		t.Fatalf("save broken torrent config: %v", err)
	}

	configPath, err := store.ConfigPath(broken.ID)
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if len(data) < 10 {
		t.Fatalf("config too short to meaningfully truncate: %d bytes", len(data))
	}
	// Cut it off partway through, as a torn write would.
	if err := os.WriteFile(configPath, data[:len(data)/2], 0o644); err != nil {
		t.Fatalf("truncate config: %v", err)
	}

	out := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	root := NewRootCmd()
	root.SetArgs([]string{"list"})
	root.SetOut(out)
	root.SetErr(stderr)

	if err := root.Execute(); err != nil {
		t.Fatalf("list returned an error instead of degrading gracefully: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "healthy01") || !strings.Contains(got, "ok-torrent") {
		t.Fatalf("healthy torrent missing from output:\n%s", got)
	}
	if !strings.Contains(got, "broken001") || !strings.Contains(got, "error: unreadable config") {
		t.Fatalf("broken torrent not reported distinctly in output:\n%s", got)
	}
	if stderr.String() == "" {
		t.Fatal("expected a warning on stderr about the unreadable config")
	}
}
