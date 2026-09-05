package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pratts/tocli/internal/engine"
	"github.com/pratts/tocli/internal/store"
	"github.com/pratts/tocli/internal/tui"
	"github.com/pratts/tocli/internal/tui/actions"
)

func newPauseCmd() *cobra.Command {
	var tuiFlag bool
	cmd := &cobra.Command{
		Use:   "pause [id]",
		Short: "Pause a running torrent by terminating its background process",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && !tuiFlag {
				return runPauseDirect(cmd, args[0])
			}
			return runPauseTUI(cmd)
		},
	}
	cmd.Flags().BoolVarP(&tuiFlag, "tui", "i", false, "show the interactive picker even when an id is given")
	return cmd
}

func runPauseDirect(cmd *cobra.Command, id string) error {
	out := cmd.OutOrStdout()

	outcome, err := engine.PauseTorrent(id)
	if err != nil {
		return err
	}
	if !outcome.Signaled {
		fmt.Fprintf(out, "torrent %s is not running (status: %s)\n", id, outcome.Config.Status)
		return nil
	}
	fmt.Fprintf(out, "sent pause signal to torrent %s\n", id)
	return nil
}

// runPauseTUI shows a picker of currently running torrents only -- there's
// no point offering to pause something already paused/completed/crashed --
// and pauses whichever is selected via the same engine.PauseTorrent the
// direct path calls. Like resume, this is a standalone one-shot flow: it
// reports back and returns to the shell rather than opening the full
// dashboard, since the user asked to pause one thing, not start
// monitoring everything.
func runPauseTUI(cmd *cobra.Command) error {
	ids, err := store.ListIDs()
	if err != nil {
		return fmt.Errorf("list torrents: %w", err)
	}

	var items []actions.TorrentItem
	for _, id := range ids {
		tc, err := store.LoadTorrentConfig(id)
		if err != nil {
			continue
		}
		if err := store.ReconcileLiveness(tc); err != nil {
			continue
		}
		if tc.Status == store.StatusRunning {
			items = append(items, actions.TorrentItem{ID: tc.ID, Name: tc.Name, Status: string(tc.Status)})
		}
	}

	picker, err := tui.Run(actions.NewPicker("Pause which torrent?", items, "no running torrents to pause"))
	if err != nil {
		return fmt.Errorf("run picker: %w", err)
	}
	if picker.Cancelled || picker.Selected == nil {
		return nil
	}

	id := picker.Selected.ID
	outcome, err := engine.PauseTorrent(id)

	var msg string
	switch {
	case err != nil:
		msg = fmt.Sprintf("failed to pause %s: %v", id, err)
	case !outcome.Signaled:
		msg = fmt.Sprintf("%s was not running (status: %s)", id, outcome.Config.Status)
	default:
		msg = fmt.Sprintf("paused %s", outcome.Config.Name)
	}

	if _, ackErr := tui.Run(actions.NewAck(msg)); ackErr != nil {
		return fmt.Errorf("run confirmation: %w", ackErr)
	}
	return err
}
