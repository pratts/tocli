package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/pratts/tocli/internal/process"
	"github.com/pratts/tocli/internal/store"
)

// terminateTimeout is how long remove waits for a graceful SIGTERM shutdown
// before escalating to SIGKILL.
const terminateTimeout = 5 * time.Second

// terminateFunc is a package variable, rather than calling process.Terminate
// directly, so tests can substitute a spy to confirm remove doesn't signal
// a torrent it already knows isn't running.
var terminateFunc = process.Terminate

func newRemoveCmd() *cobra.Command {
	var withData bool
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Stop and forget a torrent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(cmd, args[0], withData)
		},
	}
	cmd.Flags().BoolVar(&withData, "with-data", false, "also delete the downloaded files")
	return cmd
}

func runRemove(cmd *cobra.Command, id string, withData bool) error {
	tc, err := store.LoadTorrentConfig(id)
	if err != nil {
		return fmt.Errorf("load torrent %s: %w", id, err)
	}

	// Reconcile before deciding whether there's anything to signal: a
	// stored "running" status might be stale (the process crashed, or the
	// machine rebooted since), in which case there's nothing live to stop.
	// Skipping straight to cleanup avoids signaling a pid that's already
	// known dead -- or, worse, an unrelated process that has since reused
	// it after a reboot.
	if err := store.ReconcileLiveness(tc); err != nil {
		return fmt.Errorf("check torrent %s liveness: %w", id, err)
	}

	if tc.Status == store.StatusRunning {
		if err := terminateFunc(tc.PID, terminateTimeout); err != nil {
			return fmt.Errorf("stop torrent %s: %w", id, err)
		}
	}

	dir, err := store.TorrentDir(id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove torrent directory: %w", err)
	}

	if withData {
		if err := os.RemoveAll(tc.SavePath); err != nil {
			return fmt.Errorf("remove downloaded data: %w", err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "removed torrent %s\n", id)
	return nil
}
