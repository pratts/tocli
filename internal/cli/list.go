package cli

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/pratts/tocli/internal/humanize"
	"github.com/pratts/tocli/internal/store"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tracked torrents and their status",
		Args:  cobra.NoArgs,
		RunE:  runList,
	}
}

func runList(cmd *cobra.Command, _ []string) error {
	ids, err := store.ListIDs()
	if err != nil {
		return fmt.Errorf("list torrents: %w", err)
	}
	if len(ids) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no torrents tracked")
		return nil
	}
	sort.Strings(ids)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tPERCENT\tRATE\tPEERS")
	for _, id := range ids {
		tc, err := store.LoadTorrentConfig(id)
		if err != nil {
			// A config.json that fails to parse (corrupt or truncated --
			// e.g. the parent died mid-write, or the disk filled up) must
			// not take the whole command down with it. Surface it as its
			// own row so it's visible in the normal listing, not just
			// buried in stderr, and let the user `tocli remove` it by hand.
			fmt.Fprintf(w, "%s\t?\terror: unreadable config\t-\t-\t-\n", id)
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %v\n", id, err)
			continue
		}

		if err := store.ReconcileLiveness(tc); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to update stale status for %s: %v\n", id, err)
		}

		percent, rate, peers := "-", "-", "-"
		if st, err := store.LoadState(id); err == nil {
			percent = fmt.Sprintf("%.1f%%", st.Percent)
			rate = humanize.Rate(st.DownloadRateBps)
			peers = fmt.Sprintf("%d/%d", st.ActivePeers, st.TotalPeers)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", tc.ID, tc.Name, tc.Status, percent, rate, peers)
	}
	return w.Flush()
}
