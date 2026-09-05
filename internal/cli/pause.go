package cli

import (
	"fmt"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/pratts/tocli/internal/process"
	"github.com/pratts/tocli/internal/store"
)

func newPauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause <id>",
		Short: "Pause a running torrent by terminating its background process",
		Args:  cobra.ExactArgs(1),
		RunE:  runPause,
	}
}

func runPause(cmd *cobra.Command, args []string) error {
	id := args[0]
	out := cmd.OutOrStdout()

	tc, err := store.LoadTorrentConfig(id)
	if err != nil {
		return fmt.Errorf("load torrent %s: %w", id, err)
	}

	// Reconcile before trusting a stored "running" status: the process may
	// have died without going through pause (kill -9, crash, OOM), leaving
	// config.json stale. Signaling a dead pid would otherwise either error
	// outright or, worse, silently hit an unrelated process that has since
	// reused the same pid.
	if err := store.ReconcileLiveness(tc); err != nil {
		return fmt.Errorf("check torrent %s liveness: %w", id, err)
	}

	if tc.Status != store.StatusRunning {
		fmt.Fprintf(out, "torrent %s is not running (status: %s)\n", id, tc.Status)
		return nil
	}

	// SIGTERM only, no escalation: internal/engine.Run's own handler is
	// responsible for closing the torrent.Client cleanly and marking status
	// "paused" once it's done. Forceful SIGKILL is reserved for `remove`,
	// where we actually need the directory gone right away.
	if err := process.Signal(tc.PID, syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal torrent %s: %w", id, err)
	}
	fmt.Fprintf(out, "sent pause signal to torrent %s\n", id)
	return nil
}
