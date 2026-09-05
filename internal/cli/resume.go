package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pratts/tocli/internal/engine"
	"github.com/pratts/tocli/internal/store"
	"github.com/pratts/tocli/internal/tui"
	"github.com/pratts/tocli/internal/tui/actions"
)

func newResumeCmd() *cobra.Command {
	var tuiFlag bool
	cmd := &cobra.Command{
		Use:   "resume [id]",
		Short: "Resume a paused or stopped torrent from its cached metadata",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && !tuiFlag {
				return runResumeDirect(cmd, args[0])
			}
			return runResumeTUI(cmd)
		},
	}
	cmd.Flags().BoolVarP(&tuiFlag, "tui", "i", false, "show the interactive picker even when an id is given")
	return cmd
}

func runResumeDirect(cmd *cobra.Command, id string) error {
	out := cmd.OutOrStdout()

	outcome, err := engine.ResumeTorrent(id)
	if err != nil {
		return err
	}
	if outcome.AlreadyRunning {
		fmt.Fprintf(out, "torrent %s is already running (pid %d)\n", id, outcome.Config.PID)
		return nil
	}
	fmt.Fprintf(out, "resumed torrent %s (%s)\n", id, outcome.Config.Name)
	return nil
}

// runResumeTUI shows a picker of resumable torrents (paused, crashed,
// interrupted, error -- anything Status.IsResumable() allows) and resumes
// whichever the user selects, via the same engine.ResumeTorrent the direct
// path calls. It's a standalone one-shot flow, not the dashboard: the user
// asked to resume one thing, so once that's done it reports back and
// returns to the shell, the same as the direct command would, rather than
// dropping into a full live monitoring screen the user didn't ask for.
func runResumeTUI(cmd *cobra.Command) error {
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
		if tc.Status.IsResumable() {
			items = append(items, actions.TorrentItem{ID: tc.ID, Name: tc.Name, Status: string(tc.Status)})
		}
	}

	picker, err := tui.Run(actions.NewPicker("Resume which torrent?", items, "no resumable torrents"))
	if err != nil {
		return fmt.Errorf("run picker: %w", err)
	}
	if picker.Cancelled || picker.Selected == nil {
		return nil
	}

	id := picker.Selected.ID
	outcome, err := engine.ResumeTorrent(id)

	var msg string
	switch {
	case err != nil:
		msg = fmt.Sprintf("failed to resume %s: %v", id, err)
	case outcome.AlreadyRunning:
		msg = fmt.Sprintf("%s is already running", id)
	default:
		msg = fmt.Sprintf("resumed %s", outcome.Config.Name)
	}

	if _, ackErr := tui.Run(actions.NewAck(msg)); ackErr != nil {
		return fmt.Errorf("run confirmation: %w", ackErr)
	}
	return err
}
