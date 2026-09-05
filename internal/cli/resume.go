package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pratts/tocli/internal/store"
)

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <id>",
		Short: "Resume a paused or stopped torrent from its cached metadata",
		Args:  cobra.ExactArgs(1),
		RunE:  runResume,
	}
}

func runResume(cmd *cobra.Command, args []string) error {
	id := args[0]
	out := cmd.OutOrStdout()

	tc, err := store.LoadTorrentConfig(id)
	if err != nil {
		return fmt.Errorf("load torrent %s: %w", id, err)
	}

	// Reconcile first: a status of "running" might be stale (process
	// crashed, or the machine rebooted since it was recorded). This flips
	// tc.Status to "crashed"/"interrupted" in that case, both of which are
	// resumable, so the switch below doesn't need to special-case them.
	if err := store.ReconcileLiveness(tc); err != nil {
		return fmt.Errorf("check torrent %s liveness: %w", id, err)
	}

	switch {
	case tc.Status == store.StatusRunning:
		fmt.Fprintf(out, "torrent %s is already running (pid %d)\n", id, tc.PID)
		return nil
	case tc.Status.IsResumable():
		// resumable
	default:
		return fmt.Errorf("torrent %s cannot be resumed from status %q", id, tc.Status)
	}

	metainfoPath, err := store.MetainfoPath(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(metainfoPath); err != nil {
		return fmt.Errorf("cached metainfo missing for %s: %w", id, err)
	}

	if err := spawnAndTrack(tc); err != nil {
		return err
	}
	fmt.Fprintf(out, "resumed torrent %s (%s)\n", id, tc.Name)
	return nil
}
