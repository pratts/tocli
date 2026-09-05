package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/spf13/cobra"

	"github.com/pratts/tocli/internal/config"
	"github.com/pratts/tocli/internal/engine"
	"github.com/pratts/tocli/internal/humanize"
	"github.com/pratts/tocli/internal/process"
	"github.com/pratts/tocli/internal/store"
)

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <file-or-magnet>",
		Short: "Resolve a torrent's metadata and start downloading it in the background",
		Args:  cobra.ExactArgs(1),
		RunE:  runStart,
	}
}

func runStart(cmd *cobra.Command, args []string) error {
	source := args[0]
	out := cmd.OutOrStdout()

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	mi, err := engine.ResolveMetainfo(source, out)
	if err != nil {
		return fmt.Errorf("resolve torrent metadata: %w", err)
	}

	info, err := mi.UnmarshalInfo()
	if err != nil {
		return fmt.Errorf("parse torrent info: %w", err)
	}

	id := store.DeriveID(mi)

	exists, err := store.Exists(id)
	if err != nil {
		return fmt.Errorf("check existing torrent: %w", err)
	}
	if exists {
		existing, err := store.LoadTorrentConfig(id)
		if err != nil {
			return fmt.Errorf("load existing torrent config: %w", err)
		}
		fmt.Fprintf(out, "torrent %s is already tracked as %q (status: %s); use `tocli resume %s` if it isn't running\n",
			id, existing.Name, existing.Status, id)
		return nil
	}

	fmt.Fprintln(out, "\nFiles in torrent:")
	for _, f := range engine.ListFiles(info) {
		fmt.Fprintf(out, "  %s (%s)\n", f.Path, humanize.Bytes(f.Length))
	}
	fmt.Fprintf(out, "\nTotal size: %s\n", humanize.Bytes(info.TotalLength()))

	if !confirm("\nStart download? [Y/N]: ") {
		fmt.Fprintln(out, "aborted")
		return nil
	}

	if err := store.InitTorrentDir(id); err != nil {
		return fmt.Errorf("create torrent directory: %w", err)
	}

	metainfoPath, err := store.MetainfoPath(id)
	if err != nil {
		return err
	}
	// Cache the resolved .torrent bytes to disk so pause/resume (and any
	// future retry) never needs to re-fetch magnet metadata from the swarm.
	if err := writeMetainfoFile(metainfoPath, mi); err != nil {
		return fmt.Errorf("cache metainfo: %w", err)
	}

	savePath := store.DefaultSavePath(globalCfg.BaseDownloadDir, info.BestName(), id)
	if err := os.MkdirAll(savePath, 0o755); err != nil {
		return fmt.Errorf("create download directory: %w", err)
	}

	tc := &store.TorrentConfig{
		ID:       id,
		Name:     info.BestName(),
		InfoHash: mi.HashInfoBytes().HexString(),
		Source:   source,
		SavePath: savePath,
		Status:   store.StatusStopped, // flipped to running by spawnAndTrack once the child is up
		AddedAt:  time.Now(),
	}
	if err := store.SaveTorrentConfig(tc); err != nil {
		return fmt.Errorf("write torrent config: %w", err)
	}

	if err := spawnAndTrack(tc); err != nil {
		return err
	}

	fmt.Fprintf(out, "\nstarted torrent %s (%s), downloading in background\n", id, tc.Name)
	return nil
}

// spawnAndTrack launches the background `__run` process for tc and records
// its pid, shared by both `start` and `resume`.
func spawnAndTrack(tc *store.TorrentConfig) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve tocli executable path: %w", err)
	}
	logPath, err := store.LogPath(tc.ID)
	if err != nil {
		return err
	}

	pid, err := process.SpawnDetached(exe, []string{"__run", tc.ID}, logPath)
	if err != nil {
		return fmt.Errorf("spawn background process: %w", err)
	}

	bootID, err := process.BootID()
	if err != nil {
		// Not fatal: on a platform without boot-id support, liveness
		// checks just fall back to a plain pid probe (see
		// store.ReconcileLiveness), which is what we'd have done anyway
		// before boot-id tracking existed.
		bootID = ""
	}

	tc.PID = pid
	tc.BootID = bootID
	tc.Status = store.StatusRunning
	if err := store.SaveTorrentConfig(tc); err != nil {
		return fmt.Errorf("record spawned pid: %w", err)
	}
	return nil
}

func writeMetainfoFile(path string, mi *metainfo.MetaInfo) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := mi.Write(f); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
