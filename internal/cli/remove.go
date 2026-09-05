package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pratts/tocli/internal/engine"
	"github.com/pratts/tocli/internal/store"
	"github.com/pratts/tocli/internal/tui"
	"github.com/pratts/tocli/internal/tui/actions"
)

func newRemoveCmd() *cobra.Command {
	var withData bool
	var tuiFlag bool
	cmd := &cobra.Command{
		Use:   "remove [id]",
		Short: "Stop and forget a torrent",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && !tuiFlag {
				return runRemoveDirect(cmd, args[0], withData)
			}
			return runRemoveTUI(cmd)
		},
	}
	cmd.Flags().BoolVar(&withData, "with-data", false, "also delete the downloaded files")
	cmd.Flags().BoolVarP(&tuiFlag, "tui", "i", false, "show the interactive picker even when an id is given")
	return cmd
}

func runRemoveDirect(cmd *cobra.Command, id string, withData bool) error {
	outcome, err := engine.RemoveTorrent(id, withData)
	fmt.Fprintln(cmd.OutOrStdout(), engine.DescribeRemove(id, outcome, err))
	return err
}

// runRemoveTUI shows a picker of all torrents (any status -- unlike
// pause/resume there's no status restriction, since removing is always
// possible) and, on selection, the same list-only/with-data/cancel
// submenu the dashboard's inline remove uses. Like pause/resume, this is a
// standalone one-shot flow that reports back and returns to the shell.
func runRemoveTUI(cmd *cobra.Command) error {
	ids, err := store.ListIDs()
	if err != nil {
		return fmt.Errorf("list torrents: %w", err)
	}

	var items []actions.TorrentItem
	for _, id := range ids {
		tc, err := store.LoadTorrentConfig(id)
		if err != nil {
			items = append(items, actions.TorrentItem{ID: id, Name: id, Status: "error: unreadable config"})
			continue
		}
		_ = store.ReconcileLiveness(tc)
		items = append(items, actions.TorrentItem{ID: tc.ID, Name: tc.Name, Status: string(tc.Status)})
	}

	picker, err := tui.Run(actions.NewPicker("Remove which torrent?", items, "no torrents tracked"))
	if err != nil {
		return fmt.Errorf("run picker: %w", err)
	}
	if picker.Cancelled || picker.Selected == nil {
		return nil
	}

	menu, err := tui.Run(actions.RemoveMenu(picker.Selected.Name))
	if err != nil {
		return fmt.Errorf("run menu: %w", err)
	}
	if menu.Cancelled || menu.Selected == "" {
		return nil
	}

	id := picker.Selected.ID
	withData := menu.Selected == "with-data"
	outcome, removeErr := engine.RemoveTorrent(id, withData)

	if _, ackErr := tui.Run(actions.NewAck(engine.DescribeRemove(id, outcome, removeErr))); ackErr != nil {
		return fmt.Errorf("run confirmation: %w", ackErr)
	}
	return removeErr
}
